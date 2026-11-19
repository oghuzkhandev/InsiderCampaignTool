package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
)

// delayLimiter, gerçek bir hız sınırı beklemesi taklit eder.
type delayLimiter struct{ d time.Duration }

func (l delayLimiter) Wait(ctx context.Context) error {
	t := time.NewTimer(l.d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitSnapshot(w *Worker) metrics.HistogramSnapshot { return w.Metrics().rateLimitWait.Snapshot() }

func exposition(t *testing.T, m *Metrics) string {
	t.Helper()
	var b strings.Builder
	mw := metrics.NewWriter(&b)
	m.Write(mw)
	if mw.Err() != nil {
		t.Fatal(mw.Err())
	}
	return b.String()
}

// Limiter beklemesi ratelimit_wait_seconds'a yansır: tek deneme, tek gözlem.
func TestRateLimitWaitRecordsLimiterWait(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov, WithLimiter(ChannelSMS, delayLimiter{d: 40 * time.Millisecond}))

	w.process(context.Background(), testJob())

	s := waitSnapshot(w)
	if s.Count != 1 {
		t.Fatalf("deneme başına tek gözlem bekleniyordu: %d", s.Count)
	}
	if s.SumSeconds < 0.04 || s.SumSeconds > 1 {
		t.Fatalf("limiter beklemesi (≥40 ms) yansımalı: %v sn", s.SumSeconds)
	}
}

// Gönderim anı kapısının bekletmeleri de toplama eklenir (her biri ayrı ayrı değil,
// denemenin toplam beklemesi tek gözlem).
func TestRateLimitWaitAddsGateWaitsIntoASingleObservation(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	gate := &fakeGate{waits: []time.Duration{20 * time.Millisecond, 30 * time.Millisecond}}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))
	w.sleep = func(context.Context, time.Duration) error { return nil } // beklemeden geç

	w.process(context.Background(), testJob())

	s := waitSnapshot(w)
	if s.Count != 1 {
		t.Fatalf("tek gözlem bekleniyordu: %d", s.Count)
	}
	if s.SumSeconds < 0.05 || s.SumSeconds > 0.5 {
		t.Fatalf("kapı beklemeleri (20+30 ms) toplanmalı: %v sn", s.SumSeconds)
	}
}

// Hiç beklemeyen deneme de gözlenir (sıfıra yakın): dağılım, işlerin ne kadarının
// beklemediğini gösterebilsin.
func TestRateLimitWaitObservesAttemptsThatDidNotWait(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob())

	if s := waitSnapshot(w); s.Count != 1 || s.SumSeconds > 0.5 {
		t.Fatalf("beklemeyen deneme gözlenmeli: %+v", s)
	}
}

// Kapı beklerken kapanış gelirse bekleme yaşandığı kadarıyla gözlenir ve gönderim olmaz.
func TestRateLimitWaitIsObservedWhenShutdownInterruptsTheGate(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	gate := &fakeGate{waits: []time.Duration{time.Hour}}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))
	w.sleep = func(context.Context, time.Duration) error { return errors.New("kapanış") }

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 0 {
		t.Fatal("kapanışta gönderim olmamalı")
	}
	if s := waitSnapshot(w); s.Count != 1 {
		t.Fatalf("gözlem bekleniyordu: %+v", s)
	}
}

// Limiter beklerken iptal edilen iş (kapanış) gözlenmez: provider çağrısına hiç
// yaklaşmadı ve lease ile kurtarılacak.
func TestRateLimitWaitIsNotObservedWhenCancelledInsideTheLimiter(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov, WithLimiter(ChannelSMS, delayLimiter{d: time.Hour}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w.process(ctx, testJob())

	if s := waitSnapshot(w); s.Count != 0 {
		t.Fatalf("iptal edilen bekleme gözlenmemeli: %+v", s)
	}
}

func TestMetricsWriteExposesEverySeriesEvenBeforeAnyActivity(t *testing.T) {
	out := exposition(t, NewMetrics())
	for _, want := range []string{
		"# TYPE messages_sent_total counter\nmessages_sent_total 0\n",
		"# TYPE send_attempts_total counter\n",
		`send_attempts_total{outcome="accepted"} 0`,
		`send_attempts_total{outcome="rate_limited"} 0`,
		`send_attempts_total{outcome="server_error"} 0`,
		`send_attempts_total{outcome="network_error"} 0`,
		`send_attempts_total{outcome="rejected"} 0`,
		`send_attempts_total{outcome="invalid_response"} 0`,
		"stale_result_total 0\n",
		"reaper_requeued_total 0\n",
		"# TYPE ratelimit_wait_seconds histogram\n",
		`ratelimit_wait_seconds_bucket{le="+Inf"} 0`,
		"ratelimit_wait_seconds_count 0\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("çıktıda %q yok:\n%s", want, out)
		}
	}
}

// Sıfır değerli Metrics (NewMetrics kullanılmadan) çökmez.
func TestZeroValueMetricsWriteDoesNotPanic(t *testing.T) {
	out := exposition(t, &Metrics{})
	if !strings.Contains(out, "messages_sent_total 0\n") {
		t.Fatalf("çıktı: %s", out)
	}
}

// Gönderim sonrası sayaçlar çıktıya yansır; API'nin göremediği worker sayaçları
// yalnızca worker çıktısındadır (ADR-016).
func TestMetricsWriteReflectsProcessedMessage(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob())

	out := exposition(t, w.Metrics())
	for _, want := range []string{
		"messages_sent_total 1\n",
		`send_attempts_total{outcome="accepted"} 1`,
		"ratelimit_wait_seconds_count 1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("çıktıda %q yok:\n%s", want, out)
		}
	}
}
