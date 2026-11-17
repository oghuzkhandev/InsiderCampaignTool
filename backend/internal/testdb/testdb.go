// Package testdb, entegrasyon testleri için yalnızca campaigns_test veritabanını
// hazırlar. Uygulama veritabanına (campaigns) hiçbir zaman bağlanmaz.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // "pgx" sürücüsünü kaydeder

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
)

// Name, testlerin yazabileceği tek veritabanının adıdır (ADR-012).
const Name = "campaigns_test"

// defaultAdminURL, yalnızca CREATE DATABASE için kullanılan bakım
// veritabanına (postgres) giden varsayılan bağlantıdır. TEST_POSTGRES_URL ile
// değiştirilebilir.
const defaultAdminURL = "postgres://campaigns:campaigns@127.0.0.1:5432/postgres?sslmode=disable"

// Open, campaigns_test veritabanını oluşturur (yoksa), ortak migration'ları
// uygular, tabloları temizler ve bağlantıyı döndürür. Her yazmadan önce
// bağlantının gerçek veritabanı adı doğrulanır; farklıysa test durur.
func Open(t testing.TB) *sql.DB {
	t.Helper()

	adminURL := os.Getenv("TEST_POSTGRES_URL")
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	if err := ensureDatabase(adminURL); err != nil {
		t.Fatalf("testdb: %s hazırlanamadı (Postgres çalışıyor mu? `make test-db`): %v", Name, err)
	}

	testURL, err := withDatabase(adminURL, Name)
	if err != nil {
		t.Fatalf("testdb: %v", err)
	}
	db, err := sql.Open("pgx", testURL)
	if err != nil {
		t.Fatalf("testdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := RequireTestDatabase(db); err != nil {
		t.Fatalf("testdb: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("testdb: migration: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE campaigns, user_consents, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("testdb: temizleme: %v", err)
	}
	return db
}

// RequireTestDatabase, bağlantının bildirdiği gerçek veritabanı adının tam
// olarak campaigns_test olduğunu doğrular.
func RequireTestDatabase(db *sql.DB) error {
	var current string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&current); err != nil {
		return fmt.Errorf("veritabanı adı okunamadı: %w", err)
	}
	if current != Name {
		return fmt.Errorf("bağlantı %q veritabanına gidiyor, beklenen %q; yazma durduruldu", current, Name)
	}
	return nil
}

func migrate(db *sql.DB) error {
	// Yazmadan hemen önce tekrar doğrula.
	if err := RequireTestDatabase(db); err != nil {
		return err
	}
	return database.Migrate(context.Background(), db)
}

// ensureDatabase, bakım veritabanı üzerinden yalnızca campaigns_test'i
// oluşturur. Başka hiçbir veritabanında işlem yapmaz.
func ensureDatabase(adminURL string) error {
	admin, err := sql.Open("pgx", adminURL)
	if err != nil {
		return err
	}
	defer admin.Close()

	var exists bool
	if err := admin.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, Name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := admin.Exec(`CREATE DATABASE ` + Name); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P04" { // başka süreç oluşturdu
			return nil
		}
		return err
	}
	return nil
}

func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
