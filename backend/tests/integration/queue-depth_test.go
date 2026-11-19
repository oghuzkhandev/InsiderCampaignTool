package integration

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// addChannelMessage, belirli kanal, durum ve deneme sayısıyla tek bir mesaj ekler.
// delayed true ise next_attempt_at gelecektedir (retry beklemesi).
func addChannelMessage(t *testing.T, db *sql.DB, campaignID, channel, status string, attempts int, delayed bool) {
	t.Helper()
	uid := mustUser(t, db)
	_, err := db.Exec(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key,
		                       status, attempts, next_attempt_at, sent_at, claim_token, claimed_at)
		 VALUES ($1, $2, $3::text, 'x@example.test', '', $4, $5::text, $6,
		         CASE WHEN $7::boolean THEN now() + interval '1 hour' ELSE now() END,
		         CASE WHEN $5::text = 'sent' THEN now() END,
		         CASE WHEN $5::text = 'sending' THEN gen_random_uuid() END,
		         CASE WHEN $5::text = 'sending' THEN now() END)`,
		campaignID, uid, channel, idemKey(campaignID, uid, channel), status, attempts, delayed)
	if err != nil {
		t.Fatalf("mesaj eklenemedi (%s/%s): %v", channel, status, err)
	}
}

func queueDepthOf(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	depth, err := campaign.NewStore(db).QueueDepth(context.Background())
	if err != nil {
		t.Fatalf("kuyruk derinliği: %v", err)
	}
	return depth
}

// Sabit veri kümesi: yalnızca `queued` mesajlar kanal bazında sayılır. Denenmiş
// (retrying) ve gelecekteki next_attempt_at'li mesajlar kuyruktadır; sending ve
// terminal durumlar sayılmaz.
func TestQueueDepthCountsOnlyQueuedMessagesPerChannel(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")

	// sms: 3 queued (1'i hiç denenmemiş, 1'i denenmiş, 1'i retry beklemesinde)
	addChannelMessage(t, db, id, "sms", "queued", 0, false)
	addChannelMessage(t, db, id, "sms", "queued", 2, false)
	addChannelMessage(t, db, id, "sms", "queued", 1, true)
	addChannelMessage(t, db, id, "sms", "sending", 1, false)
	addChannelMessage(t, db, id, "sms", "sent", 1, false)
	// email: 2 queued
	addChannelMessage(t, db, id, "email", "queued", 0, false)
	addChannelMessage(t, db, id, "email", "queued", 0, false)
	addChannelMessage(t, db, id, "email", "failed", 5, false)
	addChannelMessage(t, db, id, "email", "skipped", 1, false)

	got := queueDepthOf(t, db)
	if got["sms"] != 3 || got["email"] != 2 {
		t.Fatalf("sms=3 ve email=2 bekleniyordu: %v", got)
	}
}

// Veritabanında mesaj yokken iki kanal da sıfırla raporlanır (metrik eksik kalmaz).
func TestQueueDepthIsZeroForBothChannelsWhenNothingIsQueued(t *testing.T) {
	db := testdb.Open(t)

	got := queueDepthOf(t, db)
	if len(got) != 2 || got["sms"] != 0 || got["email"] != 0 {
		t.Fatalf("iki kanal da 0 bekleniyordu: %v", got)
	}
}

// Değer tüm kampanyaları kapsar ve bellekte tutulmaz: worker çalışmıyorken yeni
// mesajlar eklendikçe artar ve veritabanı değişince hemen yansır.
func TestQueueDepthAccumulatesAcrossCampaignsWhileNoWorkerRuns(t *testing.T) {
	db := testdb.Open(t)
	first := approvedCampaignWithStatus(t, db, "sending")
	second := approvedCampaignWithStatus(t, db, "sending")

	addChannelMessage(t, db, first, "sms", "queued", 0, false)
	if got := queueDepthOf(t, db); got["sms"] != 1 {
		t.Fatalf("1 bekliyor: %v", got)
	}
	addChannelMessage(t, db, second, "sms", "queued", 0, false)
	addChannelMessage(t, db, second, "sms", "queued", 0, false)
	if got := queueDepthOf(t, db); got["sms"] != 3 {
		t.Fatalf("worker yokken birikim görünmeli (3): %v", got)
	}

	// Bir mesaj işlendi: değer düşer.
	if _, err := db.Exec(`UPDATE messages SET status = 'sent', sent_at = now() WHERE id = (SELECT min(id) FROM messages WHERE campaign_id = $1)`, first); err != nil {
		t.Fatal(err)
	}
	if got := queueDepthOf(t, db); got["sms"] != 2 {
		t.Fatalf("işlenen mesaj düşmeli (2): %v", got)
	}
}

// API'nin /metrics gövdesi gerçek veritabanından hesaplanır; worker sayaçlarını içermez.
func TestAPIMetricsBodyReportsRealQueueDepthAndNoWorkerCounters(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addChannelMessage(t, db, id, "sms", "queued", 0, false)
	addChannelMessage(t, db, id, "sms", "queued", 1, false)
	addChannelMessage(t, db, id, "email", "queued", 0, false)
	var failures metrics.Counter
	failures.Add(2)

	h := metrics.Mux(httpapi.MetricsCollector(campaign.NewStore(db), &failures))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("200 bekleniyordu: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`queue_depth{channel="sms"} 2` + "\n",
		`queue_depth{channel="email"} 1` + "\n",
		"llm_validation_failures_total 2\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("gövdede %q yok:\n%s", want, body)
		}
	}
	for _, name := range []string{"messages_sent_total", "send_attempts_total", "stale_result_total", "reaper_requeued_total", "ratelimit_wait_seconds"} {
		if strings.Contains(body, name) {
			t.Errorf("API çıktısında worker metriği %q olmamalı", name)
		}
	}
}

// Veritabanı okunamazsa scrape başarısız olur (503); kuyruk derinliği 0 gibi görünmez.
func TestAPIMetricsReturns503WhenTheDatabaseCannotBeRead(t *testing.T) {
	url, err := testdb.URL()
	if err != nil {
		t.Fatal(err)
	}
	_ = testdb.Open(t) // veritabanı hazır olsun
	closed, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close() // kapalı havuz: her sorgu hata verir

	h := metrics.Mux(httpapi.MetricsCollector(campaign.NewStore(closed), &metrics.Counter{}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("503 bekleniyordu: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "queue_depth") {
		t.Fatalf("kısmi çıktı sunulmamalı: %q", rec.Body.String())
	}
}
