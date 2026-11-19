package metrics

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func render(t *testing.T, f func(w *Writer)) string {
	t.Helper()
	var b strings.Builder
	w := NewWriter(&b)
	f(w)
	if w.Err() != nil {
		t.Fatal(w.Err())
	}
	return b.String()
}

func TestCounterWritesHelpTypeAndValue(t *testing.T) {
	got := render(t, func(w *Writer) { w.Counter("messages_sent_total", "Gönderilen mesaj.", 7) })
	want := "# HELP messages_sent_total Gönderilen mesaj.\n# TYPE messages_sent_total counter\nmessages_sent_total 7\n"
	if got != want {
		t.Fatalf("çıktı:\n%q\nbeklenen:\n%q", got, want)
	}
}

func TestLabeledFamiliesWriteHeaderOnceAndEscapeLabelValues(t *testing.T) {
	got := render(t, func(w *Writer) {
		w.GaugeVec("queue_depth", "Kuyruk.", []LabeledValue{
			{Labels: []Label{{"channel", "email"}}, Value: 4},
			{Labels: []Label{{"channel", `s"m\s` + "\n"}}, Value: 0},
		})
	})
	want := "# HELP queue_depth Kuyruk.\n# TYPE queue_depth gauge\n" +
		`queue_depth{channel="email"} 4` + "\n" +
		`queue_depth{channel="s\"m\\s\n"} 0` + "\n"
	if got != want {
		t.Fatalf("çıktı:\n%q\nbeklenen:\n%q", got, want)
	}
	if strings.Count(got, "# TYPE") != 1 {
		t.Fatal("TYPE satırı bir kez yazılmalı")
	}
}

func TestHelpEscapesNewlineAndBackslash(t *testing.T) {
	got := render(t, func(w *Writer) { w.Counter("x_total", "a\nb\\c", 0) })
	if !strings.HasPrefix(got, "# HELP x_total a\\nb\\\\c\n") {
		t.Fatalf("HELP kaçışı yanlış: %q", got)
	}
}

func TestCounterIgnoresNegativeAndNilIsSafe(t *testing.T) {
	var c Counter
	c.Add(3)
	c.Add(-5)
	c.Add(0)
	if c.Value() != 3 {
		t.Fatalf("sayaç azalmamalı: %d", c.Value())
	}
	var nilCounter *Counter
	nilCounter.Inc() // panik vermemeli
	if nilCounter.Value() != 0 {
		t.Fatal("nil sayaç 0 olmalı")
	}
}

func TestCounterIsSafeUnderConcurrency(t *testing.T) {
	var c Counter
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if c.Value() != 5000 {
		t.Fatalf("beklenen 5000: %d", c.Value())
	}
}

func TestHistogramBucketsAreCumulativeAndInfEqualsCount(t *testing.T) {
	h := NewHistogram([]float64{0.01, 0.1, 1})
	for _, d := range []time.Duration{
		5 * time.Millisecond,   // le 0.01
		10 * time.Millisecond,  // sınırda: le 0.01 (le = küçük eşit)
		50 * time.Millisecond,  // le 0.1
		2 * time.Second,        // +Inf
		-1 * time.Millisecond,  // negatif sıfır sayılır: le 0.01
		900 * time.Millisecond, // le 1
	} {
		h.Observe(d)
	}
	s := h.Snapshot()
	wantCum := []int64{3, 4, 5, 6}
	for i, w := range wantCum {
		if s.Cumulative[i] != w {
			t.Fatalf("kova %d: %d, beklenen %d (hepsi %v)", i, s.Cumulative[i], w, s.Cumulative)
		}
	}
	if s.Count != 6 {
		t.Fatalf("Count: %d", s.Count)
	}
	if got, want := s.SumSeconds, 0.005+0.010+0.050+2+0+0.9; got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("Sum: %v, beklenen %v", got, want)
	}

	out := render(t, func(w *Writer) { w.Histogram("ratelimit_wait_seconds", "Bekleme.", s) })
	for _, line := range []string{
		"# TYPE ratelimit_wait_seconds histogram",
		`ratelimit_wait_seconds_bucket{le="0.01"} 3`,
		`ratelimit_wait_seconds_bucket{le="0.1"} 4`,
		`ratelimit_wait_seconds_bucket{le="1"} 5`,
		`ratelimit_wait_seconds_bucket{le="+Inf"} 6`,
		"ratelimit_wait_seconds_count 6",
	} {
		if !strings.Contains(out, line+"\n") {
			t.Errorf("çıktıda %q yok:\n%s", line, out)
		}
	}
}

func TestEmptyHistogramStillExposesZeroSeries(t *testing.T) {
	out := render(t, func(w *Writer) {
		w.Histogram("h_seconds", "H.", NewHistogram(DefaultWaitBuckets).Snapshot())
	})
	if !strings.Contains(out, `h_seconds_bucket{le="+Inf"} 0`+"\n") || !strings.Contains(out, "h_seconds_count 0\n") || !strings.Contains(out, "h_seconds_sum 0\n") {
		t.Fatalf("boş histogram sıfır serileri yazmalı:\n%s", out)
	}
}

func TestHistogramRejectsUnsortedBounds(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("sırasız sınırlar panik vermeli")
		}
	}()
	NewHistogram([]float64{1, 0.5})
}

func TestHistogramIsSafeUnderConcurrency(t *testing.T) {
	h := NewHistogram(DefaultWaitBuckets)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.Observe(time.Duration(j) * time.Millisecond)
				_ = h.Snapshot()
			}
		}()
	}
	wg.Wait()
	s := h.Snapshot()
	if s.Count != 2000 || s.Cumulative[len(s.Cumulative)-1] != 2000 {
		t.Fatalf("2000 gözlem bekleniyordu: %+v", s)
	}
}

func TestHandlerServesCollectedMetricsWithPrometheusContentType(t *testing.T) {
	h := Handler(func(_ context.Context, w *Writer) error {
		w.Counter("a_total", "A.", 1)
		return nil
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != ContentType {
		t.Fatalf("200 ve Prometheus içerik türü bekleniyordu: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "a_total 1\n") {
		t.Fatalf("gövde: %q", rec.Body.String())
	}
}

// Okunamayan bir değer 0 gibi görünmemeli: kısmi çıktı sunulmaz, scrape başarısız olur.
func TestHandlerReturns503WithoutPartialOutputWhenCollectFails(t *testing.T) {
	h := Handler(func(_ context.Context, w *Writer) error {
		w.Counter("a_total", "A.", 1)
		return errors.New("veritabanı kapalı")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("503 bekleniyordu: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "a_total") || strings.Contains(rec.Body.String(), "veritabanı") {
		t.Fatalf("kısmi çıktı veya iç hata sızmamalı: %q", rec.Body.String())
	}
}

func TestHandlerBoundsCollectByTimeout(t *testing.T) {
	var gotDeadline bool
	h := Handler(func(ctx context.Context, _ *Writer) error {
		_, gotDeadline = ctx.Deadline()
		return nil
	})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !gotDeadline {
		t.Fatal("collect süre sınırı olan bir ctx almalı")
	}
}

func TestHandlerRejectsNonGet(t *testing.T) {
	h := Handler(func(context.Context, *Writer) error { return nil })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") == "" {
		t.Fatalf("405 ve Allow bekleniyordu: %d %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestServeServesMetricsAndStopsCleanlyWhenContextIsCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, func(_ context.Context, w *Writer) error { w.Counter("a_total", "A.", 5); return nil })
	}()

	resp, err := http.Get("http://" + ln.Addr().String() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "a_total 5\n") {
		t.Fatalf("metrikler sunulmalı: %d %q", resp.StatusCode, body)
	}
	// Ana uygulama yolları bu dinleyicide yok.
	other, err := http.Get("http://" + ln.Addr().String() + "/api/campaigns")
	if err != nil {
		t.Fatal(err)
	}
	_ = other.Body.Close()
	if other.StatusCode != http.StatusNotFound {
		t.Fatalf("iç dinleyici yalnızca /metrics sunmalı: %d", other.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("temiz kapanış nil dönmeli: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ctx iptalinden sonra dinleyici kapanmadı")
	}
	if c, err := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond); err == nil {
		_ = c.Close()
		t.Fatal("kapanan dinleyici bağlantı kabul etmemeli")
	}
}

// İç dinleyici yalnızca /metrics sunar; başka hiçbir yol açık değildir.
func TestMuxServesOnlyMetrics(t *testing.T) {
	m := Mux(func(_ context.Context, w *Writer) error { w.Counter("a_total", "A.", 1); return nil })
	for path, want := range map[string]int{"/metrics": 200, "/": 404, "/api/campaigns": 404, "/healthz": 404, "/metrics/x": 404} {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, beklenen %d", path, rec.Code, want)
		}
	}
}
