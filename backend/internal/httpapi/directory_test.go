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

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// Kişi dizini süzgeçleri: GET /api/audience (city, sms, email, search) ve
// GET /api/audience/cities. Gerçek veritabanı davranışı
// tests/integration/audience-directory_test.go'dadır; burada HTTP sözleşmesi sınanır.

type fakeDirectory struct {
	page      audience.MemberPage
	cities    []string
	err       error
	citiesErr error
	calls     int

	gotFilter segment.DirectoryFilter
	gotPage   int
	gotLimit  int
}

func (f *fakeDirectory) Directory(_ context.Context, filter segment.DirectoryFilter, page, limit int) (audience.MemberPage, error) {
	f.calls++
	f.gotFilter, f.gotPage, f.gotLimit = filter, page, limit
	return f.page, f.err
}

func (f *fakeDirectory) Cities(context.Context) ([]string, error) { return f.cities, f.citiesErr }

func directoryRequest(dir DirectoryReader, method, target string) (*httptest.ResponseRecorder, string) {
	var logs strings.Builder
	h := New(Deps{Directory: dir, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec, logs.String()
}

func TestDirectoryPassesFiltersPageAndLimitToTheReader(t *testing.T) {
	dir := &fakeDirectory{page: audience.MemberPage{Total: 3, Members: []audience.Member{}}}

	rec, _ := directoryRequest(dir, http.MethodGet,
		"/api/audience?page=3&limit=5&search=%20Ayşe%20&city=%C4%B0zmir&sms=eligible&email=no_consent")

	if rec.Code != http.StatusOK {
		t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
	}
	want := segment.DirectoryFilter{Search: "Ayşe", City: "İzmir", SMS: "eligible", Email: "no_consent"}
	if dir.gotFilter != want {
		t.Fatalf("süzgeç %+v, beklenen %+v", dir.gotFilter, want)
	}
	if dir.gotPage != 3 || dir.gotLimit != 5 {
		t.Fatalf("sayfa %d, limit %d", dir.gotPage, dir.gotLimit)
	}
	var body memberListBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Page != 3 || body.Total != 3 {
		t.Fatalf("gövde: %v %s", err, rec.Body.String())
	}
}

func TestDirectoryFiltersAreOptional(t *testing.T) {
	dir := &fakeDirectory{page: audience.MemberPage{Members: []audience.Member{}}}

	rec, _ := directoryRequest(dir, http.MethodGet, "/api/audience")

	if rec.Code != http.StatusOK || dir.gotFilter != (segment.DirectoryFilter{}) {
		t.Fatalf("durum %d, süzgeç %+v", rec.Code, dir.gotFilter)
	}
}

func TestDirectoryRejectsInvalidFiltersWithoutTouchingTheReader(t *testing.T) {
	long := strings.Repeat("a", 65)
	for _, query := range []string{
		"?sms=maybe", "?email=ELIGIBLE", "?sms=eligible,no_consent", "?email=%20eligible",
		"?city=" + long, "?city=a%00b", "?search=" + strings.Repeat("a", 101),
	} {
		t.Run(query, func(t *testing.T) {
			dir := &fakeDirectory{}

			rec, _ := directoryRequest(dir, http.MethodGet, "/api/audience"+query)

			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
			}
			if dir.calls != 0 {
				t.Fatal("geçersiz süzgeçte veritabanı okunmamalı")
			}
		})
	}
}

func TestDirectoryDoesNotLeakInternalErrors(t *testing.T) {
	const secret = "password=hunter2"
	dir := &fakeDirectory{err: errors.New(secret)}

	rec, logs := directoryRequest(dir, http.MethodGet, "/api/audience?sms=eligible")

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("durum %d, gövde %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs, "hunter2") {
		t.Fatal("ayrıntı loga yazılmalı")
	}
}

func TestDirectoryCitiesReturnsTheOptions(t *testing.T) {
	dir := &fakeDirectory{cities: []string{"Adana", "Ankara", "İzmir"}}

	rec, _ := directoryRequest(dir, http.MethodGet, "/api/audience/cities")

	var body struct {
		Cities []string `json:"cities"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil ||
		strings.Join(body.Cities, ",") != "Adana,Ankara,İzmir" {
		t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDirectoryCitiesEmptyIsAnEmptyListNotNull(t *testing.T) {
	dir := &fakeDirectory{cities: []string{}}

	rec, _ := directoryRequest(dir, http.MethodGet, "/api/audience/cities")

	if !strings.Contains(rec.Body.String(), `"cities":[]`) {
		t.Fatalf("boş liste [] olmalı: %s", rec.Body.String())
	}
}

func TestDirectoryCitiesErrorsAndMethods(t *testing.T) {
	const secret = "host=db.internal"
	rec, logs := directoryRequest(&fakeDirectory{citiesErr: errors.New(secret)}, http.MethodGet, "/api/audience/cities")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db.internal") || !strings.Contains(logs, "db.internal") {
		t.Fatalf("durum %d, gövde %s", rec.Code, rec.Body.String())
	}

	rec, _ = directoryRequest(&fakeDirectory{}, http.MethodPost, "/api/audience/cities")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST: durum %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}

	rec, _ = directoryRequest(nil, http.MethodGet, "/api/audience/cities")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("bağlı değilken 503 bekleniyordu: %d", rec.Code)
	}
}
