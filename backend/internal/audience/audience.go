// Package audience, doğrulanmış bir segmentin kitle sayılarını PostgreSQL'den okur.
package audience

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// Counts, bir segmentin üç kitle sayısıdır.
type Counts struct {
	// Total, kanal iznine bakılmadan eşleşen benzersiz kişi sayısıdır.
	Total int
	// SMS ve Email, ilgili kanal için açıkça izin vermiş kişi sayılarıdır.
	// İki kanala da izni olan kişi ikisinde de sayılır; toplamları Total değildir.
	SMS   int
	Email int
}

// HasRecipients, en az bir kanalda uygun alıcı olup olmadığını söyler. Toplam
// eşleşme olsa bile uygun alıcı yoksa kampanya gönderime onaylanamaz.
func (c Counts) HasRecipients() bool { return c.SMS > 0 || c.Email > 0 }

// Querier, tek satır döndüren sorgu çalıştırabilen bir veritabanıdır.
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Count, segmentin sorgusunu parametreli olarak çalıştırır. now referans
// zamandır (30 günlük pencere gibi sınırlar buna göre hesaplanır).
func Count(ctx context.Context, db Querier, seg segment.Segment, now time.Time) (Counts, error) {
	q, err := segment.BuildCount(seg, now)
	if err != nil {
		return Counts{}, err
	}
	var c Counts
	if err := db.QueryRowContext(ctx, q.SQL, q.Args...).Scan(&c.Total, &c.SMS, &c.Email); err != nil {
		return Counts{}, fmt.Errorf("audience: kitle sayıları okunamadı: %w", err)
	}
	return c, nil
}

// Counter, bir veritabanına bağlı Count çağrısıdır; HTTP katmanı gibi
// çağıranların küçük bir arayüze bağlanmasını sağlar.
type Counter struct {
	db Querier
}

// NewCounter, verilen veritabanıyla bir Counter oluşturur.
func NewCounter(db Querier) *Counter { return &Counter{db: db} }

// Count, Counter'ın veritabanında Count çalıştırır.
func (c *Counter) Count(ctx context.Context, seg segment.Segment, now time.Time) (Counts, error) {
	return Count(ctx, c.db, seg, now)
}
