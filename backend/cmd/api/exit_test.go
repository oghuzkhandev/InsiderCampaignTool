package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Regresyon: main, run()'dan dönen her hatayı "api başlatılamadı" diye logluyordu.
// Sunucu çalışırken veya kapanırken oluşan hatalar (örn. kapanış süresinin aşılıp
// açık isteklerin kesilmesi) başlatma hatası değildir; mesaj yanıltıcıydı.
func TestExitMessageDistinguishesStartupFromServingAndShutdownFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"başlatma hatası (veritabanı)", errors.New("database: bağlanılamadı"), "api başlatılamadı"},
		{"başlatma hatası sarılmış", fmt.Errorf("ek bağlam: %w", errors.New("x")), "api başlatılamadı"},
		{"çalışırken beklenmeyen durma", &servingError{err: errors.New("accept tcp: too many open files")}, "api sunucusu beklenmedik şekilde durdu"},
		{"kapanış süresi aşıldı", &servingError{err: fmt.Errorf("%w: %w", errShutdownIncomplete, errors.New("context deadline exceeded"))}, "api kapanışı süre içinde tamamlanamadı; açık istekler kesildi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitMessage(tt.err); got != tt.want {
				t.Fatalf("exitMessage = %q, beklenen %q", got, tt.want)
			}
			if strings.Contains(tt.want, "başlatılamadı") != (tt.want == "api başlatılamadı") {
				t.Fatal("test tablosu tutarsız")
			}
		})
	}
}

// serve, sunucu başladıktan sonraki hataları tipli döndürür: kapanış süresinin
// aşılması hem errShutdownIncomplete hem *servingError'dır ve ayrıntı (Unwrap)
// korunur; böylece main doğru mesajı seçer.
func TestServeReportsAnExceededShutdownWindowAsServingError(t *testing.T) {
	mux := http.NewServeMux()
	release := make(chan struct{})
	started := make(chan struct{})
	mux.HandleFunc("GET /hang", func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})
	t.Cleanup(func() { close(release) })
	addr, cancel, done := startServe(t, mux, 100*time.Millisecond)

	go func() { _, _ = http.Get("http://" + addr + "/hang") }()
	<-started
	cancel()

	err := wantReturn(t, done, 3*time.Second)
	var se *servingError
	if !errors.As(err, &se) {
		t.Fatalf("*servingError bekleniyordu: %T %v", err, err)
	}
	if !errors.Is(err, errShutdownIncomplete) {
		t.Fatalf("errShutdownIncomplete bekleniyordu: %v", err)
	}
	if got := exitMessage(err); strings.Contains(got, "başlatılamadı") {
		t.Fatalf("kapanış hatası başlatma hatası gibi bildirilmemeli: %q", got)
	}
}
