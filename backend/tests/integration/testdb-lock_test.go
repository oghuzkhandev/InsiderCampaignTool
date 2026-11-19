package integration

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// probeTB, testdb.Open'ın goroutine'den güvenle çağrılmasını sağlar: Cleanup'ları
// biriktirir (kilit çağıran test bitince değil, istenen anda bırakılır) ve
// Fatalf'u test goroutine'i dışında güvenli olacak şekilde Goexit'e çevirir.
type probeTB struct {
	testing.TB
	mu       sync.Mutex
	cleanups []func()
	failure  string
}

func (p *probeTB) Cleanup(f func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanups = append(p.cleanups, f)
}

func (p *probeTB) Fatalf(format string, args ...any) {
	p.mu.Lock()
	p.failure = fmt.Sprintf(format, args...)
	p.mu.Unlock()
	runtime.Goexit()
}

// runCleanups, biriken Cleanup'ları testing ile aynı sırada (sondan başa) çalıştırır.
func (p *probeTB) runCleanups() {
	p.mu.Lock()
	fns := p.cleanups
	p.cleanups = nil
	p.mu.Unlock()
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}

// `go test ./...` paketleri eşzamanlı çalıştırır. campaigns_test'i birden çok paket
// kullandığında (integration ve e2e), biri migration Down/Up veya TRUNCATE
// yaparken diğeri çalışmamalı: testdb.Open, süreçler arası bir PostgreSQL
// advisory lock alır. Başka bir süreç kilidi tutarken Open beklemeli, kilit
// bırakılınca devam etmelidir.
func TestTestDBOpenWaitsWhileAnotherProcessHoldsTheTestDatabase(t *testing.T) {
	// Veritabanı ilk koşuda yoksa oluşsun; alt test bitince kilit bırakılır.
	t.Run("hazırlık", func(t *testing.T) { testdb.Open(t) })

	url, err := testdb.URL()
	if err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("pgx", url) // "başka bir test süreci"
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	conn, err := other.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, testdb.LockKey); err != nil {
		t.Fatal(err)
	}
	held := true
	defer func() {
		if held { // test erken biterse kilit bağlantıyla birlikte düşer; yine de açıkça bırak
			_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, testdb.LockKey)
		}
	}()

	probe := &probeTB{TB: t}
	var opened atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		testdb.Open(probe)
		opened.Store(true)
	}()

	time.Sleep(500 * time.Millisecond)
	if opened.Load() {
		t.Fatal("başka süreç kilidi tutarken testdb.Open beklemeliydi (paketler arası yarış: migration ve TRUNCATE çakışır)")
	}

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, testdb.LockKey); err != nil {
		t.Fatal(err)
	}
	held = false
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("kilit bırakıldı ama testdb.Open devam etmedi")
	}
	probe.mu.Lock()
	failure := probe.failure
	probe.mu.Unlock()
	if failure != "" || !opened.Load() {
		t.Fatalf("testdb.Open kilit bırakılınca başarıyla dönmeli: %q", failure)
	}

	// Test bitince (Cleanup) kilit bırakılır: başka bir Open takılmadan açılır.
	probe.runCleanups()
	released := make(chan struct{})
	go func() {
		defer close(released)
		p2 := &probeTB{TB: t}
		testdb.Open(p2)
		p2.runCleanups()
	}()
	select {
	case <-released:
	case <-time.After(15 * time.Second):
		t.Fatal("Cleanup kilidi bırakmadı: sonraki testdb.Open takıldı")
	}
}
