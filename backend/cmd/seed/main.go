// Komut seed, demo kullanıcılarını ve kanal izinlerini yazar. Tek seferlik bir
// servistir; DATABASE_URL ile belirtilen veritabanında çalışır. Tekrar
// çalıştırmak var olan kullanıcılara dokunmaz.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed başarısız", "error", err)
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

	res, err := seed.Run(ctx, db, time.Now().UTC())
	if err != nil {
		return err
	}
	slog.Info("seed tamamlandı", "inserted", res.Inserted, "skipped", res.Skipped)
	return nil
}
