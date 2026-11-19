package worker

import (
	"math"
	"sync"
	"time"
)

// Gate, GERÇEK GÖNDERİM ANINDAKİ hız kapısıdır: provider çağrısından hemen önce
// (ön kontrolden sonra) bir kez sorulur. Limiter (IntervalLimiter) işleri
// ön kontrolden ÖNCE aralıklar; ama limiter'dan geçen işler ön kontrolde
// (veritabanı) veya bir süreç duraklamasında (GC, SIGSTOP, VM dondurma) takılıp
// birlikte serbest kalabilir. Kapı hızı gönderim anında, gerçek saatle zorlar ve
// böyle bir birikimin kayan pencere sınırını aşmasını engeller (ADR-005).
type Gate interface {
	// Take, bir gönderim jetonu almaya çalışır. 0 döndürürse izin verilmiştir
	// (jeton tüketildi) ve çağıran hemen göndermelidir. Sıfırdan büyük döndürürse
	// jeton tüketilmemiştir: o kadar beklenip (ön kontrol yenilenerek) yeniden
	// denenmelidir.
	Take() time.Duration
}

// TokenGate, jeton kovasıdır: hız saniyede rate jeton, kapasite burst jeton. Kova
// başlangıçta doludur. Kanıtlı üst sınır: herhangi bir T süresinde en çok
// burst + rate·T gönderim. Boşta geçen süre burst'ten fazla birikmez; böylece uzun
// bir duraklamadan sonra bile en çok burst iş aynı anda geçer, gerisi hızla akar.
type TokenGate struct {
	rate  float64
	burst float64

	mu     sync.Mutex
	tokens float64
	last   time.Time

	now func() time.Time
}

// NewTokenGate, saniyede ratePerSecond jeton üreten ve en çok burst jeton biriktiren
// bir kapı oluşturur. Her ikisi de pozitif olmalıdır.
func NewTokenGate(ratePerSecond float64, burst int) *TokenGate {
	return &TokenGate{
		rate:   ratePerSecond,
		burst:  float64(burst),
		tokens: float64(burst),
		now:    time.Now,
	}
}

// Take, Gate arayüzünü uygular.
func (g *TokenGate) Take() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	if g.last.IsZero() {
		g.last = now
	}
	if elapsed := now.Sub(g.last); elapsed > 0 {
		g.tokens = math.Min(g.burst, g.tokens+elapsed.Seconds()*g.rate)
		g.last = now
	}
	if g.tokens >= 1 {
		g.tokens--
		return 0
	}
	// Bir sonraki jetona kadar kalan süre (yukarı yuvarlanır: erken uyanıp tekrar
	// reddedilmesin).
	return time.Duration(math.Ceil((1 - g.tokens) / g.rate * float64(time.Second)))
}
