package campaign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// StatusCompleted, bütün mesajların işi bitmiş kampanyanın durumudur (başarısız
// veya atlanan mesaj olması tamamlanmayı engellemez).
const StatusCompleted = "completed"

// failure_reason değerleri; worker.FailureExhausted ve worker.FailureRejected ile
// aynıdır (ADR-006). Paketler birbirine bağımlı olmasın diye burada ayrıca
// tanımlıdır; entegrasyon testi ikisinin eşit kaldığını doğrular.
const (
	failureExhausted = "exhausted"
	failureRejected  = "rejected"
)

// DeliveryCounts, bir kampanyanın mesajlarının durum dağılımıdır (TRK-001).
// Queued yalnızca hiç denenmemiş (attempts = 0), Retrying denenmiş ve yeniden
// sırada bekleyen (queued ve attempts > 0) mesajlardır; ikisi birbirine dahil
// değildir.
type DeliveryCounts struct {
	Queued   int
	Retrying int
	Sending  int
	Sent     int
	Failed   int
	Skipped  int
}

// FailureCounts, başarısız mesajların nedenidir. Toplamı DeliveryCounts.Failed'i
// aşmaz: nedeni kaydedilmemiş (veya bilinmeyen) bir başarısızlık yalnızca Failed'e
// sayılır.
type FailureCounts struct {
	// Exhausted: deneme sınırı doldu, sonuç doğrulanamadı.
	Exhausted int
	// Rejected: sağlayıcı isteği kesin olarak reddetti.
	Rejected int
}

// DeliverySnapshot, kampanyanın gönderim durumunun o anki görüntüsüdür. Tek bir
// sorguyla okunur, bu yüzden sayılar birbiriyle tutarlıdır.
type DeliverySnapshot struct {
	Status   string
	Messages DeliveryCounts
	Failures FailureCounts
}

// DeliverySnapshot, kampanyanın durumunu ve mesaj sayılarını okur. Kampanya yoksa
// (veya kimlik geçerli bir uuid değilse) ErrNotFound döner. Mesajı olmayan
// kampanya (ör. draft) için bütün sayılar sıfırdır.
func (s *Store) DeliverySnapshot(ctx context.Context, id string) (DeliverySnapshot, error) {
	var snap DeliverySnapshot
	err := s.db.QueryRowContext(ctx,
		`SELECT c.status,
		        count(*) FILTER (WHERE m.status = 'queued' AND m.attempts = 0),
		        count(*) FILTER (WHERE m.status = 'queued' AND m.attempts > 0),
		        count(*) FILTER (WHERE m.status = 'sending'),
		        count(*) FILTER (WHERE m.status = 'sent'),
		        count(*) FILTER (WHERE m.status = 'failed'),
		        count(*) FILTER (WHERE m.status = 'skipped'),
		        count(*) FILTER (WHERE m.status = 'failed' AND m.failure_reason = $2),
		        count(*) FILTER (WHERE m.status = 'failed' AND m.failure_reason = $3)
		 FROM campaigns c
		 LEFT JOIN messages m ON m.campaign_id = c.id
		 WHERE c.id = $1
		 GROUP BY c.status`,
		id, failureExhausted, failureRejected,
	).Scan(&snap.Status,
		&snap.Messages.Queued, &snap.Messages.Retrying, &snap.Messages.Sending,
		&snap.Messages.Sent, &snap.Messages.Failed, &snap.Messages.Skipped,
		&snap.Failures.Exhausted, &snap.Failures.Rejected)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err) {
			return DeliverySnapshot{}, ErrNotFound
		}
		return DeliverySnapshot{}, fmt.Errorf("campaign: gönderim durumu okunamadı: %w", err)
	}
	return snap, nil
}
