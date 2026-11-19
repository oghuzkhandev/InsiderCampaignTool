package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePinger, bir veritabanı ping'i taklit eder. err boşsa ping başarılıdır;
// block true ise ctx bitene kadar bekler (takılan bağlantı).
type fakePinger struct {
	err   error
	block bool
}

func (p fakePinger) PingContext(ctx context.Context) error {
	if p.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return p.err
}

// togglePinger, testin ortasında erişimin kesilip dönmesini taklit eder.
type togglePinger struct {
	mu   sync.Mutex
	down bool
}

func (p *togglePinger) set(down bool) { p.mu.Lock(); p.down = down; p.mu.Unlock() }
func (p *togglePinger) PingContext(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.down {
		return errors.New("dial tcp postgres:5432: connection refused (user=secret)")
	}
	return nil
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestReadyzIs200WhenTheDatabaseIsReachable(t *testing.T) {
	rec := get(newMux(fakePinger{}, nil), "/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %q", got)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

// Veritabanına erişilemezse /readyz 503 döner ama /healthz 200 kalır (süreç
// yaşıyor); hata ayrıntısı (adres, kullanıcı) yanıta sızmaz.
func TestReadyzIs503WhileHealthzStays200WhenTheDatabaseIsDown(t *testing.T) {
	mux := newMux(fakePinger{err: errors.New("dial tcp postgres:5432: connection refused (user=secret)")}, nil)

	ready := get(mux, "/readyz")
	live := get(mux, "/healthz")

	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz status = %d, 503 bekleniyordu", ready.Code)
	}
	if live.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, 200 kalmalı", live.Code)
	}
	if got := strings.TrimSpace(ready.Body.String()); got != `{"status":"unavailable"}` {
		t.Errorf("body = %q", got)
	}
	for _, leak := range []string{"secret", "postgres", "5432", "refused"} {
		if strings.Contains(ready.Body.String(), leak) {
			t.Errorf("yanıt %q ayrıntısını sızdırmamalı: %s", leak, ready.Body.String())
		}
	}
}

// Erişim kesilip dönünce /readyz önce 503, sonra yeniden 200 olur.
func TestReadyzRecoversWhenTheDatabaseReturns(t *testing.T) {
	p := &togglePinger{}
	mux := newMux(p, nil)

	if get(mux, "/readyz").Code != http.StatusOK {
		t.Fatal("başlangıçta 200 bekleniyordu")
	}
	p.set(true)
	if get(mux, "/readyz").Code != http.StatusServiceUnavailable {
		t.Fatal("erişim kesilince 503 bekleniyordu")
	}
	p.set(false)
	if get(mux, "/readyz").Code != http.StatusOK {
		t.Fatal("erişim dönünce yeniden 200 bekleniyordu")
	}
}

// Takılan bir ping, süre sınırında 503 olur; probe asılı kalmaz.
func TestReadyzTimesOutOnAHungDatabase(t *testing.T) {
	rd := newReadiness(fakePinger{block: true}, nil)
	rd.timeout = 50 * time.Millisecond

	start := time.Now()
	rec := get(rd, "/readyz")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("503 bekleniyordu: %d", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("süre sınırı işlemedi: %s", elapsed)
	}
}

func TestReadyzDefaultTimeoutIsBounded(t *testing.T) {
	if readyTimeout <= 0 || readyTimeout > 5*time.Second {
		t.Fatalf("varsayılan süre sınırı makul olmalı: %s", readyTimeout)
	}
	if newReadiness(fakePinger{}, nil).timeout != readyTimeout {
		t.Fatal("hazırlık kontrolü varsayılan süre sınırını kullanmalı")
	}
}

func TestReadyzRejectsNonGET(t *testing.T) {
	rec := httptest.NewRecorder()
	newMux(fakePinger{}, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/readyz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// Sık gelen probe'lar log'u doldurmaz: yalnızca durum değişince bir satır yazılır.
func TestReadyzLogsOnlyOnStateChanges(t *testing.T) {
	var logs strings.Builder
	p := &togglePinger{}
	mux := newMux(p, slog.New(slog.NewTextHandler(&logs, nil)))

	for i := 0; i < 3; i++ {
		get(mux, "/readyz")
	}
	if logs.Len() != 0 {
		t.Fatalf("sağlıklıyken log yazılmamalı: %q", logs.String())
	}
	p.set(true)
	for i := 0; i < 5; i++ {
		get(mux, "/readyz")
	}
	if n := strings.Count(logs.String(), "level=WARN"); n != 1 {
		t.Fatalf("5 başarısız kontrol tek uyarı yazmalı, %d yazdı:\n%s", n, logs.String())
	}
	p.set(false)
	for i := 0; i < 3; i++ {
		get(mux, "/readyz")
	}
	if n := strings.Count(logs.String(), "level=INFO"); n != 1 {
		t.Fatalf("toparlanma tek bilgi satırı yazmalı, %d yazdı:\n%s", n, logs.String())
	}
}
