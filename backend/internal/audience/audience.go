// Package audience, doğrulanmış bir segmentin kitle sayılarını PostgreSQL'den okur.
package audience

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// Counts, bir segmentin üç kitle sayısıdır.
type Counts struct {
	// Total, kanal iznine bakılmadan eşleşen benzersiz kişi sayısıdır.
	Total int
	// SMS ve Email, ilgili kanala uygun kişi sayılarıdır: o kanal için açıkça izin
	// vermiş VE adresi (telefon veya e-posta) olan kişiler (ADR-023). İki kanala
	// uygun kişi ikisinde de sayılır; toplamları Total değildir.
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

// Recipient, segmente uyan ve en az bir kanala uygun (açık izin ve adres) bir
// kullanıcıdır.
type Recipient struct {
	UserID    int64
	FirstName string // kayıtlı değer; boş olabilir
	Phone     string // SMS uygunsa dolu
	Email     string // e-posta uygunsa dolu

	// SMSEligible ve EmailEligible, ilgili kanal için açık izin ve adres
	// bulunduğunu gösterir (kitle sayılarıyla aynı kural).
	SMSEligible   bool
	EmailEligible bool
}

// asciiSpace, veritabanındaki adres kontrolüyle aynı boşluk sınıfıdır.
const asciiSpace = " \t\n\r\f\v"

// SMSAddress, SMS gönderilebilecek telefonu döndürür. Kanala uygun değilse
// ikinci değer false olur.
func (r Recipient) SMSAddress() (string, bool) {
	phone := strings.Trim(r.Phone, asciiSpace)
	return phone, r.SMSEligible && phone != ""
}

// EmailAddress, e-posta gönderilebilecek adresi döndürür. Kanala uygun değilse
// ikinci değer false olur.
func (r Recipient) EmailAddress() (string, bool) {
	email := strings.Trim(r.Email, asciiSpace)
	return email, r.EmailEligible && email != ""
}

// Rows, birden çok satır döndüren sorgu çalıştırabilen bir veritabanıdır
// (*sql.DB veya *sql.Tx).
type Rows interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Recipients, segmente uyan ve en az bir kanala uygun (izin ve adres) kullanıcıları
// kimliğe göre sıralı okur. Tek sorgudur; kanal izinleri aynı anlık görüntüden
// gelir. Koşulsuz segment için hata döner ve sorgu çalışmaz.
func Recipients(ctx context.Context, db Rows, seg segment.Segment, now time.Time) ([]Recipient, error) {
	q, err := segment.BuildRecipients(seg, now)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, q.SQL, q.Args...)
	if err != nil {
		return nil, fmt.Errorf("audience: alıcılar okunamadı: %w", err)
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var (
			r           Recipient
			name, phone sql.NullString
		)
		if err := rows.Scan(&r.UserID, &name, &phone, &r.Email, &r.SMSEligible, &r.EmailEligible); err != nil {
			return nil, fmt.Errorf("audience: alıcı okunamadı: %w", err)
		}
		r.FirstName, r.Phone = name.String, phone.String
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audience: alıcılar okunamadı: %w", err)
	}
	return out, nil
}
