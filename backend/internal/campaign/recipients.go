package campaign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Alıcı listesi süzgeçlerinin geçerli değerleri. Mesaj durumu ve kanal, messages tablosundaki
// CHECK kısıtlarıyla aynıdır.
var (
	recipientChannels = map[string]bool{"sms": true, "email": true}
	recipientStatuses = map[string]bool{"queued": true, "sending": true, "sent": true, "failed": true, "skipped": true}
)

// ErrInvalidRecipientFilter, bilinmeyen kanal veya durum süzgecidir.
var ErrInvalidRecipientFilter = errors.New("campaign: alıcı süzgeci geçersiz")

// RecipientFilter, kampanyanın alıcı listesini daraltır. Boş alan o süzgeci uygulamaz.
type RecipientFilter struct {
	// Channel: sms veya email.
	Channel string
	// Status: queued, sending, sent, failed veya skipped.
	Status string
	// Search: ad, e-posta veya telefonda büyük/küçük harf gözetmeden aranan metin (LIKE joker
	// karakteri değil, düz metindir).
	Search string
}

// Recipient, kampanyanın bir mesajının alıcısıdır: kim, hangi kanaldan, hangi adrese ve ne oldu.
// Adres onay anındaki kopyadır (messages.recipient); sonradan kullanıcı kaydı değişse de değişmez.
type Recipient struct {
	MessageID int64
	UserID    int64
	// FirstName, onay anında mesaja kopyalanan addır; yoksa nil.
	FirstName     *string
	Channel       string
	Address       string
	Status        string
	Attempts      int
	SentAt        *time.Time
	FailureReason *string
}

// RecipientPage, alıcı listesinin bir sayfasıdır.
type RecipientPage struct {
	Items []Recipient
	// Total, süzgece uyan mesaj sayısıdır.
	Total int
}

// ListRecipients, kampanyanın mesajlarını alıcılarıyla sayfalı okur (salt okunur). Toplam ve sayfa
// tek ifadede, aynı anlık görüntüden gelir. Sıra: önce ad (yoksa adres), sonra kanal, sonra mesaj
// kimliği; böylece aynı kişinin SMS ve e-postası yan yana durur ve sıra kararlıdır. Kampanya yoksa
// (geçersiz uuid dahil) ErrNotFound döner. page 1'den başlar.
func (s *Store) ListRecipients(ctx context.Context, id string, f RecipientFilter, page, limit int) (RecipientPage, error) {
	if page < 1 || limit < 1 {
		return RecipientPage{}, ErrInvalidLimit
	}
	if (f.Channel != "" && !recipientChannels[f.Channel]) || (f.Status != "" && !recipientStatuses[f.Status]) {
		return RecipientPage{}, ErrInvalidRecipientFilter
	}

	// Kampanyanın varlığı ayrı okunur: boş bir liste ile "kampanya yok" ayırt edilebilsin.
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM campaigns WHERE id = $1)`, id).Scan(&exists); err != nil {
		return RecipientPage{}, recipientReadError(err)
	}
	if !exists {
		return RecipientPage{}, ErrNotFound
	}

	rows, err := s.db.QueryContext(ctx, `
WITH matched AS (
  SELECT m.id, m.user_id, m.first_name, m.channel, m.recipient, m.status, m.attempts, m.sent_at, m.failure_reason
  FROM messages m
  WHERE m.campaign_id = $1
    AND ($2 = '' OR m.channel = $2)
    AND ($3 = '' OR m.status = $3)
    AND ($4 = '' OR strpos(lower(m.first_name || ' ' || m.recipient), lower($4)) > 0)
), total AS (
  SELECT count(*) AS n FROM matched
), page AS (
  SELECT * FROM matched
  ORDER BY lower(coalesce(nullif(btrim(first_name), ''), recipient)), channel, id
  LIMIT $5 OFFSET $6
)
SELECT total.n, page.id, page.user_id, page.first_name, page.channel, page.recipient,
       page.status, page.attempts, page.sent_at, page.failure_reason
FROM total LEFT JOIN page ON true
ORDER BY lower(coalesce(nullif(btrim(page.first_name), ''), page.recipient)), page.channel, page.id`,
		id, f.Channel, f.Status, f.Search, limit, (page-1)*limit)
	if err != nil {
		return RecipientPage{}, recipientReadError(err)
	}
	defer rows.Close()

	out := RecipientPage{Items: []Recipient{}}
	for rows.Next() {
		var (
			messageID, userID sql.NullInt64
			first, reason     sql.NullString
			channel, address  sql.NullString
			status            sql.NullString
			attempts          sql.NullInt64
			sent              sql.NullTime
		)
		if err := rows.Scan(&out.Total, &messageID, &userID, &first, &channel, &address,
			&status, &attempts, &sent, &reason); err != nil {
			return RecipientPage{}, recipientReadError(err)
		}
		if !messageID.Valid {
			continue // sayfa boş: yalnızca toplam geldi
		}
		r := Recipient{
			MessageID: messageID.Int64, UserID: userID.Int64, Channel: channel.String,
			Address: address.String, Status: status.String, Attempts: int(attempts.Int64),
		}
		// Boş veya yalnızca boşluktan oluşan ad "adsız"dır; arayüze sahte değer gitmez.
		if first.Valid && strings.TrimSpace(first.String) != "" {
			name := first.String
			r.FirstName = &name
		}
		if sent.Valid {
			at := sent.Time.UTC()
			r.SentAt = &at
		}
		if reason.Valid {
			v := reason.String
			r.FailureReason = &v
		}
		out.Items = append(out.Items, r)
	}
	if err := rows.Err(); err != nil {
		return RecipientPage{}, recipientReadError(err)
	}
	return out, nil
}

func recipientReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err) {
		return ErrNotFound
	}
	return fmt.Errorf("campaign: alıcılar okunamadı: %w", err)
}
