package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func startServe(t *testing.T, handler http.Handler, timeout time.Duration) (addr string, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { errCh <- serve(ctx, srv, ln, timeout) }()
	return ln.Addr().String(), cancel, errCh
}

func wantReturn(t *testing.T, done <-chan error, within time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatal("serve zamanında dönmedi (süreç kapanmıyor)")
		return nil
	}
}

// Regresyon: SIGTERM yakalanıyor ama sunucu bağlamı izlemediği için süreç
// kapanmıyordu.
func TestServeStopsWhenContextIsCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("pong")) })
	addr, cancel, done := startServe(t, mux, time.Second)

	resp, err := http.Get("http://" + addr + "/ping")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	cancel()
	if err := wantReturn(t, done, 2*time.Second); err != nil {
		t.Fatalf("düzgün kapanışta hata beklenmez: %v", err)
	}
	if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("kapanan sunucu yeni bağlantı kabul etmemeli")
	}
}

func TestServeLetsInFlightRequestFinish(t *testing.T) {
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("tamam"))
	})
	addr, cancel, done := startServe(t, mux, 2*time.Second)

	type result struct {
		body string
		code int
		err  error
	}
	res := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			res <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		res <- result{body: string(b), code: resp.StatusCode}
	}()

	<-started
	cancel() // istek sürerken kapanış başlar

	r := <-res
	if r.err != nil || r.code != http.StatusOK || r.body != "tamam" {
		t.Fatalf("devam eden istek tamamlanmalı: %+v", r)
	}
	if err := wantReturn(t, done, 2*time.Second); err != nil {
		t.Fatalf("serve = %v", err)
	}
}

func TestServeGivesUpAfterShutdownTimeout(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stuck", func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	})
	addr, cancel, done := startServe(t, mux, 100*time.Millisecond)
	t.Cleanup(func() { close(release) })

	go func() {
		resp, err := http.Get("http://" + addr + "/stuck")
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-started
	cancel()

	if err := wantReturn(t, done, 2*time.Second); err == nil {
		t.Fatal("süre dolunca takılan istek için hata dönmeli")
	}
}
