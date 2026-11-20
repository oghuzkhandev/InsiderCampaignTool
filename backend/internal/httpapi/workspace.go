package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/templates"
)

type CampaignPageReader interface {
	ListPage(context.Context, int, int, string) (campaign.Page, error)
}
type OverviewReader interface {
	Overview(context.Context) (campaign.Overview, error)
}
type DirectoryReader interface {
	Directory(ctx context.Context, f segment.DirectoryFilter, page, limit int) (audience.MemberPage, error)
	Cities(ctx context.Context) ([]string, error)
}
type TemplateStore interface {
	List(context.Context, int, int) ([]templates.Template, int, error)
	Save(context.Context, string, string, content.Content) (templates.Template, error)
	Delete(context.Context, string) error
}

func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	p, ok := parsePositiveInt(r.URL.Query(), "page", 1)
	l, valid := parsePositiveInt(r.URL.Query(), "limit", 10)
	if !ok || !valid || p > 100000 {
		writeError(w, 400, "INVALID_REQUEST", "Sayfa ve limit pozitif tam sayı olmalı; sayfa en fazla 100000 olabilir.")
		return 0, 0, false
	}
	return p, min(l, 20), true
}

func (s *server) getOverview(w http.ResponseWriter, r *http.Request) {
	if s.Overview == nil {
		writeError(w, 503, "UNAVAILABLE", "Genel bakış şu an kullanılamıyor.")
		return
	}
	o, err := s.Overview.Overview(r.Context())
	if err != nil {
		s.internalError(w, r, "genel bakış okunamadı", err)
		return
	}
	writeJSON(w, 200, o)
}

func (s *server) listCampaignPage(w http.ResponseWriter, r *http.Request) {
	p, l, ok := pagination(w, r)
	if !ok {
		return
	}
	state := r.URL.Query().Get("status")
	switch state {
	case "", "draft", "approved", "sending", "completed":
	default:
		writeError(w, 400, "INVALID_REQUEST", "Kampanya durumu geçersiz.")
		return
	}
	if s.CampaignPages == nil {
		writeError(w, 503, "UNAVAILABLE", "Kampanya listesi şu an kullanılamıyor.")
		return
	}
	result, err := s.CampaignPages.ListPage(r.Context(), p, l, state)
	if err != nil {
		s.internalError(w, r, "kampanyalar okunamadı", err)
		return
	}
	writeJSON(w, 200, result)
}

// directoryFilter, kişi dizininin `city`, `sms` ve `email` süzgeçlerini okur ve doğrular.
// Geçersiz değer `400` olur ve veritabanı okunmaz; sessizce yok sayılmaz. `sms` ve `email`
// yalnızca eligible, no_consent veya no_address olabilir. Şehir en fazla 64 karakterdir
// (segmentteki sınırla aynı).
func directoryFilter(w http.ResponseWriter, r *http.Request, search string) (segment.DirectoryFilter, bool) {
	q := r.URL.Query()
	f := segment.DirectoryFilter{
		Search: search,
		City:   strings.TrimSpace(q.Get("city")),
		SMS:    q.Get("sms"),
		Email:  q.Get("email"),
	}
	if utf8.RuneCountInString(f.City) > 64 || strings.ContainsRune(f.City, '\x00') {
		writeError(w, 400, "INVALID_REQUEST", "Şehir en fazla 64 karakter olabilir.")
		return f, false
	}
	for name, v := range map[string]string{"sms": f.SMS, "email": f.Email} {
		switch v {
		case "", "eligible", "no_consent", "no_address":
		default:
			writeError(w, 400, "INVALID_REQUEST", name+" yalnızca eligible, no_consent veya no_address olabilir.")
			return f, false
		}
	}
	return f, true
}

// listDirectoryCities, kişi dizini süzgecinin şehir seçeneklerini döndürür.
func (s *server) listDirectoryCities(w http.ResponseWriter, r *http.Request) {
	if s.Directory == nil {
		writeError(w, 503, "UNAVAILABLE", "Kişi listesi şu an kullanılamıyor.")
		return
	}
	cities, err := s.Directory.Cities(r.Context())
	if err != nil {
		s.internalError(w, r, "şehirler okunamadı", err)
		return
	}
	writeJSON(w, 200, struct {
		Cities []string `json:"cities"`
	}{cities})
}

func (s *server) listDirectory(w http.ResponseWriter, r *http.Request) {
	p, l, ok := pagination(w, r)
	if !ok {
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if utf8.RuneCountInString(search) > 100 || strings.ContainsRune(search, '\x00') {
		writeError(w, 400, "INVALID_REQUEST", "Arama en fazla 100 karakter olabilir.")
		return
	}
	filter, ok := directoryFilter(w, r, search)
	if !ok {
		return
	}
	if s.Directory == nil {
		writeError(w, 503, "UNAVAILABLE", "Kişi listesi şu an kullanılamıyor.")
		return
	}
	result, err := s.Directory.Directory(r.Context(), filter, p, l)
	if err != nil {
		s.internalError(w, r, "kitleler okunamadı", err)
		return
	}
	response := memberListResponse{Page: p, Limit: l, Total: result.Total, Items: []memberResponse{}}
	for _, m := range result.Members {
		response.Items = append(response.Items, memberResponse{ID: m.ID, FirstName: m.FirstName, City: m.City, Email: m.Email, Phone: m.Phone, SMSStatus: string(m.SMSStatus), EmailStatus: string(m.EmailStatus)})
	}
	writeJSON(w, 200, response)
}

func (s *server) listTemplates(w http.ResponseWriter, r *http.Request) {
	p, l, ok := pagination(w, r)
	if !ok {
		return
	}
	if s.Templates == nil {
		writeError(w, 503, "UNAVAILABLE", "Şablonlar şu an kullanılamıyor.")
		return
	}
	items, total, err := s.Templates.List(r.Context(), p, l)
	if err != nil {
		s.internalError(w, r, "şablonlar okunamadı", err)
		return
	}
	writeJSON(w, 200, struct {
		Items []templates.Template `json:"items"`
		Total int                  `json:"total"`
		Page  int                  `json:"page"`
		Limit int                  `json:"limit"`
	}{items, total, p, l})
}

func (s *server) saveTemplate(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeError(w, 415, "UNSUPPORTED_MEDIA_TYPE", "İstek application/json olmalı.")
		return
	}
	var input struct {
		Name         string `json:"name"`
		SMSText      string `json:"sms_text"`
		EmailSubject string `json:"email_subject"`
		EmailBody    string `json:"email_body"`
	}
	if status, code, msg, ok := decodeBody(w, r, &input); !ok {
		writeError(w, status, code, msg)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 || strings.ContainsRune(input.Name, '\x00') {
		writeError(w, 422, "INVALID_TEMPLATE_NAME", "Şablon adı 1 ile 100 karakter arasında olmalı.")
		return
	}
	raw, err := json.Marshal(content.Content{SMSText: input.SMSText, EmailSubject: input.EmailSubject, EmailBody: input.EmailBody})
	if err != nil {
		s.internalError(w, r, "şablon okunamadı", err)
		return
	}
	c, issues := content.Validate(raw, content.Options{})
	if len(issues) > 0 {
		reasons := make([]reason, 0, len(issues))
		for _, issue := range issues {
			reasons = append(reasons, reason{Code: string(issue.Code), Field: issue.Field})
		}
		writeError(w, 422, "CONTENT_INVALID", "Şablon içeriği geçersiz. Alanları kontrol edin.", reasons...)
		return
	}
	if s.Templates == nil {
		writeError(w, 503, "UNAVAILABLE", "Şablonlar şu an kullanılamıyor.")
		return
	}
	t, err := s.Templates.Save(r.Context(), r.PathValue("id"), input.Name, c)
	if errors.Is(err, templates.ErrNotFound) {
		writeError(w, 404, "TEMPLATE_NOT_FOUND", "Şablon bulunamadı.")
		return
	}
	if err != nil {
		s.internalError(w, r, "şablon kaydedilemedi", err)
		return
	}
	status := 200
	if r.Method == http.MethodPost {
		status = 201
		w.Header().Set("Location", "/api/templates/"+t.ID)
	}
	writeJSON(w, status, t)
}

func (s *server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	if s.Templates == nil {
		writeError(w, 503, "UNAVAILABLE", "Şablonlar şu an kullanılamıyor.")
		return
	}
	err := s.Templates.Delete(r.Context(), r.PathValue("id"))
	if errors.Is(err, templates.ErrNotFound) {
		writeError(w, 404, "TEMPLATE_NOT_FOUND", "Şablon bulunamadı.")
		return
	}
	if err != nil {
		s.internalError(w, r, "şablon silinemedi", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
