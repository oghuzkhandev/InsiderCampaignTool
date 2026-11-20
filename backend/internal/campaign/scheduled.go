package campaign

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ScheduledItem, onaylanmış ve gönderimi ileri bir zamana planlanmış, henüz başlamamış bir
// kampanyadır.
type ScheduledItem struct {
	ID          string
	Brief       string
	ScheduledAt time.Time
	ApprovedAt  time.Time
	// Queued, kampanyanın gönderilmeyi bekleyen (queued) mesaj sayısıdır.
	Queued int
	// Due, planlanan zamanın geldiği ama gönderimin henüz başlamadığı anlamına gelir (ör. worker
	// kapalı). İşler kaybolmaz; worker açılınca alınır.
	Due bool
}

// ScheduledPage, planlı kampanyaların bir sayfasıdır.
type ScheduledPage struct {
	Items []ScheduledItem
	Total int
	Page  int
	Limit int
	// ServerTime, sayfanın okunduğu andaki veritabanı saatidir. İstemci "ne kadar kaldı"
	// hesabını kendi saatine değil buna göre yapar; cihaz saati yanlış olsa da sayaç doğru kalır.
	ServerTime time.Time
}

// ListScheduled, gönderimi başlamamış planlı kampanyaları (durum `approved` ve `scheduled_at`
// dolu) en yakın zamanı başta olacak biçimde sayfalı okar. Toplam, sayfa ve sunucu saati tek
// ifadede, aynı anlık görüntüden gelir. Gönderim başlayınca (ilk mesaj sahiplenilince kampanya
// `sending` olur) kampanya listeden kendiliğinden çıkar. page 1'den başlar.
func (s *Store) ListScheduled(ctx context.Context, page, limit int) (ScheduledPage, error) {
	if page < 1 || limit < 1 {
		return ScheduledPage{}, ErrInvalidLimit
	}
	rows, err := s.db.QueryContext(ctx, `
WITH total AS (
  SELECT count(*) AS n FROM campaigns WHERE status = 'approved' AND scheduled_at IS NOT NULL
), page AS (
  SELECT c.id::text AS id, c.brief, c.scheduled_at, c.approved_at,
         (SELECT count(*) FROM messages m WHERE m.campaign_id = c.id AND m.status = 'queued') AS queued
  FROM campaigns c
  WHERE c.status = 'approved' AND c.scheduled_at IS NOT NULL
  ORDER BY c.scheduled_at, c.id
  LIMIT $1 OFFSET $2
)
SELECT total.n, now(), page.id, page.brief, page.scheduled_at, page.approved_at, page.queued,
       page.scheduled_at <= now()
FROM total LEFT JOIN page ON true
ORDER BY page.scheduled_at, page.id`, limit, (page-1)*limit)
	if err != nil {
		return ScheduledPage{}, fmt.Errorf("campaign: planlı kampanyalar okunamadı: %w", err)
	}
	defer rows.Close()

	out := ScheduledPage{Items: []ScheduledItem{}, Page: page, Limit: limit}
	for rows.Next() {
		var (
			id, brief sql.NullString
			scheduled sql.NullTime
			approved  sql.NullTime
			queued    sql.NullInt64
			due       sql.NullBool
		)
		if err := rows.Scan(&out.Total, &out.ServerTime, &id, &brief, &scheduled, &approved, &queued, &due); err != nil {
			return ScheduledPage{}, fmt.Errorf("campaign: planlı kampanya okunamadı: %w", err)
		}
		out.ServerTime = out.ServerTime.UTC()
		if !id.Valid {
			continue // sayfa boş: yalnızca toplam ve sunucu saati geldi
		}
		out.Items = append(out.Items, ScheduledItem{
			ID:          id.String,
			Brief:       brief.String,
			ScheduledAt: scheduled.Time.UTC(),
			ApprovedAt:  approved.Time.UTC(),
			Queued:      int(queued.Int64),
			Due:         due.Bool,
		})
	}
	if err := rows.Err(); err != nil {
		return ScheduledPage{}, fmt.Errorf("campaign: planlı kampanyalar okunamadı: %w", err)
	}
	return out, nil
}
