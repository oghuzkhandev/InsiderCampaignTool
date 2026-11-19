// Package worker, onaylanan kampanyaların mesajlarını kuyruktan alıp mock
// provider'a gönderir (SND-001, ADR-002, ADR-005, ADR-006).
//
// Kuyruk PostgreSQL'dedir. Worker boş kapasitesi kadar işi `FOR UPDATE SKIP
// LOCKED` ile sahiplenir, her provider çağrısının önünde kanal başına ayrı hız
// sınırı bekler, gönderimden hemen önce kanal iznini ve sahipliği kontrol eder
// ve sonucu yalnızca güncel claim token ile yazar.
package worker

import (
	"math/rand/v2"
	"time"
)

// failure_reason değerleri (ADR-006). Başka değer yoktur.
const (
	// FailureExhausted: deneme sınırı doldu; son deneme geçici hatayla (429,
	// 5xx, timeout, bağlantı hatası, lease kaybı) bitti ve sonuç doğrulanamadı.
	FailureExhausted = "exhausted"
	// FailureRejected: provider, 429 dışındaki bir 4xx ile isteği kesin reddetti.
	FailureRejected = "rejected"
)

// Action, bir gönderim denemesinin ardından mesaja uygulanacak işlemdir.
type Action int

const (
	// ActionSent: provider kabul etti; mesaj `sent` olur.
	ActionSent Action = iota + 1
	// ActionRequeue: geçici hata ve deneme hakkı var; mesaj gecikmeyle `queued` olur.
	ActionRequeue
	// ActionFail: mesaj `failed` olur.
	ActionFail
)

// Decision, bir denemenin sonucuna göre verilen karardır.
type Decision struct {
	Action Action
	// Delay, ActionRequeue için bir sonraki denemeye kadar beklemedir.
	Delay time.Duration
	// FailureReason, ActionFail için failure_reason değeridir.
	FailureReason string
	// LastError, kısa hata açıklamasıdır (last_error).
	LastError string
}

// Decide, provider sonucuna göre mesaja ne yapılacağını belirler (ADR-006).
//
//   - Kabul: sent.
//   - 429 dışındaki 4xx: retry yok, failed/rejected.
//   - 429, 5xx, timeout ve bağlantı hataları: deneme hakkı varsa backoff ile
//     yeniden kuyruğa; `attempts` azami değere ulaşmışsa doğrudan failed/exhausted.
//   - 429'da Retry-After backoff'tan büyükse ona uyulur: o süreden önce
//     yeniden deneme yapılmaz.
//
// attempts, mesajın şimdiye kadarki claim sayısıdır (bu deneme dahil).
func Decide(res SendResult, attempts, maxAttempts int, backoff func(attempt int) time.Duration) Decision {
	switch res.Kind {
	case KindAccepted:
		return Decision{Action: ActionSent}
	case KindRejected:
		return Decision{Action: ActionFail, FailureReason: FailureRejected, LastError: res.Err}
	}
	if attempts >= maxAttempts {
		return Decision{Action: ActionFail, FailureReason: FailureExhausted, LastError: res.Err}
	}
	return Decision{Action: ActionRequeue, Delay: max(backoff(attempts), res.RetryAfter), LastError: res.Err}
}

// Backoff, tam jitter'lı üstel bekleme hesaplar (ARCHITECTURE §8): n. deneme
// için [0, min(Cap, Base·2^(n-1))] aralığından rastgele bir süre.
type Backoff struct {
	Base time.Duration
	Cap  time.Duration
	// Rand, [0,1) döndürür; testlerde belirlenimli yapılır.
	Rand func() float64
}

// DefaultBackoff, taban 2 sn, tavan 60 sn'lik backoff'tur.
func DefaultBackoff() Backoff {
	return Backoff{Base: 2 * time.Second, Cap: 60 * time.Second, Rand: rand.Float64}
}

// Ceiling, n. deneme için jitter öncesi üst sınırı döndürür.
func (b Backoff) Ceiling(attempt int) time.Duration {
	ceil := b.Base
	for i := 1; i < attempt && ceil < b.Cap; i++ {
		ceil *= 2
	}
	return min(ceil, b.Cap)
}

// Delay, n. deneme (n >= 1) sonrası beklenecek süredir.
func (b Backoff) Delay(attempt int) time.Duration {
	r := b.Rand
	if r == nil {
		r = rand.Float64
	}
	return time.Duration(r() * float64(b.Ceiling(attempt)))
}
