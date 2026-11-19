package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
)

func smsJob() Job {
	return Job{ID: 1, Channel: ChannelSMS, Recipient: "+905550000001", IdempotencyKey: "camp:7:sms"}
}

func emailJob() Job {
	return Job{ID: 2, Channel: ChannelEmail, Recipient: "ada@example.test", IdempotencyKey: "camp:7:email"}
}

var rendered = content.Content{SMSText: "Merhaba Ada", EmailSubject: "Konu", EmailBody: "Gövde"}

func TestHTTPProviderSendsSMSWithIdempotencyKey(t *testing.T) {
	var (
		mu      sync.Mutex
		gotPath string
		gotKey  string
		gotType string
		gotBody map[string]string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotPath, gotKey, gotType = r.URL.Path, r.Header.Get("Idempotency-Key"), r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"message_id":"m-1"}`)
	}))
	defer srv.Close()

	res := NewHTTPProvider(srv.URL+"/").Send(context.Background(), smsJob(), rendered)
	if res.Kind != KindAccepted {
		t.Fatalf("kabul bekleniyordu: %+v", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/v1/sms" || gotKey != "camp:7:sms" || gotType != "application/json" {
		t.Fatalf("istek: yol=%q anahtar=%q tür=%q", gotPath, gotKey, gotType)
	}
	if len(gotBody) != 2 || gotBody["to"] != "+905550000001" || gotBody["text"] != "Merhaba Ada" {
		t.Fatalf("SMS gövdesi: %v", gotBody)
	}
}

func TestHTTPProviderSendsEmail(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"message_id":"m-2"}`)
	}))
	defer srv.Close()

	res := NewHTTPProvider(srv.URL).Send(context.Background(), emailJob(), rendered)
	if res.Kind != KindAccepted {
		t.Fatalf("kabul bekleniyordu: %+v", res)
	}
	if gotPath != "/v1/email" || len(gotBody) != 3 || gotBody["to"] != "ada@example.test" ||
		gotBody["subject"] != "Konu" || gotBody["body"] != "Gövde" {
		t.Fatalf("e-posta isteği: yol=%q gövde=%v", gotPath, gotBody)
	}
}

func TestHTTPProviderClassifiesResponses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    Kind
		wantErr string
	}{
		{"200", 200, `{"message_id":"m"}`, KindAccepted, ""},
		{"202", 202, `{"message_id":"m"}`, KindAccepted, ""},
		{"429", 429, `{"error":{"code":"RATE_LIMITED"}}`, KindRateLimited, "HTTP 429"},
		{"500", 500, `{"error":{"code":"INTERNAL"}}`, KindServerError, "HTTP 500 INTERNAL"},
		{"502", 502, `bozuk`, KindServerError, "HTTP 502"},
		{"503", 503, ``, KindServerError, "HTTP 503"},
		{"400 geçersiz alıcı", 400, `{"error":{"code":"INVALID_RECIPIENT"}}`, KindRejected, "HTTP 400 INVALID_RECIPIENT"},
		{"404", 404, ``, KindRejected, "HTTP 404"},
		{"422 anahtar çakışması", 422, `{"error":{"code":"IDEMPOTENCY_KEY_REUSED"}}`, KindRejected, "HTTP 422 IDEMPOTENCY_KEY_REUSED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
			if res.Kind != tc.want || res.Err != tc.wantErr {
				t.Fatalf("sonuç = %+v, beklenen tür %s hata %q", res, tc.want, tc.wantErr)
			}
		})
	}
}

// Sözleşme: kabul `2xx` + `{"message_id": "<dolu metin>"}`. Gövde eksik, boş, JSON
// olmayan, message_id'siz veya okunamayan (yarıda kesilen) bir `2xx` KABUL
// sayılmaz: sonuç doğrulanamadı demektir. Mesaj doğrudan `sent` olmaz; aynı
// idempotency anahtarıyla yeniden denenir (güvenli: provider kayıtlı sonucu döndürür).
func TestHTTPProviderDoesNotTreat2xxWithoutValidBodyAsAccepted(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"200 gövdesiz", 200, ``},
		{"204 gövdesiz", 204, ``},
		{"boş nesne", 200, `{}`},
		{"boş message_id", 200, `{"message_id":""}`},
		{"boşluk message_id", 200, `{"message_id":"  "}`},
		{"null message_id", 200, `{"message_id":null}`},
		{"sayı message_id", 200, `{"message_id":123}`},
		{"JSON olmayan", 200, `OK`},
		{"HTML (araya giren proxy)", 200, `<html>gateway</html>`},
		{"dizi", 200, `["m-1"]`},
		{"başka alan", 200, `{"id":"m-1"}`},
		{"yarım JSON", 200, `{"message_id":"m-`},
		{"202 gövdesiz", 202, ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
			if res.Kind != KindInvalidResponse || res.Err == "" {
				t.Fatalf("geçersiz yanıt (sonuç doğrulanamadı) bekleniyordu: %+v", res)
			}
		})
	}

	t.Run("gövde okunurken kesilir", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100") // 100 bayt vaat edilir, 10 yazılır
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"message_`)
			w.(http.Flusher).Flush() // 2xx başlığı ve gövdenin bir kısmı istemciye ulaşır
			panic(http.ErrAbortHandler)
		}))
		defer srv.Close()
		res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
		if res.Kind != KindInvalidResponse {
			t.Fatalf("okunamayan gövde geçersiz yanıt sayılmalı: %+v", res)
		}
	})

	t.Run("hata metni gövde içeriği taşımaz", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `gizli-icerik +905550000001`)
		}))
		defer srv.Close()
		res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
		if strings.Contains(res.Err, "gizli") || strings.Contains(res.Err, "+9055") {
			t.Fatalf("last_error yanıt gövdesini içermemeli: %q", res.Err)
		}
	})
}

// message_id'nin geçerli olduğu 2xx yanıtlar kabul sayılmaya devam eder (ek alanlar zararsız).
func TestHTTPProviderAcceptsValid2xxBodies(t *testing.T) {
	for name, body := range map[string]string{
		"yalnızca message_id": `{"message_id":"m-1"}`,
		"ek alan":             `{"message_id":"m-1","status":"queued"}`,
		"boşluklu":            "  {\"message_id\": \"m-1\"}\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			if res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered); res.Kind != KindAccepted {
				t.Fatalf("kabul bekleniyordu: %+v", res)
			}
		})
	}
}

// 429 ayrı bir sonuçtur: kalıcı 4xx (rejected) değil, yeniden denenebilir.
func TestHTTPProvider429IsNotRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
	if res.Kind != KindRateLimited || res.RetryAfter != 3*time.Second {
		t.Fatalf("429 rate_limited ve Retry-After=3s olmalı: %+v", res)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"3", 3 * time.Second},
		{" 7 ", 7 * time.Second},
		{"-5", 0},
		{"abc", 0},
		{"1.5", 0},
		{"86400", 24 * time.Hour},
		{"86401", 0}, // makul sınırın üstü geçersiz sayılır
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second},
		{now.Add(-10 * time.Second).Format(http.TimeFormat), 0}, // geçmiş tarih
		{"Mon, 99 Foo 2026 25:61:61 GMT", 0},
	}
	for _, tc := range tests {
		if got := parseRetryAfter(tc.in, now); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %s, beklenen %s", tc.in, got, tc.want)
		}
	}
}

// Hata metni alıcı adresini veya mesaj metnini taşımamalıdır (last_error'a yazılır).
func TestHTTPProviderErrorDoesNotLeakRequestOrResponseContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"code":"INTERNAL","message":"+905550000001 Merhaba Ada"}}`)
	}))
	defer srv.Close()

	res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
	if strings.Contains(res.Err, "+9055") || strings.Contains(res.Err, "Merhaba") {
		t.Fatalf("hata metni içerik taşıyor: %q", res.Err)
	}
}

func TestHTTPProviderNetworkFailuresAreTransient(t *testing.T) {
	t.Run("yanıt alınamadı (bağlantı kesildi)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))
		defer srv.Close()
		res := NewHTTPProvider(srv.URL).Send(context.Background(), smsJob(), rendered)
		if res.Kind != KindNetworkError || res.Err == "" {
			t.Fatalf("network_error bekleniyordu: %+v", res)
		}
	})

	t.Run("zaman aşımı", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
		defer srv.Close()
		defer close(release)

		p := NewHTTPProvider(srv.URL)
		p.Client.Timeout = 50 * time.Millisecond
		res := p.Send(context.Background(), smsJob(), rendered)
		if res.Kind != KindNetworkError || res.Err != "zaman aşımı" {
			t.Fatalf("zaman aşımı bekleniyordu: %+v", res)
		}
	})

	t.Run("sunucuya erişilemiyor", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()
		res := NewHTTPProvider(url).Send(context.Background(), smsJob(), rendered)
		if res.Kind != KindNetworkError {
			t.Fatalf("network_error bekleniyordu: %+v", res)
		}
	})

	t.Run("bağlam iptali", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
		defer srv.Close()
		defer close(release)

		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(30 * time.Millisecond); cancel() }()
		start := time.Now()
		res := NewHTTPProvider(srv.URL).Send(ctx, smsJob(), rendered)
		if res.Kind != KindNetworkError || res.Err != "iptal edildi" || time.Since(start) > 2*time.Second {
			t.Fatalf("iptal hemen ve 'iptal edildi' ile dönmeli: %+v (%s)", res, time.Since(start))
		}
	})
}

func TestHTTPProviderRejectsUnknownChannelWithoutRequest(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer srv.Close()

	job := smsJob()
	job.Channel = "push"
	res := NewHTTPProvider(srv.URL).Send(context.Background(), job, rendered)
	if res.Kind != KindRejected || calls.Load() != 0 {
		t.Fatalf("bilinmeyen kanal istek yapmadan reddedilmeli: %+v çağrı=%d", res, calls.Load())
	}
}

// net/http, Idempotency-Key başlıklı ve gövdesi yeniden okunabilen isteği
// "tekrarlanabilir" sayar; yeniden kullanılan bağlantıda yanıt okunamazsa isteği
// sessizce bir kez daha gönderir. Bu, hız sınırlayıcıyı atlar ve "2 HTTP isteği"
// gibi sayımları bozar. Tekrar denemeyi yalnızca worker yönetmelidir.
func TestHTTPProviderDoesNotSilentlyResendOnReusedConnection(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 2 {
			panic(http.ErrAbortHandler) // ikinci isteğin yanıtı kaybolur
		}
		_, _ = io.WriteString(w, `{"message_id":"m-3"}`)
	}))
	defer srv.Close()

	// Keep-alive açık (üretimdeki gibi): ikinci istek ilk bağlantıyı yeniden kullanır.
	p := &HTTPProvider{BaseURL: srv.URL, Client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{}}}

	if res := p.Send(context.Background(), smsJob(), rendered); res.Kind != KindAccepted {
		t.Fatalf("hazırlık: %+v", res)
	}
	res := p.Send(context.Background(), smsJob(), rendered)

	if res.Kind != KindNetworkError {
		t.Fatalf("kaybolan yanıt network_error olmalı (sessiz yeniden gönderim bunu gizlerdi): %+v", res)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("provider'a tam 2 istek ulaşmalı, ulaşan: %d (sessiz yeniden gönderim var)", got)
	}
}
