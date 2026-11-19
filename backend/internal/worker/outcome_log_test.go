package worker

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// providerReturning, her çağrıda aynı sonucu veren bir provider döndürür.
func providerReturning(res SendResult) *fakeProvider {
	return &fakeProvider{fn: func(context.Context, Job) SendResult { return res }}
}

// Regresyon: yeniden deneme logu her sonuç türü için "geçici hata; yeniden
// denenecek" diyordu. 429, 5xx, kayıp yanıt (sonuç belirsiz) ve geçersiz 2xx yanıt
// aynı etiketi alıyor, gerçek tür yalnızca hata metninden dolaylı çıkıyordu.
// Operatör için özellikle belirsiz sonuçlar ayırt edilmeliydi: provider isteği
// kabul etmiş olabilir.
func TestRetryLogNamesTheActualOutcomeKind(t *testing.T) {
	tests := []struct {
		kind    Kind
		res     SendResult
		want    []string // log satırında bulunmalı
		notWant []string // bulunmamalı
		level   string
	}{
		{KindRateLimited, SendResult{Kind: KindRateLimited, Err: "HTTP 429"},
			[]string{"hız sınırı", "429", "tür=rate_limited"}, []string{"geçici hata"}, "level=INFO"},
		{KindServerError, SendResult{Kind: KindServerError, Err: "HTTP 500 INTERNAL"},
			[]string{"sunucu hatası", "tür=server_error"}, []string{"geçici hata", "hız sınırı"}, "level=INFO"},
		{KindNetworkError, SendResult{Kind: KindNetworkError, Err: "yanıt alınamadı"},
			[]string{"yanıt alınamadı", "sonuç belirsiz", "aynı idempotency anahtarıyla", "tür=network_error"}, []string{"geçici hata"}, "level=WARN"},
		{KindInvalidResponse, SendResult{Kind: KindInvalidResponse, Err: "geçersiz yanıt: message_id yok"},
			[]string{"yanıtı geçersiz", "kabul doğrulanamadı", "aynı idempotency anahtarıyla", "tür=invalid_response"}, []string{"geçici hata"}, "level=WARN"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			var logs strings.Builder
			w := newTestWorker(t, testConfig(), newFakeRepo(), providerReturning(tt.res),
				WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))

			w.process(context.Background(), testJob()) // deneme 1 < azami: yeniden kuyruğa

			line := logs.String()
			for _, s := range tt.want {
				if !strings.Contains(line, s) {
					t.Errorf("log %q içermeli:\n%s", s, line)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(line, s) {
					t.Errorf("log %q içermemeli:\n%s", s, line)
				}
			}
			if !strings.Contains(line, tt.level) {
				t.Errorf("log düzeyi %s olmalı:\n%s", tt.level, line)
			}
		})
	}
}

// Deneme hakkı bitince ve kalıcı reddedilince log, mesajın neden başarısız
// olduğunu söyler: ikisi aynı "mesaj başarısız" değildir (ADR-006).
func TestFailureLogExplainsWhyTheMessageFailed(t *testing.T) {
	t.Run("exhausted", func(t *testing.T) {
		var logs strings.Builder
		job := testJob()
		job.Attempts = testConfig().MaxAttempts // hak bitti
		w := newTestWorker(t, testConfig(), newFakeRepo(),
			providerReturning(SendResult{Kind: KindNetworkError, Err: "yanıt alınamadı"}),
			WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))

		w.process(context.Background(), job)

		for _, s := range []string{"deneme sınırı doldu", "sonuç doğrulanamadı", "sebep=exhausted"} {
			if !strings.Contains(logs.String(), s) {
				t.Errorf("log %q içermeli:\n%s", s, logs.String())
			}
		}
	})
	t.Run("rejected", func(t *testing.T) {
		var logs strings.Builder
		w := newTestWorker(t, testConfig(), newFakeRepo(),
			providerReturning(SendResult{Kind: KindRejected, Err: "HTTP 422 INVALID_RECIPIENT"}),
			WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))

		w.process(context.Background(), testJob())

		for _, s := range []string{"kalıcı olarak reddetti", "yeniden denenmeyecek", "sebep=rejected"} {
			if !strings.Contains(logs.String(), s) {
				t.Errorf("log %q içermeli:\n%s", s, logs.String())
			}
		}
		if strings.Contains(logs.String(), "deneme sınırı") {
			t.Errorf("reddedilen mesaj deneme sınırı gibi anlatılmamalı:\n%s", logs.String())
		}
	})
}
