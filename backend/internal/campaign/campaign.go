// Package campaign, draft kampanyayı PostgreSQL'e kaydeder ve kimliğiyle okur.
package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// StatusDraft, yeni kampanyanın durumudur.
const StatusDraft = "draft"

// ErrNotFound, kampanya bulunamadığında veya kimlik geçerli bir uuid
// olmadığında döner.
var ErrNotFound = errors.New("campaign: bulunamadı")

// Campaign, kaydedilmiş bir kampanyadır.
type Campaign struct {
	ID        string
	Brief     string
	Segment   segment.Segment
	Status    string
	Audience  audience.Counts
	CreatedAt time.Time
	// ApprovedContentVersion ve ApprovedAt, kampanya onaylandıysa dolu olur.
	ApprovedContentVersion *int
	ApprovedAt             *time.Time
}

// Kampanya listesinin sınırları (CMP-001): varsayılan 10, en fazla 20.
const (
	DefaultListLimit = 10
	MaxListLimit     = 20
)

// ErrInvalidLimit, liste sınırı 1 ile MaxListLimit arasında olmadığında döner.
var ErrInvalidLimit = errors.New("campaign: liste sınırı geçersiz")

// Summary, kampanya listesindeki bir satırdır: kampanyayı tanıtmaya ve doğru ekrana
// götürmeye yetecek kadar bilgi. Segment, içerik ve kitle sayıları bilerek yoktur.
type Summary struct {
	ID        string
	Brief     string
	Status    string
	CreatedAt time.Time
}

// DB, Store'un ihtiyaç duyduğu veritabanı işlemleridir.
type DB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Store, kampanyaları PostgreSQL'de saklar.
type Store struct {
	db DB
}

// NewStore, verilen veritabanıyla bir Store oluşturur.
func NewStore(db DB) *Store { return &Store{db: db} }

// Create, doğrulanmış segmenti ve kitle sayılarıyla draft bir kampanya kaydeder.
// Segment, modelin ham metni değil doğrulanmış yapı olarak yazılır.
func (s *Store) Create(ctx context.Context, brief string, seg segment.Segment, counts audience.Counts) (Campaign, error) {
	stored, err := json.Marshal(seg)
	if err != nil {
		return Campaign{}, fmt.Errorf("campaign: segment saklanamaz: %w", err)
	}

	c := Campaign{Brief: brief, Segment: seg, Status: StatusDraft, Audience: counts}
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email)
		 VALUES ($1, $2::jsonb, $3, $4, $5)
		 RETURNING id, created_at`,
		brief, string(stored), counts.Total, counts.SMS, counts.Email,
	).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return Campaign{}, fmt.Errorf("campaign: kaydedilemedi: %w", err)
	}
	return c, nil
}

// Get, kampanyayı kimliğiyle okur. Bulunamazsa ErrNotFound döner.
func (s *Store) Get(ctx context.Context, id string) (Campaign, error) {
	var (
		c   Campaign
		raw []byte
	)
	var (
		approvedVersion sql.NullInt32
		approvedAt      sql.NullTime
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, brief, segment, status, audience_total, audience_sms, audience_email, created_at,
		        approved_content_version, approved_at
		 FROM campaigns WHERE id = $1`, id,
	).Scan(&c.ID, &c.Brief, &raw, &c.Status, &c.Audience.Total, &c.Audience.SMS, &c.Audience.Email, &c.CreatedAt,
		&approvedVersion, &approvedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err) {
			return Campaign{}, ErrNotFound
		}
		return Campaign{}, fmt.Errorf("campaign: okunamadı: %w", err)
	}

	if approvedVersion.Valid {
		v := int(approvedVersion.Int32)
		c.ApprovedContentVersion = &v
	}
	if approvedAt.Valid {
		t := approvedAt.Time.UTC()
		c.ApprovedAt = &t
	}

	c.Segment, err = segment.ParseStored(raw)
	if err != nil {
		return Campaign{}, fmt.Errorf("campaign: saklanan segment geçersiz: %w", err)
	}
	return c, nil
}

// List, en yeni kampanyayı başa alarak en çok limit kampanya döndürür. Sıralama
// oluşturulma zamanına göredir; aynı anda oluşanlar kimliğe göre ayrılır, böylece sıra
// her çağrıda aynıdır. Hiç kampanya yoksa boş (nil olmayan) liste döner. Limit 1 ile
// MaxListLimit arasında olmalıdır, aksi halde ErrInvalidLimit döner.
func (s *Store) List(ctx context.Context, limit int) ([]Summary, error) {
	if limit < 1 || limit > MaxListLimit {
		return nil, ErrInvalidLimit
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, brief, status, created_at
		 FROM campaigns
		 ORDER BY created_at DESC, id DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("campaign: liste okunamadı: %w", err)
	}
	defer rows.Close()

	list := make([]Summary, 0, limit)
	for rows.Next() {
		var c Summary
		if err := rows.Scan(&c.ID, &c.Brief, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("campaign: liste satırı okunamadı: %w", err)
		}
		c.CreatedAt = c.CreatedAt.UTC()
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("campaign: liste okunamadı: %w", err)
	}
	return list, nil
}

// isInvalidUUID, "uuid biçimi geçersiz" (22P02) hatasını tanır.
func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}
