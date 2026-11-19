package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
)

// ContentGenerator, brief'ten doğrulanmış içerik üretir (content.Generator).
type ContentGenerator interface {
	Generate(ctx context.Context, brief string, opts content.Options, onDelta func(content.Delta)) (content.Content, error)
}

// ContentStore, doğrulanmış içerik sürümlerini saklar (campaign.ContentStore).
type ContentStore interface {
	Save(ctx context.Context, campaignID, source string, c content.Content) (campaign.ContentVersion, error)
	Latest(ctx context.Context, campaignID string) (campaign.ContentVersion, bool, error)
	MaxFirstNameRunes(ctx context.Context) (int, error)
}

// contentVersionResponse, kaydedilmiş bir içerik sürümüdür. `validated` SSE
// olayının verisi, PUT yanıtı ve GET'teki `content` alanı aynı biçimdedir.
type contentVersionResponse struct {
	ContentVersion int       `json:"content_version"`
	Source         string    `json:"source"`
	SMSText        string    `json:"sms_text"`
	EmailSubject   string    `json:"email_subject"`
	EmailBody      string    `json:"email_body"`
	CreatedAt      time.Time `json:"created_at"`
}

func toContentResponse(v campaign.ContentVersion) *contentVersionResponse {
	return &contentVersionResponse{
		ContentVersion: v.Version,
		Source:         v.Source,
		SMSText:        v.Content.SMSText,
		EmailSubject:   v.Content.EmailSubject,
		EmailBody:      v.Content.EmailBody,
		CreatedAt:      v.CreatedAt.UTC(),
	}
}

// loadDraft, kampanyayı okur ve draft olduğunu doğrular. Sorun varsa yanıtı
// yazar ve false döner.
func (s *server) loadDraft(w http.ResponseWriter, r *http.Request) (campaign.Campaign, bool) {
	c, err := s.Campaigns.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
		} else {
			s.internalError(w, r, "kampanya okunamadı", err)
		}
		return campaign.Campaign{}, false
	}
	if c.Status != campaign.StatusDraft {
		writeNotDraft(w)
		return campaign.Campaign{}, false
	}
	return c, true
}

func writeNotDraft(w http.ResponseWriter) {
	writeError(w, http.StatusConflict, "CAMPAIGN_NOT_DRAFT", "Kampanya onaylandı; içerik artık değiştirilemez.")
}

// contentIssues, doğrulama sorunlarını yanıttaki sebep kodlarına çevirir. Model
// veya kullanıcı metni ham olarak iletilmez; Issue.Text yalnızca temizlenmiş
// ve kısaltılmış değişken adıdır.
func contentIssues(issues []content.Issue) []reason {
	out := make([]reason, 0, len(issues))
	for _, is := range issues {
		out = append(out, reason{Code: string(is.Code), Field: is.Field, Text: is.Text})
	}
	return out
}

// putContent, insan düzenlemesini aynı doğrulayıcıdan geçirir ve yeni sürüm
// olarak kaydeder. Onaydan sonra 409 döner.
func (s *server) putContent(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "İstek application/json olmalı.")
		return
	}
	var raw json.RawMessage
	if status, code, msg, ok := decodeBody(w, r, &raw); !ok {
		writeError(w, status, code, msg)
		return
	}
	c, ok := s.loadDraft(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	maxName, err := s.Contents.MaxFirstNameRunes(ctx)
	if err != nil {
		s.internalError(w, r, "en uzun ad okunamadı", err)
		return
	}
	edited, issues := content.Validate(raw, content.Options{MaxNameRunes: maxName})
	if len(issues) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "CONTENT_INVALID", "İçerik doğrulanamadı.", contentIssues(issues)...)
		return
	}

	v, err := s.Contents.Save(ctx, c.ID, campaign.SourceHuman, edited)
	if err != nil {
		switch {
		case errors.Is(err, campaign.ErrNotDraft):
			writeNotDraft(w)
		case errors.Is(err, campaign.ErrNotFound):
			writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
		default:
			s.internalError(w, r, "içerik kaydedilemedi", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, toContentResponse(v))
}

// errorEvent, SSE `error` olayının verisidir. Partial yalnızca doğrulanamayan
// ama çözümlenebilen metinleri taşır; doğrulanmamıştır, insan düzenlemesine
// başlangıç olarak sunulur ve kaydedilmemiştir.
type errorEvent struct {
	Code    string           `json:"code"`
	Message string           `json:"message"`
	Reasons []reason         `json:"reasons,omitempty"`
	Partial *content.Content `json:"partial,omitempty"`
}

// generateContent, içeriği üretir ve SSE ile akıtır (`delta`, `validated`,
// `error`). Akış sırasında gösterilen metin geçici önizlemedir. Yalnızca
// tamamı doğrulanıp kaydedildikten (commit) sonra `validated` gönderilir.
// Akış başlamadan önceki sorunlar (kampanya yok, draft değil) düz JSON hata
// olarak döner.
func (s *server) generateContent(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadDraft(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	maxName, err := s.Contents.MaxFirstNameRunes(ctx)
	if err != nil {
		s.internalError(w, r, "en uzun ad okunamadı", err)
		return
	}

	// Akışa yazılamazsa (istemci gitti veya yazma süresi doldu) üretim iptal
	// edilir: kimsenin görmediği bir üretim için LLM maliyeti ve kayıt olmaz.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	es := newEventStream(w, s.sseWriteTimeout(), cancel)
	if es.failed {
		return
	}
	generated, err := s.Content.Generate(ctx, c.Brief, content.Options{MaxNameRunes: maxName}, func(d content.Delta) {
		es.send("delta", d)
	})
	if err != nil {
		s.sendGenerateError(es, r, err)
		return
	}
	if ctx.Err() != nil || es.failed {
		return // istemci ayrıldı; kimsenin görmediği bir sürüm kaydedilmez
	}

	v, err := s.Contents.Save(ctx, c.ID, campaign.SourceLLM, generated)
	if err != nil {
		switch {
		case errors.Is(err, campaign.ErrNotDraft):
			es.send("error", errorEvent{Code: "CAMPAIGN_NOT_DRAFT", Message: "Kampanya üretim sırasında onaylandı; yeni sürüm kaydedilmedi."})
		case errors.Is(err, campaign.ErrNotFound):
			es.send("error", errorEvent{Code: "CAMPAIGN_NOT_FOUND", Message: "Kampanya bulunamadı."})
		default:
			s.logStreamError(r, "içerik kaydedilemedi", err)
			es.send("error", errorEvent{Code: "INTERNAL", Message: "Beklenmeyen bir hata oluştu."})
		}
		return
	}
	es.send("validated", toContentResponse(v))
}

// sendGenerateError, üretim hatalarını `error` olayına çevirir.
func (s *server) sendGenerateError(es *eventStream, r *http.Request, err error) {
	var invalid *content.InvalidError
	switch {
	case es.failed:
		// Akış koptu; yazacak kimse yok ve bu bir uygulama hatası değildir.
	case errors.Is(err, context.Canceled):
		// İstemci bağlantıyı kapattı; yazacak kimse yok.
	case errors.As(err, &invalid):
		s.LLMValidationFailures.Inc() // model çıktısı doğrulamadan geçemedi
		ev := errorEvent{
			Code:    "CONTENT_INVALID",
			Message: "Üretilen içerik doğrulanamadı. Alanları düzenleyebilir veya yeniden üretebilirsiniz.",
			Reasons: contentIssues(invalid.Issues),
		}
		if invalid.Partial != (content.Content{}) {
			p := invalid.Partial
			ev.Partial = &p
		}
		es.send("error", ev)
	case errors.Is(err, llm.ErrTimeout):
		es.send("error", errorEvent{Code: "LLM_TIMEOUT", Message: "İçerik üretimi zaman aşımına uğradı. Yeniden üretin."})
	case errors.Is(err, llm.ErrRateLimited):
		es.send("error", errorEvent{Code: "LLM_RATE_LIMITED", Message: "İçerik servisi şu an yoğun. Biraz sonra yeniden üretin."})
	case errors.Is(err, llm.ErrStreamInterrupted):
		es.send("error", errorEvent{Code: "STREAM_INTERRUPTED", Message: "Üretim yarıda kesildi. Yeniden üretin."})
	default:
		s.logStreamError(r, "içerik üretilemedi", err)
		es.send("error", errorEvent{Code: "INTERNAL", Message: "Beklenmeyen bir hata oluştu."})
	}
}

func (s *server) logStreamError(r *http.Request, msg string, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	s.Logger.Error(msg, "method", r.Method, "path", r.URL.Path, "error", err)
}

// defaultSSEWriteTimeout, akıştaki her yazma için varsayılan süredir.
const defaultSSEWriteTimeout = 10 * time.Second

func (s *server) sseWriteTimeout() time.Duration {
	if s.SSEWriteTimeout > 0 {
		return s.SSEWriteTimeout
	}
	return defaultSSEWriteTimeout
}

// eventStream, Server-Sent Events yanıtını yazar. Her olay yazıldıktan sonra
// hemen flush edilir; araya giren tamponlama olmamalıdır (nginx tarafında
// proxy_buffering kapalı, ADR-014). Yalnızca tek goroutine kullanır.
//
// Her yazmadan önce yazma süresi yenilenir: okumayan (yavaş) bir istemci
// toplam süreyi değil yalnızca tek bir yazmayı sınırlar. Yazma veya flush
// hatası (istemci gitti, süre doldu) akışı bozulmuş sayar: failed true olur,
// sonraki olaylar yazılmaz ve onFail çağrılır (üretimi iptal eder).
type eventStream struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	timeout time.Duration
	onFail  func()
	failed  bool
}

func newEventStream(w http.ResponseWriter, timeout time.Duration, onFail func()) *eventStream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	h.Set("X-Content-Type-Options", "nosniff")
	es := &eventStream{w: w, rc: http.NewResponseController(w), timeout: timeout, onFail: onFail}
	es.arm()
	w.WriteHeader(http.StatusOK)
	es.flush() // başlıklar hemen gitsin
	return es
}

// arm, bir sonraki yazma için süreyi yeniler. Desteklemeyen yazıcılar
// (ör. httptest.ResponseRecorder) sessizce atlanır.
func (e *eventStream) arm() {
	_ = e.rc.SetWriteDeadline(time.Now().Add(e.timeout))
}

// flush, tamponu boşaltır; hata akışı bozulmuş sayar. Flush desteklenmiyorsa
// akış çalışır ama tamponlanabilir; bu bir bağlantı hatası değildir.
func (e *eventStream) flush() {
	if err := e.rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		e.fail()
	}
}

func (e *eventStream) fail() {
	if e.failed {
		return
	}
	e.failed = true
	if e.onFail != nil {
		e.onFail()
	}
}

// send, tek bir olayı yazar ve flush eder. JSON tek satırdır (kompakt), bu
// yüzden `data:` alanı bölünmez. Akış daha önce bozulduysa hiçbir şey yazılmaz.
func (e *eventStream) send(event string, data any) {
	if e.failed {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		b = []byte(`{"code":"INTERNAL","message":"Beklenmeyen bir hata oluştu."}`)
		event = "error"
	}
	e.arm()
	if _, err := fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		e.fail()
		return
	}
	e.flush()
}
