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

// GET /api/scheduled: gönderimi ileri bir zamana planlanmış, başlamamış kampanyalar. Gerçek
// veritabanı davranışı tests/integration/scheduled-list_test.go'dadır; burada HTTP sözleşmesi sınanır.

type fakeScheduled struct {
	page  campaign.ScheduledPage
	err   error
	calls int

	gotPage, gotLimit int
}

func (f *fakeScheduled) ListScheduled(_ context.Context, page, limit int) (campaign.ScheduledPage, error) {
	f.calls++
	f.gotPage, f.gotLimit = page, limit
	return f.page, f.err
}

func scheduledRequest(reader ScheduledReader, method, target string) (*httptest.ResponseRecorder, string) {
	var logs strings.Builder
	h := New(Deps{Scheduled: reader, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec, logs.String()
}

var scheduledNow = time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)

func TestScheduledReturnsItemsInReaderOrderWithServerTime(t *testing.T) {
	soon := scheduledNow.Add(90 * time.Minute)
	reader := &fakeScheduled{page: campaign.ScheduledPage{
		Total: 2, ServerTime: scheduledNow.In(time.FixedZone("TRT", 3*3600)),
		Items: []campaign.ScheduledItem{
			{ID: "a", Brief: "Yakın", ScheduledAt: soon, ApprovedAt: scheduledNow.Add(-time.Hour), Queued: 191},
			{ID: "b", Brief: "Zamanı geldi", ScheduledAt: scheduledNow.Add(-time.Minute), ApprovedAt: scheduledNow.Add(-time.Hour), Queued: 5, Due: true},
		},
	}}

	rec, _ := scheduledRequest(reader, http.MethodGet, "/api/scheduled")

	if rec.Code != http.StatusOK {
		t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			ID          string `json:"id"`
			Brief       string `json:"brief"`
			ScheduledAt string `json:"scheduled_at"`
			ApprovedAt  string `json:"approved_at"`
			Queued      int    `json:"queued"`
			Due         bool   `json:"due"`
		} `json:"items"`
		Total      int    `json:"total"`
		Page       int    `json:"page"`
		Limit      int    `json:"limit"`
		ServerTime string `json:"server_time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 || body.Items[0].ID != "a" || body.Items[1].ID != "b" {
		t.Fatalf("sıra okuyucunun sırası olmalı: %+v", body.Items)
	}
	if body.Items[0].Queued != 191 || body.Items[0].Due || !body.Items[1].Due {
		t.Fatalf("kuyruk sayısı ve due: %+v", body.Items)
	}
	if body.Total != 2 || body.Page != 1 || body.Limit != 10 {
		t.Fatalf("sayfa bilgisi: %+v", body)
	}
	// Zamanlar UTC yazılır (saat dilimi ne olursa olsun); sunucu saati de.
	if body.ServerTime != "2026-11-21T09:00:00Z" || body.Items[0].ScheduledAt != "2026-11-21T10:30:00Z" {
		t.Fatalf("zamanlar UTC olmalı: %s / %s", body.ServerTime, body.Items[0].ScheduledAt)
	}
}

func TestScheduledDefaultsAndLimitCap(t *testing.T) {
	reader := &fakeScheduled{}

	if rec, _ := scheduledRequest(reader, http.MethodGet, "/api/scheduled"); rec.Code != http.StatusOK ||
		reader.gotPage != 1 || reader.gotLimit != 10 {
		t.Fatalf("varsayılan sayfa 1, limit 10: durum %d, %d/%d", rec.Code, reader.gotPage, reader.gotLimit)
	}
	if rec, _ := scheduledRequest(reader, http.MethodGet, "/api/scheduled?page=3&limit=500"); rec.Code != http.StatusOK ||
		reader.gotPage != 3 || reader.gotLimit != 20 {
		t.Fatalf("limit 20'ye indirilmeli: durum %d, %d/%d", rec.Code, reader.gotPage, reader.gotLimit)
	}
}

func TestScheduledEmptyListIsAnEmptyArrayNotNull(t *testing.T) {
	reader := &fakeScheduled{page: campaign.ScheduledPage{ServerTime: scheduledNow}}

	rec, _ := scheduledRequest(reader, http.MethodGet, "/api/scheduled")

	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("boş liste [] olmalı: %s", rec.Body.String())
	}
}

func TestScheduledRejectsInvalidPagingWithoutTouchingTheReader(t *testing.T) {
	for _, query := range []string{"?page=0", "?page=x", "?page=100001", "?limit=0", "?limit=-1", "?limit=abc"} {
		reader := &fakeScheduled{}

		rec, _ := scheduledRequest(reader, http.MethodGet, "/api/scheduled"+query)

		if rec.Code != http.StatusBadRequest || reader.calls != 0 {
			t.Errorf("%s: durum %d, okuma %d", query, rec.Code, reader.calls)
		}
	}
}

func TestScheduledDoesNotLeakInternalErrors(t *testing.T) {
	reader := &fakeScheduled{err: errors.New("password=hunter2")}

	rec, logs := scheduledRequest(reader, http.MethodGet, "/api/scheduled")

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("durum %d, gövde %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs, "hunter2") {
		t.Fatal("ayrıntı loga yazılmalı")
	}
}

func TestScheduledUnavailableWithoutReaderAndOtherMethods(t *testing.T) {
	if rec, _ := scheduledRequest(nil, http.MethodGet, "/api/scheduled"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("bağlı değilken 503 bekleniyordu: %d", rec.Code)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec, _ := scheduledRequest(&fakeScheduled{}, method, "/api/scheduled")
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
			t.Errorf("%s: durum %d, Allow %q", method, rec.Code, rec.Header().Get("Allow"))
		}
	}
}
