package mockprovider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxBodyBytes, istek gövdesi sınırıdır.
const maxBodyBytes = 64 << 10

// Kanal adları, mesajlardaki kanal değerleriyle aynıdır.
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// Hata kodları (yanıt gövdesinde `error.code`).
const (
	CodeKeyRequired      = "IDEMPOTENCY_KEY_REQUIRED"
	CodeKeyReused        = "IDEMPOTENCY_KEY_REUSED"
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeInvalidRecipient = "INVALID_RECIPIENT"
	CodeRateLimited      = "RATE_LIMITED"
	CodeInternal         = "INTERNAL"
)

// smsRecipient, seed telefonlarının biçimidir (+90555XXXXXXX); yalnızca rakam
// ve isteğe bağlı baştaki '+'.
var smsRecipient = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

// Handler, sağlayıcının HTTP arayüzüdür.
type Handler struct {
	cfg   Config
	store Store
	stats *Stats
	log   *slog.Logger
	mux   *http.ServeMux

	// Testlerde değiştirilebilir bağımlılıklar.
	now   func() time.Time
	rand  func() float64
	sleep func(ctx context.Context, d time.Duration) error
}

// Option, Handler'ı özelleştirir.
type Option func(*Handler)

// WithClock, zaman kaynağını değiştirir.
func WithClock(now func() time.Time) Option { return func(h *Handler) { h.now = now } }

// WithRand, hata olasılığı kaynağını değiştirir ([0,1) döndürmelidir).
func WithRand(r func() float64) Option { return func(h *Handler) { h.rand = r } }

// WithSleep, gecikme beklemesini değiştirir.
func WithSleep(s func(ctx context.Context, d time.Duration) error) Option {
	return func(h *Handler) { h.sleep = s }
}

// WithLogger, günlükçüyü değiştirir.
func WithLogger(l *slog.Logger) Option { return func(h *Handler) { h.log = l } }

// WithStats, istatistik deposunu değiştirir (testler dışarıdan okuyabilsin diye).
func WithStats(s *Stats) Option { return func(h *Handler) { h.stats = s } }

// New, verilen yapılandırma ve kabul deposuyla bir Handler oluşturur.
func New(cfg Config, store Store, opts ...Option) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	h := &Handler{
		cfg:   cfg,
		store: store,
		stats: NewStats(),
		log:   slog.Default(),
		now:   time.Now,
		rand:  rand.Float64,
		sleep: sleepContext,
	}
	for _, o := range opts {
		o(h)
	}
	h.mux = http.NewServeMux()
	h.mux.HandleFunc("POST /v1/sms", h.send(ChannelSMS))
	h.mux.HandleFunc("POST /v1/email", h.send(ChannelEmail))
	h.mux.HandleFunc("GET /admin/stats", h.adminStats)
	h.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return h, nil
}

// Stats, sağlayıcının istatistiklerini döndürür.
func (h *Handler) Stats() *Stats { return h.stats }

// ServeHTTP, http.Handler arayüzünü uygular.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

type smsRequest struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

type emailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// send, bir kanalın gönderim uç noktasıdır. Sıra: varış kaydı → doğrulama →
// gecikme → rastgele hata (429, 500) → kalıcı kabul → yanıt (düşürülebilir).
func (h *Handler) send(channel string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Her istek, hata alsa bile "sağlayıcıya ulaşan istek"tir (SND-001: kayan
		// pencere sınırı retry'lar dahil bütün istekleri sayar).
		h.stats.attempt(channel, h.now())

		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			h.stats.invalid()
			writeError(w, http.StatusBadRequest, CodeKeyRequired, "Idempotency-Key başlığı zorunludur")
			return
		}
		req, status, code, msg := h.parse(w, r, channel)
		if status != 0 {
			h.stats.invalid()
			writeError(w, status, code, msg)
			return
		}
		req.Key = key

		if err := h.sleep(r.Context(), h.cfg.Latency); err != nil {
			return // istemci ayrıldı
		}

		if h.roll(h.cfg.RateLimitRate) {
			h.stats.limited()
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(h.cfg.RetryAfter)))
			writeError(w, http.StatusTooManyRequests, CodeRateLimited, "çok fazla istek")
			return
		}
		if h.roll(h.cfg.ServerErrorRate) {
			h.stats.failed()
			writeError(w, http.StatusInternalServerError, CodeInternal, "geçici sağlayıcı hatası")
			return
		}

		acc, err := h.store.Accept(r.Context(), req)
		switch {
		case errors.Is(err, ErrKeyReused):
			h.stats.keyReused()
			writeError(w, http.StatusUnprocessableEntity, CodeKeyReused,
				"Idempotency-Key farklı bir alıcı, kanal veya içerikle kullanıldı")
			return
		case err != nil:
			if r.Context().Err() == nil {
				h.log.Error("kabul kaydedilemedi", "error", err)
			}
			h.stats.failed()
			writeError(w, http.StatusInternalServerError, CodeInternal, "geçici sağlayıcı hatası")
			return
		}
		h.stats.accepted(key, acc.Created)

		if h.dropResponse(acc) {
			// Kabul commit edildi; yanıt hiç yazılmadan bağlantı kesilir.
			h.stats.dropped()
			panic(http.ErrAbortHandler)
		}
		writeJSON(w, http.StatusOK, map[string]string{"message_id": acc.MessageID})
	}
}

// dropResponse, bu başarılı kabulün yanıtının düşürülüp düşürülmeyeceğini
// belirler (ADR-025).
func (h *Handler) dropResponse(acc Acceptance) bool {
	switch h.cfg.Mode {
	case ModeAcceptThenDrop:
		return acc.Created // yalnızca bir anahtarın İLK kabul yanıtı
	case ModeDropAllResponses:
		return true // kabul edilen isteğin her yanıtı
	default:
		return false
	}
}

func (h *Handler) roll(p float64) bool { return p > 0 && h.rand() < p }

// parse, gövdeyi ve alıcıyı doğrular, tekilleştirme özetini üretir. Hata varsa
// durum kodu sıfırdan farklı döner.
func (h *Handler) parse(w http.ResponseWriter, r *http.Request, channel string) (req Request, status int, code, msg string) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()

	req.Channel = channel
	switch channel {
	case ChannelSMS:
		var in smsRequest
		if err := dec.Decode(&in); err != nil || strings.TrimSpace(in.Text) == "" {
			return Request{}, http.StatusBadRequest, CodeInvalidRequest, "geçersiz SMS isteği"
		}
		if !smsRecipient.MatchString(in.To) {
			return Request{}, http.StatusBadRequest, CodeInvalidRecipient, "geçersiz telefon numarası"
		}
		req.Recipient = in.To
		req.ContentHash = sha256.Sum256([]byte(in.Text))
	default:
		var in emailRequest
		if err := dec.Decode(&in); err != nil || strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.Body) == "" {
			return Request{}, http.StatusBadRequest, CodeInvalidRequest, "geçersiz e-posta isteği"
		}
		if !validEmail(in.To) {
			return Request{}, http.StatusBadRequest, CodeInvalidRecipient, "geçersiz e-posta adresi"
		}
		req.Recipient = in.To
		// Konu satır sonu içeremez, bu yüzden NUL ayırıcı belirsizlik yaratmaz.
		req.ContentHash = sha256.Sum256([]byte(in.Subject + "\x00" + in.Body))
	}
	return req, 0, "", ""
}

func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1 && strings.Count(s, "@") == 1 && !strings.ContainsAny(s, " \t\r\n")
}

func (h *Handler) adminStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.stats.Snapshot())
}

// retryAfterSeconds, süreyi Retry-After başlığının tam sayı saniyesine yukarı
// yuvarlar.
func retryAfterSeconds(d time.Duration) int {
	return int(math.Ceil(d.Seconds()))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
