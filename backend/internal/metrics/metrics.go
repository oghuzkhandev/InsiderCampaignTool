// Package metrics, Prometheus metin biçiminde (text exposition format 0.0.4)
// metrik sunmak için gereken küçük parçaları içerir: sayaç, histogram, yazıcı ve
// HTTP handler'ı. Dış bağımlılık yoktur; API ve worker aynı paketi kullanır
// (ARCHITECTURE §12, ADR-016).
//
// Süreç içi değerler (Counter, Histogram) bellekte tutulur ve süreç yeniden
// başlayınca sıfırlanır. Veritabanından hesaplanan gauge'lar (queue_depth) her
// okunuşta Handler'ın collect fonksiyonunda üretilir; bellekte tutulmaz.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Counter, yalnızca artan bir sayaçtır. Sıfır değeri kullanıma hazırdır; nil
// Counter'ın Inc ve Add'i hiçbir şey yapmaz (bağlanmamış sayaç sessizce yok
// sayılır, panik vermez).
type Counter struct{ n atomic.Int64 }

// Inc, sayacı bir artırır.
func (c *Counter) Inc() { c.Add(1) }

// Add, sayacı n kadar artırır. Negatif değerler yok sayılır (sayaç azalmaz).
func (c *Counter) Add(n int64) {
	if c == nil || n <= 0 {
		return
	}
	c.n.Add(n)
}

// Value, sayacın o anki değeridir.
func (c *Counter) Value() int64 {
	if c == nil {
		return 0
	}
	return c.n.Load()
}

// Histogram, süre gözlemlerini sabit üst sınırlı kovalara (saniye) böler.
// Eşzamanlı kullanıma uygundur.
type Histogram struct {
	bounds   []float64
	buckets  []atomic.Int64 // len(bounds)+1: sonuncusu +Inf; kova başına (kümülatif değil)
	count    atomic.Int64
	sumNanos atomic.Int64
}

// DefaultWaitBuckets, hız sınırı beklemeleri için kova üst sınırlarıdır (saniye).
// Aralık 10 ms (100/sn hız) ile birkaç saniye arasındadır.
var DefaultWaitBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewHistogram, artan sırada verilen üst sınırlarla bir histogram oluşturur.
// bounds artan olmalıdır; aksi halde panik verir (programlama hatası).
func NewHistogram(bounds []float64) *Histogram {
	if !sort.Float64sAreSorted(bounds) {
		panic("metrics: histogram sınırları artan olmalı")
	}
	b := append([]float64(nil), bounds...)
	return &Histogram{bounds: b, buckets: make([]atomic.Int64, len(b)+1)}
}

// Observe, bir süreyi gözlemler. Negatif süre sıfır sayılır.
func (h *Histogram) Observe(d time.Duration) {
	if h == nil {
		return
	}
	if d < 0 {
		d = 0
	}
	sec := d.Seconds()
	i := sort.SearchFloat64s(h.bounds, sec) // ilk bound >= sec: `le` kovası
	h.buckets[i].Add(1)
	h.count.Add(1)
	h.sumNanos.Add(int64(d))
}

// HistogramSnapshot, histogramın tutarlı bir kopyasıdır. Kovalar kümülatiftir.
type HistogramSnapshot struct {
	Bounds     []float64
	Cumulative []int64 // len(Bounds)+1: son eleman +Inf = Count
	Count      int64
	SumSeconds float64
}

// Snapshot, o anki değerleri döndürür. Gözlem sırasında çağrılırsa kova toplamı ile
// Count ±1 farklı olabilir; kümülatif değerler en son kovayı Count sayar, böylece
// `le="+Inf"` her zaman `_count` ile aynıdır.
func (h *Histogram) Snapshot() HistogramSnapshot {
	if h == nil {
		return HistogramSnapshot{}
	}
	s := HistogramSnapshot{Bounds: h.bounds, Cumulative: make([]int64, len(h.buckets))}
	var run int64
	for i := range h.buckets {
		run += h.buckets[i].Load()
		s.Cumulative[i] = run
	}
	s.Count = run
	s.Cumulative[len(s.Cumulative)-1] = run
	s.SumSeconds = float64(h.sumNanos.Load()) / float64(time.Second)
	return s
}

// Label, bir metriğin etiketidir.
type Label struct{ Name, Value string }

// Writer, metrikleri Prometheus metin biçiminde yazar. Aynı ad iki kez
// tanımlanırsa (HELP/TYPE) çağıranın hatasıdır.
type Writer struct {
	w   io.Writer
	err error
}

// NewWriter, w'ye yazan bir Writer döndürür.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Err, ilk yazma hatasıdır.
func (w *Writer) Err() error { return w.err }

func (w *Writer) printf(format string, args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.w, format, args...)
}

// Counter, etiketsiz bir sayacı yazar.
func (w *Writer) Counter(name, help string, value int64) {
	w.header(name, help, "counter")
	w.sample(name, formatInt(value))
}

// CounterVec, etiketli bir sayaç ailesini yazar; HELP/TYPE bir kez yazılır.
func (w *Writer) CounterVec(name, help string, samples []LabeledValue) {
	w.header(name, help, "counter")
	for _, s := range samples {
		w.sample(name, formatInt(s.Value), s.Labels...)
	}
}

// GaugeVec, etiketli bir gauge ailesini yazar.
func (w *Writer) GaugeVec(name, help string, samples []LabeledValue) {
	w.header(name, help, "gauge")
	for _, s := range samples {
		w.sample(name, formatInt(s.Value), s.Labels...)
	}
}

// Histogram, bir histogramı `_bucket`, `_sum` ve `_count` satırlarıyla yazar.
func (w *Writer) Histogram(name, help string, h HistogramSnapshot) {
	w.header(name, help, "histogram")
	for i, bound := range h.Bounds {
		w.sample(name+"_bucket", formatInt(h.Cumulative[i]), Label{"le", formatFloat(bound)})
	}
	if len(h.Cumulative) > 0 {
		w.sample(name+"_bucket", formatInt(h.Cumulative[len(h.Cumulative)-1]), Label{"le", "+Inf"})
	}
	w.sample(name+"_sum", formatFloat(h.SumSeconds))
	w.sample(name+"_count", formatInt(h.Count))
}

// LabeledValue, etiketleri olan tek bir tam sayı örneğidir.
type LabeledValue struct {
	Labels []Label
	Value  int64
}

func (w *Writer) header(name, help, typ string) {
	w.printf("# HELP %s %s\n", name, escapeHelp(help))
	w.printf("# TYPE %s %s\n", name, typ)
}

func (w *Writer) sample(name, value string, labels ...Label) {
	if len(labels) == 0 {
		w.printf("%s %s\n", name, value)
		return
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = l.Name + `="` + escapeLabel(l.Value) + `"`
	}
	w.printf("%s{%s} %s\n", name, strings.Join(parts, ","), value)
}

func formatInt(v int64) string { return strconv.FormatInt(v, 10) }

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func escapeHelp(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(s)
}

func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// collectTimeout, tek bir okumanın (collect) süre sınırıdır. Takılan bir
// veritabanı sorgusu scrape'i sonsuza dek asmasın.
const collectTimeout = 3 * time.Second

// ContentType, Prometheus metin biçiminin içerik türüdür.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// Handler, GET /metrics isteğini karşılar. collect, bu okuma için metrikleri
// yazar; hata dönerse yanıt 503 olur ve kısmi çıktı SUNULMAZ: kuyruk derinliği gibi
// bir değerin okunamaması "değer 0" gibi görünmemeli, scrape başarısız olmalıdır.
func Handler(collect func(ctx context.Context, w *Writer) error) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			rw.Header().Set("Allow", "GET, HEAD")
			http.Error(rw, "yöntem desteklenmiyor", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), collectTimeout)
		defer cancel()

		var b strings.Builder
		w := NewWriter(&b)
		if err := collect(ctx, w); err != nil || w.Err() != nil {
			http.Error(rw, "metrikler okunamadı", http.StatusServiceUnavailable)
			return
		}
		rw.Header().Set("Content-Type", ContentType)
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(rw, b.String())
	})
}

// Mux, yalnızca GET /metrics sunan bir yönlendirici döndürür. İç dinleyici
// (host'a yayımlanmayan ayrı port) bunu kullanır; başka hiçbir yol sunmaz.
func Mux(collect func(ctx context.Context, w *Writer) error) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler(collect))
	return mux
}

// shutdownTimeout, iç dinleyicinin kapanışta devam eden scrape'e tanıdığı süredir.
const shutdownTimeout = 2 * time.Second

// Serve, ln üzerinde yalnızca /metrics sunar ve ctx iptal edilince kapanır
// (temiz kapanışta nil döner). Dinleyici, uygulamanın ana portundan AYRI ve
// Compose'ta host'a yayımlanmayan bir porttadır (ADR-016): metrikler yalnızca Docker
// iç ağından erişilir. ln'yi çağıran açar; böylece port çakışması başlangıçta
// görünür hale gelir, süreç sessizce metriksiz çalışmaz.
func Serve(ctx context.Context, ln net.Listener, collect func(ctx context.Context, w *Writer) error) error {
	srv := &http.Server{
		Handler:           Mux(collect),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
