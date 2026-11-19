package database

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Erişilemeyen bir adres (kapalı port): OpenLazy hata vermeden döner, Open ise
// erişimi doğruladığı için hata verir.
const unreachableURL = "postgres://u:p@127.0.0.1:1/campaigns?sslmode=disable"

func TestOpenLazyDoesNotRequireReachableDatabase(t *testing.T) {
	db, err := OpenLazy(unreachableURL)
	if err != nil {
		t.Fatalf("OpenLazy erişilemeyen veritabanında hata vermemeli: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err == nil {
		t.Fatal("ilk kullanımda erişim hatası görünmeli")
	}
}

func TestOpenFailsWhenDatabaseIsUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if db, err := Open(ctx, unreachableURL); err == nil {
		_ = db.Close()
		t.Fatal("Open erişilemeyen veritabanında hata vermeli")
	}
}

func TestURLFromEnv(t *testing.T) {
	t.Setenv(EnvURL, "")
	if _, err := URLFromEnv(); !errors.Is(err, ErrNoURL) {
		t.Fatalf("tanımsız değişken için ErrNoURL bekleniyordu: %v", err)
	}

	t.Setenv(EnvURL, "postgres://u:p@localhost:5432/campaigns")
	got, err := URLFromEnv()
	if err != nil || got != "postgres://u:p@localhost:5432/campaigns" {
		t.Fatalf("URLFromEnv = %q, %v", got, err)
	}
}
