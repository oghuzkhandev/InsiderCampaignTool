package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// maxBodyBytes, istek gövdesi sınırıdır. Brief sınırı (rune sayısı) segment
// paketindedir; bu yalnızca aşırı büyük gövdeleri keser.
const maxBodyBytes = 64 << 10

type createRequest struct {
	Brief string `json:"brief"`
}

type audienceResponse struct {
	Total int `json:"total"`
	SMS   int `json:"sms"`
	Email int `json:"email"`
}

type campaignResponse struct {
	ID        string           `json:"id"`
	Brief     string           `json:"brief"`
	Status    string           `json:"status"`
	Segment   segment.Segment  `json:"segment"`
	Audience  audienceResponse `json:"audience"`
	CreatedAt time.Time        `json:"created_at"`
	// Content, varsa son doğrulanmış içerik sürümüdür.
	Content *contentVersionResponse `json:"content,omitempty"`
	// ApprovedContentVersion ve ApprovedAt, kampanya onaylandıysa bulunur.
	ApprovedContentVersion *int       `json:"approved_content_version,omitempty"`
	ApprovedAt             *time.Time `json:"approved_at,omitempty"`
}

func toResponse(c campaign.Campaign) campaignResponse {
	return campaignResponse{
		ApprovedContentVersion: c.ApprovedContentVersion,
		ApprovedAt:             c.ApprovedAt,
		ID:                     c.ID,
		Brief:                  c.Brief,
		Status:                 c.Status,
		Segment:                c.Segment,
		Audience:               audienceResponse{Total: c.Audience.Total, SMS: c.Audience.SMS, Email: c.Audience.Email},
		CreatedAt:              c.CreatedAt.UTC(),
	}
}

// createCampaign, brief'ten segment üretir, kitle sayılarını okur ve draft
// kampanyayı kaydeder. Segment geçersizse kitle sorgusu çalışmaz ve kampanya
// oluşmaz.
func (s *server) createCampaign(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "İstek application/json olmalı.")
		return
	}

	var req createRequest
	if status, code, msg, ok := decodeBody(w, r, &req); !ok {
		writeError(w, status, code, msg)
		return
	}

	brief := strings.TrimSpace(req.Brief)
	ctx := r.Context()

	seg, err := s.Segments.Generate(ctx, brief)
	if err != nil {
		s.writeGenerateError(w, r, err)
		return
	}
	counts, err := s.Audience.Count(ctx, seg, s.Now())
	if err != nil {
		s.internalError(w, r, "kitle sayıları okunamadı", err)
		return
	}
	created, err := s.Campaigns.Create(ctx, brief, seg, counts)
	if err != nil {
		s.internalError(w, r, "kampanya kaydedilemedi", err)
		return
	}

	w.Header().Set("Location", "/api/campaigns/"+created.ID)
	writeJSON(w, http.StatusCreated, toResponse(created))
}

// getCampaign, kampanyayı kimliğiyle okur.
func (s *server) getCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := s.Campaigns.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
			return
		}
		s.internalError(w, r, "kampanya okunamadı", err)
		return
	}
	resp := toResponse(c)
	v, found, err := s.Contents.Latest(r.Context(), c.ID)
	if err != nil {
		s.internalError(w, r, "içerik sürümü okunamadı", err)
		return
	}
	if found {
		resp.Content = toContentResponse(v)
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeGenerateError, segment üretiminin hatalarını HTTP yanıtına çevirir.
func (s *server) writeGenerateError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *segment.InvalidError
	switch {
	case errors.Is(err, segment.ErrEmptyBrief):
		writeError(w, http.StatusUnprocessableEntity, "BRIEF_EMPTY", "Brief boş olamaz.")
	case errors.Is(err, segment.ErrBriefTooLong):
		writeError(w, http.StatusUnprocessableEntity, "BRIEF_TOO_LONG", "Brief çok uzun.")
	case errors.As(err, &invalid):
		s.LLMValidationFailures.Inc() // model çıktısı doğrulamadan geçemedi
		reasons := make([]reason, 0, len(invalid.Issues))
		for _, is := range invalid.Issues {
			reasons = append(reasons, reason{Code: string(is.Code), Field: is.Field, Text: is.Text})
		}
		writeError(w, http.StatusUnprocessableEntity, "SEGMENT_INVALID", "Brief'ten geçerli bir segment çıkarılamadı.", reasons...)
	case errors.Is(err, llm.ErrTimeout):
		writeError(w, http.StatusGatewayTimeout, "LLM_TIMEOUT", "Segment üretimi zaman aşımına uğradı. Yeniden deneyin.")
	case errors.Is(err, llm.ErrRateLimited):
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusServiceUnavailable, "LLM_RATE_LIMITED", "Segment servisi şu an yoğun. Biraz sonra yeniden deneyin.")
	default:
		s.internalError(w, r, "segment üretilemedi", err)
	}
}

// internalError, ayrıntıyı yalnızca loga yazar; istemciye genel bir mesaj döner.
// İstemci bağlantıyı kapattıysa yanıt yazılmaz.
func (s *server) internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	s.Logger.Error(msg, "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "INTERNAL", "Beklenmeyen bir hata oluştu.")
}

func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// errTrailingData, ilk JSON değerinden sonra başka bir değer geldiğini gösterir.
var errTrailingData = errors.New("fazladan veri")

// decodeBody, gövdeyi katı biçimde okur: bilinmeyen alan, birden fazla JSON
// değeri ve aşırı büyük gövde reddedilir. Okuma sırasında limit aşılırsa 413
// döner; geçerli JSON'dan sonra gelen boşluklar sırasında aşılırsa da.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) (status int, code, msg string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err == nil {
		// İlk değerden sonra yalnızca boşluk olmalı. Okuma hatası (ör. limit
		// aşımı) olduğu gibi korunur; başka bir değer varsa fazladan veridir.
		switch _, extra := dec.Token(); {
		case errors.Is(extra, io.EOF):
			return 0, "", "", true
		case extra == nil:
			err = errTrailingData
		default:
			err = extra
		}
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "İstek gövdesi çok büyük.", false
	}
	return http.StatusBadRequest, "INVALID_REQUEST", "İstek gövdesi geçerli bir JSON değil veya beklenmeyen alan içeriyor.", false
}
