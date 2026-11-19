package main

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
)

// staticDeliveries, her zaman gönderimin sürdüğü bir kampanya gösterir.
type staticDeliveries struct{}

func (staticDeliveries) DeliverySnapshot(context.Context, string) (campaign.DeliverySnapshot, error) {
	return campaign.DeliverySnapshot{Status: "sending", Messages: campaign.DeliveryCounts{Queued: 1}}, nil
}

// startAPIServe, gerçek dashboard uç noktasını main.go'daki kurulumla (newServer)
// çalıştırır.
func startAPIServe(t *testing.T, timeout time.Duration, interval time.Duration) (addr string, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := newServer(ln.Addr().String(), func(shutdown <-chan struct{}) http.Handler {
		return httpapi.New(httpapi.Deps{
			Deliveries:       staticDeliveries{},
			DeliveryInterval: interval,
			Shutdown:         shutdown,
		})
	})
	errCh := make(chan error, 1)
	go func() { errCh <- serve(ctx, srv, ln, timeout) }()
	return ln.Addr().String(), cancel, errCh
}

// Açık bir dashboard akışı varken sunucu kapanırsa akış kendiliğinden biter ve
// serve, kapanış süresini (burada 5 sn) beklemeden temizce döner. SIGTERM sırasında
// aktif SSE akışı (açık kalan madde) bu testle kapsanır.
func TestServeShutsDownPromptlyWhileADashboardStreamIsOpen(t *testing.T) {
	addr, cancel, done := startAPIServe(t, 5*time.Second, time.Hour)

	resp, err := http.Get("http://" + addr + "/api/campaigns/44444444-4444-4444-4444-444444444444/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "event: snapshot") {
		t.Fatalf("akış başlamalı ve ilk olay snapshot olmalı: %q %v", line, err)
	}

	begin := time.Now()
	cancel() // SIGTERM
	if err := wantReturn(t, done, 3*time.Second); err != nil {
		t.Fatalf("temiz kapanış bekleniyordu (akış kendiliğinden bitmeli): %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 2*time.Second {
		t.Fatalf("kapanış açık akışı bekledi: %s", elapsed)
	}

	// İstemci tarafında da akış sona erer (EOF).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := br.ReadString('\n'); err != nil {
			return
		}
	}
	t.Fatal("kapanıştan sonra istemci akışı sürüyor")
}

// Kapanış başladıktan sonra yeni bağlantı kabul edilmez.
func TestServeRefusesNewDashboardStreamsAfterShutdownStarts(t *testing.T) {
	addr, cancel, done := startAPIServe(t, time.Second, time.Hour)
	cancel()
	if err := wantReturn(t, done, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("kapanan sunucu yeni bağlantı kabul etmemeli")
	}
}
