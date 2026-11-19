package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// newMux, API'nin sağlık uçlarını kurar: `/healthz` (liveness, veritabanına
// bakmaz) ve `/readyz` (veritabanına erişim; erişilemezse 503).
func newMux(db pinger, log *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /readyz", newReadiness(db, log))
	return mux
}

// newHandler, API'nin bütün bağımlılıklarını bağlar ve doğrular. Zorunlu bir
// bağımlılık eksikse hata döner: süreç başlamaz, ilk istekte panik vermez.
func newHandler(db *sql.DB, client llm.Provider, shutdown <-chan struct{}, llmFailures *metrics.Counter, log *slog.Logger) (http.Handler, error) {
	deps := httpapi.Deps{
		Segments:              &segment.Generator{LLM: client},
		Audience:              audience.NewCounter(db),
		Campaigns:             campaign.NewStore(db),
		Content:               &content.Generator{LLM: client},
		Contents:              campaign.NewContentStore(db),
		Approver:              campaign.NewApprover(db),
		Deliveries:            campaign.NewStore(db),
		LLMValidationFailures: llmFailures,
		Shutdown:              shutdown,
	}
	if err := deps.Validate(); err != nil {
		return nil, err
	}
	mux := newMux(db, log)
	mux.Handle("/api/", httpapi.New(deps))
	return mux, nil
}

// newLLM, LLM_PROVIDER değerine göre model istemcisi seçer. Varsayılan mock'tur;
// testler ve demo gerçek API anahtarı gerektirmez.
func newLLM(provider string) (llm.Provider, error) {
	switch provider {
	case "", "mock":
		return llm.NewMock(), nil
	default:
		return nil, fmt.Errorf("desteklenmeyen LLM_PROVIDER %q (şimdilik yalnızca mock)", provider)
	}
}

// metricsAddrEnv, iç /metrics dinleyicisinin adresidir (örn. ":9090"). Boşsa
// dinleyici açılmaz.
const metricsAddrEnv = "METRICS_ADDR"

func main() {
	if err := run(); err != nil {
		slog.Error(exitMessage(err), "error", err)
		os.Exit(1)
	}
}

// errShutdownIncomplete, kapanış süresi dolduğunda açık isteklerin kesildiğini
// gösterir.
var errShutdownIncomplete = errors.New("kapanış tamamlanamadı")

// servingError, sunucu çalışmaya başladıktan sonraki (çalışırken veya kapanırken)
// bir hatadır; başlatma hatası değildir.
type servingError struct{ err error }

func (e *servingError) Error() string { return e.err.Error() }
func (e *servingError) Unwrap() error { return e.err }

// exitMessage, süreç hatayla çıkarken loglanan mesajı seçer. Başlatma hatası ile
// çalışırken veya kapanırken oluşan hata ayrı söylenir; aksi halde SIGTERM'den
// sonra süre aşımı "başlatılamadı" gibi görünürdü.
func exitMessage(err error) string {
	var se *servingError
	switch {
	case errors.Is(err, errShutdownIncomplete):
		return "api kapanışı süre içinde tamamlanamadı; açık istekler kesildi"
	case errors.As(err, &se):
		return "api sunucusu beklenmedik şekilde durdu"
	default:
		return "api başlatılamadı"
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	url, err := database.URLFromEnv()
	if err != nil {
		return err
	}
	db, err := database.Open(ctx, url)
	if err != nil {
		return err
	}
	defer db.Close()

	client, err := newLLM(os.Getenv("LLM_PROVIDER"))
	if err != nil {
		return err
	}

	// llm_validation_failures_total süreç belleğinde tutulur (ADR-016).
	llmFailures := &metrics.Counter{}

	var handlerErr error
	srv := newServer(":"+port, func(shutdown <-chan struct{}) http.Handler {
		h, err := newHandler(db, client, shutdown, llmFailures, slog.Default())
		handlerErr = err
		return h
	})
	if handlerErr != nil {
		return handlerErr
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	// API metrikleri ana porttan AYRI, iç bir dinleyicide sunulur ve Compose'ta
	// host'a yayımlanmaz (ADR-016). Port açılamazsa süreç başlamaz.
	if addr := os.Getenv(metricsAddrEnv); addr != "" {
		mln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("metrics dinleyicisi açılamadı (%s=%q): %w", metricsAddrEnv, addr, err)
		}
		go func() {
			if err := metrics.Serve(ctx, mln, httpapi.MetricsCollector(campaign.NewStore(db), llmFailures)); err != nil {
				slog.Error("metrics dinleyicisi durdu", "error", err)
			}
		}()
		slog.Info("api metrics dinleyicisi", "addr", mln.Addr().String())
	}

	slog.Info("api başlıyor", "addr", srv.Addr)
	if err := serve(ctx, srv, ln, shutdownTimeout); err != nil {
		return err
	}
	slog.Info("api durdu")
	return nil
}

// newServer, sunucuyu kurar. handler, kapanış başlayınca kapanan bir kanal alır:
// dashboard olay akışı gibi uzun ömürlü yanıtlar bu kanalı izleyip kendiliğinden
// biter. Aksi halde Shutdown, açık her akışı shutdownTimeout dolana kadar beklerdi.
func newServer(addr string, handler func(shutdown <-chan struct{}) http.Handler) *http.Server {
	shutdown := make(chan struct{})
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler(shutdown),
		ReadHeaderTimeout: 5 * time.Second,
	}
	var once sync.Once
	srv.RegisterOnShutdown(func() { once.Do(func() { close(shutdown) }) })
	return srv
}

// shutdownTimeout, SIGTERM sonrası devam eden isteklere tanınan süredir
// (ARCHITECTURE: kapanış 15 sn).
const shutdownTimeout = 15 * time.Second

// serve, srv'yi ln üzerinde çalıştırır. ctx iptal edilince (SIGTERM, Ctrl+C) yeni
// bağlantı kabul etmeyi bırakır, devam eden isteklerin bitmesini en fazla
// timeout kadar bekler ve düzgünce kapanır.
//
// Dönen hatalar *servingError'dır (başlatma hatası değil); kapanış süresi aşılırsa
// ayrıca errShutdownIncomplete'dir.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return &servingError{err: err}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return &servingError{err: fmt.Errorf("%w: %w", errShutdownIncomplete, err)}
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return &servingError{err: err}
	}
	return nil
}
