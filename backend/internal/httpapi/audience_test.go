package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// ADR-031: GET /api/campaigns/{id}/audience (segmente uyan kişiler, sayfalı). Gerçek
// veritabanı davranışı tests/integration/audience-members_test.go'da; burada HTTP sözleşmesi
// sınanır.

const memberCampaignID = "11111111-1111-1111-1111-111111111111"

type fakeMembers struct {
	page  audience.MemberPage
	err   error
	calls int

	gotSeg   segment.Segment
	gotNow   time.Time
	gotPage  int
	gotLimit int
}

func (f *fakeMembers) ListMembers(_ context.Context, seg segment.Segment, now time.Time, page, limit int) (audience.MemberPage, error) {
	f.calls++
	f.gotSeg, f.gotNow, f.gotPage, f.gotLimit = seg, now, page, limit
	return f.page, f.err
}

type memberListBody struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Items []struct {
		ID          int64   `json:"id"`
		FirstName   *string `json:"first_name"`
		City        string  `json:"city"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		SMSStatus   string  `json:"sms_status"`
		EmailStatus string  `json:"email_status"`
	} `json:"items"`
}

// getMembers, kayıtlı kampanyayı hazırlayıp listeyi ister.
func (h *harness) getMembers(query string) (*memberListBody, string, int) {
	h.store.getResult = campaign.Campaign{ID: memberCampaignID, Segment: h.gen.seg}
	rec := h.do(http.MethodGet, "/api/campaigns/"+memberCampaignID+"/audience"+query, "", "")
	var body memberListBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return &body, rec.Body.String(), rec.Code
}

func str(s string) *string { return &s }

func TestAudienceMembersReturnsThePageInStoreOrderWithExplicitNulls(t *testing.T) {
	h := newHarness(t)
	h.members.page = audience.MemberPage{
		Total: 23,
		Members: []audience.Member{
			{ID: 5, FirstName: str("Ayşe"), City: "İstanbul", Email: str("ayse@example.test"), Phone: str("+905550000001"),
				SMSStatus: audience.ChannelEligible, EmailStatus: audience.ChannelEligible},
			// Adı, telefonu ve izni olmayan kişi: eksik bilgi null, sebep durumda.
			{ID: 9, City: "İstanbul", Email: str("x@example.test"),
				SMSStatus: audience.ChannelNoConsent, EmailStatus: audience.ChannelNoAddress},
		},
	}

	body, raw, status := h.getMembers("?page=2&limit=2")

	if status != http.StatusOK {
		t.Fatalf("durum %d\n%s", status, raw)
	}
	if body.Page != 2 || body.Limit != 2 || body.Total != 23 || len(body.Items) != 2 {
		t.Fatalf("sayfa bilgisi: %+v", body)
	}
	first, second := body.Items[0], body.Items[1]
	if first.ID != 5 || second.ID != 9 {
		t.Fatalf("sıra deponun sırası olmalı: %d, %d", first.ID, second.ID)
	}
	if first.FirstName == nil || *first.FirstName != "Ayşe" || first.SMSStatus != "eligible" || first.EmailStatus != "eligible" {
		t.Fatalf("ilk kişi: %+v", first)
	}
	if second.FirstName != nil || second.Phone != nil {
		t.Fatalf("eksik ad ve telefon null olmalı: %+v", second)
	}
	if second.SMSStatus != "no_consent" || second.EmailStatus != "no_address" {
		t.Fatalf("durum sebepleri: %+v", second)
	}
	// Eksik alanlar JSON'da açıkça null'dır (alanın hiç olmaması veya boş metin değil).
	for _, want := range []string{`"first_name":null`, `"phone":null`} {
		if !strings.Contains(raw, want) {
			t.Errorf("yanıt %s içermeli: %s", want, raw)
		}
	}
	if strings.Contains(raw, `"first_name":""`) || strings.Contains(raw, `"phone":""`) {
		t.Errorf("boş metin yollanmamalı: %s", raw)
	}
}

func TestAudienceMembersItemHasOnlyTheDocumentedFields(t *testing.T) {
	h := newHarness(t)
	h.members.page = audience.MemberPage{Total: 1, Members: []audience.Member{{ID: 1, City: "Ankara", SMSStatus: audience.ChannelEligible, EmailStatus: audience.ChannelEligible}}}

	_, raw, _ := h.getMembers("")

	var generic struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &generic); err != nil || len(generic.Items) != 1 {
		t.Fatalf("yanıt çözülemedi: %v\n%s", err, raw)
	}
	got := make([]string, 0, len(generic.Items[0]))
	for k := range generic.Items[0] {
		got = append(got, k)
	}
	want := map[string]bool{"id": true, "first_name": true, "city": true, "email": true, "phone": true, "sms_status": true, "email_status": true}
	if len(got) != len(want) {
		t.Fatalf("alanlar %v, beklenen %v", got, want)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("belgelenmemiş alan: %s", k)
		}
	}
}

func TestAudienceMembersDefaultsAndUsesTheStoredSegmentAndReferenceTime(t *testing.T) {
	h := newHarness(t)

	body, _, status := h.getMembers("")

	if status != http.StatusOK {
		t.Fatalf("durum %d", status)
	}
	if h.members.gotPage != 1 || h.members.gotLimit != 10 {
		t.Fatalf("varsayılan sayfa 1 ve limit 10 olmalı: %d, %d", h.members.gotPage, h.members.gotLimit)
	}
	if body.Page != 1 || body.Limit != 10 {
		t.Fatalf("yanıt kullanılan değerleri yansıtmalı: %+v", body)
	}
	// Kampanyanın kayıtlı, doğrulanmış segmenti kullanılır; referans zaman istek anıdır.
	if len(h.members.gotSeg.Conditions) == 0 || len(h.members.gotSeg.Conditions) != len(h.gen.seg.Conditions) {
		t.Fatalf("kayıtlı segment kullanılmalı: %+v", h.members.gotSeg)
	}
	if !h.members.gotNow.Equal(fixedNow) {
		t.Fatalf("referans zaman Deps.Now olmalı: %v", h.members.gotNow)
	}
	if h.store.gotID != memberCampaignID {
		t.Fatalf("kampanya kimliğiyle okunmalı: %q", h.store.gotID)
	}
}

// Boş değer verilmemiş sayılır (varsayılan), geçersiz sayılmaz.
func TestAudienceMembersTreatsEmptyParametersAsDefaults(t *testing.T) {
	h := newHarness(t)

	_, _, status := h.getMembers("?page=&limit=")

	if status != http.StatusOK || h.members.gotPage != 1 || h.members.gotLimit != 10 {
		t.Fatalf("durum %d, sayfa %d, limit %d", status, h.members.gotPage, h.members.gotLimit)
	}
}

func TestAudienceMembersLimitAboveTheMaximumIsLoweredNotRejected(t *testing.T) {
	h := newHarness(t)

	body, _, status := h.getMembers("?limit=500")

	if status != http.StatusOK || h.members.gotLimit != audience.MaxMemberLimit || body.Limit != audience.MaxMemberLimit {
		t.Fatalf("limit %d'e indirilmeli: durum %d, depoya %d, yanıt %d", audience.MaxMemberLimit, status, h.members.gotLimit, body.Limit)
	}
}

func TestAudienceMembersRejectsInvalidParametersWithoutTouchingTheStores(t *testing.T) {
	for _, query := range []string{
		"?page=0", "?page=-1", "?page=abc", "?page=1.5", "?page=100001", "?page=99999999999999999999",
		"?limit=0", "?limit=-3", "?limit=abc", "?limit=2.5",
	} {
		t.Run(query, func(t *testing.T) {
			h := newHarness(t)

			h.store.getResult = campaign.Campaign{ID: memberCampaignID, Segment: h.gen.seg}
			rec := h.do(http.MethodGet, "/api/campaigns/"+memberCampaignID+"/audience"+query, "", "")

			wantError(t, rec, http.StatusBadRequest, "INVALID_REQUEST")
			if h.members.calls != 0 || h.store.gotID != "" {
				t.Fatalf("depolar okunmamalı: members=%d, kampanya=%q", h.members.calls, h.store.gotID)
			}
		})
	}
}

func TestAudienceMembersLastAllowedPageIsAccepted(t *testing.T) {
	h := newHarness(t)

	_, _, status := h.getMembers("?page=100000")

	if status != http.StatusOK || h.members.gotPage != 100000 {
		t.Fatalf("durum %d, sayfa %d", status, h.members.gotPage)
	}
}

// Son sayfanın ötesi hata değil: boş liste ve doğru toplam döner (null değil).
func TestAudienceMembersPageBeyondTheEndIsAnEmptyListWithTheTotal(t *testing.T) {
	h := newHarness(t)
	h.members.page = audience.MemberPage{Total: 7, Members: []audience.Member{}}

	body, raw, status := h.getMembers("?page=9")

	if status != http.StatusOK || body.Total != 7 || len(body.Items) != 0 {
		t.Fatalf("durum %d, gövde %+v", status, body)
	}
	if !strings.Contains(raw, `"items":[]`) {
		t.Fatalf("boş sayfa [] olmalı, null değil: %s", raw)
	}
}

func TestAudienceMembersUnknownCampaignIs404AndTheListIsNotRead(t *testing.T) {
	h := newHarness(t)
	h.store.getErr = campaign.ErrNotFound

	rec := h.do(http.MethodGet, "/api/campaigns/"+memberCampaignID+"/audience", "", "")

	wantError(t, rec, http.StatusNotFound, "CAMPAIGN_NOT_FOUND")
	if h.members.calls != 0 {
		t.Fatal("kampanya yokken liste okunmamalı")
	}
}

func TestAudienceMembersDoesNotLeakInternalErrors(t *testing.T) {
	const secret = "password=hunter2 host=db.internal"
	for name, arrange := range map[string]func(h *harness){
		"kampanya okunamadı": func(h *harness) { h.store.getErr = errors.New(secret) },
		"liste okunamadı":    func(h *harness) { h.members.err = errors.New(secret) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.store.getResult = campaign.Campaign{ID: memberCampaignID, Segment: h.gen.seg}
			arrange(h)

			rec := h.do(http.MethodGet, "/api/campaigns/"+memberCampaignID+"/audience", "", "")

			wantError(t, rec, http.StatusInternalServerError, "INTERNAL")
			if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "db.internal") {
				t.Fatalf("iç hata ayrıntısı istemciye sızdı: %s", rec.Body.String())
			}
			if !strings.Contains(h.logs.String(), "hunter2") {
				t.Fatal("ayrıntı loga yazılmalı")
			}
		})
	}
}

func TestAudienceMembersIsReadOnly(t *testing.T) {
	h := newHarness(t)

	if _, _, status := h.getMembers(""); status != http.StatusOK {
		t.Fatalf("durum %d", status)
	}

	if len(h.gen.briefs) != 0 {
		t.Fatalf("model çağrılmamalı: %v", h.gen.briefs)
	}
	if h.counter.calls != 0 || len(h.store.created) != 0 {
		t.Fatalf("kitle sayılmamalı ve kampanya yazılmamalı: sayım=%d, yazılan=%d", h.counter.calls, len(h.store.created))
	}
}

func TestAudienceMembersRejectsOtherMethods(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		h := newHarness(t)

		rec := h.do(method, "/api/campaigns/"+memberCampaignID+"/audience", "application/json", "{}")

		wantError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		if got := rec.Header().Get("Allow"); got != http.MethodGet {
			t.Errorf("%s: Allow = %q", method, got)
		}
	}
}
