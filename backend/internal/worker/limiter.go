package worker

import (
	"context"
	"sync"
	"time"
)

// Limiter, bir kanalın provider çağrılarını hız sınırına tabi tutar. Her provider
// çağrısının (retry'lar dahil) önünde Wait çağrılır (ADR-005).
type Limiter interface {
	Wait(ctx context.Context) error
}

// IntervalLimiter, çağrıları en az 1/rate aralıkla planlar (burst 1). Süreç
// içindedir, replikalar arasında paylaşılmaz (ADR-005).
//
// Hedef 95/s'dir: brief'in 100/s sınırına pay bırakır. Planlama zamanları
// arasındaki aralık değişmez, ama gerçek istekler zamanlayıcı ve ağ
// gecikmeleri yüzünden birbirine yaklaşabilir; bu yüzden asıl sınır, provider'ın
// aldığı isteklerin kayan pencere testiyle doğrulanır.
type IntervalLimiter struct {
	interval time.Duration

	mu   sync.Mutex
	next time.Time

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// NewIntervalLimiter, saniyede ratePerSecond çağrıya izin veren bir limiter
// oluşturur. ratePerSecond pozitif olmalıdır.
func NewIntervalLimiter(ratePerSecond float64) *IntervalLimiter {
	return &IntervalLimiter{
		interval: time.Duration(float64(time.Second) / ratePerSecond),
		now:      time.Now,
		sleep:    sleepContext,
	}
}

// Wait, sıradaki çağrı yuvası gelene kadar bekler. Yuvalar çağrı sırasına göre
// verilir; bekleyen çağrı ctx iptal edilince ctx.Err() ile döner. İptal edilen
// çağrının yuvası boşa gider (sonraki çağrılar öne alınmaz), bu da sınırı
// aşmaz.
func (l *IntervalLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := l.now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()

	return l.sleep(ctx, slot.Sub(now))
}

// sleepContext, ctx iptal edilirse erken döner.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
