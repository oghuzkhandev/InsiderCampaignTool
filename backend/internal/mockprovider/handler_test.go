package mockprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// memStore, Store'un bellekteki taklididir (PostgreSQL'siz birim testleri için).
// Gerçek atomiklik PostgreSQL entegrasyon testlerinde sınanır.
type memStore struct {
	mu   sync.Mutex
	rows map[string]memRow
	err  error
}

type memRow struct {
	req Request
	id  string
}

func newMemStore() *memStore { return &memStore{rows: map[string]memRow{}} }

func (s *memStore) Accept(_ context.Context, r Request) (Acceptance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Acceptance{}, s.err
	}
	if row, ok := s.rows[r.Key]; ok {
		if row.req != r {
			return Acceptance{}, ErrKeyReused
		}
		return Acceptance{MessageID: row.id}, nil
	}
	id := "msg-" + strconv.Itoa(len(s.rows)+1)
	s.rows[r.Key] = memRow{req: r, id: id}
	return Acceptance{MessageID: id, Created: true}, nil
}

func (s *memStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// newTestServer, handler'ı gerçek bir HTTP sunucusunda çalıştırır (yanıt
// düşürme gerçek bağlantı gerektirir).
func newTestServer(t *testing.T, cfg Config, opts ...Option) (*httptest.Server, *Handler, *memStore) {
	t.Helper()
	store := newMemStore()
	opts = append([]Option{WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)
	h, err := New(cfg, store, opts...)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, h, store
}

// Bağlantılar yeniden kullanılmaz: net/http, Idempotency-Key başlıklı isteği
// yeniden kullanılan bağlantıda sessizce tekrar gönderebilir ve sayımları bozar.
var client = &http.Client{
	Timeout:   5 * time.Second,
	Transport: &http.Transport{DisableKeepAlives: true},
}

const (
	smsBody   = `{"to":"+905550000001","text":"Merhaba Ada"}`
	emailBody = `{"to":"ada@example.test","subject":"Konu","body":"Gövde"}`
)

func post(t *testing.T, srv *httptest.Server, path, key, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	return client.Do(req)
}

type reply struct {
	status     int
	messageID  string
	errorCode  string
	retryAfter string
}

func mustPost(t *testing.T, srv *httptest.Server, path, key, body string) reply {
	t.Helper()
	resp, err := post(t, srv, path, key, body)
	if err != nil {
		t.Fatalf("istek başarısız: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		MessageID string `json:"message_id"`
		Error     struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return reply{resp.StatusCode, out.MessageID, out.Error.Code, resp.Header.Get("Retry-After")}
}

func TestSendAcceptsAndReplaysRecordedResult(t *testing.T) {
	srv, h, store := newTestServer(t, Config{})

	first := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody)
	if first.status != 200 || first.messageID == "" {
		t.Fatalf("ilk istek 200 ve message_id dönmeli: %+v", first)
	}
	again := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody)
	if again.status != 200 || again.messageID != first.messageID {
		t.Fatalf("tekrar istek kayıtlı sonucu almalı: %+v / %+v", first, again)
	}

	snap := h.Stats().Snapshot()
	if store.count() != 1 || snap.UniqueAccepts != 1 || snap.DuplicateRequests != 1 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("tek kabul, bir tekrar isteği, sıfır tekrar teslimat bekleniyordu: kayıt=%d %+v", store.count(), snap)
	}
	if snap.TotalRequests != 2 {
		t.Fatalf("iki istek sayılmalı: %d", snap.TotalRequests)
	}

	// E-posta kanalı da aynı kalıpla çalışır.
	e := mustPost(t, srv, "/v1/email", "k:1:email", emailBody)
	if e.status != 200 || e.messageID == "" {
		t.Fatalf("e-posta kabul edilmeli: %+v", e)
	}
}

func TestSendRejectsInvalidRequestsWithoutAccepting(t *testing.T) {
	srv, h, store := newTestServer(t, Config{})

	tests := []struct {
		name, path, key, body string
		code                  string
	}{
		{"anahtar yok", "/v1/sms", "", smsBody, CodeKeyRequired},
		{"boş anahtar", "/v1/sms", "  ", smsBody, CodeKeyRequired},
		{"bozuk JSON", "/v1/sms", "k", `{`, CodeInvalidRequest},
		{"bilinmeyen alan", "/v1/sms", "k", `{"to":"+905550000001","text":"x","extra":1}`, CodeInvalidRequest},
		{"boş metin", "/v1/sms", "k", `{"to":"+905550000001","text":" "}`, CodeInvalidRequest},
		{"geçersiz telefon", "/v1/sms", "k", `{"to":"abc","text":"x"}`, CodeInvalidRecipient},
		{"kısa telefon", "/v1/sms", "k", `{"to":"+90","text":"x"}`, CodeInvalidRecipient},
		{"geçersiz e-posta", "/v1/email", "k", `{"to":"ada","subject":"s","body":"b"}`, CodeInvalidRecipient},
		{"iki @", "/v1/email", "k", `{"to":"a@b@c","subject":"s","body":"b"}`, CodeInvalidRecipient},
		{"boş konu", "/v1/email", "k", `{"to":"a@b.test","subject":"","body":"b"}`, CodeInvalidRequest},
		{"boş gövde", "/v1/email", "k", `{"to":"a@b.test","subject":"s","body":""}`, CodeInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustPost(t, srv, tc.path, tc.key, tc.body)
			if got.status != http.StatusBadRequest || got.errorCode != tc.code {
				t.Fatalf("400 %s bekleniyordu: %+v", tc.code, got)
			}
		})
	}
	if store.count() != 0 {
		t.Fatalf("geçersiz istek kabul edilmemeli: %d kayıt", store.count())
	}
	if got := h.Stats().Snapshot().InvalidRequests; got != len(tests) {
		t.Fatalf("geçersiz istekler sayılmalı: %d != %d", got, len(tests))
	}
}

// Aynı anahtarla farklı alıcı, kanal veya içerik gelirse provider isteği
// reddeder ve kayıtlı sonucu döndürmez (ADR-025). Orijinal istek etkilenmez.
func TestSendRejectsKeyReuseWithDifferentRequest(t *testing.T) {
	srv, h, store := newTestServer(t, Config{})
	first := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody)
	if first.status != 200 {
		t.Fatalf("hazırlık: %+v", first)
	}

	tests := []struct {
		name, path, body string
	}{
		{"farklı alıcı", "/v1/sms", `{"to":"+905550000002","text":"Merhaba Ada"}`},
		{"farklı içerik", "/v1/sms", `{"to":"+905550000001","text":"Başka metin"}`},
		{"farklı kanal", "/v1/email", emailBody},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustPost(t, srv, tc.path, "k:1:sms", tc.body)
			if got.status != http.StatusUnprocessableEntity || got.errorCode != CodeKeyReused || got.messageID != "" {
				t.Fatalf("422 %s bekleniyordu, kayıtlı sonuç dönmemeli: %+v", CodeKeyReused, got)
			}
		})
	}

	if store.count() != 1 {
		t.Fatalf("yalnızca ilk kabul kalmalı: %d", store.count())
	}
	if got := h.Stats().Snapshot().KeyReuseRejected; got != len(tests) {
		t.Fatalf("çakışmalar sayılmalı: %d", got)
	}
	// Aynı istek hâlâ kayıtlı sonucu alır.
	if again := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody); again.status != 200 || again.messageID != first.messageID {
		t.Fatalf("özgün istek etkilenmemeli: %+v", again)
	}
}

func TestSendEmailKeyReuseWithDifferentSubjectOrBody(t *testing.T) {
	srv, _, _ := newTestServer(t, Config{})
	if r := mustPost(t, srv, "/v1/email", "k:1:email", emailBody); r.status != 200 {
		t.Fatalf("hazırlık: %+v", r)
	}
	for name, body := range map[string]string{
		"farklı konu":  `{"to":"ada@example.test","subject":"Başka","body":"Gövde"}`,
		"farklı gövde": `{"to":"ada@example.test","subject":"Konu","body":"Başka"}`,
		// Konu ve gövde arasındaki sınır kayması aynı özeti vermemeli.
		"sınır kayması": `{"to":"ada@example.test","subject":"Konu\u0000","body":"Gövde"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := mustPost(t, srv, "/v1/email", "k:1:email", body); got.status != 422 || got.errorCode != CodeKeyReused {
				t.Fatalf("422 bekleniyordu: %+v", got)
			}
		})
	}
}

func TestSendInjectsRateLimitWithRetryAfterAndDoesNotAccept(t *testing.T) {
	srv, h, store := newTestServer(t, Config{RateLimitRate: 1, RetryAfter: 1500 * time.Millisecond})

	got := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody)
	if got.status != http.StatusTooManyRequests || got.retryAfter != "2" {
		t.Fatalf("429 ve Retry-After=2 (yukarı yuvarlanmış) bekleniyordu: %+v", got)
	}
	if store.count() != 0 {
		t.Fatal("429 kabul kaydı oluşturmamalı")
	}
	snap := h.Stats().Snapshot()
	if snap.RateLimited != 1 || snap.TotalRequests != 1 || len(snap.Attempts["sms"]) != 1 {
		t.Fatalf("429 da bir deneme olarak sayılmalı: %+v", snap)
	}
}

func TestSendInjectsServerErrorAndDoesNotAccept(t *testing.T) {
	srv, h, store := newTestServer(t, Config{ServerErrorRate: 1})

	got := mustPost(t, srv, "/v1/email", "k:1:email", emailBody)
	if got.status != http.StatusInternalServerError {
		t.Fatalf("500 bekleniyordu: %+v", got)
	}
	if store.count() != 0 || h.Stats().Snapshot().ServerErrors != 1 {
		t.Fatalf("500 kabul kaydı oluşturmamalı: kayıt=%d", store.count())
	}
}

func TestSendRateLimitIsCheckedBeforeServerError(t *testing.T) {
	srv, _, _ := newTestServer(t, Config{RateLimitRate: 1, ServerErrorRate: 1})
	if got := mustPost(t, srv, "/v1/sms", "k", smsBody); got.status != http.StatusTooManyRequests {
		t.Fatalf("önce 429 değerlendirilmeli: %+v", got)
	}
}

func TestSendRandomFaultsUseConfiguredProbability(t *testing.T) {
	// 0.9 çekilişi %50'lik oranın altında değil: istek kabul edilir.
	srv, _, store := newTestServer(t, Config{RateLimitRate: 0.5, ServerErrorRate: 0.5},
		WithRand(func() float64 { return 0.9 }))
	if got := mustPost(t, srv, "/v1/sms", "k", smsBody); got.status != 200 {
		t.Fatalf("200 bekleniyordu: %+v", got)
	}
	if store.count() != 1 {
		t.Fatal("kabul kaydedilmeli")
	}
	// 0.1 çekilişi oranın altında: hata.
	srv2, _, _ := newTestServer(t, Config{RateLimitRate: 0.5}, WithRand(func() float64 { return 0.1 }))
	if got := mustPost(t, srv2, "/v1/sms", "k", smsBody); got.status != http.StatusTooManyRequests {
		t.Fatalf("429 bekleniyordu: %+v", got)
	}
}

func TestSendAppliesConfiguredLatency(t *testing.T) {
	var slept []time.Duration
	srv, _, _ := newTestServer(t, Config{Latency: 40 * time.Millisecond},
		WithSleep(func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }))
	mustPost(t, srv, "/v1/sms", "k", smsBody)
	if len(slept) != 1 || slept[0] != 40*time.Millisecond {
		t.Fatalf("40ms gecikme uygulanmalı: %v", slept)
	}
}

func TestSendReturns500WhenStoreFails(t *testing.T) {
	srv, h, store := newTestServer(t, Config{})
	store.err = errors.New("veritabanı yok")
	if got := mustPost(t, srv, "/v1/sms", "k", smsBody); got.status != http.StatusInternalServerError {
		t.Fatalf("500 bekleniyordu: %+v", got)
	}
	if h.Stats().Snapshot().UniqueAccepts != 0 {
		t.Fatal("kayıt yazılamadıysa kabul sayılmamalı")
	}
}

// accept_then_drop ana senaryoda yalnızca ilk kabul yanıtını düşürür: kayıt
// commit edilir, istemci yanıt alamaz; tekrar isteği kayıtlı sonucu alır ve
// sonraki yanıtlar düşürülmez.
func TestAcceptThenDropDropsOnlyTheFirstAcceptResponse(t *testing.T) {
	srv, h, store := newTestServer(t, Config{Mode: ModeAcceptThenDrop})

	if _, err := post(t, srv, "/v1/sms", "k:1:sms", smsBody); err == nil {
		t.Fatal("ilk yanıt düşürülmeli (istemci hata almalı)")
	}
	if store.count() != 1 {
		t.Fatalf("yanıt düşse de kabul kaydedilmiş olmalı: %d", store.count())
	}

	second := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody)
	third := mustPost(t, srv, "/v1/sms", "k:1:sms", smsBody)
	if second.status != 200 || second.messageID == "" || third != second {
		t.Fatalf("tekrar istekleri aynı kayıtlı sonucu almalı: %+v / %+v", second, third)
	}

	snap := h.Stats().Snapshot()
	if snap.TotalRequests != 3 || snap.UniqueAccepts != 1 || snap.DuplicateRequests != 2 ||
		snap.DuplicateDeliveries != 0 || snap.ResponsesDropped != 1 {
		t.Fatalf("3 istek, 1 kabul, 2 tekrar, 0 tekrar teslimat, 1 düşen yanıt bekleniyordu: %+v", snap)
	}

	// Her anahtarın ilk yanıtı düşer; başka anahtar için de ilk yanıt düşer.
	if _, err := post(t, srv, "/v1/sms", "k:2:sms", smsBody); err == nil {
		t.Fatal("yeni anahtarın ilk yanıtı da düşmeli")
	}
}

// drop_all_responses, kabul edilen isteğin HER yanıtını düşürür ve ayrı bir
// moddur. Kabul yine tek kez kaydedilir.
func TestDropAllResponsesDropsEveryAcceptedResponse(t *testing.T) {
	srv, h, store := newTestServer(t, Config{Mode: ModeDropAllResponses})

	for i := 0; i < 5; i++ {
		if _, err := post(t, srv, "/v1/sms", "k:1:sms", smsBody); err == nil {
			t.Fatalf("%d. yanıt düşmeli", i+1)
		}
	}
	if store.count() != 1 {
		t.Fatalf("beş istek tek kabul kaydı bırakmalı: %d", store.count())
	}
	snap := h.Stats().Snapshot()
	if snap.TotalRequests != 5 || snap.UniqueAccepts != 1 || snap.DuplicateRequests != 4 ||
		snap.DuplicateDeliveries != 0 || snap.ResponsesDropped != 5 {
		t.Fatalf("5 istek, 1 kabul, 4 tekrar, 5 düşen yanıt bekleniyordu: %+v", snap)
	}

	// Reddedilen istekler (geçersiz alıcı, anahtar çakışması) yanıt alır: düşürme
	// yalnızca KABUL edilen isteklere uygulanır.
	if got := mustPost(t, srv, "/v1/sms", "k:2:sms", `{"to":"abc","text":"x"}`); got.status != 400 {
		t.Fatalf("geçersiz istek 400 almalı: %+v", got)
	}
	if got := mustPost(t, srv, "/v1/sms", "k:1:sms", `{"to":"+905550000002","text":"x"}`); got.status != 422 {
		t.Fatalf("çakışma 422 almalı: %+v", got)
	}
}

func TestAdminStatsReportsAttemptTimestampsPerChannelIncludingRetries(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(10 * time.Millisecond)
		return now
	}
	srv, _, _ := newTestServer(t, Config{}, WithClock(clock))

	mustPost(t, srv, "/v1/sms", "a", smsBody)
	mustPost(t, srv, "/v1/sms", "a", smsBody) // tekrar da bir denemedir
	mustPost(t, srv, "/v1/email", "b", emailBody)
	mustPost(t, srv, "/v1/sms", "", smsBody) // anahtarsız istek de sağlayıcıya ulaşmıştır

	resp, err := client.Get(srv.URL + "/admin/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var snap Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Attempts["sms"]) != 3 || len(snap.Attempts["email"]) != 1 || snap.TotalRequests != 4 {
		t.Fatalf("kanal bazında her deneme (retry ve hatalı istekler dahil) sayılmalı: %+v", snap)
	}
	for i := 1; i < len(snap.Attempts["sms"]); i++ {
		if !snap.Attempts["sms"][i].After(snap.Attempts["sms"][i-1]) {
			t.Fatal("zaman damgaları artan sırada ve ayrı olmalı")
		}
	}
}

func TestMaxInWindow(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	at := func(ms ...int) []time.Time {
		var out []time.Time
		for _, m := range ms {
			out = append(out, base.Add(time.Duration(m)*time.Millisecond))
		}
		return out
	}
	tests := []struct {
		name string
		ts   []time.Time
		want int
	}{
		{"boş", nil, 0},
		{"tek", at(0), 1},
		{"hepsi pencerede", at(0, 100, 900), 3},
		// Tam 1 sn arayla olanlar farklı pencerelerdedir (yarı açık pencere).
		{"tam bir saniye arayla", at(0, 1000), 1},
		{"bir milisaniye eksik", at(0, 999), 2},
		{"sırasız girdi", at(900, 0, 100), 3},
		{"kayan pencere", at(0, 500, 1100, 1400, 1600), 3}, // 500..1400 içinde 500,1100,1400
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaxInWindow(tc.ts, time.Second); got != tc.want {
				t.Fatalf("MaxInWindow = %d, beklenen %d", got, tc.want)
			}
		})
	}
}

func TestConfigValidateAndFromEnv(t *testing.T) {
	for name, c := range map[string]Config{
		"negatif gecikme": {Latency: -1},
		"negatif retry":   {RetryAfter: -1},
		"429 oranı > 1":   {RateLimitRate: 1.5},
		"500 oranı < 0":   {ServerErrorRate: -0.1},
		"bilinmeyen mod":  {Mode: "kaos"},
	} {
		if err := c.Validate(); err == nil {
			t.Errorf("%s: geçersiz yapılandırma reddedilmeli", name)
		}
	}
	if _, err := New(Config{Mode: "kaos"}, newMemStore()); err == nil {
		t.Error("New geçersiz yapılandırmayı reddetmeli")
	}

	t.Setenv(EnvLatency, "75ms")
	t.Setenv(EnvRateLimitRate, "0.25")
	t.Setenv(EnvErrorRate, "0.1")
	t.Setenv(EnvRetryAfter, "3s")
	t.Setenv(EnvMode, string(ModeAcceptThenDrop))
	got, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Latency: 75 * time.Millisecond, RateLimitRate: 0.25, ServerErrorRate: 0.1,
		RetryAfter: 3 * time.Second, Mode: ModeAcceptThenDrop}
	if got != want {
		t.Fatalf("ConfigFromEnv = %+v, beklenen %+v", got, want)
	}

	t.Setenv(EnvMode, "kaos")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("bilinmeyen mod reddedilmeli")
	}
	t.Setenv(EnvMode, "")
	t.Setenv(EnvLatency, "hızlı")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("geçersiz süre reddedilmeli")
	}
}
