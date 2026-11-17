package database

import (
	"errors"
	"testing"
)

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
