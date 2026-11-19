package worker

import (
	"sync/atomic"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
)

// Kind, bir provider çağrısının sonuç türüdür (`send_attempts_total{outcome}`).
type Kind string

const (
	// KindAccepted: provider isteği kabul etti (2xx).
	KindAccepted Kind = "accepted"
	// KindRateLimited: 429. Geçici hata; Retry-After'a uyulur.
	KindRateLimited Kind = "rate_limited"
	// KindServerError: 5xx. Geçici hata.
	KindServerError Kind = "server_error"
	// KindNetworkError: zaman aşımı, bağlantı hatası veya yanıtın alınamaması.
	// Geçici hata; sonuç belirsizdir (provider kabul etmiş olabilir).
	KindNetworkError Kind = "network_error"
	// KindRejected: 429 dışındaki 4xx. Kalıcı hata.
	KindRejected Kind = "rejected"
	// KindInvalidResponse: 2xx döndü ama gövde eksik, okunamıyor veya sözleşmeye
	// (`{"message_id": "..."}`) uymuyor. Kabul DOĞRULANAMADI: mesaj `sent` sayılmaz,
	// geçici hata gibi aynı idempotency anahtarıyla yeniden denenir.
	KindInvalidResponse Kind = "invalid_response"
)

var allKinds = []Kind{KindAccepted, KindRateLimited, KindServerError, KindNetworkError, KindRejected, KindInvalidResponse}

// Metrics, worker sayaçlarıdır (ARCHITECTURE §12). Süreç belleğinde tutulur;
// worker yeniden başlayınca sıfırlanır. API bu sayaçları göremez (ADR-016).
type Metrics struct {
	messagesSent   atomic.Int64
	staleResults   atomic.Int64
	reaperRequeued atomic.Int64
	attempts       [6]atomic.Int64 // allKinds sırasıyla
	// rateLimitWait, bir gönderim denemesinin provider çağrısından önce hız sınırı
	// yüzünden beklediği toplam süredir (limiter + gönderim anı kapısı).
	rateLimitWait *metrics.Histogram
}

// NewMetrics, sıfırlanmış sayaçlarla bir Metrics oluşturur.
func NewMetrics() *Metrics {
	return &Metrics{rateLimitWait: metrics.NewHistogram(metrics.DefaultWaitBuckets)}
}

// RateLimitWait, bir denemenin hız sınırı yüzünden beklediği süreyi gözlemler.
// Beklemeyen deneme sıfır saniye olarak gözlenir: dağılım, işlerin ne kadarının
// hiç beklemediğini de gösterir.
func (m *Metrics) RateLimitWait(d time.Duration) { m.rateLimitWait.Observe(d) }

// Write, worker metriklerini Prometheus metin biçiminde yazar (ARCHITECTURE §12).
// Her seri, hiç gözlenmemiş olsa da sıfırla yazılır; böylece "worker çalışıyor ama
// henüz hiçbir şey göndermedi" ile "metrik yok" ayırt edilir.
func (m *Metrics) Write(w *metrics.Writer) {
	s := m.Snapshot()
	w.Counter("messages_sent_total", "Provider tarafından kabul edilip sent yazılan mesaj sayısı.", s.MessagesSent)
	attempts := make([]metrics.LabeledValue, 0, len(allKinds))
	for _, k := range allKinds {
		attempts = append(attempts, metrics.LabeledValue{Labels: []metrics.Label{{Name: "outcome", Value: string(k)}}, Value: s.Attempts[k]})
	}
	w.CounterVec("send_attempts_total", "Provider çağrılarının sonuç türüne göre sayısı.", attempts)
	w.Counter("stale_result_total", "Eski claim token yüzünden yok sayılan sonuç sayısı.", s.StaleResults)
	w.Counter("reaper_requeued_total", "Lease'i dolduğu için reaper'ın yeniden kuyruğa aldığı mesaj sayısı.", s.ReaperRequeued)
	w.Histogram("ratelimit_wait_seconds", "Bir gönderim denemesinin provider çağrısından önce hız sınırı yüzünden beklediği süre.", m.rateLimitWait.Snapshot())
}

// MetricsSnapshot, sayaçların o anki değerleridir.
type MetricsSnapshot struct {
	MessagesSent   int64
	StaleResults   int64
	ReaperRequeued int64
	Attempts       map[Kind]int64
}

func kindIndex(k Kind) int {
	for i, kk := range allKinds {
		if kk == k {
			return i
		}
	}
	return -1
}

// Attempt, bir provider çağrısının sonucunu sayar.
func (m *Metrics) Attempt(k Kind) {
	if i := kindIndex(k); i >= 0 {
		m.attempts[i].Add(1)
	}
}

// MessageSent, kabul edilip `sent` yazılan bir mesajı sayar.
func (m *Metrics) MessageSent() { m.messagesSent.Add(1) }

// StaleResult, eski claim token yüzünden yok sayılan bir sonucu sayar.
func (m *Metrics) StaleResult() { m.staleResults.Add(1) }

// ReaperRequeued, reaper'ın yeniden kuyruğa aldığı mesajları sayar.
func (m *Metrics) ReaperRequeued(n int) { m.reaperRequeued.Add(int64(n)) }

// Snapshot, sayaçların kopyasını döndürür.
func (m *Metrics) Snapshot() MetricsSnapshot {
	s := MetricsSnapshot{
		MessagesSent:   m.messagesSent.Load(),
		StaleResults:   m.staleResults.Load(),
		ReaperRequeued: m.reaperRequeued.Load(),
		Attempts:       make(map[Kind]int64, len(allKinds)),
	}
	for i, k := range allKinds {
		s.Attempts[k] = m.attempts[i].Load()
	}
	return s
}
