package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

const campaignID = "44444444-4444-4444-4444-444444444444"

var (
	contentPath = "/api/campaigns/" + campaignID + "/content"
	savedAt     = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	goodContent = content.Content{
		SMSText:      "Merhaba {{first_name}}, %30 indirim!",
		EmailSubject: "Sepetin seni bekliyor",
		EmailBody:    "Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada.",
	}
)

// eventLog, olayların sırasını (store çağrıları ve yanıt yazımları) kaydeder.
type eventLog struct {
	mu  sync.Mutex
	log []string
}

func (l *eventLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log = append(l.log, s)
}

func (l *eventLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.log...)
}

type fakeContentGen struct {
	deltas []content.Delta
	result content.Content
	err    error

	calls    int
	gotBrief string
	gotOpts  content.Options
	// onDone, deltalardan sonra ve dönmeden önce çalışır.
	onDone func()
}

func (f *fakeContentGen) Generate(_ context.Context, brief string, opts content.Options, onDelta func(content.Delta)) (content.Content, error) {
	f.calls++
	f.gotBrief, f.gotOpts = brief, opts
	for _, d := range f.deltas {
		onDelta(d)
	}
	if f.onDone != nil {
		f.onDone()
	}
	if f.err != nil {
		return content.Content{}, f.err
	}
	return f.result, nil
}

type savedContent struct {
	campaignID, source string
	content            content.Content
}

type fakeContents struct {
	maxName    int
	maxNameErr error

	saveVersion int
	saveErr     error
	saved       []savedContent

	latest      campaign.ContentVersion
	latestFound bool
	latestErr   error

	log *eventLog
}

func (f *fakeContents) Save(_ context.Context, id, source string, c content.Content) (campaign.ContentVersion, error) {
	if f.log != nil {
		f.log.add("save")
	}
	f.saved = append(f.saved, savedContent{id, source, c})
	if f.saveErr != nil {
		return campaign.ContentVersion{}, f.saveErr
	}
	v := f.saveVersion
	if v == 0 {
		v = 1
	}
	return campaign.ContentVersion{Version: v, Source: source, Content: c, CreatedAt: savedAt}, nil
}

func (f *fakeContents) Latest(context.Context, string) (campaign.ContentVersion, bool, error) {
	return f.latest, f.latestFound, f.latestErr
}

func (f *fakeContents) MaxFirstNameRunes(context.Context) (int, error) {
	return f.maxName, f.maxNameErr
}

func validSeg(t *testing.T) segment.Segment {
	t.Helper()
	seg, issues := segment.Validate([]byte(`{"all":[{"field":"order_count","op":"gte","value":2}],"unsupported":[]}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	return seg
}

func draftCampaign(h *harness) {
	h.store.getResult = campaign.Campaign{ID: campaignID, Brief: "sepet hatırlatması", Status: campaign.StatusDraft}
}

type sseEvent struct {
	Name string
	Data string
}

// parseSSE, gövdeyi olaylara ayırır ve biçimi katı doğrular: her olay
// `event:` ve tek satırlık `data:` içerir, boş satırla biter.
func parseSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	for _, block := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") || !strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("geçersiz SSE bloğu: %q", block)
		}
		events = append(events, sseEvent{Name: strings.TrimPrefix(lines[0], "event: "), Data: strings.TrimPrefix(lines[1], "data: ")})
	}
	if !strings.HasSuffix(body, "\n\n") && body != "" {
		t.Fatalf("son olay boş satırla bitmeli: %q", body)
	}
	return events
}

func names(events []sseEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Name)
	}
	return out
}

func postContent(h *harness) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, contentPath, "", "")
}

func TestGenerateContentStreamsDeltasThenValidated(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.content.maxName = 17
	h.content.saveVersion = 3
	h.cgen.deltas = []content.Delta{
		{Field: "sms_text", Text: "Merhaba ", Replace: true},
		{Field: "sms_text", Text: "{{first_name}}"},
		{Field: "email_subject", Text: "Konu \"x\"\n", Replace: true},
	}
	h.cgen.result = goodContent

	rec := postContent(h)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	hd := rec.Header()
	if !strings.HasPrefix(hd.Get("Content-Type"), "text/event-stream") || hd.Get("Cache-Control") != "no-store" ||
		hd.Get("X-Accel-Buffering") != "no" || hd.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("SSE başlıkları: %v", hd)
	}
	if !rec.Flushed {
		t.Fatal("olaylar flush edilmeli")
	}

	events := parseSSE(t, rec.Body.String())
	if got := names(events); !reflect.DeepEqual(got, []string{"delta", "delta", "delta", "validated"}) {
		t.Fatalf("olay sırası = %v", got)
	}
	wantDeltas := []string{
		`{"field":"sms_text","text":"Merhaba ","replace":true}`,
		`{"field":"sms_text","text":"{{first_name}}"}`,
		`{"field":"email_subject","text":"Konu \"x\"\n","replace":true}`,
	}
	for i, w := range wantDeltas {
		if events[i].Data != w {
			t.Fatalf("delta %d:\n %s\nbeklenen:\n %s", i, events[i].Data, w)
		}
	}
	wantValidated := `{"content_version":3,"source":"llm",` +
		`"sms_text":"Merhaba {{first_name}}, %30 indirim!","email_subject":"Sepetin seni bekliyor",` +
		`"email_body":"Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada.","created_at":"2026-11-18T10:00:00Z"}`
	if events[3].Data != wantValidated {
		t.Fatalf("validated:\n %s\nbeklenen:\n %s", events[3].Data, wantValidated)
	}

	// Bağımlılıklar doğru girdilerle çağrıldı.
	if h.cgen.gotBrief != "sepet hatırlatması" {
		t.Fatalf("üretim kayıtlı brief ile çağrılmalı: %q", h.cgen.gotBrief)
	}
	if h.cgen.gotOpts.MaxNameRunes != 17 {
		t.Fatalf("en uzun ad doğrulayıcıya iletilmeli: %+v", h.cgen.gotOpts)
	}
	want := []savedContent{{campaignID, campaign.SourceLLM, goodContent}}
	if !reflect.DeepEqual(h.content.saved, want) {
		t.Fatalf("kayıt = %+v", h.content.saved)
	}
}

// Her olay JSON'un tek satırıdır: metindeki satır sonları kaçışlıdır, böylece
// SSE çerçevesi bozulmaz.
func TestSSEEventsNeverBreakFramingOnNewlines(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.cgen.deltas = []content.Delta{{Field: "email_body", Text: "a\n\nevent: error\ndata: x\n\n", Replace: true}}
	h.cgen.result = goodContent

	events := parseSSE(t, postContent(h).Body.String())
	if got := names(events); !reflect.DeepEqual(got, []string{"delta", "validated"}) {
		t.Fatalf("metindeki satır sonları olay uyduramamalı: %v", got)
	}
}

// `validated` yalnızca kayıt (commit) tamamlandıktan sonra gönderilir.
type orderedWriter struct {
	http.ResponseWriter
	log *eventLog
}

func (w orderedWriter) Write(b []byte) (int, error) {
	for _, name := range []string{"delta", "validated", "error"} {
		if strings.HasPrefix(string(b), "event: "+name+"\n") {
			w.log.add("write:" + name)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w orderedWriter) Flush() { w.ResponseWriter.(http.Flusher).Flush() }

func TestValidatedIsSentOnlyAfterSave(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	log := &eventLog{}
	h.content.log = log
	h.cgen.deltas = []content.Delta{{Field: "sms_text", Text: "a", Replace: true}}
	h.cgen.result = goodContent

	req := httptest.NewRequest(http.MethodPost, contentPath, nil)
	h.handler.ServeHTTP(orderedWriter{httptest.NewRecorder(), log}, req)

	want := []string{"write:delta", "save", "write:validated"}
	if got := log.all(); !reflect.DeepEqual(got, want) {
		t.Fatalf("sıra = %v, beklenen %v", got, want)
	}
}

func TestGenerateContentFailuresSendErrorEventAndSaveNothing(t *testing.T) {
	partial := content.Content{SMSText: "çok uzun", EmailSubject: "Konu"}
	tests := []struct {
		name       string
		err        error
		code       string
		wantReason []string
		wantPart   *content.Content
	}{
		{"zaman aşımı", llm.ErrTimeout, "LLM_TIMEOUT", nil, nil},
		{"429", llm.ErrRateLimited, "LLM_RATE_LIMITED", nil, nil},
		{"kesilen akış", llm.ErrStreamInterrupted, "STREAM_INTERRUPTED", nil, nil},
		{"geçersiz içerik", &content.InvalidError{
			Issues:  []content.Issue{{Code: content.CodeTooLong, Field: "sms_text"}, {Code: content.CodeUnknownVariable, Field: "email_body", Text: "city"}},
			Partial: partial,
		}, "CONTENT_INVALID", []string{"TOO_LONG:sms_text", "UNKNOWN_VARIABLE:email_body"}, &partial},
		{"hiç metin çözümlenemedi", &content.InvalidError{Issues: []content.Issue{{Code: content.CodeMalformedOutput}}}, "CONTENT_INVALID", []string{"MALFORMED_OUTPUT:"}, nil},
		{"beklenmeyen hata", errors.New("bağlantı koptu: secret-host"), "INTERNAL", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			h.cgen.deltas = []content.Delta{{Field: "sms_text", Text: "kısmi", Replace: true}}
			h.cgen.err = tt.err

			rec := postContent(h)

			if rec.Code != http.StatusOK {
				t.Fatalf("akış başladıktan sonra durum 200 kalır: %d", rec.Code)
			}
			events := parseSSE(t, rec.Body.String())
			last := events[len(events)-1]
			if last.Name != "error" {
				t.Fatalf("son olay error olmalı: %v", names(events))
			}
			for _, e := range events {
				if e.Name == "validated" {
					t.Fatal("hata durumunda validated gönderilmemeli")
				}
			}
			var ev struct {
				Code    string                         `json:"code"`
				Message string                         `json:"message"`
				Reasons []struct{ Code, Field string } `json:"reasons"`
				Partial *content.Content               `json:"partial"`
			}
			if err := json.Unmarshal([]byte(last.Data), &ev); err != nil {
				t.Fatal(err)
			}
			if ev.Code != tt.code || ev.Message == "" {
				t.Fatalf("error olayı = %+v", ev)
			}
			var gotReasons []string
			for _, r := range ev.Reasons {
				gotReasons = append(gotReasons, r.Code+":"+r.Field)
			}
			if !reflect.DeepEqual(gotReasons, tt.wantReason) {
				t.Fatalf("sebepler = %v, beklenen %v", gotReasons, tt.wantReason)
			}
			if !reflect.DeepEqual(ev.Partial, tt.wantPart) {
				t.Fatalf("partial = %+v, beklenen %+v", ev.Partial, tt.wantPart)
			}
			if len(h.content.saved) != 0 {
				t.Fatalf("hata durumunda sürüm kaydedilmemeli: %+v", h.content.saved)
			}
			if tt.code == "INTERNAL" {
				if strings.Contains(rec.Body.String(), "secret-host") {
					t.Fatal("iç hata ayrıntısı akışa sızmamalı")
				}
				if !strings.Contains(h.logs.String(), "secret-host") {
					t.Fatal("ayrıntı loga yazılmalı")
				}
			}
		})
	}
}

func TestGenerateContentSaveFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		code string
	}{
		{"üretim sırasında onaylandı", campaign.ErrNotDraft, "CAMPAIGN_NOT_DRAFT"},
		{"kampanya silindi", campaign.ErrNotFound, "CAMPAIGN_NOT_FOUND"},
		{"veritabanı hatası", errors.New("pq: secret-host"), "INTERNAL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			h.cgen.result = goodContent
			h.content.saveErr = tt.err

			rec := postContent(h)

			events := parseSSE(t, rec.Body.String())
			if got := names(events); !reflect.DeepEqual(got, []string{"error"}) {
				t.Fatalf("olaylar = %v", got)
			}
			if !strings.Contains(events[0].Data, `"code":"`+tt.code+`"`) {
				t.Fatalf("error olayı = %s", events[0].Data)
			}
			if strings.Contains(rec.Body.String(), "secret-host") {
				t.Fatal("iç hata ayrıntısı sızmamalı")
			}
		})
	}
}

// Akış başlamadan önceki sorunlar düz JSON hatadır ve akış hiç başlamaz.
func TestGenerateContentPreStreamErrors(t *testing.T) {
	t.Run("kampanya yok", func(t *testing.T) {
		h := newHarness(t)
		h.store.getErr = campaign.ErrNotFound
		wantError(t, postContent(h), http.StatusNotFound, "CAMPAIGN_NOT_FOUND")
		if h.cgen.calls != 0 {
			t.Fatal("üretim çağrılmamalı")
		}
	})
	for _, status := range []string{"approved", "sending", "completed"} {
		t.Run("draft değil: "+status, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			h.store.getResult.Status = status
			wantError(t, postContent(h), http.StatusConflict, "CAMPAIGN_NOT_DRAFT")
			if h.cgen.calls != 0 || len(h.content.saved) != 0 {
				t.Fatal("üretim ve kayıt olmamalı")
			}
		})
	}
	t.Run("kampanya okunamadı", func(t *testing.T) {
		h := newHarness(t)
		h.store.getErr = errors.New("secret-host")
		rec := postContent(h)
		wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
		if strings.Contains(rec.Body.String(), "secret-host") {
			t.Fatal("sızıntı")
		}
	})
	t.Run("en uzun ad okunamadı", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		h.content.maxNameErr = errors.New("secret-host")
		wantError(t, postContent(h), http.StatusInternalServerError, "INTERNAL")
		if h.cgen.calls != 0 {
			t.Fatal("üretim çağrılmamalı")
		}
	})
}

// İstemci ayrılırsa (bağlam iptali) hata olayı yazılmaz ve kayıt yapılmaz.
func TestGenerateContentClientDisconnectSavesNothing(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	ctx, cancel := context.WithCancel(context.Background())
	h.cgen.err = context.Canceled
	h.cgen.onDone = cancel

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, contentPath, nil)
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)

	if len(parseSSE(t, rec.Body.String())) != 0 {
		t.Fatalf("olay yazılmamalı: %q", rec.Body.String())
	}
	if len(h.content.saved) != 0 {
		t.Fatal("kayıt yapılmamalı")
	}

	// Üretim başarılı olsa bile istemci ayrıldıysa kimsenin görmediği sürüm kaydedilmez.
	h = newHarness(t)
	draftCampaign(h)
	ctx, cancel = context.WithCancel(context.Background())
	h.cgen.result = goodContent
	h.cgen.onDone = cancel
	h.handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodPost, contentPath, nil))
	if len(h.content.saved) != 0 {
		t.Fatal("istemci ayrıldıysa kayıt yapılmamalı")
	}
}

// Gerçek bir sunucuda ilk delta, üretim bitmeden istemciye ulaşmalıdır
// (tamponlama yok). Sıra kanallarla kurulur; zamanlamaya dayanmaz.
func TestDeltasReachClientBeforeGenerationFinishes(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)

	release := make(chan struct{})
	started := make(chan struct{})
	h.cgen.deltas = []content.Delta{{Field: "sms_text", Text: "ilk", Replace: true}}
	h.cgen.result = goodContent
	h.cgen.onDone = func() {
		close(started)
		<-release // istemci ilk deltayı okuyana kadar üretim bitmez
	}
	srv := httptest.NewServer(h.handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+contentPath, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	first := make(chan string, 1)
	go func() {
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data: ") {
				first <- sc.Text()
				return
			}
		}
		first <- ""
	}()

	select {
	case line := <-first:
		if line != `data: {"field":"sms_text","text":"ilk","replace":true}` {
			t.Fatalf("ilk veri satırı = %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ilk delta üretim bitmeden gelmeliydi (tamponlama?)")
	}
	<-started
	if len(h.content.saved) != 0 {
		t.Fatal("üretim bitmeden kayıt olmamalı")
	}
	close(release)

	// Kalanı oku: validated ile biter.
	var last string
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: ") {
			last = sc.Text()
		}
	}
	if last != "event: validated" {
		t.Fatalf("son olay = %q", last)
	}
}

func TestContentRoutesRejectOtherMethods(t *testing.T) {
	h := newHarness(t)
	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPatch} {
		rec := h.do(method, contentPath, "", "")
		wantError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		if rec.Header().Get("Allow") != "POST, PUT" {
			t.Fatalf("%s: Allow = %q", method, rec.Header().Get("Allow"))
		}
	}
}

func putContentBody(h *harness, body string) *httptest.ResponseRecorder {
	return h.do(http.MethodPut, contentPath, "application/json", body)
}

const validPut = `{"sms_text":"Merhaba {{first_name}}, %30 indirim!","email_subject":"Sepetin seni bekliyor","email_body":"Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada."}`

func TestPutContentSavesHumanVersion(t *testing.T) {
	h := newHarness(t)
	draftCampaign(h)
	h.content.saveVersion = 4
	h.content.maxName = 9

	rec := putContentBody(h, validPut)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	want := `{"content_version":4,"source":"human",` +
		`"sms_text":"Merhaba {{first_name}}, %30 indirim!","email_subject":"Sepetin seni bekliyor",` +
		`"email_body":"Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada.","created_at":"2026-11-18T10:00:00Z"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("gövde:\n%s\nbeklenen:\n%s", got, want)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("başlıklar: %v", rec.Header())
	}
	wantSaved := []savedContent{{campaignID, campaign.SourceHuman, goodContent}}
	if !reflect.DeepEqual(h.content.saved, wantSaved) {
		t.Fatalf("kayıt = %+v", h.content.saved)
	}
	if h.cgen.calls != 0 {
		t.Fatal("düzenleme LLM çağırmamalı")
	}
}

func TestPutContentRejectsInvalidContentWithReasons(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		reasons []string
	}{
		{"SMS çok uzun", `{"sms_text":"` + strings.Repeat("a", 161) + `","email_subject":"k","email_body":"g"}`, []string{"TOO_LONG:sms_text"}},
		{"konu çok uzun", `{"sms_text":"s","email_subject":"` + strings.Repeat("ş", 81) + `","email_body":"g"}`, []string{"TOO_LONG:email_subject"}},
		{"bilinmeyen değişken", `{"sms_text":"s","email_subject":"k","email_body":"Merhaba {{city}}"}`, []string{"UNKNOWN_VARIABLE:email_body"}},
		{"alan eksik", `{"sms_text":"s","email_subject":"k"}`, []string{"MISSING_FIELD:email_body"}},
		{"boş SMS", `{"sms_text":"  ","email_subject":"k","email_body":"g"}`, []string{"EMPTY_FIELD:sms_text"}},
		{"fazladan alan", `{"sms_text":"s","email_subject":"k","email_body":"g","extra":"x"}`, []string{"MALFORMED_OUTPUT:extra"}},
		{"string olmayan alan", `{"sms_text":5,"email_subject":"k","email_body":"g"}`, []string{"MALFORMED_OUTPUT:sms_text"}},
		{"konuda satır sonu", `{"sms_text":"s","email_subject":"a\nb","email_body":"g"}`, []string{"INVALID_CHARACTER:email_subject"}},
		{"JSON ama nesne değil", `["a"]`, []string{"MALFORMED_OUTPUT:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			rec := putContentBody(h, tt.body)

			e := wantError(t, rec, http.StatusUnprocessableEntity, "CONTENT_INVALID")
			var got []string
			for _, r := range e.Error.Reasons {
				got = append(got, r.Code+":"+r.Field)
			}
			if !reflect.DeepEqual(got, tt.reasons) {
				t.Fatalf("sebepler = %v, beklenen %v", got, tt.reasons)
			}
			if len(h.content.saved) != 0 {
				t.Fatal("geçersiz düzenleme kaydedilmemeli")
			}
		})
	}
}

func TestPutContentUsesLongestNameForLengthCheck(t *testing.T) {
	body := `{"sms_text":"` + strings.Repeat("a", 140) + `{{first_name}}","email_subject":"k","email_body":"g"}`

	h := newHarness(t)
	draftCampaign(h)
	h.content.maxName = 20 // 140 + 20 = 160
	if rec := putContentBody(h, body); rec.Code != http.StatusOK {
		t.Fatalf("160 kabul edilmeli: %d %s", rec.Code, rec.Body.String())
	}
	h = newHarness(t)
	draftCampaign(h)
	h.content.maxName = 21 // 161
	wantError(t, putContentBody(h, body), http.StatusUnprocessableEntity, "CONTENT_INVALID")
}

func TestPutContentRequestErrors(t *testing.T) {
	t.Run("JSON değil", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		wantError(t, h.do(http.MethodPut, contentPath, "text/plain", validPut), http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE")
	})
	for name, body := range map[string]string{
		"boş gövde":     ``,
		"bozuk JSON":    `{"sms_text":`,
		"fazladan JSON": validPut + ` {}`,
		"düz metin":     `merhaba`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			wantError(t, putContentBody(h, body), http.StatusBadRequest, "INVALID_REQUEST")
			if len(h.content.saved) != 0 {
				t.Fatal("kayıt olmamalı")
			}
		})
	}
	t.Run("çok büyük gövde", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		big := `{"sms_text":"` + strings.Repeat("a", maxBodyBytes) + `","email_subject":"k","email_body":"g"}`
		wantError(t, putContentBody(h, big), http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
	})
}

func TestPutContentCampaignStateErrors(t *testing.T) {
	t.Run("kampanya yok", func(t *testing.T) {
		h := newHarness(t)
		h.store.getErr = campaign.ErrNotFound
		wantError(t, putContentBody(h, validPut), http.StatusNotFound, "CAMPAIGN_NOT_FOUND")
	})
	for _, status := range []string{"approved", "sending", "completed"} {
		t.Run("draft değil: "+status, func(t *testing.T) {
			h := newHarness(t)
			draftCampaign(h)
			h.store.getResult.Status = status
			wantError(t, putContentBody(h, validPut), http.StatusConflict, "CAMPAIGN_NOT_DRAFT")
			if len(h.content.saved) != 0 {
				t.Fatal("kayıt olmamalı")
			}
		})
	}
	// Kontrolden sonra, kayıt kilidi altında onaylanmış çıkarsa da 409.
	t.Run("kayıt sırasında onaylandı", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		h.content.saveErr = campaign.ErrNotDraft
		wantError(t, putContentBody(h, validPut), http.StatusConflict, "CAMPAIGN_NOT_DRAFT")
	})
	t.Run("kayıt sırasında silindi", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		h.content.saveErr = campaign.ErrNotFound
		wantError(t, putContentBody(h, validPut), http.StatusNotFound, "CAMPAIGN_NOT_FOUND")
	})
	t.Run("kayıt hatası sızmaz", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		h.content.saveErr = errors.New("secret-host")
		rec := putContentBody(h, validPut)
		wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
		if strings.Contains(rec.Body.String(), "secret-host") {
			t.Fatal("sızıntı")
		}
	})
	t.Run("en uzun ad okunamadı", func(t *testing.T) {
		h := newHarness(t)
		draftCampaign(h)
		h.content.maxNameErr = errors.New("x")
		wantError(t, putContentBody(h, validPut), http.StatusInternalServerError, "INTERNAL")
	})
}

func TestGetCampaignIncludesLatestContentWhenPresent(t *testing.T) {
	h := newHarness(t)
	h.store.getResult = campaign.Campaign{
		ID: campaignID, Brief: "b", Status: campaign.StatusDraft, Segment: validSeg(t),
		CreatedAt: time.Date(2026, 11, 17, 9, 30, 0, 0, time.UTC),
	}
	h.content.latestFound = true
	h.content.latest = campaign.ContentVersion{Version: 2, Source: campaign.SourceHuman, Content: goodContent, CreatedAt: savedAt}

	rec := h.do(http.MethodGet, "/api/campaigns/"+campaignID, "", "")

	want := `"content":{"content_version":2,"source":"human",` +
		`"sms_text":"Merhaba {{first_name}}, %30 indirim!","email_subject":"Sepetin seni bekliyor",` +
		`"email_body":"Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada.","created_at":"2026-11-18T10:00:00Z"}}`
	if rec.Code != http.StatusOK || !strings.HasSuffix(strings.TrimSpace(rec.Body.String()), want) {
		t.Fatalf("%d\n%s\nson ek beklenen:\n%s", rec.Code, rec.Body.String(), want)
	}
}

func TestGetCampaignContentReadFailureDoesNotLeak(t *testing.T) {
	h := newHarness(t)
	h.store.getResult = campaign.Campaign{ID: campaignID, Brief: "b", Status: campaign.StatusDraft, Segment: validSeg(t)}
	h.content.latestErr = errors.New("secret-host")

	rec := h.do(http.MethodGet, "/api/campaigns/"+campaignID, "", "")

	wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
	if strings.Contains(rec.Body.String(), "secret-host") {
		t.Fatal("sızıntı")
	}
}
