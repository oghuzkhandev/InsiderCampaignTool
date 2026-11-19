package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// pinger, veritabanına erişilip erişilemediğini sınar (*sql.DB).
type pinger interface {
	PingContext(ctx context.Context) error
}

// readyTimeout, tek bir hazırlık kontrolünün süre sınırıdır. Takılan bir
// bağlantı probe'u sonsuza dek asmasın; süre dolunca API hazır değildir.
const readyTimeout = 2 * time.Second

// readiness, `/readyz` işleyicisidir (TRK-001). Veritabanına erişilebiliyorsa 200,
// erişilemiyorsa 503 döner. Liveness (`/healthz`) veritabanına bakmaz: veritabanı
// kesintisinde süreç yeniden başlatılmaz, yalnızca trafikten çıkarılır.
type readiness struct {
	db      pinger
	timeout time.Duration
	log     *slog.Logger
	// failing, son kontrolün başarısız olup olmadığıdır; yalnızca durum
	// değişince loglanır (probe sık gelir, log dolmasın).
	failing atomic.Bool
}

func newReadiness(db pinger, log *slog.Logger) *readiness {
	if log == nil {
		log = slog.Default()
	}
	return &readiness{db: db, timeout: readyTimeout, log: log}
}

func (rd *readiness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), rd.timeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := rd.db.PingContext(ctx); err != nil {
		if rd.failing.CompareAndSwap(false, true) {
			rd.log.Warn("veritabanına erişilemiyor; /readyz 503 dönüyor", "error", err)
		}
		// Hata ayrıntısı (adres, kullanıcı) istemciye yazılmaz.
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable"}`))
		return
	}
	if rd.failing.CompareAndSwap(true, false) {
		rd.log.Info("veritabanı erişilebilir; /readyz yeniden 200")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
