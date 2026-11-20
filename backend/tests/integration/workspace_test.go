package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/templates"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

func TestWorkspaceTemplatesCRUDAndValidation(t *testing.T) {
	db := testdb.Open(t)
	h := httpapi.New(httpapi.Deps{Templates: templates.NewStore(db), Overview: campaign.NewStore(db)})
	body := `{"name":"Sepet","sms_text":"Merhaba {{first_name}}","email_subject":"Fırsat","email_body":"Sepetiniz sizi bekliyor."}`
	created := apiDo(h, http.MethodPost, "/api/templates", body)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var item templates.Template
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || created.Header().Get("Location") != "/api/templates/"+item.ID {
		t.Fatal("missing identity")
	}
	for _, bad := range []string{
		`{"name":"","sms_text":"x","email_subject":"x","email_body":"x"}`,
		`{"name":"Bad","sms_text":"{{password}}","email_subject":"x","email_body":"x"}`,
	} {
		if rec := apiDo(h, "PUT", "/api/templates/"+item.ID, bad); rec.Code != 422 {
			t.Fatalf("invalid: %d %s", rec.Code, rec.Body)
		}
	}
	list := apiDo(h, "GET", "/api/templates?page=1&limit=10", "")
	var page struct {
		Items []templates.Template
		Total int
	}
	if list.Code != 200 {
		t.Fatalf("list: %s", list.Body)
	}
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Name != "Sepet" {
		t.Fatalf("rejected edit changed template: %+v", page)
	}
	updated := apiDo(h, "PUT", "/api/templates/"+item.ID, `{"name":"Yeni","sms_text":"Yeni metin","email_subject":"Konu","email_body":"Gövde"}`)
	if updated.Code != 200 {
		t.Fatalf("update: %d %s", updated.Code, updated.Body)
	}
	overview := apiDo(h, "GET", "/api/overview", "")
	var counts campaign.Overview
	if err := json.Unmarshal(overview.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if overview.Code != 200 || counts.Templates != 1 || counts.Campaigns != 0 {
		t.Fatalf("overview: %s", overview.Body)
	}
	for i, want := range []int{204, 404} {
		if rec := apiDo(h, "DELETE", "/api/templates/"+item.ID, ""); rec.Code != want {
			t.Fatalf("delete %d: %d", i, rec.Code)
		}
	}
	if rec := apiDo(h, "PUT", "/api/templates/not-a-uuid", body); rec.Code != 404 {
		t.Fatalf("invalid id: %d", rec.Code)
	}
}

func TestWorkspacePagesDirectoryAndOverview(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	create := newAPI(db, llm.NewMock())
	var newest string
	for i := 0; i < 13; i++ {
		r := apiPost(create, exampleBriefText)
		if r.Code != 201 {
			t.Fatalf("create: %d %s", r.Code, r.Body)
		}
		newest = decodeCampaign(t, r).ID
	}
	store := campaign.NewStore(db)
	h := httpapi.New(httpapi.Deps{Campaigns: store, CampaignPages: store, Overview: store, Directory: audience.NewLister(db)})
	for _, tc := range []struct {
		path        string
		size, total int
	}{
		{"/api/campaigns?page=1&limit=10", 10, 13},
		{"/api/campaigns?page=2&limit=10", 3, 13},
		{"/api/campaigns?page=3&limit=10", 0, 13},
		{"/api/campaigns?page=1&status=approved", 0, 0},
		{"/api/campaigns?page=1&limit=99", 13, 13},
	} {
		r := apiDo(h, "GET", tc.path, "")
		var p campaign.Page
		if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if r.Code != 200 || len(p.Campaigns) != tc.size || p.Total != tc.total {
			t.Fatalf("%s: %d %s", tc.path, r.Code, r.Body)
		}
		if tc.path == "/api/campaigns?page=1&limit=10" && p.Campaigns[0].ID != newest {
			t.Fatal("wrong ordering")
		}
	}
	r := apiDo(h, "GET", "/api/audience?page=1&limit=10", "")
	var directory struct {
		Total int
		Items []struct{ Email *string }
	}
	if err := json.Unmarshal(r.Body.Bytes(), &directory); err != nil {
		t.Fatal(err)
	}
	if r.Code != 200 || directory.Total != 3009 || len(directory.Items) != 10 {
		t.Fatalf("directory: %d %s", r.Code, r.Body)
	}
	for _, query := range []string{"%27%20OR%201%3D1--", "%25"} {
		r = apiDo(h, "GET", "/api/audience?search="+query, "")
		var p struct{ Total int }
		if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if r.Code != 200 || p.Total != 0 {
			t.Fatalf("search treated as SQL or wildcard: %d %s", r.Code, r.Body)
		}
	}
	r = apiDo(h, "GET", "/api/overview", "")
	var counts campaign.Overview
	if err := json.Unmarshal(r.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if r.Code != 200 || counts.Campaigns != 13 || counts.Draft != 13 || counts.Users != 3009 || counts.Pending != 0 {
		t.Fatalf("overview: %s", r.Body)
	}
}

func TestWorkspaceRejectsInvalidParametersBeforeDependencies(t *testing.T) {
	h := httpapi.New(httpapi.Deps{})
	for _, path := range []string{"/api/campaigns?page=0", "/api/campaigns?page=1&status=unknown", "/api/audience?limit=-1", "/api/templates?page=abc"} {
		if r := apiDo(h, "GET", path, ""); r.Code != 400 {
			t.Fatalf("%s: %d", path, r.Code)
		}
	}
}

func TestTemplateMigrationDownAndUpPreservesCampaigns(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	t.Cleanup(func() {
		if err := database.Migrate(context.Background(), db); err != nil {
			t.Error(err)
		}
	})
	h := newAPI(db, llm.NewMock())
	id := decodeCampaign(t, apiPost(h, exampleBriefText)).ID
	if id == "" {
		t.Fatal("campaign not created")
	}
	if _, err := db.Exec(`INSERT INTO content_templates(name,sms_text,email_subject,email_body) VALUES ('Demo','SMS','Konu','Gövde')`); err != nil {
		t.Fatal(err)
	}
	p := newGooseProvider(t, db)
	if _, err := p.DownTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	var table *string
	if err := db.QueryRow(`SELECT to_regclass('public.content_templates')::text`).Scan(&table); err != nil {
		t.Fatal(err)
	}
	if table != nil {
		t.Fatal("template table survived Down")
	}
	var campaigns int
	if err := db.QueryRow(`SELECT count(*) FROM campaigns WHERE id=$1`, id).Scan(&campaigns); err != nil {
		t.Fatal(err)
	}
	if campaigns != 1 {
		t.Fatal("Down changed campaign data")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var templates int
	if err := db.QueryRow(`SELECT count(*) FROM content_templates`).Scan(&templates); err != nil {
		t.Fatal(err)
	}
	if templates != 0 {
		t.Fatal("Down/Up must recreate empty template storage")
	}
}
