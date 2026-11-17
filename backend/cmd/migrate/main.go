// Komut migrate, bekleyen veritabanı migration'larını uygular. Tek seferlik
// bir servistir; DATABASE_URL ile belirtilen veritabanında çalışır.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migrate başarısız", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	url, err := database.URLFromEnv()
	if err != nil {
		return err
	}
	db, err := database.Open(ctx, url)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		return err
	}
	slog.Info("migration tamamlandı")
	return nil
}
