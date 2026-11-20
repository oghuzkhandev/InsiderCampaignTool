package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// CampaignApprover, kampanyayı insan onayıyla onaylar (campaign.Approver).
type CampaignApprover interface {
	ApproveScheduled(ctx context.Context, campaignID string, version int, now time.Time, scheduled *time.Time) (campaign.ApprovalResult, error)
}

type approveRequest struct {
	// ContentVersion işaretçidir: eksik alan ile 0 ayırt edilir.
	ContentVersion *int       `json:"content_version"`
	ScheduledAt    *time.Time `json:"scheduled_at"`
}

type channelCountsResponse struct {
	SMS   int `json:"sms"`
	Email int `json:"email"`
	Total int `json:"total"`
}

type approveResponse struct {
	ID                     string                `json:"id"`
	Status                 string                `json:"status"`
	ApprovedContentVersion int                   `json:"approved_content_version"`
	ApprovedAt             time.Time             `json:"approved_at"`
	ScheduledAt            *time.Time            `json:"scheduled_at,omitempty"`
	Queued                 channelCountsResponse `json:"queued"`
}

func toChannelCounts(c campaign.ChannelCounts) channelCountsResponse {
	return channelCountsResponse{SMS: c.SMS, Email: c.Email, Total: c.Total()}
}

// approveCampaign, kampanyayı verilen içerik sürümüyle onaylar. Onay, onaylanan
// sürüm ve `queued` mesaj işleri aynı transaction'da yazılır. İlk onay ve
// onaylanan aynı sürümle tekrar `200` döner (ADR-011).
func (s *server) approveCampaign(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "İstek application/json olmalı.")
		return
	}
	var req approveRequest
	if status, code, msg, ok := decodeBody(w, r, &req); !ok {
		writeError(w, status, code, msg)
		return
	}
	if req.ContentVersion == nil || *req.ContentVersion < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "content_version gerekli ve pozitif bir tam sayı olmalı.")
		return
	}

	res, err := s.Approver.ApproveScheduled(r.Context(), r.PathValue("id"), *req.ContentVersion, s.Now(), req.ScheduledAt)
	if err != nil {
		s.writeApproveError(w, r, err)
		return
	}

	resp := approveResponse{
		ID:                     res.CampaignID,
		Status:                 res.Status,
		ApprovedContentVersion: res.ContentVersion,
		ApprovedAt:             res.ApprovedAt.UTC(),
		ScheduledAt:            res.ScheduledAt,
		Queued:                 toChannelCounts(res.Queued),
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeApproveError, onay hatalarını HTTP yanıtına çevirir. Hiçbir durumda
// gönderim işi oluşmamıştır (transaction geri alındı).
func (s *server) writeApproveError(w http.ResponseWriter, r *http.Request, err error) {
	var render *campaign.RenderLimitError
	switch {
	case errors.Is(err, campaign.ErrSchedulePast):
		writeError(w, http.StatusUnprocessableEntity, "SCHEDULE_IN_PAST", "Gönderim zamanı gelecekte olmalı.")
	case errors.Is(err, campaign.ErrScheduleConflict):
		writeError(w, http.StatusConflict, "SCHEDULE_CONFLICT", "Kampanya farklı bir gönderim zamanı ile zaten onaylandı.")
	case errors.Is(err, campaign.ErrNotFound):
		writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
	case errors.Is(err, campaign.ErrNoContent):
		writeError(w, http.StatusConflict, "NO_VALIDATED_CONTENT", "Kampanyanın onaylanabilir (doğrulanmış) bir içerik sürümü yok.")
	case errors.Is(err, campaign.ErrStaleVersion):
		writeError(w, http.StatusConflict, "STALE_CONTENT_VERSION", "Gördüğünüz içerik sürümü güncel değil. Sayfayı yenileyip son sürümü onaylayın.")
	case errors.Is(err, campaign.ErrApprovedWithOtherVersion):
		writeError(w, http.StatusConflict, "CAMPAIGN_ALREADY_APPROVED", "Kampanya başka bir içerik sürümüyle zaten onaylandı.")
	case errors.Is(err, campaign.ErrUnfilteredSegment):
		writeError(w, http.StatusUnprocessableEntity, "UNFILTERED_SEGMENT", "Filtresiz kitle gönderime onaylanamaz.")
	case errors.Is(err, campaign.ErrNoRecipients):
		writeError(w, http.StatusUnprocessableEntity, "NO_ELIGIBLE_RECIPIENTS", "Hiçbir kanalda uygun alıcı yok; kampanya onaylanamaz.")
	case errors.As(err, &render):
		reasons := make([]reason, 0, len(render.Overflows))
		for _, o := range render.Overflows {
			reasons = append(reasons, reason{Code: "TOO_LONG", Field: o.Field, Count: o.Count, Text: clipText(o.Example, 64)})
		}
		writeError(w, http.StatusUnprocessableEntity, "RENDER_LIMIT_EXCEEDED",
			"Kişiselleştirme sonrası bazı alıcıların metni sınırı aşıyor. Metni kısaltın.", reasons...)
	default:
		s.internalError(w, r, "kampanya onaylanamadı", err)
	}
}

// clipText, kullanıcı verisinden gelen örnek adı kontrol karakterlerinden
// arındırır ve rune sayısına göre kırpar.
func clipText(s string, max int) string {
	out := make([]rune, 0, max)
	for _, r := range s {
		if len(out) == max {
			break
		}
		if r < 0x20 || r == 0x7f {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}
