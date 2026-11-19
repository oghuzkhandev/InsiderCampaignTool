package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// DeliveryReader, kampanyanın gönderim durumunu okur (campaign.Store).
type DeliveryReader interface {
	DeliverySnapshot(ctx context.Context, id string) (campaign.DeliverySnapshot, error)
}

const (
	// defaultDeliveryInterval: iki durum görüntüsü arasındaki süre (TRK-001).
	defaultDeliveryInterval = time.Second
	// defaultDeliveryQueryTimeout: tek bir durum okumasının süre sınırı. Arayüz
	// 8 sn görüntü gelmezse bağlantıyı takılmış sayar; takılan bir okuma bu sürenin
	// çok altında `error` olayına dönüşmelidir.
	defaultDeliveryQueryTimeout = 3 * time.Second
)

func (s *server) deliveryInterval() time.Duration {
	if s.DeliveryInterval > 0 {
		return s.DeliveryInterval
	}
	return defaultDeliveryInterval
}

func (s *server) deliveryQueryTimeout() time.Duration {
	if s.DeliveryQueryTimeout > 0 {
		return s.DeliveryQueryTimeout
	}
	return defaultDeliveryQueryTimeout
}

type deliveryCountsResponse struct {
	Queued   int `json:"queued"`
	Retrying int `json:"retrying"`
	Sending  int `json:"sending"`
	Sent     int `json:"sent"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
}

type failureCountsResponse struct {
	Exhausted int `json:"exhausted"`
	Rejected  int `json:"rejected"`
}

// deliverySnapshotResponse, `snapshot` olayının gövdesidir (ARCHITECTURE §5).
type deliverySnapshotResponse struct {
	Status   string                 `json:"status"`
	Messages deliveryCountsResponse `json:"messages"`
	Failures failureCountsResponse  `json:"failures"`
}

func toDeliveryResponse(s campaign.DeliverySnapshot) deliverySnapshotResponse {
	return deliverySnapshotResponse{
		Status: s.Status,
		Messages: deliveryCountsResponse{
			Queued: s.Messages.Queued, Retrying: s.Messages.Retrying, Sending: s.Messages.Sending,
			Sent: s.Messages.Sent, Failed: s.Messages.Failed, Skipped: s.Messages.Skipped,
		},
		Failures: failureCountsResponse{Exhausted: s.Failures.Exhausted, Rejected: s.Failures.Rejected},
	}
}

// readDelivery, durumu süre sınırıyla okur.
func (s *server) readDelivery(ctx context.Context, id string) (campaign.DeliverySnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, s.deliveryQueryTimeout())
	defer cancel()
	return s.Deliveries.DeliverySnapshot(ctx, id)
}

// deliveryEvents, kampanyanın gönderim durumunu SSE ile akıtır (TRK-001).
//
//   - Bağlantı açılır açılmaz ilk `snapshot`, sonra her aralıkta bir tane. Kuyrukta
//     hiçbir şey değişmese de her aralıkta görüntü yollanır: arayüz görüntü
//     gelmeyen bağlantıyı takılmış sayıp yeniler, sessiz ama sağlıklı bir bağlantıyı
//     ayırt edemez. Görüntü aynı zamanda heartbeat'tir.
//   - Kampanya yoksa (geçersiz uuid dahil) akış başlamadan düz JSON 404.
//   - Okuma hata verir veya süre sınırını aşarsa `error` olayı yollanır ve akış
//     kapanır; arayüz yeniden bağlanır. Sessiz kalınmaz.
//   - Kampanya `completed` olunca son görüntü yollanır ve akış biter.
//   - İstemci ayrılınca veya sunucu kapanmaya başlayınca (Deps.Shutdown) akış biter;
//     bekleyen goroutine kalmaz.
func (s *server) deliveryEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	snap, err := s.readDelivery(ctx, id)
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
			return
		}
		s.internalError(w, r, "gönderim durumu okunamadı", err)
		return
	}

	es := newEventStream(w, s.sseWriteTimeout(), nil)
	if es.failed {
		return
	}
	es.send("snapshot", toDeliveryResponse(snap))

	ticker := time.NewTicker(s.deliveryInterval())
	defer ticker.Stop()
	for {
		// Gönderim bitti: son görüntü yollandı, izlenecek bir şey kalmadı.
		if es.failed || snap.Status == campaign.StatusCompleted {
			return
		}
		select {
		case <-ctx.Done():
			return // istemci ayrıldı
		case <-s.Shutdown:
			return // sunucu kapanıyor; akış kendiliğinden bitsin
		case <-ticker.C:
		}

		snap, err = s.readDelivery(ctx, id)
		switch {
		case err == nil:
			es.send("snapshot", toDeliveryResponse(snap))
		case ctx.Err() != nil:
			return // istemci ayrıldı
		case errors.Is(err, campaign.ErrNotFound):
			es.send("error", errorEvent{Code: "CAMPAIGN_NOT_FOUND", Message: "Kampanya bulunamadı."})
			return
		default:
			s.logStreamError(r, "gönderim durumu okunamadı", err)
			es.send("error", errorEvent{Code: "INTERNAL", Message: "Gönderim durumu okunamadı."})
			return
		}
	}
}
