// Package database, PostgreSQL bağlantısını açar ve migration'ları uygular.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // "pgx" sürücüsünü kaydeder
	"github.com/pressly/goose/v3"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/migrations"
)

// EnvURL, uygulama bağlantı adresinin okunduğu ortam değişkenidir.
const EnvURL = "DATABASE_URL"

// ErrNoURL, DATABASE_URL tanımlı değilse döner.
var ErrNoURL = errors.New("database: " + EnvURL + " tanımlı değil")

// URLFromEnv, bağlantı adresini ortam değişkeninden okur.
func URLFromEnv() (string, error) {
	url := os.Getenv(EnvURL)
	if url == "" {
		return "", ErrNoURL
	}
	return url, nil
}

// Open, bağlantı havuzunu açar ve veritabanına erişildiğini doğrular.
func Open(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("database: bağlantı açılamadı: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database: veritabanına erişilemedi: %w", err)
	}
	return db, nil
}

// Migrate, gömülü goose migration'larının bekleyenlerini uygular.
func Migrate(ctx context.Context, db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("database: migration sağlayıcısı: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("database: migration: %w", err)
	}
	return nil
}
