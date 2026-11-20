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
	"sync"
	"testing"
	"time"

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

// LockKey, campaigns_test'i kullanan testleri SÜREÇLER ARASI sıraya koyan
// PostgreSQL advisory lock anahtarıdır ("camptest"). `go test ./...` paketleri
// eşzamanlı çalıştırır; birden çok paket aynı veritabanını kullanınca biri
// migration Down/Up veya TRUNCATE yaparken diğeri çalışırdı. Open kilidi alır ve
// test bitince (t.Cleanup) bırakır; kilit oturum düzeyindedir, süreç ölürse
// bağlantıyla birlikte kendiliğinden düşer.
const LockKey int64 = 0x63616d7074657374

// lockWait, başka bir paketin testinin kilidi bırakmasını bekleme sınırıdır.
const lockWait = 5 * time.Minute

// lock, süreç içinde referans sayımlıdır: aynı süreçteki iç içe Open çağrıları
// kendi kendini kilitlemez; kilit en dıştaki test bitince bırakılır.
var lock struct {
	mu   sync.Mutex
	refs int
	conn *sql.Conn
}

func acquireLock(t testing.TB, db *sql.DB) {
	t.Helper()
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.refs == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), lockWait)
		defer cancel()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("testdb: kilit bağlantısı açılamadı: %v", err)
		}
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, LockKey); err != nil {
			_ = conn.Close()
			t.Fatalf("testdb: %s kilidi %s içinde alınamadı (başka bir test paketi uzun süre kullanıyor olabilir): %v", Name, lockWait, err)
		}
		lock.conn = conn
	}
	lock.refs++
	t.Cleanup(releaseLock)
}

func releaseLock() {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	lock.refs--
	if lock.refs == 0 && lock.conn != nil {
		_, _ = lock.conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, LockKey)
		_ = lock.conn.Close()
		lock.conn = nil
	}
}

// URL, campaigns_test veritabanının bağlantı adresini döndürür. Alt süreç olarak
// başlatılan worker ve mock provider'a verilir (ADR-015). Adres her zaman
// campaigns_test'i gösterir; ortamdaki DATABASE_URL asla kullanılmaz. Veritabanı
// Open ile hazırlanır, bu işlev yalnızca adresi üretir.
func URL() (string, error) {
	adminURL := os.Getenv("TEST_POSTGRES_URL")
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	return withDatabase(adminURL, Name)
}

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
	// Migration ve temizlikten ÖNCE: başka bir test paketi şemayı değiştiriyor
	// veya veriyi temizliyor olabilir.
	// Migration ve temizlikten ÖNCE: başka bir test paketi şemayı değiştiriyor
	// veya veriyi temizliyor olabilir.
	acquireLock(t, db)
	if err := migrate(db); err != nil {
		t.Fatalf("testdb: migration: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE content_templates, mock_provider_acceptances, messages, campaign_contents, campaigns, user_consents, users RESTART IDENTITY CASCADE`); err != nil {
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
