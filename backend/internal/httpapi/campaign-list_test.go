package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// CMP-001: GET /api/campaigns (son kampanyalar). Veritabanı davranışı (sıralama, gerçek
// sınır) tests/integration/campaign-list_test.go'da; burada HTTP sözleşmesi sınanır.

type listBody struct {
	Campaigns []struct {
		ID        string `json:"id"`
		Brief     string `json:"brief"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
	} `json:"campaigns"`
}

func (h *harness) list(query string) (status int, body listBody, raw string) {
	rec := h.do(http.MethodGet, "/api/campaigns"+query, "", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body, rec.Body.String()
}

func summary(id, brief, status string, at time.Time) campaign.Summary {
	return campaign.Summary{ID: id, Brief: brief, Status: status, CreatedAt: at}
}

func TestListCampaignsReturnsRowsInStoreOrder(t *testing.T) {
	h := newHarness(t)
	t1 := time.Date(2026, 11, 19, 21, 0, 0, 0, time.FixedZone("TRT", 3*3600))
	t2 := time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)
	h.store.listResult = []campaign.Summary{
		summary("a", "Yeni brief", "completed", t1),
		summary("b", "Eski brief", "draft", t2),
	}

	status, body, _ := h.list("")

	if status != http.StatusOK {
		t.Fatalf("durum %d", status)
	}
	if len(body.Campaigns) != 2 || body.Campaigns[0].ID != "a" || body.Campaigns[1].ID != "b" {
		t.Fatalf("sıra korunmalı: %+v", body.Campaigns)
	}
	first := body.Campaigns[0]
	if first.Brief != "Yeni brief" || first.Status != "completed" {
		t.Fatalf("alanlar: %+v", first)
	}
	// Zaman UTC olarak yazılır (saat dilimi ne olursa olsun).
	if first.CreatedAt != "2026-11-19T18:00:00Z" {
		t.Fatalf("created_at UTC olmalı: %q", first.CreatedAt)
	}
}

func TestListCampaignsRowHasOnlyTheDocumentedFields(t *testing.T) {
	h := newHarness(t)
	h.store.listResult = []campaign.Summary{summary("a", "b", "draft", fixedNow)}

	_, _, raw := h.list("")

	var generic struct {
		Campaigns []map[string]any `json:"campaigns"`
	}
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for k := range generic.Campaigns[0] {
		got = append(got, k)
	}
	if len(got) != 4 {
		t.Fatalf("satırda yalnızca id, brief, status, created_at olmalı (kitle sayısı, segment, içerik yok): %v", got)
	}
	for _, forbidden := range []string{"audience", "segment", "content", "approved_at"} {
		if strings.Contains(raw, `"`+forbidden+`"`) {
			t.Fatalf("%q listede olmamalı: %s", forbidden, raw)
		}
	}
}

func TestListCampaignsEmptyIsAnEmptyArrayNotNull(t *testing.T) {
	h := newHarness(t)
	h.store.listResult = nil // depo nil döndürse de

	status, _, raw := h.list("")

	if status != http.StatusOK {
		t.Fatalf("durum %d", status)
	}
	if strings.TrimSpace(raw) != `{"campaigns":[]}` {
		t.Fatalf("boş liste [] olmalı: %s", raw)
	}
}

func TestListCampaignsLimit(t *testing.T) {
	for _, tt := range []struct {
		name  string
		query string
		want  int
	}{
		{"varsayılan 10", "", campaign.DefaultListLimit},
		{"boş değer varsayılana düşer", "?limit=", campaign.DefaultListLimit},
		{"açıkça 5", "?limit=5", 5},
		{"alt sınır 1", "?limit=1", 1},
		{"üst sınır 20", "?limit=20", 20},
		{"20'den büyük 20'ye indirilir", "?limit=21", 20},
		{"çok büyük de 20'ye indirilir", "?limit=1000", 20},
		{"başka parametreler yok sayılır", "?foo=bar&limit=3", 3},
		{"tekrarlanan parametrede ilki", "?limit=4&limit=9", 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			status, _, raw := h.list(tt.query)
			if status != http.StatusOK {
				t.Fatalf("durum %d: %s", status, raw)
			}
			if h.store.gotLimit != tt.want {
				t.Fatalf("depoya giden sınır %d, beklenen %d", h.store.gotLimit, tt.want)
			}
		})
	}
}

func TestListCampaignsRejectsInvalidLimitWithoutQueryingTheStore(t *testing.T) {
	for _, raw := range []string{"0", "-1", "abc", "1.5", "10x", " 5", "99999999999999999999"} {
		t.Run(raw, func(t *testing.T) {
			h := newHarness(t)

			rec := h.do(http.MethodGet, "/api/campaigns?limit="+url.QueryEscape(raw), "", "")

			wantError(t, rec, http.StatusBadRequest, "INVALID_REQUEST")
			if h.store.listCalls != 0 {
				t.Fatalf("geçersiz sınırda depo çağrılmamalı (%d kez)", h.store.listCalls)
			}
			if !strings.Contains(decodeError(t, rec).Error.Message, strconv.Itoa(campaign.MaxListLimit)) {
				t.Fatalf("mesaj üst sınırı söylemeli: %s", rec.Body.String())
			}
		})
	}
}

func TestListCampaignsStoreFailureIsInternalAndDoesNotLeak(t *testing.T) {
	h := newHarness(t)
	h.store.listErr = errors.New("connection to secret-host refused")

	rec := h.do(http.MethodGet, "/api/campaigns", "", "")

	wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
	if strings.Contains(rec.Body.String(), "secret-host") {
		t.Fatal("iç hata ayrıntısı sızmamalı")
	}
	if !strings.Contains(h.logs.String(), "secret-host") {
		t.Fatal("ayrıntı loga yazılmalı")
	}
}

func TestListCampaignsDoesNotTouchOtherDependencies(t *testing.T) {
	h := newHarness(t)
	h.store.listResult = []campaign.Summary{summary("a", "b", "draft", fixedNow)}

	h.list("")

	if len(h.gen.briefs) != 0 || len(h.store.created) != 0 || h.counter.calls != 0 {
		t.Fatal("liste LLM'i, kitle sayımını veya kayıt oluşturmayı çağırmamalı")
	}
}
