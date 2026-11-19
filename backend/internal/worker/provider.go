package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
)

// Kanal adları mesaj satırlarındaki değerlerle aynıdır.
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// ProviderTimeout, tek bir provider isteğinin zaman aşımıdır (ARCHITECTURE §8).
const ProviderTimeout = 10 * time.Second

// Job, claim edilmiş ve gönderilecek bir mesajdır.
type Job struct {
	ID         int64
	CampaignID string
	UserID     int64
	Channel    string
	Recipient  string
	// ClaimToken, bu sahiplenmeye özgüdür; sonuç yalnızca bu token ile yazılır.
	ClaimToken string
	// Attempts, bu claim dahil işleme denemesi sayısıdır.
	Attempts int
	// IdempotencyKey, mesaj boyunca sabittir (kampanya:kullanıcı:kanal).
	IdempotencyKey string

	// FirstName ve Content, render için okunur. Content, kampanyanın
	// approved_content_version ile belirlenen sürümüdür.
	FirstName string
	Content   content.Content
}

// SendResult, bir provider çağrısının sınıflandırılmış sonucudur.
type SendResult struct {
	Kind Kind
	// RetryAfter, 429 yanıtındaki geçerli Retry-After değeridir (yoksa 0).
	RetryAfter time.Duration
	// Err, last_error'a yazılacak kısa açıklamadır. İstek ve yanıt içeriği
	// (alıcı adresi, metin) içermez.
	Err string
}

// Provider, mesajı sağlayıcıya gönderir. Bağlam iptali çağrıyı keser.
type Provider interface {
	Send(ctx context.Context, job Job, rendered content.Content) SendResult
}

// HTTPProvider, mock provider'a (veya aynı sözleşmeyi uygulayan bir servise)
// HTTP ile gönderir: `POST /v1/sms` ve `POST /v1/email`, `Idempotency-Key`
// başlığıyla (ARCHITECTURE §11).
type HTTPProvider struct {
	BaseURL string
	Client  *http.Client
}

// NewHTTPProvider, ProviderTimeout zaman aşımlı bir istemci kurar.
func NewHTTPProvider(baseURL string) *HTTPProvider {
	return &HTTPProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: ProviderTimeout},
	}
}

// Send, mesajı gönderir ve sonucu sınıflandırır:
//
//   - 2xx: kabul.
//   - 429: geçici; geçerli Retry-After taşınır.
//   - 5xx, zaman aşımı, bağlantı hatası, yanıtın alınamaması: geçici.
//   - 429 dışındaki 4xx: kalıcı.
func (p *HTTPProvider) Send(ctx context.Context, job Job, rendered content.Content) SendResult {
	path, body, err := requestFor(job, rendered)
	if err != nil {
		return SendResult{Kind: KindRejected, Err: err.Error()}
	}

	// Gövde GetBody'siz verilir (bytes.Reader doğrudan verilmez). Aksi hâlde
	// net/http, Idempotency-Key başlıklı isteği "tekrarlanabilir" sayar ve yeniden
	// kullanılan bir bağlantıda yanıt okunamazsa isteği SESSİZCE bir kez daha
	// gönderir: hız sınırlayıcıyı atlar ve provider'a görünmeyen ekstra istek
	// yapar. Tekrar denemeyi yalnızca worker yönetir.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+path, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return SendResult{Kind: KindRejected, Err: "istek oluşturulamadı"}
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", job.IdempotencyKey)

	resp, err := p.Client.Do(req)
	if err != nil {
		return SendResult{Kind: KindNetworkError, Err: networkError(err)}
	}
	defer resp.Body.Close()
	// Gövde sınırlı okunup tüketilir (bağlantının yeniden kullanılabilmesi için).
	// Okuma hatası (yarıda kesilen gövde) yalnızca 2xx'te anlamlıdır.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	errCode := errorCode(body)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// 2xx tek başına kabul değildir: sözleşme `{"message_id": "<dolu>"}` ister.
		// Gövde eksik, okunamıyor veya uymuyorsa kabul doğrulanamadı; mesaj `sent`
		// olmaz ve aynı idempotency anahtarıyla yeniden denenir. Gövde içeriği
		// last_error'a yazılmaz.
		if readErr != nil {
			return SendResult{Kind: KindInvalidResponse, Err: "geçersiz yanıt: gövde okunamadı"}
		}
		if !hasMessageID(body) {
			return SendResult{Kind: KindInvalidResponse, Err: "geçersiz yanıt: message_id yok"}
		}
		return SendResult{Kind: KindAccepted}
	case resp.StatusCode == http.StatusTooManyRequests:
		return SendResult{Kind: KindRateLimited, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()), Err: "HTTP 429"}
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return SendResult{Kind: KindRejected, Err: statusError(resp.StatusCode, errCode)}
	default: // 5xx ve beklenmeyen diğer durumlar
		return SendResult{Kind: KindServerError, Err: statusError(resp.StatusCode, errCode)}
	}
}

// requestFor, kanala göre istek yolunu ve gövdesini üretir.
func requestFor(job Job, rendered content.Content) (path string, body []byte, err error) {
	var payload any
	switch job.Channel {
	case ChannelSMS:
		path, payload = "/v1/sms", map[string]string{"to": job.Recipient, "text": rendered.SMSText}
	case ChannelEmail:
		path, payload = "/v1/email", map[string]string{"to": job.Recipient, "subject": rendered.EmailSubject, "body": rendered.EmailBody}
	default:
		return "", nil, fmt.Errorf("bilinmeyen kanal %q", job.Channel)
	}
	body, err = json.Marshal(payload)
	return path, body, err
}

// maxResponseBytes, okunan yanıt gövdesinin üst sınırıdır. Daha büyük gövde kesilir
// ve (2xx için) geçersiz sayılır.
const maxResponseBytes = 64 << 10

// hasMessageID, gövdenin `{"message_id": "<boş olmayan metin>"}` içerdiğini doğrular.
// Fazladan alanlar zararsızdır; boş, null, metin olmayan veya JSON olmayan gövde
// geçersizdir.
func hasMessageID(body []byte) bool {
	var out struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false
	}
	return strings.TrimSpace(out.MessageID) != ""
}

// errorCode, provider hata gövdesindeki `error.code` değerini okur (yoksa boş).
func errorCode(body []byte) string {
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &out)
	return out.Error.Code
}

func statusError(status int, code string) string {
	if code == "" {
		return "HTTP " + strconv.Itoa(status)
	}
	return "HTTP " + strconv.Itoa(status) + " " + code
}

// networkError, kısa ve içerik taşımayan bir hata açıklaması üretir.
func networkError(err error) string {
	var nerr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()):
		return "zaman aşımı"
	case errors.Is(err, context.Canceled):
		return "iptal edildi"
	default:
		return "bağlantı hatası veya yanıt alınamadı"
	}
}

// parseRetryAfter, Retry-After başlığını (tam sayı saniye veya HTTP tarihi)
// çözer. Geçersiz, negatif veya geçmişteki değer için 0 döner; o durumda yalnızca
// backoff uygulanır.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 || secs > int64(24*time.Hour/time.Second) {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 && d <= 24*time.Hour {
			return d
		}
	}
	return 0
}
