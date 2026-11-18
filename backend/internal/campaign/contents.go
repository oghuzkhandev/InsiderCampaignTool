package campaign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
)

// İçerik sürümünün kaynakları.
const (
	SourceLLM   = "llm"
	SourceHuman = "human"
)

// ErrNotDraft, kampanya draft olmadığı için içerik sürümü yazılamadığını
// gösterir (onaylanmış, gönderilen veya tamamlanmış kampanya).
var ErrNotDraft = errors.New("campaign: kampanya draft değil")

// ContentVersion, kaydedilmiş doğrulanmış bir içerik sürümüdür. Satırlar
// değişmez (ARCHITECTURE §4).
type ContentVersion struct {
	Version   int
	Source    string
	Content   content.Content
	CreatedAt time.Time
}

// ContentDB, ContentStore'un ihtiyaç duyduğu veritabanı işlemleridir.
type ContentDB interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ContentStore, doğrulanmış içerik sürümlerini PostgreSQL'de saklar. Yalnızca
// internal/content doğrulayıcısından geçmiş içerik verilmelidir; veritabanı
// yalnızca temel bir ikinci savunma sağlar (migration 00002).
type ContentStore struct {
	db ContentDB
}

// NewContentStore, verilen veritabanıyla bir ContentStore oluşturur.
func NewContentStore(db ContentDB) *ContentStore { return &ContentStore{db: db} }

// Save, doğrulanmış içeriği kampanyanın yeni sürümü olarak kaydeder. Kampanya
// satırı FOR UPDATE ile kilitlenir ve durum kilit altında kontrol edilir:
// eşzamanlı onay varsa beklenir ve onaydan sonra ErrNotDraft döner. Sürüm
// numarası kilit altında max(version)+1 olarak verilir (kesintisiz 1..N).
// Kampanya yoksa veya kimlik geçerli bir uuid değilse ErrNotFound döner.
func (s *ContentStore) Save(ctx context.Context, campaignID, source string, c content.Content) (ContentVersion, error) {
	if source != SourceLLM && source != SourceHuman {
		return ContentVersion{}, fmt.Errorf("campaign: geçersiz içerik kaynağı %q", source)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ContentVersion{}, fmt.Errorf("campaign: transaction açılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM campaigns WHERE id = $1 FOR UPDATE`, campaignID).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err):
		return ContentVersion{}, ErrNotFound
	case err != nil:
		return ContentVersion{}, fmt.Errorf("campaign: kampanya kilitlenemedi: %w", err)
	}
	if status != StatusDraft {
		return ContentVersion{}, ErrNotDraft
	}

	v := ContentVersion{Source: source, Content: c}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO campaign_contents (campaign_id, version, source, sms_text, email_subject, email_body)
		 VALUES ($1, (SELECT coalesce(max(version), 0) + 1 FROM campaign_contents WHERE campaign_id = $1), $2, $3, $4, $5)
		 RETURNING version, created_at`,
		campaignID, source, c.SMSText, c.EmailSubject, c.EmailBody,
	).Scan(&v.Version, &v.CreatedAt)
	if err != nil {
		// Veritabanı tetikleyicisi (onaylanmış kampanyaya sürüm eklenemez) ikinci
		// savunmadır; uygulama kilidiyle aynı sonucu verir.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgRestrictViolation {
			return ContentVersion{}, ErrNotDraft
		}
		return ContentVersion{}, fmt.Errorf("campaign: içerik sürümü kaydedilemedi: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ContentVersion{}, fmt.Errorf("campaign: içerik sürümü onaylanamadı (commit): %w", err)
	}
	return v, nil
}

// Latest, kampanyanın son doğrulanmış içerik sürümünü döndürür. Sürüm yoksa
// (veya kampanya yoksa) ikinci değer false olur.
func (s *ContentStore) Latest(ctx context.Context, campaignID string) (ContentVersion, bool, error) {
	var v ContentVersion
	err := s.db.QueryRowContext(ctx,
		`SELECT version, source, sms_text, email_subject, email_body, created_at
		 FROM campaign_contents WHERE campaign_id = $1 ORDER BY version DESC LIMIT 1`, campaignID,
	).Scan(&v.Version, &v.Source, &v.Content.SMSText, &v.Content.EmailSubject, &v.Content.EmailBody, &v.CreatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err):
		return ContentVersion{}, false, nil
	case err != nil:
		return ContentVersion{}, false, fmt.Errorf("campaign: içerik sürümü okunamadı: %w", err)
	}
	return v, true, nil
}

// MaxFirstNameRunes, kullanıcılar arasındaki en uzun gerçek first_name'in kod
// noktası sayısını döndürür (kenar boşlukları atılır; ad yoksa 0). İçerik
// doğrulayıcısı {{first_name}} için bu uzunluğu en kötü durum olarak kullanır
// (ADR-022). Kapsam bilerek tüm kullanıcılardır: segmentten bağımsız,
// tutucu bir üst sınırdır. Onayda her alıcı için kesin kontrol yapılır.
func (s *ContentStore) MaxFirstNameRunes(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT coalesce(max(char_length(btrim(first_name))), 0) FROM users`,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("campaign: en uzun ad okunamadı: %w", err)
	}
	return n, nil
}

// pgRestrictViolation, tetikleyicilerin kullandığı restrict_violation SQLSTATE'idir.
const pgRestrictViolation = "23001"
