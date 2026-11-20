package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

type approveCall struct {
	id      string
	version int
	now     time.Time
}

type fakeApprover struct {
	result    campaign.ApprovalResult
	err       error
	calls     []approveCall
	scheduled *time.Time
}

func (f *fakeApprover) ApproveScheduled(ctx context.Context, id string, version int, now time.Time, scheduled *time.Time) (campaign.ApprovalResult, error) {
	f.scheduled = scheduled
	return f.Approve(ctx, id, version, now)
}

func (f *fakeApprover) Approve(_ context.Context, id string, version int, now time.Time) (campaign.ApprovalResult, error) {
	f.calls = append(f.calls, approveCall{id, version, now})
	return f.result, f.err
}

var approvedAt = time.Date(2026, 11, 18, 11, 0, 0, 0, time.UTC)

func TestApprovePassesScheduleAndReturnsIt(t *testing.T) {
	h := newHarness(t)
	at := time.Date(2030, 6, 15, 11, 30, 0, 0, time.UTC)
	h.approve.result = okResult()
	h.approve.result.ScheduledAt = &at
	r := postApprove(h, `{"content_version":1,"scheduled_at":"2030-06-15T14:30:00+03:00"}`)
	if r.Code != 200 || h.approve.scheduled == nil || !h.approve.scheduled.Equal(at) {
		t.Fatalf("schedule not passed: %d %s", r.Code, r.Body)
	}
	if !strings.Contains(r.Body.String(), `"scheduled_at":"2030-06-15T11:30:00Z"`) {
		t.Fatalf("schedule missing: %s", r.Body)
	}
}

func approvePath() string { return "/api/campaigns/" + campaignID + "/approve" }

func postApprove(h *harness, body string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, approvePath(), "application/json", body)
}

func okResult() campaign.ApprovalResult {
	return campaign.ApprovalResult{
		CampaignID: campaignID, Status: "approved", ContentVersion: 3, ApprovedAt: approvedAt,
		Queued: campaign.ChannelCounts{SMS: 141, Email: 151},
	}
}

func TestApproveSuccessResponseAndArguments(t *testing.T) {
	h := newHarness(t)
	h.approve.result = okResult()

	rec := postApprove(h, `{"content_version":3}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "without_address") {
		t.Fatalf("adressiz alıcı alanı artık yok (ADR-023 B): %s", rec.Body.String())
	}
	want := `{"id":"` + campaignID + `","status":"approved","approved_content_version":3,` +
		`"approved_at":"2026-11-18T11:00:00Z","queued":{"sms":141,"email":151,"total":292}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("gövde:\n%s\nbeklenen:\n%s", got, want)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("başlıklar: %v", rec.Header())
	}
	wantCalls := []approveCall{{campaignID, 3, fixedNow}}
	if !reflect.DeepEqual(h.approve.calls, wantCalls) {
		t.Fatalf("çağrı = %+v, beklenen %+v", h.approve.calls, wantCalls)
	}
}

// Onaylanan aynı sürümle tekrar da 200'dür ve mevcut sonucu döndürür.
func TestApproveReplayIsAlso200(t *testing.T) {
	h := newHarness(t)
	res := okResult()
	res.Status, res.Replayed = "sending", true
	h.approve.result = res

	rec := postApprove(h, `{"content_version":3}`)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"sending"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestApproveErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"kampanya yok", campaign.ErrNotFound, http.StatusNotFound, "CAMPAIGN_NOT_FOUND"},
		{"doğrulanmış içerik yok", campaign.ErrNoContent, http.StatusConflict, "NO_VALIDATED_CONTENT"},
		{"eski sürüm", campaign.ErrStaleVersion, http.StatusConflict, "STALE_CONTENT_VERSION"},
		{"başka sürümle onaylı", campaign.ErrApprovedWithOtherVersion, http.StatusConflict, "CAMPAIGN_ALREADY_APPROVED"},
		{"filtresiz segment", campaign.ErrUnfilteredSegment, http.StatusUnprocessableEntity, "UNFILTERED_SEGMENT"},
		{"uygun alıcı yok", campaign.ErrNoRecipients, http.StatusUnprocessableEntity, "NO_ELIGIBLE_RECIPIENTS"},
		{"sarılmış hata da eşlenir", errors.Join(errors.New("ctx"), campaign.ErrStaleVersion), http.StatusConflict, "STALE_CONTENT_VERSION"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.approve.err = tt.err
			wantError(t, postApprove(h, `{"content_version":2}`), tt.status, tt.code)
		})
	}
}

func TestApproveRenderLimitErrorListsAffectedRecipients(t *testing.T) {
	h := newHarness(t)
	h.approve.err = &campaign.RenderLimitError{Overflows: []campaign.FieldOverflow{
		{Field: "sms_text", Count: 12, Example: "Zeynepşşşşş"},
		{Field: "email_subject", Count: 1, Example: "Değerli müşterimiz"},
	}}

	e := wantError(t, postApprove(h, `{"content_version":2}`), http.StatusUnprocessableEntity, "RENDER_LIMIT_EXCEEDED")

	want := []reason{
		{Code: "TOO_LONG", Field: "sms_text", Count: 12, Text: "Zeynepşşşşş"},
		{Code: "TOO_LONG", Field: "email_subject", Count: 1, Text: "Değerli müşterimiz"},
	}
	if !reflect.DeepEqual(e.Error.Reasons, want) {
		t.Fatalf("sebepler = %+v", e.Error.Reasons)
	}
}

// Örnek ad kullanıcı verisidir: kontrol karakterleri temizlenir ve kırpılır.
func TestApproveRenderLimitExampleNameIsSanitized(t *testing.T) {
	h := newHarness(t)
	h.approve.err = &campaign.RenderLimitError{Overflows: []campaign.FieldOverflow{
		{Field: "sms_text", Count: 1, Example: "Ali\r\nSet-Cookie: x" + strings.Repeat("ü", 200)},
	}}

	e := wantError(t, postApprove(h, `{"content_version":2}`), http.StatusUnprocessableEntity, "RENDER_LIMIT_EXCEEDED")

	text := e.Error.Reasons[0].Text
	if strings.ContainsAny(text, "\r\n") || len([]rune(text)) > 64 {
		t.Fatalf("örnek ad temizlenmeli ve 64 karaktere kırpılmalı: %q (%d)", text, len([]rune(text)))
	}
}

func TestApproveInternalErrorDoesNotLeak(t *testing.T) {
	h := newHarness(t)
	h.approve.err = errors.New("bağlantı koptu: secret-host")

	rec := postApprove(h, `{"content_version":2}`)

	wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
	if strings.Contains(rec.Body.String(), "secret-host") || !strings.Contains(h.logs.String(), "secret-host") {
		t.Fatal("ayrıntı yalnızca loga yazılmalı")
	}
}

// Geçersiz istekler onaylayıcıya hiç ulaşmaz.
func TestApproveRejectsBadRequestsWithoutCallingApprover(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
		code        string
	}{
		{"JSON değil", "text/plain", `{"content_version":1}`, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
		{"boş gövde", "application/json", ``, http.StatusBadRequest, "INVALID_REQUEST"},
		{"bozuk JSON", "application/json", `{"content_version":`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"fazladan alan", "application/json", `{"content_version":1,"x":2}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"fazladan JSON", "application/json", `{"content_version":1} {}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sürüm yok", "application/json", `{}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sürüm null", "application/json", `{"content_version":null}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sürüm sıfır", "application/json", `{"content_version":0}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"negatif sürüm", "application/json", `{"content_version":-1}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"ondalık sürüm", "application/json", `{"content_version":1.5}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"metin sürüm", "application/json", `{"content_version":"1"}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"çok büyük gövde", "application/json", `{"content_version":1,"pad":"` + strings.Repeat("a", maxBodyBytes) + `"}`, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			wantError(t, h.do(http.MethodPost, approvePath(), tt.contentType, tt.body), tt.status, tt.code)
			if len(h.approve.calls) != 0 {
				t.Fatalf("onaylayıcı çağrılmamalı: %+v", h.approve.calls)
			}
		})
	}
}

func TestApproveRouteRejectsOtherMethods(t *testing.T) {
	h := newHarness(t)
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := h.do(m, approvePath(), "", "")
		wantError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		if rec.Header().Get("Allow") != "POST" {
			t.Fatalf("%s: Allow = %q", m, rec.Header().Get("Allow"))
		}
	}
	if len(h.approve.calls) != 0 {
		t.Fatal("onaylayıcı çağrılmamalı")
	}
}

// GET, onaylanan kampanyada onay bilgisini de döndürür.
func TestGetCampaignIncludesApprovalInfoWhenApproved(t *testing.T) {
	h := newHarness(t)
	v := 2
	at := approvedAt
	h.store.getResult = campaign.Campaign{
		ID: campaignID, Brief: "b", Status: "approved", Segment: validSeg(t),
		ApprovedContentVersion: &v, ApprovedAt: &at,
		CreatedAt: time.Date(2026, 11, 17, 9, 30, 0, 0, time.UTC),
	}
	rec := h.do(http.MethodGet, "/api/campaigns/"+campaignID, "", "")

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"approved_content_version":2,"approved_at":"2026-11-18T11:00:00Z"`) {
		t.Fatalf("%d\n%s", rec.Code, rec.Body.String())
	}
}
