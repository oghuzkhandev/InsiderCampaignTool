package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClock, uyumayan ve istenen zamana ilerletilebilen bir saattir. Wait'in
// uyku isteklerini kaydeder.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sleeps = append(c.sleeps, d)
	return nil
}

func limiterWith(rate float64, c *fakeClock) *IntervalLimiter {
	l := NewIntervalLimiter(rate)
	l.now, l.sleep = c.Now, c.Sleep
	return l
}

func TestIntervalLimiterSpacesCallsByTheInterval(t *testing.T) {
	c := newFakeClock()
	l := limiterWith(100, c) // 10 ms

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Zaman ilerlemediği için her çağrının yuvası bir aralık sonradır: 0, 10, 20, 30 ms.
	want := []time.Duration{0, 10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	if len(c.sleeps) != len(want) {
		t.Fatalf("uyku sayısı = %d: %v", len(c.sleeps), c.sleeps)
	}
	for i := range want {
		if c.sleeps[i] != want[i] {
			t.Fatalf("%d. çağrının beklemesi = %s, beklenen %s (hepsi: %v)", i, c.sleeps[i], want[i], c.sleeps)
		}
	}
}

func TestIntervalLimiterDoesNotBankIdleTime(t *testing.T) {
	// Burst 1: uzun bir boşluktan sonra bile arka arkaya iki çağrı aynı anda
	// geçemez.
	c := newFakeClock()
	l := limiterWith(100, c)
	ctx := context.Background()

	_ = l.Wait(ctx)
	c.Advance(5 * time.Second) // uzun boşluk
	_ = l.Wait(ctx)
	_ = l.Wait(ctx)

	if got := c.sleeps[1]; got != 0 {
		t.Fatalf("boşluktan sonraki ilk çağrı beklememeli: %s", got)
	}
	if got := c.sleeps[2]; got != 10*time.Millisecond {
		t.Fatalf("ardından gelen çağrı bir aralık beklemeli (birikmiş hak yok): %s", got)
	}
}

func TestIntervalLimiterRateOf95PerSecond(t *testing.T) {
	rate := 95.0
	l := NewIntervalLimiter(rate)
	want := time.Duration(float64(time.Second) / rate)
	if l.interval != want {
		t.Fatalf("aralık = %s, beklenen %s", l.interval, want)
	}
}

func TestIntervalLimiterReturnsContextErrorWhenCancelled(t *testing.T) {
	l := NewIntervalLimiter(1) // 1 sn aralık
	ctx, cancel := context.WithCancel(context.Background())

	if err := l.Wait(ctx); err != nil { // ilk yuva hemen
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- l.Wait(ctx) }() // ikinci yuva ~1 sn sonra
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("context.Canceled bekleniyordu: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("iptal edilen Wait hemen dönmeli")
	}
}

// Gerçek zamanla: eşzamanlı çağrılar yine de aralıkla ayrılır.
func TestIntervalLimiterSpacesConcurrentCallsInRealTime(t *testing.T) {
	const n = 30
	l := NewIntervalLimiter(500) // 2 ms aralık

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Wait(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	// n çağrı en az (n-1)·aralık sürer; limiter hiçbir çağrıyı erken bırakmamalı.
	elapsed, atLeast := time.Since(start), time.Duration(n-1)*2*time.Millisecond
	if elapsed < atLeast-2*time.Millisecond {
		t.Fatalf("%d çağrı %s sürdü, en az ~%s beklenirdi", n, elapsed, atLeast)
	}
}
