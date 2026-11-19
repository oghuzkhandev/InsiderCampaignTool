package campaign

import (
	"context"
	"fmt"
)

// QueueChannels, kuyruk derinliğinin raporlandığı kanallardır. Hiç mesajı olmayan
// kanal da 0 ile raporlanır: "kuyruk boş" ile "metrik yok" ayırt edilebilsin.
// `messages.channel` CHECK kısıtıyla aynı iki değerdir.
var QueueChannels = []string{"sms", "email"}

// QueueDepth, kanal başına `status = 'queued'` mesaj sayısını döndürür
// (`queue_depth{channel}`, ADR-016, TRK-001). Değer her çağrıda veritabanından
// hesaplanır, bellekte tutulmaz: worker durduğunda da birikim görünür. Yeniden
// denenmek üzere bekleyen mesajlar (attempts > 0) ve gelecekteki `next_attempt_at`
// zamanlı olanlar da kuyruktadır ve sayılır; `sending` ve terminal durumlar
// sayılmaz. Sorgu `messages_claim_idx` kısmi indeksini kullanır.
func (s *Store) QueueDepth(ctx context.Context) (map[string]int, error) {
	var sms, email int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FILTER (WHERE channel = 'sms'),
		        count(*) FILTER (WHERE channel = 'email')
		 FROM messages WHERE status = 'queued'`).Scan(&sms, &email)
	if err != nil {
		return nil, fmt.Errorf("campaign: kuyruk derinliği okunamadı: %w", err)
	}
	return map[string]int{"sms": sms, "email": email}, nil
}
