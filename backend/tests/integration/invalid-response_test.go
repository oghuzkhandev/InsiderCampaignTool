package integration

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

// startCorruptingProvider, gerçek mock provider'ın önüne bir katman koyar: provider
// isteği KABUL EDER ve PostgreSQL'e kaydeder, ama worker'a giden `200` yanıtının
// gövdesi bozulur (araya giren proxy, kesilen gövde vb. taklidi). corrupt, kaçıncı
// yanıt olduğunu (1'den başlar) alır ve yeni gövdeyi (ve bozulup bozulmayacağını)
// döndürür.
func startCorruptingProvider(t *testing.T, db *sql.DB, corrupt func(n int) (body string, ok bool)) (*httptest.Server, *mockprovider.Handler) {
	t.Helper()
	inner, err := mockprovider.New(mockprovider.Config{}, mockprovider.NewPGStore(db),
		mockprovider.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	var responses atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)
		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode == http.StatusOK {
			if body, ok := corrupt(int(responses.Add(1))); ok {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, body)
				return
			}
		}
		for k, v := range res.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, inner
}

// Provider isteği kabul etti ama worker geçerli bir yanıt göremedi: mesaj doğrudan
// `sent` olmaz. Aynı idempotency anahtarıyla yeniden denenir; provider kayıtlı
// sonucu döndürür, tek kabul kaydı kalır ve mesaj ancak geçerli yanıtla `sent` olur.
func TestAcceptedButUnreadableResponseIsNotSentUntilValidResponseArrives(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	// İlk iki yanıt bozuk: gövdesiz ve message_id'siz; üçüncüsü geçerli.
	bodies := map[int]string{1: "", 2: "{}"}
	srv, prov := startCorruptingProvider(t, db, func(n int) (string, bool) {
		body, bad := bodies[n]
		return body, bad
	})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "sms")
	if m.Status != "sent" || m.Attempts != 3 || m.FailureReason.Valid {
		t.Fatalf("iki bozuk yanıttan sonra 3. denemede sent olmalı: %+v", m)
	}
	snap := prov.Stats().Snapshot()
	if snap.TotalRequests != 3 || snap.UniqueAccepts != 1 || snap.DuplicateRequests != 2 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("3 istek, 1 kabul, 2 tekrar, 0 tekrar teslimat: %+v", snap)
	}
	if got := acceptanceCount(t, db); got != 1 {
		t.Fatalf("tek kabul kaydı bekleniyordu: %d", got)
	}
	metrics := w.Metrics().Snapshot()
	if metrics.MessagesSent != 1 || metrics.Attempts[worker.KindInvalidResponse] != 2 || metrics.Attempts[worker.KindAccepted] != 1 {
		t.Fatalf("yalnızca geçerli yanıt sent sayılmalı, 2 geçersiz yanıt ayrı sayılmalı: %+v", metrics)
	}
}

// Her yanıt bozuksa mesaj hiçbir zaman `sent` olmaz: deneme sınırında
// `failed`/`exhausted` (sonuç doğrulanamadı). Provider tarafında yine tek kabul vardır.
func TestEveryAcceptResponseUnreadableEndsExhaustedNeverSent(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "email")
	srv, prov := startCorruptingProvider(t, db, func(int) (string, bool) { return `<html>gateway</html>`, true })
	cfg := fastConfig() // azami deneme 5
	w := startWorker(t, db, httpProvider(srv), cfg)

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "email")
	if m.Status != "failed" || m.FailureReason.String != worker.FailureExhausted || m.Attempts != cfg.MaxAttempts || m.SentAt.Valid {
		t.Fatalf("hiç sent olmadan failed/exhausted bekleniyordu: %+v", m)
	}
	if !strings.Contains(m.LastError.String, "geçersiz yanıt") || strings.Contains(m.LastError.String, "gateway") {
		t.Fatalf("last_error nedeni açıklamalı ama gövdeyi taşımamalı: %q", m.LastError.String)
	}
	snap := prov.Stats().Snapshot()
	if snap.UniqueAccepts != 1 || snap.DuplicateDeliveries != 0 || snap.TotalRequests != cfg.MaxAttempts {
		t.Fatalf("5 istek, 1 kabul, 0 tekrar teslimat: %+v", snap)
	}
	if got := w.Metrics().Snapshot(); got.MessagesSent != 0 || got.Attempts[worker.KindInvalidResponse] != int64(cfg.MaxAttempts) {
		t.Fatalf("hiçbir mesaj sent sayılmamalı: %+v", got)
	}
}
