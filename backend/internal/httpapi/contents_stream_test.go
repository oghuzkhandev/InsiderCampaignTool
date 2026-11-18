package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
)

// loopGen, bağlam iptal edilene kadar delta üretir (en fazla max kez). İptal
// görülürse bağlam hatasıyla döner; görülmeden biterse geçerli içerik döndürür.
// Böylece "yazma hatasında üretim durmuyor" hatası testte görünür olur.
type loopGen struct {
	text string
	max  int

	mu         sync.Mutex
	iterations int
	sawCancel  bool
	finished   chan struct{}
}

func newLoopGen(text string, max int) *loopGen {
	return &loopGen{text: text, max: max, finished: make(chan struct{})}
}

func (g *loopGen) Generate(ctx context.Context, _ string, _ content.Options, onDelta func(content.Delta)) (content.Content, error) {
	defer close(g.finished)
	for i := 0; i < g.max; i++ {
		if ctx.Err() != nil {
			g.mu.Lock()
			g.sawCancel = true
			g.mu.Unlock()
			return content.Content{}, ctx.Err()
		}
		g.mu.Lock()
		g.iterations++
		g.mu.Unlock()
		onDelta(content.Delta{Field: "sms_text", Text: g.text, Replace: i == 0})
	}
	return goodContent, nil
}

func (g *loopGen) state() (iterations int, sawCancel bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.iterations, g.sawCancel
}

// failingWriter, yazma veya flush hatası ve SetWriteDeadline çağrılarını
// kaydeden bir ResponseWriter'dır.
type failingWriter struct {
	*httptest.ResponseRecorder

	failWriteAt int // bu numaralı Write çağrısı (1'den) hata verir; 0 = hiç
	failFlushAt int // bu numaralı flush hata verir; 0 = hiç

	mu        sync.Mutex
	writes    int
	flushes   int
	deadlines []deadlineCall
}

type deadlineCall struct {
	at, deadline time.Time
	writesBefore int
}

func newFailingWriter() *failingWriter {
	return &failingWriter{ResponseRecorder: httptest.NewRecorder()}
}

func (w *failingWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	w.writes++
	n := w.writes
	w.mu.Unlock()
	if w.failWriteAt != 0 && n >= w.failWriteAt {
		return 0, errors.New("yazma hatası: bağlantı koptu")
	}
	return w.ResponseRecorder.Write(b)
}

func (w *failingWriter) FlushError() error {
	w.mu.Lock()
	w.flushes++
	n := w.flushes
	w.mu.Unlock()
	if w.failFlushAt != 0 && n >= w.failFlushAt {
		return errors.New("flush hatası: bağlantı koptu")
	}
	w.ResponseRecorder.Flush()
	return nil
}

func (w *failingWriter) SetWriteDeadline(t time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadlines = append(w.deadlines, deadlineCall{at: time.Now(), deadline: t, writesBefore: w.writes})
	return nil
}

func (w *failingWriter) counts() (writes, flushes int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writes, w.flushes
}

func serve(h *harness, w http.ResponseWriter) {
	h.handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, contentPath, nil))
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(15 * time.Second):
		t.Fatalf("%s zamanında bitmedi", what)
	}
}

// Yazma hatasında üretim durmalı, hiçbir sürüm kaydedilmemeli ve hata
// alındıktan sonra yeniden yazma denenmemelidir.
func TestSSEWriteErrorStopsGenerationAndSavesNothing(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	gen := newLoopGen("parça", 1000)
	h.handler = New(Deps{Segments: h.gen, Audience: h.counter, Campaigns: h.store, Content: gen, Contents: h.content,
		Logger: slogDiscard(h)})

	w := newFailingWriter()
	w.failWriteAt = 4 // başlık + 2 delta başarılı, 4. yazma hata
	serve(h, w)

	iterations, sawCancel := gen.state()
	if !sawCancel || iterations > 6 {
		t.Fatalf("üretim yazma hatasından hemen sonra durmalı: %d yineleme, iptal görüldü=%v", iterations, sawCancel)
	}
	if len(h.content.saved) != 0 {
		t.Fatalf("istemci kopunca sürüm kaydedilmemeli: %+v", h.content.saved)
	}
	if writes, _ := w.counts(); writes != 4 {
		t.Fatalf("hatadan sonra yeniden yazma denenmemeli: %d yazma", writes)
	}
	if strings.Contains(h.logs.String(), "ERROR") {
		t.Fatalf("istemci kopması hata olarak loglanmamalı: %s", h.logs.String())
	}
}

func TestSSEFlushErrorStopsGenerationAndSavesNothing(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	gen := newLoopGen("parça", 1000)
	h.handler = New(Deps{Segments: h.gen, Audience: h.counter, Campaigns: h.store, Content: gen, Contents: h.content,
		Logger: slogDiscard(h)})

	w := newFailingWriter()
	w.failFlushAt = 3 // ilk flush (başlıklar) ve bir delta tamam, sonrası hata
	serve(h, w)

	iterations, sawCancel := gen.state()
	if !sawCancel || iterations > 6 {
		t.Fatalf("üretim flush hatasından hemen sonra durmalı: %d yineleme, iptal görüldü=%v", iterations, sawCancel)
	}
	if len(h.content.saved) != 0 {
		t.Fatalf("sürüm kaydedilmemeli: %+v", h.content.saved)
	}
	if _, flushes := w.counts(); flushes != 3 {
		t.Fatalf("hatadan sonra yeniden flush denenmemeli: %d flush", flushes)
	}
}

// Başlıklar bile gönderilemiyorsa (istemci zaten gitmiş) LLM hiç çağrılmaz.
func TestSSEInitialFlushErrorSkipsGeneration(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.cgen.result = goodContent

	w := newFailingWriter()
	w.failFlushAt = 1
	serve(h, w)

	if h.cgen.calls != 0 {
		t.Fatal("istemci yokken LLM çağrılmamalı (maliyet)")
	}
	if len(h.content.saved) != 0 {
		t.Fatal("sürüm kaydedilmemeli")
	}
}

// Sürüm kaydedildikten (commit) sonra `validated` yazılamazsa sürüm kalır
// (geri alınamaz, istemci GET ile görebilir); ikinci bir kayıt veya yeniden
// deneme olmaz.
func TestSSEWriteErrorOnValidatedKeepsCommittedVersionWithoutRetry(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.cgen.result = goodContent

	w := newFailingWriter()
	w.failWriteAt = 1 // validated, ilk (ve tek) olay yazımı
	serve(h, w)

	if len(h.content.saved) != 1 {
		t.Fatalf("kaydedilen sürüm tam bir kez yazılmış olmalı: %+v", h.content.saved)
	}
	if writes, _ := w.counts(); writes != 1 {
		t.Fatalf("yeniden yazma denenmemeli: %d", writes)
	}
}

// Her yazmadan önce yazma süresi yenilenir: yavaş istemci toplam süreyi değil,
// tek bir yazmayı sınırlar.
func TestSSERefreshesWriteDeadlineBeforeEveryWrite(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.cgen.deltas = []content.Delta{
		{Field: "sms_text", Text: "a", Replace: true},
		{Field: "sms_text", Text: "b"},
		{Field: "email_subject", Text: "c", Replace: true},
	}
	h.cgen.result = goodContent
	const timeout = 7 * time.Second
	h.handler = New(Deps{Segments: h.gen, Audience: h.counter, Campaigns: h.store, Content: h.cgen, Contents: h.content,
		SSEWriteTimeout: timeout, Logger: slogDiscard(h)})

	w := newFailingWriter()
	serve(h, w)

	writes, _ := w.counts()
	if writes != 4 { // 3 delta + validated
		t.Fatalf("yazma sayısı = %d", writes)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// Başlık flush'ı için 1 + her olay için 1.
	if len(w.deadlines) != 1+writes {
		t.Fatalf("her yazmadan önce süre yenilenmeli: %d çağrı, %d yazma", len(w.deadlines), writes)
	}
	for i, d := range w.deadlines {
		if diff := d.deadline.Sub(d.at); diff < timeout-50*time.Millisecond || diff > timeout+50*time.Millisecond {
			t.Fatalf("çağrı %d: süre ≈ %v olmalı, fark %v", i, timeout, diff)
		}
		if i > 0 && d.deadline.Before(w.deadlines[i-1].deadline) {
			t.Fatalf("çağrı %d: süre geriye gitmemeli", i)
		}
		// Çağrı i, i. yazmadan (başlık dahil sayılmaz) önce yapılır.
		if i > 0 && d.writesBefore != i-1 {
			t.Fatalf("çağrı %d yazmadan önce yapılmalı: önceki yazma sayısı %d", i, d.writesBefore)
		}
	}
}

// Okumayan (yavaş) bir istemci, yazma süresi dolunca üretimi durdurur.
// Gerçek bir sunucu ve ham TCP bağlantısı kullanılır; zamanlama yerine süre
// aşımının kendisi sınanır.
func TestSlowClientIsCutOffByWriteDeadline(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	big := strings.Repeat("a", 1<<20) // 1 MiB: soket tamponları kısa sürede dolar
	gen := newLoopGen(big, 400)
	h.handler = New(Deps{Segments: h.gen, Audience: h.counter, Campaigns: h.store, Content: gen, Contents: h.content,
		SSEWriteTimeout: 300 * time.Millisecond, Logger: slogDiscard(h)})

	srv := httptest.NewServer(h.handler)
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close() // srv.Close'dan önce çalışır; takılan yazmayı da çözer
	if _, err := fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\n\r\n", contentPath); err != nil {
		t.Fatal(err)
	}
	// Bağlantı açık kalır ama hiçbir şey okunmaz.

	start := time.Now()
	waitClosed(t, gen.finished, "üretim (yavaş istemci kesilmeli)")
	iterations, sawCancel := gen.state()
	if !sawCancel {
		t.Fatalf("yazma süresi dolunca üretim iptal edilmeli (%d yineleme)", iterations)
	}
	if iterations >= 400 {
		t.Fatalf("tüm yinelemeler tamamlanmamalı: %d", iterations)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("süre aşımı hızlı çalışmalı: %v", elapsed)
	}
	if len(h.content.saved) != 0 {
		t.Fatal("yavaş istemciye giden akışın sonunda sürüm kaydedilmemeli")
	}
}

// slogDiscard, harness'in log tamponuna yazan bir logger döndürür.
func slogDiscard(h *harness) *slog.Logger { return slog.New(slog.NewTextHandler(h.logs, nil)) }
