package httpapi

import (
	"context"
	"fmt"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
)

// QueueDepthReader, kanal başına kuyrukta bekleyen mesaj sayısını okur
// (campaign.Store).
type QueueDepthReader interface {
	QueueDepth(ctx context.Context) (map[string]int, error)
}

// MetricsCollector, API'nin `/metrics` çıktısını üretir (ARCHITECTURE §12):
//
//   - `llm_validation_failures_total`: süreç belleğinde tutulan sayaç. Model
//     çıktısının doğrulamadan geçemediği her durumu sayar (segment ve içerik üretimi).
//   - `queue_depth{channel}`: bellekte tutulmaz; her okunuşta veritabanından
//     hesaplanır. Böylece worker durduğunda da kuyruk birikimi görünür.
//
// Worker sayaçları (messages_sent_total vb.) buraya ait DEĞİLDİR; worker kendi
// `/metrics` ucunda sunar ve API onları göremez (ADR-016). Kuyruk derinliği
// okunamazsa hata döner ve çıktı sunulmaz: okunamayan değer 0 gibi görünmemeli.
func MetricsCollector(queue QueueDepthReader, llmValidationFailures *metrics.Counter) func(ctx context.Context, w *metrics.Writer) error {
	return func(ctx context.Context, w *metrics.Writer) error {
		depth, err := queue.QueueDepth(ctx)
		if err != nil {
			return fmt.Errorf("queue_depth: %w", err)
		}
		w.Counter("llm_validation_failures_total", "Doğrulamadan geçemeyen model çıktısı sayısı (segment ve içerik üretimi).", llmValidationFailures.Value())
		samples := make([]metrics.LabeledValue, 0, len(campaign.QueueChannels))
		for _, ch := range campaign.QueueChannels {
			samples = append(samples, metrics.LabeledValue{
				Labels: []metrics.Label{{Name: "channel", Value: ch}},
				Value:  int64(depth[ch]),
			})
		}
		w.GaugeVec("queue_depth", "Kanal başına kuyrukta bekleyen (status = queued) mesaj sayısı; yeniden denenecekler dahil.", samples)
		return nil
	}
}
