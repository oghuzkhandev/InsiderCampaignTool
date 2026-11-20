package campaign

import (
	"context"
	"database/sql"
	"time"
)

type Page struct {
	Campaigns []PageItem `json:"campaigns"`
	Total     int        `json:"total"`
	Page      int        `json:"page"`
	Limit     int        `json:"limit"`
}

type PageItem struct {
	ID        string    `json:"id"`
	Brief     string    `json:"brief"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ListPage reads the count and page from the same statement snapshot, including empty pages.
func (s *Store) ListPage(ctx context.Context, page, limit int, status string) (Page, error) {
	rows, err := s.db.QueryContext(ctx, `WITH total AS (SELECT count(*) AS n FROM campaigns WHERE ($1='' OR status=$1)),
	page AS (SELECT id,brief,status,created_at FROM campaigns WHERE ($1='' OR status=$1)
	ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3)
	SELECT total.n,page.id::text,page.brief,page.status,page.created_at FROM total LEFT JOIN page ON true
	ORDER BY page.created_at DESC,page.id DESC`, status, limit, (page-1)*limit)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	out := Page{Page: page, Limit: limit, Campaigns: []PageItem{}}
	for rows.Next() {
		var id, brief, state sql.NullString
		var created sql.NullTime
		if err := rows.Scan(&out.Total, &id, &brief, &state, &created); err != nil {
			return Page{}, err
		}
		if id.Valid {
			out.Campaigns = append(out.Campaigns, PageItem{ID: id.String, Brief: brief.String, Status: state.String, CreatedAt: created.Time})
		}
	}
	return out, rows.Err()
}

type Overview struct {
	Campaigns int `json:"campaigns"`
	Draft     int `json:"draft"`
	Approved  int `json:"approved"`
	Sending   int `json:"sending"`
	Completed int `json:"completed"`
	Users     int `json:"users"`
	Sent      int `json:"sent"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
	Recovered int `json:"recovered"`
	Templates int `json:"templates"`
}

func (s *Store) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	err := s.db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER (WHERE status='draft'),
	count(*) FILTER (WHERE status='approved'),count(*) FILTER (WHERE status='sending'),count(*) FILTER (WHERE status='completed'),
	(SELECT count(*) FROM users),(SELECT count(*) FROM messages WHERE status='sent'),
	(SELECT count(*) FROM messages WHERE status='failed'),(SELECT count(*) FROM messages WHERE status IN ('queued','sending')),
	(SELECT count(*) FROM messages WHERE status='sent' AND attempts>1),(SELECT count(*) FROM content_templates) FROM campaigns`).Scan(
		&o.Campaigns, &o.Draft, &o.Approved, &o.Sending, &o.Completed, &o.Users, &o.Sent, &o.Failed, &o.Pending, &o.Recovered, &o.Templates)
	return o, err
}
