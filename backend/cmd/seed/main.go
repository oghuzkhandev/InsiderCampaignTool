// Komut seed, demo kullanıcılarını ve kanal izinlerini yazar. Tek seferlik bir
// servistir; DATABASE_URL ile belirtilen veritabanında çalışır. Tekrar
// çalıştırmak var olan kullanıcılara dokunmaz.
//
// SEED_DATASET: "demo" (varsayılan, birkaç bin rastgele kullanıcı) veya "e2e"
// (uçtan uca testler için küçük, kontrollü veri; yalnızca izole e2e ortamında
// kullanılır, bkz. compose.e2e.yaml).
package main

import (
	"context"
	"fmt"
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

	dataset := os.Getenv("SEED_DATASET")
	now := time.Now().UTC()
	var res seed.Result
	switch dataset {
	case "", seed.DatasetDemo:
		dataset = seed.DatasetDemo
		res, err = seed.Run(ctx, db, now)
	case seed.DatasetE2E:
		res, err = seed.RunUsers(ctx, db, seed.GenerateE2E(now))
	default:
		return fmt.Errorf("bilinmeyen SEED_DATASET %q (demo veya e2e)", dataset)
	}
	if err != nil {
		return err
	}
	slog.Info("seed tamamlandı", "dataset", dataset, "inserted", res.Inserted, "skipped", res.Skipped)
	return nil
}
