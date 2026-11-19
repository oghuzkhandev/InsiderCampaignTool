package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

const wiringBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

// run() içindeki gerçek bağlama (newHandler), gerçek veritabanı ve MockLLM ile
// çalışır: her bağımlılık bağlı olduğu için hiçbir rota nil işaretçi paniği
// vermez. Bu test, bağlamayı örten testin eksikliğini kapatır (açık kayıt).
func TestNewHandlerWiresEveryRouteAgainstARealDatabase(t *testing.T) {
	db := testdb.Open(t)
	h, err := newHandler(db, llm.NewMock(), nil, &metrics.Counter{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Brief'e uyan, iki kanalda izinli tek bir kullanıcı (onay için uygun alıcı gerekir).
	var uid int64
	if err := db.QueryRow(
		`INSERT INTO users (first_name, email, phone, city, last_cart_add_at)
		 VALUES ('Ada', 'ada@example.test', '+905550000001', 'İstanbul', now() - interval '5 days') RETURNING id`,
	).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, 'sms', true), ($1, 'email', true)`, uid); err != nil {
		t.Fatal(err)
	}

	do := func(method, path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	readAll := func(r *http.Response) string {
		b, _ := io.ReadAll(r.Body)
		return string(b)
	}

	// Sağlık uçları: veritabanı ayakta.
	if r := do(http.MethodGet, "/healthz", ""); r.StatusCode != http.StatusOK {
		t.Fatalf("/healthz: %d", r.StatusCode)
	}
	if r := do(http.MethodGet, "/readyz", ""); r.StatusCode != http.StatusOK {
		t.Fatalf("/readyz: %d", r.StatusCode)
	}

	// Segment üretimi + kitle sayımı + kampanya kaydı (Segments, Audience, Campaigns).
	body, _ := json.Marshal(map[string]string{"brief": wiringBrief})
	created := do(http.MethodPost, "/api/campaigns", string(body))
	raw := readAll(created)
	if created.StatusCode != http.StatusCreated && created.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/campaigns: %d %s", created.StatusCode, raw)
	}
	var c struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil || c.ID == "" {
		t.Fatalf("kampanya kimliği okunamadı: %v %s", err, raw)
	}

	if r := do(http.MethodGet, "/api/campaigns/"+c.ID, ""); r.StatusCode != http.StatusOK {
		t.Fatalf("GET kampanya: %d", r.StatusCode)
	}

	// İçerik üretimi (Content, Contents): SSE, son olay `validated`.
	gen := do(http.MethodPost, "/api/campaigns/"+c.ID+"/content", "")
	genBody := readAll(gen)
	if gen.StatusCode != http.StatusOK || !strings.Contains(genBody, "event: validated") {
		t.Fatalf("içerik üretimi: %d %s", gen.StatusCode, genBody)
	}

	// Onay (Approver).
	app := do(http.MethodPost, "/api/campaigns/"+c.ID+"/approve", `{"content_version":1}`)
	if app.StatusCode != http.StatusOK {
		t.Fatalf("onay: %d %s", app.StatusCode, readAll(app))
	}

	// Dashboard akışı (Deliveries): ilk olay hemen gelir ve onaylı kampanyayı gösterir.
	ev := do(http.MethodGet, "/api/campaigns/"+c.ID+"/events", "")
	if ev.StatusCode != http.StatusOK {
		t.Fatalf("events: %d", ev.StatusCode)
	}
	br := bufio.NewReader(ev.Body)
	deadline := time.AfterFunc(5*time.Second, func() { _ = ev.Body.Close() })
	defer deadline.Stop()
	first, _ := br.ReadString('\n')
	data, _ := br.ReadString('\n')
	if !strings.HasPrefix(first, "event: snapshot") || !strings.Contains(data, `"status":"approved"`) {
		t.Fatalf("ilk olay onaylı kampanyanın snapshot'ı olmalı: %q %q", first, data)
	}
}

// openClosedDB, açılıp hemen kapatılmış bir havuz döndürür: her sorgu hata verir.
func openClosedDB(url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	return db, db.Close()
}

// Veritabanı yoksa (kapalı havuz) /readyz 503 olur, /healthz 200 kalır: gerçek
// bağlamada da liveness ve readiness ayrıdır.
func TestNewHandlerReadyzReflectsTheRealDatabaseWhileHealthzDoesNot(t *testing.T) {
	url, err := testdb.URL()
	if err != nil {
		t.Fatal(err)
	}
	_ = testdb.Open(t) // veritabanı hazır olsun
	closed, err := openClosedDB(url)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(closed, llm.NewMock(), nil, &metrics.Counter{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	if rec := get(h, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz: %d, 503 bekleniyordu", rec.Code)
	}
	if rec := get(h, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("/healthz: %d, 200 kalmalı", rec.Code)
	}
}
