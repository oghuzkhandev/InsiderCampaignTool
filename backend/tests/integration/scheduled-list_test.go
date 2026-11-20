package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// GET /api/scheduled (Zamanlanmış sayfası): gönderimi ileri bir zamana planlanmış, başlamamış
// kampanyalar. Gerçek PostgreSQL ile. Yalnızca campaigns_test.

type scheduledPageJSON struct {
	Items []struct {
		ID          string    `json:"id"`
		Brief       string    `json:"brief"`
		ScheduledAt time.Time `json:"scheduled_at"`
		ApprovedAt  time.Time `json:"approved_at"`
		Queued      int       `json:"queued"`
		Due         bool      `json:"due"`
	} `json:"items"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	Limit      int       `json:"limit"`
	ServerTime time.Time `json:"server_time"`
}

func getScheduled(t *testing.T, h http.Handler, query string) scheduledPageJSON {
	t.Helper()
	rec := apiDo(h, http.MethodGet, "/api/scheduled"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: durum %d: %s", query, rec.Code, rec.Body.String())
	}
	var out scheduledPageJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON değil: %v\n%s", err, rec.Body.String())
	}
	return out
}

// draftWithContent, geçerli içerikli (sürüm 1) taslak bir kampanya oluşturur.
func draftWithContent(t *testing.T, h http.Handler) string {
	t.Helper()
	r := apiPost(h, exampleBriefText)
	if r.Code != http.StatusCreated {
		t.Fatalf("kampanya: %d %s", r.Code, r.Body)
	}
	id := decodeCampaign(t, r).ID
	body := `{"sms_text":"Merhaba {{first_name}}","email_subject":"Konu","email_body":"Gövde"}`
	if r = apiDo(h, http.MethodPut, "/api/campaigns/"+id+"/content", body); r.Code != http.StatusOK {
		t.Fatalf("içerik: %d %s", r.Code, r.Body)
	}
	return id
}

func approveAt(t *testing.T, h http.Handler, id string, at time.Time) {
	t.Helper()
	if r := apiDo(h, http.MethodPost, approvePath(id), approveScheduledBody(at)); r.Code != http.StatusOK {
		t.Fatalf("planlı onay: %d %s", r.Code, r.Body)
	}
}

func queuedMessages(t *testing.T, db *sql.DB, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM messages WHERE campaign_id = $1 AND status = 'queued'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestScheduledListShowsOnlyPendingScheduledCampaignsSoonestFirst(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	base := dbTime(t, db)

	later := draftWithContent(t, h)
	approveAt(t, h, later, base.Add(3*time.Hour))
	sooner := draftWithContent(t, h)
	approveAt(t, h, sooner, base.Add(time.Hour))
	immediate := draftWithContent(t, h)
	if r := apiDo(h, http.MethodPost, approvePath(immediate), `{"content_version":1}`); r.Code != http.StatusOK {
		t.Fatalf("hemen onay: %d %s", r.Code, r.Body)
	}
	draftWithContent(t, h) // onaysız taslak

	got := getScheduled(t, h, "")

	if got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("yalnızca iki planlı kampanya listelenmeli: toplam %d, sayfa %d", got.Total, len(got.Items))
	}
	if got.Items[0].ID != sooner || got.Items[1].ID != later {
		t.Fatalf("en yakın zaman başta olmalı: %s, %s (beklenen %s, %s)", got.Items[0].ID, got.Items[1].ID, sooner, later)
	}
	first := got.Items[0]
	if first.Queued == 0 || first.Queued != queuedMessages(t, db, sooner) {
		t.Fatalf("kuyruktaki mesaj sayısı %d, veritabanında %d", first.Queued, queuedMessages(t, db, sooner))
	}
	if first.Due || got.Items[1].Due {
		t.Fatal("zamanı gelmemiş kampanya due olmamalı")
	}
	if first.Brief != exampleBriefText || first.ApprovedAt.IsZero() {
		t.Fatalf("brief ve onay zamanı dolu olmalı: %+v", first)
	}
	if d := first.ScheduledAt.Sub(base.Add(time.Hour)); d < -time.Second || d > time.Second {
		t.Fatalf("planlanan zaman istenen olmalı: %v", first.ScheduledAt)
	}
	// Sunucu saati veritabanı saatidir: istemci saatine bağlı değildir.
	if d := got.ServerTime.Sub(dbTime(t, db)); d < -5*time.Second || d > 5*time.Second {
		t.Fatalf("sunucu saati veritabanı saatine yakın olmalı: %v", got.ServerTime)
	}
}

func TestScheduledListPaginatesAndPageBeyondTheEndIsEmpty(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	base := dbTime(t, db)
	var ids []string // en yakından en uzağa
	for i := range 3 {
		id := draftWithContent(t, h)
		approveAt(t, h, id, base.Add(time.Duration(i+1)*time.Hour))
		ids = append(ids, id)
	}

	for page, want := range []string{ids[0], ids[1], ids[2]} {
		got := getScheduled(t, h, "?limit=1&page="+string(rune('1'+page)))
		if got.Total != 3 || len(got.Items) != 1 || got.Items[0].ID != want {
			t.Fatalf("sayfa %d: toplam %d, %+v (beklenen %s)", page+1, got.Total, got.Items, want)
		}
	}
	beyond := getScheduled(t, h, "?limit=1&page=4")
	if beyond.Total != 3 || len(beyond.Items) != 0 || beyond.ServerTime.IsZero() {
		t.Fatalf("son sayfanın ötesi boş liste, doğru toplam ve sunucu saati vermeli: %+v", beyond)
	}
}

func TestScheduledListEmptyWhenNothingIsScheduled(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())

	got := getScheduled(t, h, "")

	if got.Total != 0 || got.Items == nil || len(got.Items) != 0 || got.ServerTime.IsZero() {
		t.Fatalf("boş liste ve sunucu saati bekleniyordu: %+v", got)
	}
}

// Planlanan zaman geldi ama gönderim başlamadıysa (worker kapalı) kampanya listede kalır ve
// `due` olur: iş kaybolmaz, görünür kalır.
func TestScheduledListFlagsDueCampaignsWhenDeliveryHasNotStarted(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	id := draftWithContent(t, h)
	approveAt(t, h, id, dbTime(t, db).Add(1500*time.Millisecond))
	if got := getScheduled(t, h, ""); len(got.Items) != 1 || got.Items[0].Due {
		t.Fatalf("henüz zamanı gelmedi: %+v", got.Items)
	}

	time.Sleep(1700 * time.Millisecond) // worker çalışmıyor: kampanya `approved` kalır

	got := getScheduled(t, h, "")
	if len(got.Items) != 1 || !got.Items[0].Due || got.Items[0].Queued == 0 {
		t.Fatalf("zamanı geçen ama başlamamış kampanya due olmalı ve mesajları kuyrukta kalmalı: %+v", got.Items)
	}
}

// Gönderim başlayınca (kampanya `sending`) veya bitince kampanya listeden çıkar.
func TestScheduledListDropsCampaignsOnceDeliveryStarts(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	id := draftWithContent(t, h)
	approveAt(t, h, id, dbTime(t, db).Add(time.Hour))
	if got := getScheduled(t, h, ""); got.Total != 1 {
		t.Fatalf("önce listelenmeli: %d", got.Total)
	}

	if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if got := getScheduled(t, h, ""); got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("gönderim başlayınca listeden çıkmalı: %+v", got)
	}

	// Okuma hiçbir şeyi değiştirmez: kampanya ve mesajlar yerinde.
	var status string
	if err := db.QueryRow(`SELECT status FROM campaigns WHERE id = $1`, id).Scan(&status); err != nil || status != "sending" {
		t.Fatalf("kampanya durumu: %q %v", status, err)
	}
}

func TestScheduledListRejectsInvalidPaging(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	for _, q := range []string{"?page=0", "?limit=0", "?page=x", "?page=100001"} {
		if rec := apiDo(h, http.MethodGet, "/api/scheduled"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: durum %d, beklenen 400", q, rec.Code)
		}
	}
}
