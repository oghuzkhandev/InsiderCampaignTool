// Komut mock-provider, SMS ve e-posta gönderimini taklit eden sahte sağlayıcıyı
// çalıştırır (SND-001, ADR-025). Gerçek hiçbir kişiye mesaj gitmez.
//
// Ortam değişkenleri: PORT (varsayılan 8082), DATABASE_URL (kabul kayıtları için)
// ve hata davranışı için MOCK_PROVIDER_LATENCY, MOCK_PROVIDER_429_RATE,
// MOCK_PROVIDER_500_RATE, MOCK_PROVIDER_RETRY_AFTER ve MOCK_PROVIDER_MODE
// (accept_then_drop veya drop_all_responses).
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

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
)

// shutdownTimeout, SIGTERM sonrası devam eden isteklere tanınan süredir
// (ARCHITECTURE §8: kapanış 15 sn).
const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("mock-provider başlatılamadı", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	cfg, err := mockprovider.ConfigFromEnv()
	if err != nil {
		return err
	}
	url, err := database.URLFromEnv()
	if err != nil {
		return err
	}
	// Veritabanı kapalıyken süreç sonlanmaz: kabul kaydı yazılamayan istekler 500
	// alır ve worker bunları yeniden dener.
	db, err := database.OpenLazy(url)
	if err != nil {
		return err
	}
	defer db.Close()

	handler, err := mockprovider.New(cfg, mockprovider.NewPGStore(db))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	slog.Info("mock-provider başlıyor", "addr", srv.Addr,
		"gecikme", cfg.Latency, "oran_429", cfg.RateLimitRate, "oran_500", cfg.ServerErrorRate,
		"retry_after", cfg.RetryAfter, "mod", string(cfg.Mode))
	if err := serve(ctx, srv, ln, shutdownTimeout); err != nil {
		return err
	}
	slog.Info("mock-provider durdu")
	return nil
}

// serve, srv'yi ln üzerinde çalıştırır. ctx iptal edilince (SIGTERM, Ctrl+C) yeni
// bağlantı kabul etmeyi bırakır, devam eden isteklerin bitmesini en fazla timeout
// kadar bekler ve düzgünce kapanır.
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
