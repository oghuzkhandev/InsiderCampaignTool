package integration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// CMP-001: son kampanyalar listesi, gerçek PostgreSQL ile. Yalnızca campaigns_test.

type listRow struct {
	ID        string `json:"id"`
	Brief     string `json:"brief"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type listJSON struct {
	Campaigns []listRow `json:"campaigns"`
}

func apiList(t *testing.T, h http.Handler, query string) (*httptest.ResponseRecorder, listJSON) {
	t.Helper()
	rec := apiDo(h, http.MethodGet, "/api/campaigns"+query, "")
	var l listJSON
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
			t.Fatalf("liste JSON değil: %v\n%s", err, rec.Body.String())
		}
	}
	return rec, l
}

func briefs(rows []listRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Brief)
	}
	return out
}

// insertDraft, belirli oluşturulma zamanıyla draft bir kampanya yazar.
func insertDraft(t *testing.T, db *sql.DB, brief string, at time.Time) string {
	t.Helper()
	seg, issues := segment.Validate([]byte(`{"all":[{"field":"city","op":"eq","value":"İstanbul"}],"unsupported":[]}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	c, err := campaign.NewStore(db).Create(t.Context(), brief, seg, audience.Counts{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE campaigns SET created_at = $1 WHERE id = $2`, at, c.ID); err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func TestCampaignListIsNewestFirstWithDefaultAndMaximumLimits(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	base := now.Truncate(time.Second)
	for i := 0; i < 25; i++ { // Brief 24 en yeni
		insertDraft(t, db, fmt.Sprintf("Brief %d", i), base.Add(time.Duration(i)*time.Minute))
	}

	want := func(newest, count int) []string {
		out := []string{}
		for i := 0; i < count; i++ {
			out = append(out, fmt.Sprintf("Brief %d", newest-i))
		}
		return out
	}
	for _, tt := range []struct {
		name  string
		query string
		count int
	}{
		{"varsayılan 10", "", 10},
		{"limit=1", "?limit=1", 1},
		{"limit=3", "?limit=3", 3},
		{"limit=20 (üst sınır)", "?limit=20", 20},
		{"limit=25 20'ye indirilir", "?limit=25", 20},
		{"limit=1000 20'ye indirilir", "?limit=1000", 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec, l := apiList(t, h, tt.query)
			if rec.Code != http.StatusOK {
				t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
			}
			got, wantBriefs := briefs(l.Campaigns), want(24, tt.count)
			if strings.Join(got, "|") != strings.Join(wantBriefs, "|") {
				t.Fatalf("sıra/sayı yanlış:\n got  %v\n want %v", got, wantBriefs)
			}
		})
	}

	for _, bad := range []string{"?limit=0", "?limit=-3", "?limit=abc", "?limit=2.5"} {
		rec, _ := apiList(t, h, bad)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: durum %d, beklenen 400", bad, rec.Code)
		}
		if code, _ := decodeAPIError(t, rec); code != "INVALID_REQUEST" {
			t.Fatalf("%s: kod %q", bad, code)
		}
	}
}

func TestCampaignListBreaksTiesByIDSoOrderIsStable(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	same := now.Truncate(time.Second)
	ids := []string{
		insertDraft(t, db, "A", same),
		insertDraft(t, db, "B", same),
		insertDraft(t, db, "C", same),
	}

	_, first := apiList(t, h, "")
	_, second := apiList(t, h, "")

	if len(first.Campaigns) != 3 {
		t.Fatalf("3 kampanya bekleniyordu: %+v", first.Campaigns)
	}
	for i := range first.Campaigns {
		if first.Campaigns[i].ID != second.Campaigns[i].ID {
			t.Fatalf("aynı zamanlı kampanyaların sırası çağrıdan çağrıya değişmemeli")
		}
		if i > 0 && first.Campaigns[i-1].ID < first.Campaigns[i].ID {
			t.Fatalf("eşit zamanda kimliğe göre azalan sıra bekleniyordu: %v", first.Campaigns)
		}
	}
	got := map[string]bool{}
	for _, r := range first.Campaigns {
		got[r.ID] = true
	}
	for _, id := range ids {
		if !got[id] {
			t.Fatalf("%s listede yok", id)
		}
	}
}

func TestCampaignListIsAnEmptyArrayWhenThereAreNoCampaigns(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())

	rec, l := apiList(t, h, "")

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"campaigns":[]}` {
		t.Fatalf("durum %d, gövde %q", rec.Code, rec.Body.String())
	}
	if l.Campaigns == nil || len(l.Campaigns) != 0 {
		t.Fatalf("boş liste bekleniyordu: %+v", l.Campaigns)
	}
}

// Her durum listede kayıtlı değeriyle görünür; satır ayrıntı endpoint'iyle tutarlıdır.
func TestCampaignListShowsRealStatusesAndMatchesTheDetailEndpoint(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.RunUsers(t.Context(), db, seed.GenerateE2E(now)); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())

	// Dört kampanya: draft, approved, sending, completed. Onaylıları gerçek onay akışı
	// yazar; sending ve completed için yalnızca durum ilerletilir (worker bu testte yok).
	want := map[string]string{}
	var order []string
	for _, status := range []string{"draft", "approved", "sending", "completed"} {
		created := decodeCampaign(t, apiPost(h, exampleBriefText))
		if status != "draft" {
			if lastEvent(t, parseEvents(t, postGenerate(h, created.ID).Body.String())).Name != "validated" {
				t.Fatal("içerik üretilmeli")
			}
			mustApproveOK(t, h, created.ID, 1)
		}
		switch status {
		case "sending":
			if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, created.ID); err != nil {
				t.Fatal(err)
			}
		case "completed":
			if _, err := db.Exec(`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`, created.ID); err != nil {
				t.Fatal(err)
			}
		}
		want[created.ID] = status
		order = append(order, created.ID)
		time.Sleep(5 * time.Millisecond) // oluşturulma zamanları ayrışsın
	}

	rec, l := apiList(t, h, "")
	if rec.Code != http.StatusOK || len(l.Campaigns) != 4 {
		t.Fatalf("durum %d, %d satır: %s", rec.Code, len(l.Campaigns), rec.Body.String())
	}
	for i, row := range l.Campaigns {
		if want[row.ID] != row.Status {
			t.Fatalf("%s: durum %q, beklenen %q", row.ID, row.Status, want[row.ID])
		}
		// En yeni başta: sıra oluşturma sırasının tersidir.
		if row.ID != order[len(order)-1-i] {
			t.Fatalf("%d. satır %s, beklenen %s", i, row.ID, order[len(order)-1-i])
		}

		detail := decodeCampaign(t, apiDo(h, http.MethodGet, "/api/campaigns/"+row.ID, ""))
		if detail.Brief != row.Brief || detail.Status != row.Status || detail.CreatedAt != row.CreatedAt {
			t.Fatalf("liste satırı ayrıntıyla tutarsız:\n liste   %+v\n ayrıntı %+v", row, detail)
		}
		if _, err := time.Parse(time.RFC3339Nano, row.CreatedAt); err != nil || !strings.HasSuffix(row.CreatedAt, "Z") {
			t.Fatalf("created_at UTC RFC3339 olmalı: %q", row.CreatedAt)
		}
	}
}

func TestCampaignListRowsCarryOnlyTheDocumentedFields(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	insertDraft(t, db, "Brief", now.Truncate(time.Second))

	rec, _ := apiList(t, h, "")

	var raw struct {
		Campaigns []map[string]json.RawMessage `json:"campaigns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "brief", "status", "created_at"} {
		if _, ok := raw.Campaigns[0][key]; !ok {
			t.Fatalf("%q yok", key)
		}
	}
	if len(raw.Campaigns[0]) != 4 {
		t.Fatalf("kitle sayısı, segment ve içerik listede olmamalı: %s", rec.Body.String())
	}
}

func TestCampaignListReturnsAThousandCharacterBriefIntact(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	long := strings.Repeat("ü", 1000)
	insertDraft(t, db, long, now.Truncate(time.Second))

	_, l := apiList(t, h, "")

	if len(l.Campaigns) != 1 || l.Campaigns[0].Brief != long {
		t.Fatal("1000 karakterlik brief kırpılmadan dönmeli (kırpma arayüzün işi)")
	}
}

func TestCampaignStoreListRejectsLimitsOutsideTheAllowedRange(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewStore(db)

	for _, limit := range []int{-1, 0, campaign.MaxListLimit + 1, 1000} {
		if _, err := store.List(t.Context(), limit); !errors.Is(err, campaign.ErrInvalidLimit) {
			t.Fatalf("limit %d: hata %v, beklenen ErrInvalidLimit", limit, err)
		}
	}
	for _, limit := range []int{1, campaign.DefaultListLimit, campaign.MaxListLimit} {
		if _, err := store.List(t.Context(), limit); err != nil {
			t.Fatalf("limit %d geçerli olmalı: %v", limit, err)
		}
	}
}
