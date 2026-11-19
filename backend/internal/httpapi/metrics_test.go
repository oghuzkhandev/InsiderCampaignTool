package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// Segment üretiminde model çıktısı doğrulamadan geçemezse sayılır; model hatası
// (zaman aşımı, 429), boş brief veya başarı sayılmaz.
func TestSegmentValidationFailuresAreCountedOnlyForInvalidModelOutput(t *testing.T) {
	invalid := &segment.InvalidError{Issues: []segment.Issue{{Code: segment.CodeUnsupportedField, Field: "favorite_color"}}}
	tests := []struct {
		name string
		err  error
		want int64
	}{
		{"geçersiz segment", invalid, 1},
		{"sarılmış geçersiz segment", errors.Join(errors.New("ek"), invalid), 1},
		{"zaman aşımı", llm.ErrTimeout, 0},
		{"429", llm.ErrRateLimited, 0},
		{"boş brief", segment.ErrEmptyBrief, 0},
		{"uzun brief", segment.ErrBriefTooLong, 0},
		{"başarı", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.gen.err = tt.err
			h.post(`{"brief":"x"}`)
			if got := h.llmFailures.Value(); got != tt.want {
				t.Fatalf("sayaç %d, beklenen %d", got, tt.want)
			}
		})
	}
}

// İçerik üretiminde yalnızca doğrulamadan geçemeyen model çıktısı sayılır.
func TestContentValidationFailuresAreCountedOnlyForInvalidModelOutput(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int64
	}{
		{"geçersiz içerik", &content.InvalidError{Issues: []content.Issue{{Code: content.CodeTooLong, Field: "sms_text"}}}, 1},
		{"hiç metin çözümlenemedi", &content.InvalidError{Issues: []content.Issue{{Code: content.CodeMalformedOutput}}}, 1},
		{"zaman aşımı", llm.ErrTimeout, 0},
		{"429", llm.ErrRateLimited, 0},
		{"kesilen akış", llm.ErrStreamInterrupted, 0},
		{"beklenmeyen hata", errors.New("x"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			h.cgen.err = tt.err
			postContent(h)
			if got := h.llmFailures.Value(); got != tt.want {
				t.Fatalf("sayaç %d, beklenen %d", got, tt.want)
			}
		})
	}
}

// Kullanıcının elle düzenlediği içerik model çıktısı değildir: geçersiz olsa da
// llm_validation_failures_total artmaz.
func TestHumanEditValidationFailureIsNotCountedAsLLMFailure(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.content.maxName = 21
	body := `{"sms_text":"` + strings.Repeat("a", 200) + `","email_subject":"k","email_body":"g"}`

	wantError(t, putContentBody(h, body), http.StatusUnprocessableEntity, "CONTENT_INVALID")

	if got := h.llmFailures.Value(); got != 0 {
		t.Fatalf("insan düzenlemesi sayılmamalı: %d", got)
	}
}

// Sayaç bağlanmamış olsa da (nil) hata yanıtı çökmez.
func TestValidationFailureWithoutCounterDoesNotPanic(t *testing.T) {
	h := newHarness(t)
	h.gen.err = &segment.InvalidError{Issues: []segment.Issue{{Code: segment.CodeUnsupportedField}}}
	noCounter := New(Deps{Segments: h.gen, Audience: h.counter, Campaigns: h.store}) // LLMValidationFailures nil

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns", strings.NewReader(`{"brief":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	noCounter.ServeHTTP(rec, req)

	wantError(t, rec, http.StatusUnprocessableEntity, "SEGMENT_INVALID")
}

// Deps.Validate, eksik zorunlu bağımlılığı adıyla bildirir; hepsi bağlıysa nil döner.
func TestDepsValidateNamesEveryMissingRequiredDependency(t *testing.T) {
	h := newHarness(t)
	full := Deps{
		Segments: h.gen, Audience: h.counter, Campaigns: h.store, Content: h.cgen,
		Contents: h.content, Approver: h.approve, Deliveries: &fakeDeliveries{},
	}
	if err := full.Validate(); err != nil {
		t.Fatalf("hepsi bağlıyken hata olmamalı: %v", err)
	}

	err := Deps{}.Validate()
	if err == nil {
		t.Fatal("boş Deps hata vermeli")
	}
	for _, name := range []string{"Segments", "Audience", "Campaigns", "Content", "Contents", "Approver", "Deliveries"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("hata %q adını içermeli: %v", name, err)
		}
	}

	// Tek bir eksik yalnızca kendi adını söyler.
	one := full
	one.Approver = nil
	err = one.Validate()
	if err == nil || !strings.Contains(err.Error(), "Approver") || strings.Contains(err.Error(), "Segments") {
		t.Fatalf("yalnızca Approver eksik bildirilmeli: %v", err)
	}
}

type fakeQueue struct {
	depth map[string]int
	err   error
}

func (f fakeQueue) QueueDepth(context.Context) (map[string]int, error) { return f.depth, f.err }

func collect(t *testing.T, q QueueDepthReader, c *metrics.Counter) (string, error) {
	t.Helper()
	var b strings.Builder
	w := metrics.NewWriter(&b)
	err := MetricsCollector(q, c)(context.Background(), w)
	return b.String(), err
}

func TestMetricsCollectorExposesQueueDepthPerChannelAndLLMFailures(t *testing.T) {
	var c metrics.Counter
	c.Add(3)

	out, err := collect(t, fakeQueue{depth: map[string]int{"sms": 5, "email": 2}}, &c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# TYPE llm_validation_failures_total counter\nllm_validation_failures_total 3\n",
		"# TYPE queue_depth gauge\n",
		`queue_depth{channel="sms"} 5` + "\n",
		`queue_depth{channel="email"} 2` + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("çıktıda %q yok:\n%s", want, out)
		}
	}
}

// Kanalı olmayan (boş) kuyruk 0 ile raporlanır, metrik eksik kalmaz.
func TestMetricsCollectorReportsZeroForChannelsWithoutMessages(t *testing.T) {
	out, err := collect(t, fakeQueue{depth: map[string]int{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `queue_depth{channel="sms"} 0`) || !strings.Contains(out, `queue_depth{channel="email"} 0`) ||
		!strings.Contains(out, "llm_validation_failures_total 0\n") {
		t.Fatalf("sıfırlar yazılmalı:\n%s", out)
	}
}

// API, worker sayaçlarını göstermez (ADR-016): onlar yalnızca worker'ın /metrics ucundadır.
func TestMetricsCollectorDoesNotExposeWorkerCounters(t *testing.T) {
	out, err := collect(t, fakeQueue{depth: map[string]int{"sms": 1}}, &metrics.Counter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"messages_sent_total", "send_attempts_total", "stale_result_total", "reaper_requeued_total", "ratelimit_wait_seconds"} {
		if strings.Contains(out, name) {
			t.Errorf("API çıktısında worker metriği %q olmamalı:\n%s", name, out)
		}
	}
}

// Okunamayan kuyruk derinliği hata verir; kısmi çıktı sunulmaz (handler 503 döner).
func TestMetricsCollectorFailsWhenQueueDepthCannotBeRead(t *testing.T) {
	_, err := collect(t, fakeQueue{err: errors.New("veritabanı kapalı")}, &metrics.Counter{})
	if err == nil {
		t.Fatal("hata bekleniyordu")
	}
}
