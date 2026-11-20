package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

var fixedNow = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

type fakeGenerator struct {
	seg    segment.Segment
	err    error
	briefs []string
}

func (f *fakeGenerator) Generate(_ context.Context, brief string) (segment.Segment, error) {
	f.briefs = append(f.briefs, brief)
	return f.seg, f.err
}

type fakeCounter struct {
	counts audience.Counts
	err    error
	calls  int
	gotNow time.Time
	gotSeg segment.Segment
}

func (f *fakeCounter) Count(_ context.Context, seg segment.Segment, now time.Time) (audience.Counts, error) {
	f.calls++
	f.gotNow, f.gotSeg = now, seg
	return f.counts, f.err
}

type fakeStore struct {
	createErr error
	created   []string // brief'ler
	gotCounts audience.Counts

	getResult campaign.Campaign
	getErr    error
	gotID     string

	listResult []campaign.Summary
	listErr    error
	gotLimit   int
	listCalls  int
}

func (f *fakeStore) List(_ context.Context, limit int) ([]campaign.Summary, error) {
	f.listCalls++
	f.gotLimit = limit
	return f.listResult, f.listErr
}

func (f *fakeStore) Create(_ context.Context, brief string, seg segment.Segment, counts audience.Counts) (campaign.Campaign, error) {
	f.created = append(f.created, brief)
	f.gotCounts = counts
	if f.createErr != nil {
		return campaign.Campaign{}, f.createErr
	}
	return campaign.Campaign{
		ID: "11111111-1111-1111-1111-111111111111", Brief: brief, Segment: seg,
		Status: campaign.StatusDraft, Audience: counts,
		CreatedAt: time.Date(2026, 11, 17, 15, 0, 0, 0, time.FixedZone("TRT", 3*3600)),
	}, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (campaign.Campaign, error) {
	f.gotID = id
	return f.getResult, f.getErr
}

type harness struct {
	gen     *fakeGenerator
	counter *fakeCounter
	members *fakeMembers
	store   *fakeStore
	cgen    *fakeContentGen
	content *fakeContents
	approve *fakeApprover
	handler http.Handler
	logs    *strings.Builder
	// llmFailures, llm_validation_failures_total sayacıdır.
	llmFailures *metrics.Counter
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	seg, issues := segment.Validate([]byte(`{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30}],"unsupported":[]}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	h := &harness{
		gen:     &fakeGenerator{seg: seg},
		counter: &fakeCounter{counts: audience.Counts{Total: 10, SMS: 6, Email: 7}},
		members: &fakeMembers{},
		store:   &fakeStore{},
		cgen:    &fakeContentGen{},
		content: &fakeContents{},
		approve: &fakeApprover{},
		logs:    &strings.Builder{},

		llmFailures: &metrics.Counter{},
	}
	h.handler = New(Deps{
		Segments:              h.gen,
		Audience:              h.counter,
		Members:               h.members,
		Campaigns:             h.store,
		Content:               h.cgen,
		Contents:              h.content,
		Approver:              h.approve,
		LLMValidationFailures: h.llmFailures,
		Now:                   func() time.Time { return fixedNow },
		Logger:                slog.New(slog.NewTextHandler(h.logs, nil)),
	})
	return h
}

func (h *harness) do(method, path, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func (h *harness) post(body string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, "/api/campaigns", "application/json", body)
}

type errorResponse struct {
	Error struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Reasons []reason `json:"reasons"`
	} `json:"error"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var e errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("hata gövdesi JSON değil: %v\n%s", err, rec.Body.String())
	}
	return e
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) errorResponse {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, beklenen %d\n%s", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	e := decodeError(t, rec)
	if e.Error.Code != code || e.Error.Message == "" {
		t.Fatalf("hata = %+v, beklenen kod %s", e.Error, code)
	}
	return e
}

func TestCreateCampaignSuccess(t *testing.T) {
	h := newHarness(t)

	rec := h.post(`{"brief":"  Son 30 günde sepete ekleyen İstanbul kullanıcıları  "}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/campaigns/11111111-1111-1111-1111-111111111111" {
		t.Fatalf("Location = %q", loc)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("güvenlik başlıkları eksik: %v", rec.Header())
	}

	// Tam yanıt sözleşmesi.
	want := `{"id":"11111111-1111-1111-1111-111111111111",` +
		`"brief":"Son 30 günde sepete ekleyen İstanbul kullanıcıları",` +
		`"status":"draft",` +
		`"segment":{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30}]},` +
		`"audience":{"total":10,"sms":6,"email":7},` +
		`"created_at":"2026-11-17T12:00:00Z"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("gövde:\n%s\nbeklenen:\n%s", got, want)
	}

	// Bağımlılıklar doğru girdilerle çağrıldı.
	if len(h.gen.briefs) != 1 || h.gen.briefs[0] != "Son 30 günde sepete ekleyen İstanbul kullanıcıları" {
		t.Fatalf("segment üretimi kırpılmış brief ile çağrılmalı: %q", h.gen.briefs)
	}
	if h.counter.calls != 1 || !h.counter.gotNow.Equal(fixedNow) || len(h.counter.gotSeg.Conditions) != 2 {
		t.Fatalf("kitle sayımı referans zaman ve doğrulanmış segmentle çağrılmalı: %+v", h.counter)
	}
	if len(h.store.created) != 1 || h.store.created[0] != h.gen.briefs[0] || h.store.gotCounts != h.counter.counts {
		t.Fatalf("kayıt kırpılmış brief ve sayılarla yapılmalı: %+v", h.store)
	}
}

func TestCreateCampaignAllowsZeroAudience(t *testing.T) {
	h := newHarness(t)
	h.counter.counts = audience.Counts{}

	rec := h.post(`{"brief":"Kars'taki kullanıcılar"}`)

	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"audience":{"total":0,"sms":0,"email":0}`) {
		t.Fatalf("sıfır eşleşme geçerli bir önizlemedir: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateCampaignRejectsBadRequestsWithoutCallingDependencies(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
		code        string
	}{
		{"içerik türü yok", "", `{"brief":"x"}`, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
		{"yanlış içerik türü", "text/plain", `{"brief":"x"}`, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
		{"charset ile json kabul", "application/json; charset=utf-8", `{"brief":"x", "extra": 1}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"bozuk json", "application/json", `{"brief":`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"boş gövde", "application/json", ``, http.StatusBadRequest, "INVALID_REQUEST"},
		{"json olmayan metin", "application/json", `brief`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"brief metin değil", "application/json", `{"brief":123}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"brief null olmayan nesne", "application/json", `{"brief":{"a":1}}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"dizi gövde", "application/json", `["brief"]`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"bilinmeyen alan", "application/json", `{"brief":"x","segment":{"all":[]}}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"birden fazla json değeri", "application/json", `{"brief":"x"}{"brief":"y"}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"gövde sonrası çöp", "application/json", `{"brief":"x"} garbage`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"gövde çok büyük", "application/json", `{"brief":"` + strings.Repeat("a", maxBodyBytes) + `"}`, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			rec := h.do(http.MethodPost, "/api/campaigns", tt.contentType, tt.body)
			wantError(t, rec, tt.status, tt.code)
			if len(h.gen.briefs) != 0 || h.counter.calls != 0 || len(h.store.created) != 0 {
				t.Fatalf("geçersiz istek bağımlılıkları çağırmamalı: %+v %+v %+v", h.gen, h.counter, h.store)
			}
		})
	}
}

func TestCreateCampaignMapsGenerationErrors(t *testing.T) {
	invalid := &segment.InvalidError{Issues: []segment.Issue{
		{Code: segment.CodeUnsupportedField, Field: "favorite_color"},
		{Code: segment.CodeUnsupportedPart, Text: "ürün kategorisi"},
	}}

	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"boş brief", segment.ErrEmptyBrief, http.StatusUnprocessableEntity, "BRIEF_EMPTY"},
		{"uzun brief", segment.ErrBriefTooLong, http.StatusUnprocessableEntity, "BRIEF_TOO_LONG"},
		{"geçersiz segment", invalid, http.StatusUnprocessableEntity, "SEGMENT_INVALID"},
		{"sarılmış geçersiz segment", errors.Join(errors.New("ek bağlam"), invalid), http.StatusUnprocessableEntity, "SEGMENT_INVALID"},
		{"LLM zaman aşımı", llm.ErrTimeout, http.StatusGatewayTimeout, "LLM_TIMEOUT"},
		{"LLM 429", llm.ErrRateLimited, http.StatusServiceUnavailable, "LLM_RATE_LIMITED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.gen.err = tt.err

			rec := h.post(`{"brief":"x"}`)

			wantError(t, rec, tt.status, tt.code)
			if h.counter.calls != 0 || len(h.store.created) != 0 {
				t.Fatal("başarısız üretimde kitle sorgusu çalışmamalı ve kampanya oluşmamalı")
			}
		})
	}
}

func TestSegmentInvalidResponseCarriesReasonCodes(t *testing.T) {
	h := newHarness(t)
	h.gen.err = &segment.InvalidError{Issues: []segment.Issue{
		{Code: segment.CodeUnsupportedField, Field: "favorite_color"},
		{Code: segment.CodeUnsupportedPart, Text: "ürün kategorisi"},
	}}

	rec := h.post(`{"brief":"x"}`)

	e := wantError(t, rec, http.StatusUnprocessableEntity, "SEGMENT_INVALID")
	want := []reason{
		{Code: "UNSUPPORTED_FIELD", Field: "favorite_color"},
		{Code: "UNSUPPORTED_PART", Text: "ürün kategorisi"},
	}
	if len(e.Error.Reasons) != 2 || e.Error.Reasons[0] != want[0] || e.Error.Reasons[1] != want[1] {
		t.Fatalf("sebepler = %+v, beklenen %+v", e.Error.Reasons, want)
	}
	if strings.Contains(rec.Body.String(), `"segment"`) || strings.Contains(rec.Body.String(), `"audience"`) {
		t.Fatal("hata yanıtı segment veya kitle sayısı içermemeli")
	}
}

func TestRateLimitedResponseHasRetryAfter(t *testing.T) {
	h := newHarness(t)
	h.gen.err = llm.ErrRateLimited
	if rec := h.post(`{"brief":"x"}`); rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 kaynaklı yanıt Retry-After taşımalı")
	}
}

func TestCreateCampaignInternalErrorsDoNotLeakDetails(t *testing.T) {
	secret := errors.New("pq: password authentication failed for user campaigns secret-host")

	tests := []struct {
		name  string
		setup func(h *harness)
	}{
		{"üretim", func(h *harness) { h.gen.err = secret }},
		{"kitle sayımı", func(h *harness) { h.counter.err = secret }},
		{"kayıt", func(h *harness) { h.store.createErr = secret }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			tt.setup(h)

			rec := h.post(`{"brief":"x"}`)

			wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
			if strings.Contains(rec.Body.String(), "secret-host") || strings.Contains(rec.Body.String(), "password") {
				t.Fatalf("iç hata ayrıntısı istemciye sızmamalı: %s", rec.Body.String())
			}
			if !strings.Contains(h.logs.String(), "secret-host") {
				t.Fatal("ayrıntı loga yazılmalı")
			}
		})
	}

	// Sayım hatasında kampanya kaydedilmemeli.
	h := newHarness(t)
	h.counter.err = secret
	h.post(`{"brief":"x"}`)
	if len(h.store.created) != 0 {
		t.Fatal("kitle sayımı başarısızsa kampanya oluşmamalı")
	}
}

func TestCreateCampaignStaysQuietWhenClientDisconnects(t *testing.T) {
	h := newHarness(t)
	h.gen.err = context.Canceled

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns", strings.NewReader(`{"brief":"x"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)

	if rec.Body.Len() != 0 || h.logs.Len() != 0 {
		t.Fatalf("kapanan bağlantı için yanıt veya hata logu yazılmamalı: %q %q", rec.Body.String(), h.logs.String())
	}
}

func TestGetCampaign(t *testing.T) {
	h := newHarness(t)
	seg, _ := segment.Validate([]byte(`{"all":[{"field":"order_count","op":"gte","value":2}],"unsupported":[]}`))
	h.store.getResult = campaign.Campaign{
		ID: "22222222-2222-2222-2222-222222222222", Brief: "iki sipariş", Segment: seg,
		Status: campaign.StatusDraft, Audience: audience.Counts{Total: 3, SMS: 2, Email: 1},
		CreatedAt: time.Date(2026, 11, 17, 9, 30, 0, 0, time.UTC),
	}

	rec := h.do(http.MethodGet, "/api/campaigns/22222222-2222-2222-2222-222222222222", "", "")

	want := `{"id":"22222222-2222-2222-2222-222222222222","brief":"iki sipariş","status":"draft",` +
		`"segment":{"all":[{"field":"order_count","op":"gte","value":2}]},` +
		`"audience":{"total":3,"sms":2,"email":1},"created_at":"2026-11-17T09:30:00Z"}`
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("%d\n%s\nbeklenen:\n%s", rec.Code, rec.Body.String(), want)
	}
	if h.store.gotID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("kimlik yoldan okunmalı: %q", h.store.gotID)
	}
	if len(h.gen.briefs) != 0 || h.counter.calls != 0 {
		t.Fatal("GET segment üretmemeli ve kitle sorgusu çalıştırmamalı")
	}
}

func TestGetCampaignErrors(t *testing.T) {
	h := newHarness(t)
	h.store.getErr = campaign.ErrNotFound
	wantError(t, h.do(http.MethodGet, "/api/campaigns/abc", "", ""), http.StatusNotFound, "CAMPAIGN_NOT_FOUND")

	h = newHarness(t)
	h.store.getErr = errors.New("bağlantı koptu: secret-host")
	rec := h.do(http.MethodGet, "/api/campaigns/33333333-3333-3333-3333-333333333333", "", "")
	wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
	if strings.Contains(rec.Body.String(), "secret-host") {
		t.Fatal("iç hata ayrıntısı sızmamalı")
	}
}

func TestRoutingErrorsAreJSON(t *testing.T) {
	h := newHarness(t)

	for _, tt := range []struct {
		method, path, allow string
		status              int
		code                string
	}{
		{http.MethodPut, "/api/campaigns", "GET, POST", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodDelete, "/api/campaigns", "GET, POST", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodDelete, "/api/campaigns/abc", "GET", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodPost, "/api/campaigns/abc", "GET", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodGet, "/api/unknown", "", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodGet, "/api/campaigns/abc/extra", "", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodPost, "/api/", "", http.StatusNotFound, "NOT_FOUND"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := h.do(tt.method, tt.path, "application/json", "{}")
			wantError(t, rec, tt.status, tt.code)
			if got := rec.Header().Get("Allow"); got != tt.allow {
				t.Fatalf("Allow = %q, beklenen %q", got, tt.allow)
			}
		})
	}
	if len(h.gen.briefs) != 0 || len(h.store.created) != 0 || h.store.gotID != "" {
		t.Fatal("yönlendirme hataları bağımlılıkları çağırmamalı")
	}
}

// Regresyon: geçerli bir JSON'dan sonra gelen boşluklar gövde sınırını aşınca
// gövde sonu kontrolü `MaxBytesError`'ı yutuyor ve 413 yerine 400 dönüyordu.
func TestCreateCampaignWhitespaceAfterValidJSONOverLimitIs413(t *testing.T) {
	const valid = `{"brief":"x"}`

	tests := []struct {
		name string
		body string
		// chunked: Content-Length bilinmeyen (parçalı) gövde.
		chunked bool
	}{
		{"geçerli JSON ve ardından 64 KiB boşluk", valid + strings.Repeat(" ", maxBodyBytes), false},
		{"sınırı bir bayt aşan gövde", valid + strings.Repeat(" ", maxBodyBytes-len(valid)+1), false},
		{"parçalı gövde: geçerli JSON ve ardından 64 KiB boşluk", valid + strings.Repeat(" ", maxBodyBytes), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.chunked {
				req.ContentLength = -1
			}
			rec := httptest.NewRecorder()

			h.handler.ServeHTTP(rec, req)

			wantError(t, rec, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
			if len(h.gen.briefs) != 0 || h.counter.calls != 0 || len(h.store.created) != 0 {
				t.Fatal("limiti aşan istek bağımlılıkları çağırmamalı")
			}
		})
	}
}

func TestCreateCampaignBodyExactlyAtLimitIsAccepted(t *testing.T) {
	const valid = `{"brief":"x"}`
	h := newHarness(t)

	// Tam sınırdaki gövde (sondaki boşluklarla) 413 değildir.
	body := valid + strings.Repeat(" ", maxBodyBytes-len(valid))
	if len(body) != maxBodyBytes {
		t.Fatalf("test gövdesi %d bayt, beklenen %d", len(body), maxBodyBytes)
	}
	rec := h.post(body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("tam sınırdaki gövde kabul edilmeli: %d %s", rec.Code, rec.Body.String())
	}
}
