package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// memberResponse, segmente uyan bir kişidir (ADR-031). Eksik ad, telefon veya e-posta
// `null` olarak gider; boş metin veya sahte değer yollanmaz.
type memberResponse struct {
	ID        int64   `json:"id"`
	FirstName *string `json:"first_name"`
	City      string  `json:"city"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	// SMSStatus ve EmailStatus: eligible, no_consent veya no_address.
	SMSStatus   string `json:"sms_status"`
	EmailStatus string `json:"email_status"`
}

type memberListResponse struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	// Total, segmente uyan benzersiz kişi sayısıdır (kanal iznine bakılmaz).
	Total int              `json:"total"`
	Items []memberResponse `json:"items"`
}

// parsePositiveInt, sorgu parametresini okur. Verilmezse (yok veya boş) def döner. Tam sayı
// olmayan veya 1'den küçük değer geçersizdir; sessizce düzeltilmez.
func parsePositiveInt(q url.Values, name string, def int) (n int, ok bool) {
	raw := q.Get(name)
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// listAudienceMembers, kampanyanın kayıtlı ve doğrulanmış segmentine uyan kişileri sayfalı
// döndürür (ADR-031). Model çağrılmaz, hiçbir kayıt değişmez. Parametreler veritabanı
// okunmadan doğrulanır. Sıra kimliğe göre artandır; sayfa son kayıttan sonraysa hata değil,
// boş `items` ve doğru `total` döner.
func (s *server) listAudienceMembers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, ok := parsePositiveInt(query, "page", 1)
	if !ok || page > audience.MaxMemberPage {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST",
			"page 1 ile "+strconv.Itoa(audience.MaxMemberPage)+" arasında bir tam sayı olmalı.")
		return
	}
	limit, ok := parsePositiveInt(query, "limit", audience.DefaultMemberLimit)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST",
			"limit 1 ile "+strconv.Itoa(audience.MaxMemberLimit)+" arasında bir tam sayı olmalı.")
		return
	}
	limit = min(limit, audience.MaxMemberLimit)

	c, err := s.Campaigns.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
			return
		}
		s.internalError(w, r, "kampanya okunamadı", err)
		return
	}

	result, err := s.Members.ListMembers(r.Context(), c.Segment, s.Now(), page, limit)
	if err != nil {
		s.internalError(w, r, "kişi listesi okunamadı", err)
		return
	}

	resp := memberListResponse{Page: page, Limit: limit, Total: result.Total,
		Items: make([]memberResponse, 0, len(result.Members))}
	for _, m := range result.Members {
		resp.Items = append(resp.Items, memberResponse{
			ID: m.ID, FirstName: m.FirstName, City: m.City, Email: m.Email, Phone: m.Phone,
			SMSStatus: string(m.SMSStatus), EmailStatus: string(m.EmailStatus),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}
