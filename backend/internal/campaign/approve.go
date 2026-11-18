package campaign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// Onay hataları. HTTP katmanı bunları durum kodlarına çevirir (ADR-011).
var (
	// ErrNoContent: kampanyanın doğrulanmış hiç içerik sürümü yok. (409)
	ErrNoContent = errors.New("campaign: doğrulanmış içerik sürümü yok")
	// ErrStaleVersion: istenen sürüm son doğrulanmış sürüm değil. (409)
	ErrStaleVersion = errors.New("campaign: içerik sürümü güncel değil")
	// ErrApprovedWithOtherVersion: kampanya başka bir sürümle zaten onaylı. (409)
	ErrApprovedWithOtherVersion = errors.New("campaign: kampanya başka bir sürümle onaylı")
	// ErrUnfilteredSegment: segment koşul içermiyor; filtresiz kitle onaylanmaz. (422)
	ErrUnfilteredSegment = errors.New("campaign: segment filtresiz")
	// ErrNoRecipients: hiçbir kanalda uygun alıcı yok. (422)
	ErrNoRecipients = errors.New("campaign: uygun alıcı yok")
)

// FieldOverflow, bir alanın alıcıya özel render sonrası sınırı aştığı alıcıları
// özetler: kaç alıcı ve bir örnek ad (render'da görünen ad).
type FieldOverflow struct {
	Field   string
	Count   int
	Example string
}

// RenderLimitError, en az bir alıcı için render edilmiş metin sınırı aşınca
// döner. Kampanya draft kalır ve hiçbir iş oluşmaz. (422)
type RenderLimitError struct {
	Overflows []FieldOverflow
}

func (e *RenderLimitError) Error() string {
	return "campaign: render sonrası uzunluk sınırı aşıldı"
}

// ChannelCounts, kanal bazında sayılardır.
type ChannelCounts struct {
	SMS   int
	Email int
}

// Total, iki kanalın toplamıdır (alıcı değil mesaj sayısı).
func (c ChannelCounts) Total() int { return c.SMS + c.Email }

// ApprovalResult, bir onayın sonucudur.
type ApprovalResult struct {
	CampaignID     string
	Status         string
	ContentVersion int
	ApprovedAt     time.Time
	// Queued, bu kampanya için oluşturulmuş (queued) mesaj sayılarıdır.
	Queued ChannelCounts
	// Replayed, kampanya bu sürümle zaten onaylıydı ve mevcut sonuç döndü.
	Replayed bool
}

// Approver, kampanyayı insan onayıyla onaylar ve gönderim işlerini yazar.
type Approver struct {
	db ContentDB
}

// NewApprover, verilen veritabanıyla bir Approver oluşturur.
func NewApprover(db ContentDB) *Approver { return &Approver{db: db} }

// Approve, kampanyayı verilen içerik sürümüyle onaylar (ARCHITECTURE §6).
//
// Her şey tek transaction'dadır: kampanya satırı FOR UPDATE ile kilitlenir;
// onay, onaylanan sürüm ve `queued` mesaj satırları birlikte commit edilir ya
// da hiçbiri yazılmaz.
//
//   - Kampanya draft değilse alıcılar yeniden hesaplanmaz: aynı sürümle
//     tekrar mevcut sonucu (Replayed), başka sürümle ErrApprovedWithOtherVersion döner.
//   - Draft ise istenen sürüm son doğrulanmış sürüm olmalıdır (ErrNoContent,
//     ErrStaleVersion), segment filtreli olmalıdır (ErrUnfilteredSegment).
//   - Uygun alıcılar kanal bazında tek sorguyla bir kez seçilir; boşsa
//     ErrNoRecipients. Her alıcı için yalnızca kendi kanalının metni render
//     edilir ve doğrulanır; biri bile sınırı aşarsa *RenderLimitError.
//   - Mesaj satırları aynı alıcı listesinden oluşturulur; ikinci kitle sorgusu
//     çalışmaz. now, segmentin tarih sınırları için referans zamandır.
//
// Uygun alıcı, ilgili kanal için açık izni VE adresi (telefon/e-posta) olan
// kişidir (ADR-023); bu kural kitle sayılarıyla aynıdır, bu yüzden kuyruk sayısı
// önizlemeyle uyuşur (aradaki fark yalnızca önizleme ile onay arasındaki
// veri/zaman değişimidir).
func (a *Approver) Approve(ctx context.Context, campaignID string, version int, now time.Time) (ApprovalResult, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("campaign: transaction açılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status          string
		approvedVersion sql.NullInt32
		approvedAt      sql.NullTime
		rawSegment      []byte
	)
	err = tx.QueryRowContext(ctx,
		`SELECT status, approved_content_version, approved_at, segment FROM campaigns WHERE id = $1 FOR UPDATE`,
		campaignID,
	).Scan(&status, &approvedVersion, &approvedAt, &rawSegment)
	switch {
	case errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err):
		return ApprovalResult{}, ErrNotFound
	case err != nil:
		return ApprovalResult{}, fmt.Errorf("campaign: kampanya kilitlenemedi: %w", err)
	}

	// Draft olmayan kampanya: alıcıları yeniden hesaplama, yalnızca mevcut sonucu ver.
	if status != StatusDraft {
		if !approvedVersion.Valid || int(approvedVersion.Int32) != version {
			return ApprovalResult{}, ErrApprovedWithOtherVersion
		}
		queued, err := queuedCounts(ctx, tx, campaignID)
		if err != nil {
			return ApprovalResult{}, err
		}
		return ApprovalResult{
			CampaignID: campaignID, Status: status, ContentVersion: version,
			ApprovedAt: approvedAt.Time.UTC(), Queued: queued, Replayed: true,
		}, nil
	}

	var latest sql.NullInt32
	if err := tx.QueryRowContext(ctx, `SELECT max(version) FROM campaign_contents WHERE campaign_id = $1`, campaignID).Scan(&latest); err != nil {
		return ApprovalResult{}, fmt.Errorf("campaign: son sürüm okunamadı: %w", err)
	}
	if !latest.Valid {
		return ApprovalResult{}, ErrNoContent
	}
	if int(latest.Int32) != version {
		return ApprovalResult{}, ErrStaleVersion
	}

	seg, err := segment.ParseStored(rawSegment)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("campaign: saklanan segment geçersiz: %w", err)
	}
	if len(seg.Conditions) == 0 {
		return ApprovalResult{}, ErrUnfilteredSegment
	}

	var text content.Content
	if err := tx.QueryRowContext(ctx,
		`SELECT sms_text, email_subject, email_body FROM campaign_contents WHERE campaign_id = $1 AND version = $2`,
		campaignID, version,
	).Scan(&text.SMSText, &text.EmailSubject, &text.EmailBody); err != nil {
		return ApprovalResult{}, fmt.Errorf("campaign: içerik okunamadı: %w", err)
	}

	// Uygun alıcılar kanal bazında bir kez seçilir.
	recipients, err := audience.Recipients(ctx, tx, seg, now)
	if err != nil {
		return ApprovalResult{}, err
	}
	var (
		sms, email  recipientBatch
		smsOver     = FieldOverflow{Field: content.FieldSMSText}
		subjectOver = FieldOverflow{Field: content.FieldEmailSubject}
	)
	for _, r := range recipients {
		if phone, ok := r.SMSAddress(); ok {
			sms.add(r.UserID, phone)
			if len(content.CheckSMS(text, r.FirstName)) > 0 {
				smsOver.note(content.DisplayName(r.FirstName))
			}
		}
		if addr, ok := r.EmailAddress(); ok {
			email.add(r.UserID, addr)
			if len(content.CheckSubject(text, r.FirstName)) > 0 {
				subjectOver.note(content.DisplayName(r.FirstName))
			}
		}
	}
	if sms.len()+email.len() == 0 {
		return ApprovalResult{}, ErrNoRecipients
	}
	if smsOver.Count > 0 || subjectOver.Count > 0 {
		rle := &RenderLimitError{}
		for _, o := range []FieldOverflow{smsOver, subjectOver} {
			if o.Count > 0 {
				rle.Overflows = append(rle.Overflows, o)
			}
		}
		return ApprovalResult{}, rle
	}

	var approvedTime time.Time
	if err := tx.QueryRowContext(ctx,
		`UPDATE campaigns SET status = 'approved', approved_content_version = $2, approved_at = now()
		 WHERE id = $1 AND status = 'draft' RETURNING approved_at`,
		campaignID, version,
	).Scan(&approvedTime); err != nil {
		return ApprovalResult{}, fmt.Errorf("campaign: kampanya onaylanamadı: %w", err)
	}

	for _, b := range []struct {
		channel string
		batch   recipientBatch
	}{{"sms", sms}, {"email", email}} {
		if b.batch.len() == 0 {
			continue
		}
		if err := insertMessages(ctx, tx, campaignID, b.channel, b.batch); err != nil {
			return ApprovalResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return ApprovalResult{}, fmt.Errorf("campaign: onay tamamlanamadı (commit): %w", err)
	}
	return ApprovalResult{
		CampaignID: campaignID, Status: "approved", ContentVersion: version,
		ApprovedAt: approvedTime.UTC(),
		Queued:     ChannelCounts{SMS: sms.len(), Email: email.len()},
	}, nil
}

// recipientBatch, bir kanal için mesaj satırlarına dönüşecek alıcılardır.
type recipientBatch struct {
	userIDs    []int64
	recipients []string
}

func (b *recipientBatch) add(userID int64, recipient string) {
	b.userIDs = append(b.userIDs, userID)
	b.recipients = append(b.recipients, recipient)
}

func (b recipientBatch) len() int { return len(b.userIDs) }

// note, sınırı aşan bir alıcıyı sayar; ilk alıcının adı örnek olarak saklanır.
func (o *FieldOverflow) note(example string) {
	if o.Count == 0 {
		o.Example = example
	}
	o.Count++
}

// insertMessages, bir kanalın mesaj satırlarını tek ifadede `queued` olarak
// yazar. idempotency_key sabit biçimdedir: kampanya:kullanıcı:kanal. Çakışma
// ON CONFLICT DO NOTHING ile tekilleştirilir (draft kampanyada beklenmez).
func insertMessages(ctx context.Context, tx *sql.Tx, campaignID, channel string, b recipientBatch) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, idempotency_key)
		 SELECT $1, t.user_id, $2::text, t.recipient, $5::text || ':' || t.user_id::text || ':' || $2::text
		 FROM unnest($3::bigint[], $4::text[]) AS t(user_id, recipient)
		 ON CONFLICT DO NOTHING`,
		campaignID, channel, b.userIDs, b.recipients, campaignID,
	)
	if err != nil {
		return fmt.Errorf("campaign: %s mesajları yazılamadı: %w", channel, err)
	}
	return nil
}

// queuedCounts, kampanyanın kanal bazında mesaj satırı sayılarını okur.
func queuedCounts(ctx context.Context, tx *sql.Tx, campaignID string) (ChannelCounts, error) {
	rows, err := tx.QueryContext(ctx, `SELECT channel, count(*) FROM messages WHERE campaign_id = $1 GROUP BY channel`, campaignID)
	if err != nil {
		return ChannelCounts{}, fmt.Errorf("campaign: mesaj sayıları okunamadı: %w", err)
	}
	defer rows.Close()
	var c ChannelCounts
	for rows.Next() {
		var channel string
		var n int
		if err := rows.Scan(&channel, &n); err != nil {
			return ChannelCounts{}, fmt.Errorf("campaign: mesaj sayısı okunamadı: %w", err)
		}
		switch channel {
		case "sms":
			c.SMS = n
		case "email":
			c.Email = n
		}
	}
	if err := rows.Err(); err != nil {
		return ChannelCounts{}, fmt.Errorf("campaign: mesaj sayıları okunamadı: %w", err)
	}
	return c, nil
}
