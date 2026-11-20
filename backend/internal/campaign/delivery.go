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

// ChannelDelivery, mesaj durum dağılımının kanallara göre kırılımıdır (TRK-001). Her durum
// için SMS ve e-posta sayılarının toplamı kampanyanın toplam sayısına eşittir.
type ChannelDelivery struct {
	SMS   DeliveryCounts
	Email DeliveryCounts
}

// add, c'nin sayılarını d'ye ekler.
func (d *DeliveryCounts) add(c DeliveryCounts) {
	d.Queued += c.Queued
	d.Retrying += c.Retrying
	d.Sending += c.Sending
	d.Sent += c.Sent
	d.Failed += c.Failed
	d.Skipped += c.Skipped
}

// DeliverySnapshot, kampanyanın gönderim durumunun o anki görüntüsüdür. Tek bir
// sorguyla okunur, bu yüzden sayılar birbiriyle tutarlıdır.
type DeliverySnapshot struct {
	Status   string
	Messages DeliveryCounts
	Failures FailureCounts
	// Channels, Messages'ın kanal (sms, email) kırılımıdır; her durum için iki kanalın toplamı
	// Messages'a eşittir.
	Channels ChannelDelivery
	// Recovered, yeniden denemeyle toparlanan mesajlardır: `sent` olan ve `attempts > 1` olanlar,
	// yani sonuçlanmadan önce birden fazla kez işlenmeye alınmış (claim edilmiş) mesajlar.
	// `attempts` claim anında artar (ARCHITECTURE §3); ikinci claim şu durumlarda olur:
	// geçici provider hatası (429, 5xx, zaman aşımı, bağlantı hatası, geçersiz 2xx) sonrası backoff;
	// kayıp yanıt (provider ilk isteği kabul etmiş olabilir, worker aynı Idempotency-Key ile
	// yeniden deneyip kayıtlı sonucu alır); lease'in dolması veya worker çökmesi sonrası reaper
	// (önceki deneme provider'a hiç ulaşmamış olabilir). Bu yüzden anlamı "ilk denemede
	// tamamlanamadı, sonraki denemede gönderildi olarak kaydedildi"dir; "ilk istek başarısız
	// oldu" demek değildir. Nedenleri ayırmaz ve provider çağrı sayısı değildir. Idempotency
	// anahtarı yüzünden toparlananlar da en çok bir kez teslim edilmiştir. Messages.Sent'i aşamaz.
	Recovered int
}

// DeliverySnapshot, kampanyanın durumunu ve mesaj sayılarını (toplam, kanal kırılımı ve
// yeniden denemeyle toparlananlar) okur. Kampanya yoksa (veya kimlik geçerli bir uuid
// değilse) ErrNotFound döner. Mesajı olmayan kampanya (ör. draft) için bütün sayılar sıfırdır.
// Sayımlar tek sorguda, kanala göre gruplanarak yapılır; toplamlar kanalların toplamıdır.
func (s *Store) DeliverySnapshot(ctx context.Context, id string) (DeliverySnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.status, m.channel,
		        count(*) FILTER (WHERE m.status = 'queued' AND m.attempts = 0),
		        count(*) FILTER (WHERE m.status = 'queued' AND m.attempts > 0),
		        count(*) FILTER (WHERE m.status = 'sending'),
		        count(*) FILTER (WHERE m.status = 'sent'),
		        count(*) FILTER (WHERE m.status = 'failed'),
		        count(*) FILTER (WHERE m.status = 'skipped'),
		        count(*) FILTER (WHERE m.status = 'failed' AND m.failure_reason = $2),
		        count(*) FILTER (WHERE m.status = 'failed' AND m.failure_reason = $3),
		        count(*) FILTER (WHERE m.status = 'sent' AND m.attempts > 1)
		 FROM campaigns c
		 LEFT JOIN messages m ON m.campaign_id = c.id
		 WHERE c.id = $1
		 GROUP BY c.status, m.channel`,
		id, failureExhausted, failureRejected)
	if err != nil {
		return DeliverySnapshot{}, deliveryReadError(err)
	}
	defer rows.Close()

	var (
		snap  DeliverySnapshot
		found bool
	)
	for rows.Next() {
		var (
			channel sql.NullString // mesajı olmayan kampanyada NULL
			c       DeliveryCounts
			f       FailureCounts
			rec     int
		)
		if err := rows.Scan(&snap.Status, &channel,
			&c.Queued, &c.Retrying, &c.Sending, &c.Sent, &c.Failed, &c.Skipped,
			&f.Exhausted, &f.Rejected, &rec); err != nil {
			return DeliverySnapshot{}, deliveryReadError(err)
		}
		found = true
		snap.Messages.add(c)
		snap.Failures.Exhausted += f.Exhausted
		snap.Failures.Rejected += f.Rejected
		snap.Recovered += rec
		switch channel.String {
		case "sms":
			snap.Channels.SMS.add(c)
		case "email":
			snap.Channels.Email.add(c)
		}
	}
	if err := rows.Err(); err != nil {
		return DeliverySnapshot{}, deliveryReadError(err)
	}
	if !found {
		return DeliverySnapshot{}, ErrNotFound
	}
	return snap, nil
}

// deliveryReadError, okuma hatasını sarar; geçersiz uuid kampanyanın olmaması demektir.
func deliveryReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err) {
		return ErrNotFound
	}
	return fmt.Errorf("campaign: gönderim durumu okunamadı: %w", err)
}
