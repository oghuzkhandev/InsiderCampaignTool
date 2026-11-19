// Komut worker, onaylanan kampanyaların mesajlarını PostgreSQL kuyruğundan alıp
// mock provider'a gönderir (SND-001). Tek süreç olarak çalışır (ADR-005).
//
// Ortam değişkenleri: DATABASE_URL, PROVIDER_URL (varsayılan
// http://mock-provider:8082) ve isteğe bağlı zamanlama ayarları
// (WORKER_CAPACITY, WORKER_MAX_ATTEMPTS, WORKER_LEASE, WORKER_REAP_INTERVAL,
// WORKER_POLL_INTERVAL, WORKER_SHUTDOWN_TIMEOUT, WORKER_RATE, WORKER_SEND_BURST,
// WORKER_BACKOFF_BASE, WORKER_BACKOFF_CAP, WORKER_DB_BACKOFF_MIN/MAX). Tanımsız ayarlar ARCHITECTURE
// §8'deki varsayılanlarda kalır; testlerde süreler kısaltılabilir. METRICS_ADDR
// (örn. ":9090") tanımlıysa o adreste yalnızca /metrics sunulur.
//
// SIGTERM veya Ctrl+C alınca yeni iş sahiplenmeyi durdurur ve devam eden
// çağrılara WORKER_SHUTDOWN_TIMEOUT (varsayılan 15 sn) tanır. Veritabanına
// erişilemese de süreç sonlanmaz; erişim dönünce kuyruğu işlemeye devam eder.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

const defaultProviderURL = "http://mock-provider:8082"

// metricsAddrEnv, iç /metrics dinleyicisinin adresidir (örn. ":9090"). Boşsa
// dinleyici açılmaz.
const metricsAddrEnv = "METRICS_ADDR"

func main() {
	if err := run(); err != nil {
		slog.Error("worker başlatılamadı", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := worker.ConfigFromEnv()
	if err != nil {
		return err
	}
	providerURL := os.Getenv("PROVIDER_URL")
	if providerURL == "" {
		providerURL = defaultProviderURL
	}
	url, err := database.URLFromEnv()
	if err != nil {
		return err
	}
	// Veritabanı kapalıyken süreç sonlanmaz: claim döngüsü backoff ile bekler.
	db, err := database.OpenLazy(url)
	if err != nil {
		return err
	}
	defer db.Close()

	w, err := worker.New(cfg, worker.NewPGRepo(db), worker.NewHTTPProvider(providerURL))
	if err != nil {
		return err
	}

	// Worker sayaçları kendi iç /metrics dinleyicisinde sunulur (ADR-016). Adres
	// Compose'ta host'a yayımlanmaz. Port açılamazsa süreç başlamaz: metriksiz
	// çalışmak sessiz bir eksik olurdu.
	if addr := os.Getenv(metricsAddrEnv); addr != "" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("metrics dinleyicisi açılamadı (%s=%q): %w", metricsAddrEnv, addr, err)
		}
		go func() {
			err := metrics.Serve(ctx, ln, func(_ context.Context, mw *metrics.Writer) error {
				w.Metrics().Write(mw)
				return nil
			})
			if err != nil {
				slog.Error("metrics dinleyicisi durdu", "error", err)
			}
		}()
		slog.Info("worker metrics dinleyicisi", "addr", ln.Addr().String())
	}

	slog.Info("worker başlıyor", "provider", providerURL,
		"kapasite", cfg.Capacity, "azami_deneme", cfg.MaxAttempts, "lease", cfg.Lease,
		"hiz_sn", cfg.Rate, "kapanis", cfg.ShutdownTimeout)
	if err := w.Run(ctx); err != nil {
		return err
	}
	slog.Info("worker durdu")
	return nil
}
