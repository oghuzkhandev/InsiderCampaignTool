// Package templates stores reusable SMS and email text independently of campaign approvals.
package templates

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
)

var ErrNotFound = errors.New("templates: bulunamadı")

type Template struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	content.Content
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DB interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type Store struct{ db DB }

func NewStore(db DB) *Store { return &Store{db: db} }

const columns = `id::text, name, sms_text, email_subject, email_body, created_at, updated_at`

func (s *Store) List(ctx context.Context, page, limit int) ([]Template, int, error) {
	rows, err := s.db.QueryContext(ctx, `WITH total AS (SELECT count(*) AS n FROM content_templates),
	page AS (SELECT `+columns+` FROM content_templates ORDER BY updated_at DESC, id DESC LIMIT $1 OFFSET $2)
	SELECT total.n, page.* FROM total LEFT JOIN page ON true ORDER BY page.updated_at DESC, page.id DESC`, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, fmt.Errorf("templates: liste: %w", err)
	}
	defer rows.Close()
	items := []Template{}
	total := 0
	for rows.Next() {
		var id, name, sms, subject, body sql.NullString
		var created, updated sql.NullTime
		if err := rows.Scan(&total, &id, &name, &sms, &subject, &body, &created, &updated); err != nil {
			return nil, 0, err
		}
		if id.Valid {
			items = append(items, Template{ID: id.String, Name: name.String, Content: content.Content{SMSText: sms.String, EmailSubject: subject.String, EmailBody: body.String}, CreatedAt: created.Time, UpdatedAt: updated.Time})
		}
	}
	return items, total, rows.Err()
}

func (s *Store) Save(ctx context.Context, id, name string, c content.Content) (Template, error) {
	query := `INSERT INTO content_templates (name,sms_text,email_subject,email_body) VALUES ($1,$2,$3,$4) RETURNING ` + columns
	args := []any{name, c.SMSText, c.EmailSubject, c.EmailBody}
	if id != "" {
		query = `UPDATE content_templates SET name=$1,sms_text=$2,email_subject=$3,email_body=$4,updated_at=now() WHERE id=$5 RETURNING ` + columns
		args = append(args, id)
	}
	var t Template
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&t.ID, &t.Name, &t.SMSText, &t.EmailSubject, &t.EmailBody, &t.CreatedAt, &t.UpdatedAt)
	return t, mapError(err)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM content_templates WHERE id=$1`, id)
	if err != nil {
		return mapError(err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func mapError(err error) error {
	var pg *pgconn.PgError
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &pg) && pg.Code == "22P02") {
		return ErrNotFound
	}
	return err
}
