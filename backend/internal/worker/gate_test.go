package worker

import (
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"
)

func gateWith(rate float64, burst int, c *fakeClock) *TokenGate {
	g := NewTokenGate(rate, burst)
	g.now = c.Now
	return g
}

func TestTokenGateStartsFullAndRefusesWhenEmpty(t *testing.T) {
	c := newFakeClock()
	g := gateWith(100, 2, c) // 10 ms/jeton, burst 2

	if w := g.Take(); w != 0 {
		t.Fatalf("1. jeton hemen verilmeli: %s", w)
	}
	if w := g.Take(); w != 0 {
		t.Fatalf("2. jeton (burst) hemen verilmeli: %s", w)
	}
	if w := g.Take(); w != 10*time.Millisecond {
		t.Fatalf("kova boş: bir sonraki jetona 10 ms beklenmeli: %s", w)
	}
}

func TestTokenGateRefillsAtTheConfiguredRateAndRefusalConsumesNothing(t *testing.T) {
	c := newFakeClock()
	g := gateWith(100, 1, c)
	g.Take() // kova boş

	// Reddedilen çağrılar jeton tüketmez ve bekleme süresi azalır.
	if w := g.Take(); w != 10*time.Millisecond {
		t.Fatalf("bekleme 10 ms olmalı: %s", w)
	}
	c.Advance(4 * time.Millisecond)
	if w := g.Take(); w != 6*time.Millisecond {
		t.Fatalf("4 ms sonra 6 ms kalmalı: %s", w)
	}
	c.Advance(6 * time.Millisecond)
	if w := g.Take(); w != 0 {
		t.Fatalf("bekleme sonrası jeton verilmeli: %s", w)
	}
	if w := g.Take(); w == 0 {
		t.Fatal("jeton tüketildi, hemen ikincisi verilmemeli")
	}
}

// Uzun boşluk burst'ten fazla jeton biriktirmez: duraklamadan sonra en çok burst iş
// aynı anda geçer.
func TestTokenGateDoesNotBankMoreThanBurstAfterALongIdle(t *testing.T) {
	c := newFakeClock()
	g := gateWith(95, 2, c)
	g.Take()
	c.Advance(10 * time.Second) // uzun duraklama

	passed := 0
	for i := 0; i < 20; i++ {
		if g.Take() == 0 {
			passed++
		}
	}
	if passed != 2 {
		t.Fatalf("duraklamadan sonra tam burst (2) iş geçmeli: %d", passed)
	}
}

// Kanıtlı sınır: herhangi bir T süresinde en çok burst + rate·T gönderim. Rastgele
// varış ve yeniden deneme zamanlarıyla, duraklamalar ve birikmiş bekleyenler dahil,
// farklı pencere uzunluklarında doğrulanır.
func TestTokenGateNeverAdmitsMoreThanBurstPlusRateTimesWindow(t *testing.T) {
	for _, cfg := range []struct {
		rate  float64
		burst int
	}{{95, 2}, {40, 1}, {200, 3}, {10, 2}} {
		c := newFakeClock()
		g := gateWith(cfg.rate, cfg.burst, c)
		rng := rand.New(rand.NewPCG(7, uint64(cfg.burst)))

		var admitted []time.Time
		// 30 "iş": her biri reddedilince önerilen süre kadar bekleyip yeniden dener;
		// ara sıra bütün zaman 700 ms atlar (süreç duraklaması) ve herkes aynı anda uyanır.
		next := make([]time.Time, 30)
		for i := range next {
			next[i] = c.Now()
		}
		for step := 0; step < 20000; step++ {
			c.Advance(time.Duration(rng.IntN(3000)) * time.Microsecond)
			if rng.IntN(2000) == 0 {
				c.Advance(700 * time.Millisecond)
			}
			for i := range next {
				if c.Now().Before(next[i]) {
					continue
				}
				if w := g.Take(); w == 0 {
					admitted = append(admitted, c.Now())
					next[i] = c.Now() // hemen bir sonraki işe geç
				} else {
					next[i] = c.Now().Add(w)
				}
			}
		}
		if len(admitted) < 100 {
			t.Fatalf("%+v: test yeterince gönderim üretmedi: %d", cfg, len(admitted))
		}
		for _, window := range []time.Duration{20 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second} {
			peak := windowPeak(admitted, window)
			limit := float64(cfg.burst) + cfg.rate*window.Seconds()
			if float64(peak) > limit+1e-9 {
				t.Errorf("hız %v burst %d: %s penceresinde %d gönderim, sınır %.2f", cfg.rate, cfg.burst, window, peak, limit)
			}
		}
	}
}

func windowPeak(ts []time.Time, w time.Duration) int {
	sorted := slices.Clone(ts)
	slices.SortFunc(sorted, func(a, b time.Time) int { return a.Compare(b) })
	best, j := 0, 0
	for i := range sorted {
		if j < i {
			j = i
		}
		for j < len(sorted) && sorted[j].Sub(sorted[i]) < w {
			j++
		}
		best = max(best, j-i)
	}
	return best
}

// Gerçek zamanla ve gerçek eşzamanlılıkla: çok sayıda goroutine kapıyı zorlar, her
// kayan 1 sn pencerede en çok rate + burst gönderim geçer.
func TestTokenGateBoundsConcurrentCallersInRealTime(t *testing.T) {
	const (
		rate  = 200.0
		burst = 2
	)
	g := NewTokenGate(rate, burst)

	var (
		mu       sync.Mutex
		admitted []time.Time
		wg       sync.WaitGroup
	)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				if w := g.Take(); w > 0 {
					time.Sleep(w)
					continue
				}
				mu.Lock()
				admitted = append(admitted, time.Now())
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	// +1: zaman damgası kaydı ile jeton alma arasındaki zamanlayıcı kayması payı.
	if peak, limit := windowPeak(admitted, time.Second), int(rate)+burst+1; peak > limit {
		t.Fatalf("kayan 1 sn pencerede %d gönderim; sınır %d", peak, limit)
	}
	if len(admitted) < 200 {
		t.Fatalf("kapı hızı hiç kullanılamamış: %d gönderim", len(admitted))
	}
}

// fakeGate, Take sonuçlarını sırayla verir; çağrı sayısını tutar.
type fakeGate struct {
	mu    sync.Mutex
	waits []time.Duration // sırayla; bitince 0
	calls int
}

func (g *fakeGate) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *fakeGate) Take() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if len(g.waits) == 0 {
		return 0
	}
	w := g.waits[0]
	g.waits = g.waits[1:]
	return w
}
