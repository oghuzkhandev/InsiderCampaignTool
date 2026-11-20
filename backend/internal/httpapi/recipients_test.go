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

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// GET /api/campaigns/{id}/recipients: HTTP sözleşmesi. Gerçek sorgu davranışı
// tests/integration/recipients_test.go'dadır.

type fakeRecipients struct {
	page  campaign.RecipientPage
	err   error
	calls int

	gotID             string
	gotFilter         campaign.RecipientFilter
	gotPage, gotLimit int
}

func (f *fakeRecipients) ListRecipients(_ context.Context, id string, flt campaign.RecipientFilter, page, limit int) (campaign.RecipientPage, error) {
	f.calls++
	f.gotID, f.gotFilter, f.gotPage, f.gotLimit = id, flt, page, limit
	return f.page, f.err
}

func recipientsRequest(reader RecipientReader, method, target string) (*httptest.ResponseRecorder, string) {
	var logs strings.Builder
	h := New(Deps{Recipients: reader, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec, logs.String()
}

func TestRecipientsReturnsItemsWithNullsForMissingValues(t *testing.T) {
	sent := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	name := "Ayşe"
	reason := "rejected"
	reader := &fakeRecipients{page: campaign.RecipientPage{Total: 2, Items: []campaign.Recipient{
		{MessageID: 7, UserID: 3, FirstName: &name, Channel: "sms", Address: "+905551112233", Status: "sent", Attempts: 2, SentAt: &sent},
		{MessageID: 8, UserID: 4, Channel: "email", Address: "a@b.co", Status: "failed", Attempts: 1, FailureReason: &reason},
	}}}

	rec, _ := recipientsRequest(reader, http.MethodGet, "/api/campaigns/abc/recipients?page=2&limit=5&channel=sms&status=sent&q=ay%C5%9Fe")

	if rec.Code != http.StatusOK {
		t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
	}
	if reader.gotID != "abc" || reader.gotPage != 2 || reader.gotLimit != 5 ||
		reader.gotFilter != (campaign.RecipientFilter{Channel: "sms", Status: "sent", Search: "ayşe"}) {
		t.Fatalf("okuyucuya giden parametreler: %q %d %d %+v", reader.gotID, reader.gotPage, reader.gotLimit, reader.gotFilter)
	}
	var body struct {
		Page, Limit, Total int
		Items              []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Page != 2 || body.Limit != 5 || body.Total != 2 || len(body.Items) != 2 {
		t.Fatalf("üst bilgi: %+v", body)
	}
	first, second := body.Items[0], body.Items[1]
	if first["first_name"] != "Ayşe" || first["address"] != "+905551112233" || first["sent_at"] != "2026-11-21T09:00:00Z" || first["failure_reason"] != nil {
		t.Fatalf("ilk alıcı: %+v", first)
	}
	if second["first_name"] != nil || second["sent_at"] != nil || second["failure_reason"] != "rejected" {
		t.Fatalf("eksik değerler null olmalı: %+v", second)
	}
}

func TestRecipientsUsesDefaultsAndCapsLimit(t *testing.T) {
	reader := &fakeRecipients{}

	recipientsRequest(reader, http.MethodGet, "/api/campaigns/abc/recipients")
	if reader.gotPage != 1 || reader.gotLimit != 10 {
		t.Fatalf("varsayılanlar 1 ve 10 olmalı: %d %d", reader.gotPage, reader.gotLimit)
	}
	recipientsRequest(reader, http.MethodGet, "/api/campaigns/abc/recipients?limit=500")
	if reader.gotLimit != 20 {
		t.Fatalf("limit en çok 20 olmalı: %d", reader.gotLimit)
	}
}

func TestRecipientsEmptyPageIsAnEmptyArrayNotNull(t *testing.T) {
	rec, _ := recipientsRequest(&fakeRecipients{page: campaign.RecipientPage{Total: 0}}, http.MethodGet, "/api/campaigns/abc/recipients")

	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("items boş dizi olmalı: %s", rec.Body.String())
	}
}

func TestRecipientsRejectsInvalidParametersWithoutReading(t *testing.T) {
	long := strings.Repeat("a", 101)
	for _, target := range []string{
		"/api/campaigns/abc/recipients?page=0",
		"/api/campaigns/abc/recipients?page=x",
		"/api/campaigns/abc/recipients?limit=0",
		"/api/campaigns/abc/recipients?channel=push",
		"/api/campaigns/abc/recipients?status=done",
		"/api/campaigns/abc/recipients?q=" + long,
	} {
		reader := &fakeRecipients{}
		rec, _ := recipientsRequest(reader, http.MethodGet, target)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
			t.Errorf("%s: 400 INVALID_REQUEST bekleniyordu: %d %s", target, rec.Code, rec.Body.String())
		}
		if reader.calls != 0 {
			t.Errorf("%s: geçersiz istekte veritabanı okunmamalı", target)
		}
	}
}

func TestRecipientsUnknownCampaignIs404(t *testing.T) {
	rec, _ := recipientsRequest(&fakeRecipients{err: campaign.ErrNotFound}, http.MethodGet, "/api/campaigns/abc/recipients")

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "CAMPAIGN_NOT_FOUND") {
		t.Fatalf("404 CAMPAIGN_NOT_FOUND bekleniyordu: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRecipientsReadFailureIs500WithoutLeakingTheCause(t *testing.T) {
	rec, logs := recipientsRequest(&fakeRecipients{err: errors.New("pq: gizli ayrıntı")}, http.MethodGet, "/api/campaigns/abc/recipients")

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "gizli") {
		t.Fatalf("500 ve ayrıntısız gövde bekleniyordu: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs, "gizli ayrıntı") {
		t.Fatalf("neden günlüğe yazılmalı: %s", logs)
	}
}

func TestRecipientsOnlyAllowsGet(t *testing.T) {
	rec, _ := recipientsRequest(&fakeRecipients{}, http.MethodPost, "/api/campaigns/abc/recipients")

	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("405 ve Allow: GET bekleniyordu: %d %q", rec.Code, rec.Header().Get("Allow"))
	}
}
