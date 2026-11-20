package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// ScheduledReader, gönderimi ileri bir zamana planlanmış kampanyaları okur (campaign.Store).
type ScheduledReader interface {
	ListScheduled(ctx context.Context, page, limit int) (campaign.ScheduledPage, error)
}

type scheduledItemResponse struct {
	ID          string    `json:"id"`
	Brief       string    `json:"brief"`
	ScheduledAt time.Time `json:"scheduled_at"`
	ApprovedAt  time.Time `json:"approved_at"`
	Queued      int       `json:"queued"`
	// Due: planlanan zaman geldi ama gönderim henüz başlamadı (ör. worker kapalı).
	Due bool `json:"due"`
}

type scheduledListResponse struct {
	Items []scheduledItemResponse `json:"items"`
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Limit int                     `json:"limit"`
	// ServerTime: istemci kalan süreyi kendi saatine değil buna göre hesaplar.
	ServerTime time.Time `json:"server_time"`
}

// listScheduled, gönderimi başlamamış planlı kampanyaları en yakın zamanı başta olacak biçimde
// sayfalı döndürür. Salt okunurdur. Sayfa ve limit veritabanı okunmadan doğrulanır (limit en çok 20).
func (s *server) listScheduled(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := pagination(w, r)
	if !ok {
		return
	}
	if s.Scheduled == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Zamanlanmış gönderimler şu an kullanılamıyor.")
		return
	}
	result, err := s.Scheduled.ListScheduled(r.Context(), page, limit)
	if err != nil {
		s.internalError(w, r, "planlı kampanyalar okunamadı", err)
		return
	}
	resp := scheduledListResponse{
		Items: make([]scheduledItemResponse, 0, len(result.Items)),
		Total: result.Total, Page: page, Limit: limit, ServerTime: result.ServerTime.UTC(),
	}
	for _, it := range result.Items {
		resp.Items = append(resp.Items, scheduledItemResponse{
			ID: it.ID, Brief: it.Brief, ScheduledAt: it.ScheduledAt.UTC(), ApprovedAt: it.ApprovedAt.UTC(),
			Queued: it.Queued, Due: it.Due,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}
