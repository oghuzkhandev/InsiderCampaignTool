package worker

import (
	"testing"
	"time"
)

func fixedBackoff(d time.Duration) func(int) time.Duration {
	return func(int) time.Duration { return d }
}

func TestDecide(t *testing.T) {
	const max = 5
	tests := []struct {
		name     string
		res      SendResult
		attempts int
		backoff  time.Duration
		want     Decision
	}{
		{"kabul", SendResult{Kind: KindAccepted}, 1, time.Second, Decision{Action: ActionSent}},
		{"kabul son denemede", SendResult{Kind: KindAccepted}, max, time.Second, Decision{Action: ActionSent}},

		// 429 dışındaki 4xx kalıcıdır: ilk denemede bile retry yok.
		{"4xx kalıcı", SendResult{Kind: KindRejected, Err: "HTTP 400 INVALID_RECIPIENT"}, 1, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureRejected, LastError: "HTTP 400 INVALID_RECIPIENT"}},
		{"anahtar çakışması 422 kalıcı", SendResult{Kind: KindRejected, Err: "HTTP 422 IDEMPOTENCY_KEY_REUSED"}, 2, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureRejected, LastError: "HTTP 422 IDEMPOTENCY_KEY_REUSED"}},

		// 429 ayrıdır ve her zaman geçicidir.
		{"429 yeniden denenir", SendResult{Kind: KindRateLimited, Err: "HTTP 429"}, 1, 2 * time.Second,
			Decision{Action: ActionRequeue, Delay: 2 * time.Second, LastError: "HTTP 429"}},
		{"429 Retry-After backoff'tan büyük", SendResult{Kind: KindRateLimited, RetryAfter: 30 * time.Second, Err: "HTTP 429"}, 1, 2 * time.Second,
			Decision{Action: ActionRequeue, Delay: 30 * time.Second, LastError: "HTTP 429"}},
		{"429 Retry-After backoff'tan küçük", SendResult{Kind: KindRateLimited, RetryAfter: time.Second, Err: "HTTP 429"}, 3, 8 * time.Second,
			Decision{Action: ActionRequeue, Delay: 8 * time.Second, LastError: "HTTP 429"}},

		{"5xx yeniden denenir", SendResult{Kind: KindServerError, Err: "HTTP 500"}, 2, 4 * time.Second,
			Decision{Action: ActionRequeue, Delay: 4 * time.Second, LastError: "HTTP 500"}},
		{"timeout yeniden denenir", SendResult{Kind: KindNetworkError, Err: "zaman aşımı"}, 4, 16 * time.Second,
			Decision{Action: ActionRequeue, Delay: 16 * time.Second, LastError: "zaman aşımı"}},

		// 2xx ama doğrulanamayan yanıt: sent değil, geçici hata gibi aynı anahtarla yeniden denenir.
		{"geçersiz yanıt yeniden denenir", SendResult{Kind: KindInvalidResponse, Err: "geçersiz yanıt: message_id yok"}, 2, 4 * time.Second,
			Decision{Action: ActionRequeue, Delay: 4 * time.Second, LastError: "geçersiz yanıt: message_id yok"}},
		{"son denemede geçersiz yanıt", SendResult{Kind: KindInvalidResponse, Err: "geçersiz yanıt: gövde okunamadı"}, max, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureExhausted, LastError: "geçersiz yanıt: gövde okunamadı"}},

		// Deneme hakkı bitmişse geçici hata doğrudan exhausted olur (kuyruğa dönmez).
		{"son denemede 5xx", SendResult{Kind: KindServerError, Err: "HTTP 500"}, max, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureExhausted, LastError: "HTTP 500"}},
		{"son denemede 429", SendResult{Kind: KindRateLimited, RetryAfter: time.Second, Err: "HTTP 429"}, max, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureExhausted, LastError: "HTTP 429"}},
		{"son denemede yanıt kaybı", SendResult{Kind: KindNetworkError, Err: "bağlantı hatası"}, max, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureExhausted, LastError: "bağlantı hatası"}},
		{"sınırı aşmış sayaç", SendResult{Kind: KindServerError, Err: "x"}, max + 1, time.Second,
			Decision{Action: ActionFail, FailureReason: FailureExhausted, LastError: "x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.res, tc.attempts, max, fixedBackoff(tc.backoff)); got != tc.want {
				t.Fatalf("Decide = %+v, beklenen %+v", got, tc.want)
			}
		})
	}
}

// Backoff, deneme numarasını bilerek çağrılır: n. deneme için n verilir.
func TestDecidePassesAttemptNumberToBackoff(t *testing.T) {
	var got []int
	for attempts := 1; attempts <= 3; attempts++ {
		Decide(SendResult{Kind: KindServerError}, attempts, 5, func(a int) time.Duration {
			got = append(got, a)
			return 0
		})
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("backoff deneme numaralarıyla çağrılmalı: %v", got)
	}
}

func TestBackoffCeilingDoublesUpToCap(t *testing.T) {
	b := DefaultBackoff()
	want := map[int]time.Duration{
		1: 2 * time.Second, 2: 4 * time.Second, 3: 8 * time.Second, 4: 16 * time.Second,
		5: 32 * time.Second, 6: 60 * time.Second, 7: 60 * time.Second, 100: 60 * time.Second,
	}
	for attempt, w := range want {
		if got := b.Ceiling(attempt); got != w {
			t.Errorf("Ceiling(%d) = %s, beklenen %s", attempt, got, w)
		}
	}
}

func TestBackoffDelayIsFullJitter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		r       float64
		attempt int
		want    time.Duration
	}{
		{"alt sınır sıfır olabilir", 0, 3, 0},
		{"orta", 0.5, 3, 4 * time.Second}, // tavan 8 sn
		{"üst sınıra yakın", 0.999, 1, time.Duration(0.999 * float64(2*time.Second))},
		{"tavan uygulanır", 0.5, 10, 30 * time.Second}, // tavan 60 sn
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := Backoff{Base: 2 * time.Second, Cap: 60 * time.Second, Rand: func() float64 { return tc.r }}
			if got := b.Delay(tc.attempt); got != tc.want {
				t.Fatalf("Delay = %s, beklenen %s", got, tc.want)
			}
		})
	}
}

func TestBackoffDelayNeverExceedsCeiling(t *testing.T) {
	b := DefaultBackoff()
	for attempt := 1; attempt <= 8; attempt++ {
		for i := 0; i < 200; i++ {
			if d := b.Delay(attempt); d < 0 || d >= b.Ceiling(attempt) {
				t.Fatalf("Delay(%d) = %s, [0, %s) aralığında olmalı", attempt, d, b.Ceiling(attempt))
			}
		}
	}
}
