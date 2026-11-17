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
}

// DB, Store'un ihtiyaç duyduğu veritabanı işlemleridir.
type DB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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
	err := s.db.QueryRowContext(ctx,
		`SELECT id, brief, segment, status, audience_total, audience_sms, audience_email, created_at
		 FROM campaigns WHERE id = $1`, id,
	).Scan(&c.ID, &c.Brief, &raw, &c.Status, &c.Audience.Total, &c.Audience.SMS, &c.Audience.Email, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isInvalidUUID(err) {
			return Campaign{}, ErrNotFound
		}
		return Campaign{}, fmt.Errorf("campaign: okunamadı: %w", err)
	}

	c.Segment, err = segment.ParseStored(raw)
	if err != nil {
		return Campaign{}, fmt.Errorf("campaign: saklanan segment geçersiz: %w", err)
	}
	return c, nil
}

// isInvalidUUID, "uuid biçimi geçersiz" (22P02) hatasını tanır.
func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}
