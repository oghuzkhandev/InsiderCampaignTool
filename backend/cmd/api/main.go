package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
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

func main() {
	if err := run(); err != nil {
		slog.Error("api başlatılamadı", "error", err)
		os.Exit(1)
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

	mux := newMux()
	mux.Handle("/api/", httpapi.New(httpapi.Deps{
		Segments:  &segment.Generator{LLM: client},
		Audience:  audience.NewCounter(db),
		Campaigns: campaign.NewStore(db),
		Content:   &content.Generator{LLM: client},
		Contents:  campaign.NewContentStore(db),
		Approver:  campaign.NewApprover(db),
	}))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	slog.Info("api başlıyor", "addr", srv.Addr)
	if err := serve(ctx, srv, ln, shutdownTimeout); err != nil {
		return err
	}
	slog.Info("api durdu")
	return nil
}

// shutdownTimeout, SIGTERM sonrası devam eden isteklere tanınan süredir
// (ARCHITECTURE: kapanış 15 sn).
const shutdownTimeout = 15 * time.Second

// serve, srv'yi ln üzerinde çalıştırır. ctx iptal edilince (SIGTERM, Ctrl+C) yeni
// bağlantı kabul etmeyi bırakır, devam eden isteklerin bitmesini en fazla
// timeout kadar bekler ve düzgünce kapanır.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("kapanış tamamlanamadı: %w", err)
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
