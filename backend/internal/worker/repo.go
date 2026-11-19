package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Precheck, provider çağrısından hemen önceki kontrolün sonucudur.
type Precheck struct {
	// Owned, mesajın hâlâ bu claim token'ına ait, `sending` durumunda ve lease'i
	// geçerli olduğunu gösterir.
	Owned bool
	// Consent, kullanıcının bu kanal için açık izni olduğunu gösterir. İzin kaydı
	// yoksa false'tur.
	Consent bool
}

// ReapResult, bir reaper turunun sonucudur.
type ReapResult struct {
	// Requeued, lease'i dolan ve deneme hakkı olduğu için yeniden kuyruğa alınanlar.
	Requeued int
	// Exhausted, deneme hakkı bittiği için `failed`/`exhausted` yapılanlar.
	Exhausted int
	// Campaigns, tamamlanma kontrolü gereken kampanyalar.
	Campaigns []string
}

// Repository, worker'ın veritabanı işlemleridir. Sonuç yazan her yöntem claim
// token ve `sending` durumunu koşul alır; eşleşmezse applied=false döner (eski
// claim sonucu, ARCHITECTURE §7).
type Repository interface {
	// Claim, kanal için en çok limit kadar vadesi gelmiş işi sahiplenir.
	Claim(ctx context.Context, channel string, limit, maxAttempts int) ([]Job, error)
	// Precheck, sahipliği, lease geçerliliğini ve kanal iznini tek sorguda okur.
	Precheck(ctx context.Context, job Job, lease time.Duration) (Precheck, error)
	MarkSent(ctx context.Context, job Job) (applied bool, err error)
	MarkFailed(ctx context.Context, job Job, reason, lastError string) (applied bool, err error)
	MarkSkipped(ctx context.Context, job Job, reason string) (applied bool, err error)
	Requeue(ctx context.Context, job Job, delay time.Duration, lastError string) (applied bool, err error)
	// CompleteCampaign, hiç `queued` veya `sending` mesajı kalmayan kampanyayı
	// tamamlar.
	CompleteCampaign(ctx context.Context, campaignID string) (completed bool, err error)
	// CompleteIdleCampaigns, mesajları zaten terminal olan ama hâlâ `approved` veya
	// `sending` görünen kampanyaları tamamlar ve kaç tane olduğunu döndürür. Sonuç
	// yazıldıktan sonra CompleteCampaign başarısız olduysa (veritabanı hatası) kampanya
	// takılı kalmasın diye reaper turlarında çağrılır.
	CompleteIdleCampaigns(ctx context.Context) (completed int, err error)
	// Reap, lease'i dolan `sending` işleri ve deneme hakkı biten işleri ele alır.
	Reap(ctx context.Context, lease time.Duration, maxAttempts int) (ReapResult, error)
}

// DB, PGRepo'nun ihtiyaç duyduğu veritabanı işlemleridir (*sql.DB sağlar).
type DB interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// PGRepo, Repository'nin PostgreSQL gerçeklemesidir.
type PGRepo struct {
	db DB
}

// NewPGRepo, verilen veritabanıyla bir PGRepo oluşturur.
func NewPGRepo(db DB) *PGRepo { return &PGRepo{db: db} }

// secs, süreyi make_interval(secs => ...) için saniyeye çevirir. Zaman
// hesapları veritabanı saatiyle yapılır; uygulama ve veritabanı saatleri
// arasındaki fark sonuçları etkilemez.
func secs(d time.Duration) float64 { return d.Seconds() }

const claimSQL = `
WITH picked AS (
    SELECT id
    FROM messages
    WHERE channel = $1
      AND status = 'queued'
      AND next_attempt_at <= now()
      AND attempts < $3
    ORDER BY next_attempt_at, id
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
UPDATE messages m
SET status = 'sending',
    claim_token = gen_random_uuid(),
    claimed_at = now(),
    attempts = m.attempts + 1
FROM picked
WHERE m.id = picked.id
RETURNING m.id, m.campaign_id::text, m.user_id, m.channel, m.recipient,
          m.claim_token::text, m.attempts, m.idempotency_key`

// Claim, vadesi gelmiş işleri kısa bir transaction'da sahiplenir. Provider
// çağrısı bu transaction'ın dışında yapılır (ADR-002). İlk mesajı sahiplenen
// worker kampanyayı `approved`'dan `sending`'e geçirir. Gönderilecek metin ve ad
// aynı transaction'da, kampanyanın onaylanan içerik sürümünden okunur.
func (r *PGRepo) Claim(ctx context.Context, channel string, limit, maxAttempts int) ([]Job, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("worker: claim transaction'ı açılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, claimSQL, channel, limit, maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("worker: claim sorgusu: %w", err)
	}
	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.CampaignID, &j.UserID, &j.Channel, &j.Recipient, &j.ClaimToken, &j.Attempts, &j.IdempotencyKey); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("worker: claim satırı okunamadı: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("worker: claim satırları okunamadı: %w", err)
	}
	_ = rows.Close()
	if len(jobs) == 0 {
		return nil, nil
	}

	ids := make([]int64, len(jobs))
	campaigns := map[string]struct{}{}
	for i, j := range jobs {
		ids[i] = j.ID
		campaigns[j.CampaignID] = struct{}{}
	}
	campaignIDs := make([]string, 0, len(campaigns))
	for id := range campaigns {
		campaignIDs = append(campaignIDs, id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaigns SET status = 'sending' WHERE id::text = ANY($1::text[]) AND status = 'approved'`,
		campaignIDs,
	); err != nil {
		return nil, fmt.Errorf("worker: kampanya sending'e geçirilemedi: %w", err)
	}

	if err := loadDetails(ctx, tx, jobs, ids); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("worker: claim tamamlanamadı (commit): %w", err)
	}
	return jobs, nil
}

// loadDetails, her işin alıcı adını ve kampanyanın onaylanan içerik sürümünü okur.
//
// Ad, mesajın KENDİ kaydından (messages.first_name) okunur; users tablosuna
// bakılmaz. Onay anında kopyalanan bu ad retry dahil her denemede aynıdır: kullanıcı
// adı sonradan değişse bile aynı idempotency anahtarıyla aynı içerik gider (ADR-026).
// Boş veya boşluk ad render sırasında fallback'e gider (content.Render).
func loadDetails(ctx context.Context, tx *sql.Tx, jobs []Job, ids []int64) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT m.id, m.first_name, cc.sms_text, cc.email_subject, cc.email_body
		 FROM messages m
		 JOIN campaigns c ON c.id = m.campaign_id
		 JOIN campaign_contents cc
		   ON cc.campaign_id = c.id AND cc.version = c.approved_content_version
		 WHERE m.id = ANY($1::bigint[])`, ids)
	if err != nil {
		return fmt.Errorf("worker: iş ayrıntıları okunamadı: %w", err)
	}
	defer rows.Close()

	byID := make(map[int64]int, len(jobs))
	for i, j := range jobs {
		byID[j.ID] = i
	}
	found := 0
	for rows.Next() {
		var (
			id        int64
			firstName string
			sms, subj string
			body      string
		)
		if err := rows.Scan(&id, &firstName, &sms, &subj, &body); err != nil {
			return fmt.Errorf("worker: iş ayrıntısı okunamadı: %w", err)
		}
		j := &jobs[byID[id]]
		j.FirstName = firstName
		j.Content.SMSText, j.Content.EmailSubject, j.Content.EmailBody = sms, subj, body
		found++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("worker: iş ayrıntıları okunamadı: %w", err)
	}
	if found != len(jobs) {
		// Yabancı anahtarlar ve onay kısıtları bunu engeller; yine de eksik içerikle
		// gönderim yapmamak için claim geri alınır.
		return fmt.Errorf("worker: %d işin onaylı içeriği bulunamadı", len(jobs)-found)
	}
	return nil
}

// Precheck, provider çağrısından hemen önce çağrılır.
func (r *PGRepo) Precheck(ctx context.Context, job Job, lease time.Duration) (Precheck, error) {
	var p Precheck
	err := r.db.QueryRowContext(ctx,
		`SELECT m.claim_token = $2::uuid AND m.status = 'sending'
		        AND m.claimed_at > now() - make_interval(secs => $3::double precision),
		        coalesce(c.granted, false)
		 FROM messages m
		 LEFT JOIN user_consents c ON c.user_id = m.user_id AND c.channel = m.channel
		 WHERE m.id = $1`,
		job.ID, job.ClaimToken, secs(lease),
	).Scan(&p.Owned, &p.Consent)
	if errors.Is(err, sql.ErrNoRows) {
		return Precheck{}, nil
	}
	if err != nil {
		return Precheck{}, fmt.Errorf("worker: ön kontrol okunamadı: %w", err)
	}
	return p, nil
}

// resultWhere, her sonuç yazımının ortak koşuludur: yalnızca güncel claim token
// ve `sending` durumu eşleşirse satır güncellenir.
const resultWhere = ` WHERE id = $1 AND claim_token = $2::uuid AND status = 'sending'`

func (r *PGRepo) write(ctx context.Context, what, query string, args ...any) (bool, error) {
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("worker: %s yazılamadı: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("worker: %s sonucu okunamadı: %w", what, err)
	}
	return n == 1, nil
}

// MarkSent, mesajı `sent` yapar.
func (r *PGRepo) MarkSent(ctx context.Context, job Job) (bool, error) {
	return r.write(ctx, "sent",
		`UPDATE messages SET status = 'sent', sent_at = now()`+resultWhere,
		job.ID, job.ClaimToken)
}

// MarkFailed, mesajı `failed` yapar ve failure_reason'ı yazar.
func (r *PGRepo) MarkFailed(ctx context.Context, job Job, reason, lastError string) (bool, error) {
	return r.write(ctx, "failed",
		`UPDATE messages SET status = 'failed', failure_reason = $3, last_error = $4`+resultWhere,
		job.ID, job.ClaimToken, reason, lastError)
}

// MarkSkipped, izin olmadığı için mesajı `skipped` yapar.
func (r *PGRepo) MarkSkipped(ctx context.Context, job Job, reason string) (bool, error) {
	return r.write(ctx, "skipped",
		`UPDATE messages SET status = 'skipped', last_error = $3`+resultWhere,
		job.ID, job.ClaimToken, reason)
}

// Requeue, mesajı delay sonrasına planlanmış şekilde `queued` yapar.
func (r *PGRepo) Requeue(ctx context.Context, job Job, delay time.Duration, lastError string) (bool, error) {
	return r.write(ctx, "retry",
		`UPDATE messages SET status = 'queued', last_error = $4,
		        next_attempt_at = now() + make_interval(secs => $3::double precision)`+resultWhere,
		job.ID, job.ClaimToken, secs(delay), lastError)
}

// CompleteCampaign, tüm mesajları terminal durumda olan kampanyayı tamamlar.
// Başarısız veya atlanan mesajlar tamamlanmayı engellemez; retry bekleyen
// (`queued`) veya `sending` mesaj engeller (ARCHITECTURE §7).
func (r *PGRepo) CompleteCampaign(ctx context.Context, campaignID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE campaigns c
		 SET status = 'completed', completed_at = now()
		 WHERE c.id = $1::uuid
		   AND c.status IN ('approved', 'sending')
		   AND NOT EXISTS (
		       SELECT 1 FROM messages m
		       WHERE m.campaign_id = c.id AND m.status IN ('queued', 'sending')
		   )`, campaignID)
	if err != nil {
		return false, fmt.Errorf("worker: kampanya tamamlanamadı: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("worker: kampanya tamamlama sonucu okunamadı: %w", err)
	}
	return n == 1, nil
}

// CompleteIdleCampaigns, CompleteCampaign ile aynı kuralı (hiç `queued` veya
// `sending` mesaj kalmadı) bütün kampanyalara uygular. En az bir mesajı olmayan
// kampanya tamamlanmaz: mesajsız `approved` kampanya "tamamlandı" sayılmamalı.
func (r *PGRepo) CompleteIdleCampaigns(ctx context.Context) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE campaigns c
		 SET status = 'completed', completed_at = now()
		 WHERE c.status IN ('approved', 'sending')
		   AND EXISTS (SELECT 1 FROM messages m WHERE m.campaign_id = c.id)
		   AND NOT EXISTS (
		       SELECT 1 FROM messages m
		       WHERE m.campaign_id = c.id AND m.status IN ('queued', 'sending')
		   )`)
	if err != nil {
		return 0, fmt.Errorf("worker: takılı kampanyalar tamamlanamadı: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("worker: takılı kampanya sonucu okunamadı: %w", err)
	}
	return int(n), nil
}

// Reap, üç durumu ele alır (ARCHITECTURE §3 ve §8):
//
//   - lease'i dolmuş `sending` iş, deneme hakkı varsa → `queued` (hemen),
//   - lease'i dolmuş `sending` iş, deneme hakkı bittiyse → `failed`/`exhausted`,
//   - `queued` ama deneme hakkı bitmiş iş (örneğin azami değer düşürüldüyse
//     claim bunu hiç almaz) → `failed`/`exhausted`.
func (r *PGRepo) Reap(ctx context.Context, lease time.Duration, maxAttempts int) (ReapResult, error) {
	var res ReapResult
	seen := map[string]struct{}{}
	collect := func(rows *sql.Rows) (int, error) {
		defer rows.Close()
		n := 0
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return n, err
			}
			seen[id] = struct{}{}
			n++
		}
		return n, rows.Err()
	}

	rows, err := r.db.QueryContext(ctx,
		`UPDATE messages SET status = 'queued', next_attempt_at = now(), last_error = 'lease doldu'
		 WHERE status = 'sending'
		   AND claimed_at < now() - make_interval(secs => $1::double precision)
		   AND attempts < $2
		 RETURNING campaign_id::text`, secs(lease), maxAttempts)
	if err != nil {
		return res, fmt.Errorf("worker: reaper (yeniden kuyruğa alma): %w", err)
	}
	if res.Requeued, err = collect(rows); err != nil {
		return res, fmt.Errorf("worker: reaper (yeniden kuyruğa alma): %w", err)
	}

	rows, err = r.db.QueryContext(ctx,
		`UPDATE messages SET status = 'failed', failure_reason = $3, last_error = 'lease doldu, deneme sınırı doldu'
		 WHERE (status = 'sending' AND claimed_at < now() - make_interval(secs => $1::double precision)
		        AND attempts >= $2)
		    OR (status = 'queued' AND attempts >= $2)
		 RETURNING campaign_id::text`, secs(lease), maxAttempts, FailureExhausted)
	if err != nil {
		return res, fmt.Errorf("worker: reaper (deneme sınırı): %w", err)
	}
	if res.Exhausted, err = collect(rows); err != nil {
		return res, fmt.Errorf("worker: reaper (deneme sınırı): %w", err)
	}

	for id := range seen {
		res.Campaigns = append(res.Campaigns, id)
	}
	return res, nil
}
