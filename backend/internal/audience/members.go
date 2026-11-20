package audience

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// Üye listesinin sınırları (ADR-031): varsayılan sayfa 10 kişi, en çok 50; sayfa numarası
// en çok 100000 (offset taşmasını önler, 50 × 100000 = 5 milyon kişiyi aşan bir demo yoktur).
const (
	DefaultMemberLimit = 10
	MaxMemberLimit     = 50
	MaxMemberPage      = 100000
)

// ChannelStatus, bir kişinin bir kanala uygunluğunu ve uygun değilse sebebini söyler.
// Kural ADR-023 ile aynıdır: kanala uygun olmak için açık izin VE adres gerekir.
type ChannelStatus string

const (
	// ChannelEligible: o kanal için açık izin var ve adres (telefon veya e-posta) dolu.
	ChannelEligible ChannelStatus = "eligible"
	// ChannelNoConsent: açık izin yok (izin kaydı hiç yok veya izin reddedilmiş). Adresin
	// olup olmadığına bakılmaz: izin yokken adres eksikliği ikinci planda kalır.
	ChannelNoConsent ChannelStatus = "no_consent"
	// ChannelNoAddress: izin var ama adres boş. Gönderilecek bir yer yok.
	ChannelNoAddress ChannelStatus = "no_address"
)

// channelStatus, izin ve adres bilgisinden durumu hesaplar.
func channelStatus(consent, address bool) ChannelStatus {
	switch {
	case !consent:
		return ChannelNoConsent
	case !address:
		return ChannelNoAddress
	default:
		return ChannelEligible
	}
}

// Member, segmente uyan bir kişidir (kanal iznine bakılmadan). Eksik bilgi nil'dir:
// ad, telefon veya e-posta boş, NULL ya da yalnızca boşluksa ilgili alan nil olur;
// çağıran bunu "yok" olarak gösterir, boş metin veya sahte değer uydurmaz.
type Member struct {
	ID        int64
	FirstName *string
	City      string
	Email     *string
	Phone     *string

	// SMSStatus ve EmailStatus, kanal uygunluklarıdır (kitle sayılarıyla aynı kural).
	// "SMS'e uygun kişi" sayısı, SMSStatus == ChannelEligible olan kişi sayısına eşittir.
	SMSStatus   ChannelStatus
	EmailStatus ChannelStatus
}

// MemberPage, üye listesinin bir sayfasıdır.
type MemberPage struct {
	// Total, segmente uyan benzersiz kişi sayısıdır (kanal iznine bakılmaz); kitle
	// önizlemesindeki toplam ile aynı tanımdır. Sayfa boş olsa da doludur.
	Total int
	// Members, kimliğe göre artan sıradaki sayfadır. Sayfa son kayıttan sonraysa boştur
	// (nil değil).
	Members []Member
}

// MemberSource, ListMembers'ın ihtiyaç duyduğu sorgu işlemidir (*sql.DB veya *sql.Tx).
type MemberSource interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// trimAddress, adresin kenar boşluklarını kanal uygunluğundaki ASCII boşluk sınıfıyla atar
// (asciiSpace); böylece "adres var" kararı listede ve kitle sayısında aynıdır.
func trimAddress(s string) string { return strings.Trim(s, asciiSpace) }

// present, değer yoksa (NULL) veya trim sonrası boşsa nil, aksi halde kırpılmış değeri döndürür.
func present(v sql.NullString, trim func(string) string) *string {
	if !v.Valid {
		return nil
	}
	s := trim(v.String)
	if s == "" {
		return nil
	}
	return &s
}

// ListMembers, segmente uyan kişilerin limit'lik bir sayfasını kimliğe göre sıralı okurken
// toplam eşleşen sayısını da aynı sorguda döndürür. page 1'den başlar. now referans zamandır.
// Koşulsuz segment için hata döner ve sorgu çalışmaz.
func ListMembers(ctx context.Context, db MemberSource, seg segment.Segment, now time.Time, page, limit int) (MemberPage, error) {
	if page < 1 || limit < 1 {
		return MemberPage{}, segment.ErrInvalidPage
	}
	q, err := segment.BuildMembers(seg, now, limit, (page-1)*limit)
	if err != nil {
		return MemberPage{}, err
	}
	return readMemberPage(ctx, db, q)
}

// Directory, kampanyadan bağımsız kişi dizininin (tüm kullanıcılar) süzgeçli bir sayfasını
// kimliğe göre sıralı okur. page 1'den başlar. Tanınmayan bir süzgeç değeri
// segment.ErrInvalidFilter döndürür ve sorgu çalışmaz.
func (l *Lister) Directory(ctx context.Context, f segment.DirectoryFilter, page, limit int) (MemberPage, error) {
	if page < 1 || limit < 1 {
		return MemberPage{}, segment.ErrInvalidPage
	}
	q, err := segment.BuildDirectory(f, limit, (page-1)*limit)
	if err != nil {
		return MemberPage{}, err
	}
	return readMemberPage(ctx, l.db, q)
}

// Cities, kullanıcıların farklı şehirlerini (dizin süzgeci seçenekleri) alfabetik okur.
func (l *Lister) Cities(ctx context.Context) ([]string, error) {
	rows, err := l.db.QueryContext(ctx, segment.BuildCities().SQL)
	if err != nil {
		return nil, fmt.Errorf("audience: şehirler okunamadı: %w", err)
	}
	defer rows.Close()
	cities := []string{}
	for rows.Next() {
		var city string
		if err := rows.Scan(&city); err != nil {
			return nil, fmt.Errorf("audience: şehir okunamadı: %w", err)
		}
		cities = append(cities, city)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audience: şehirler okunamadı: %w", err)
	}
	return cities, nil
}

func readMemberPage(ctx context.Context, db MemberSource, q segment.Query) (MemberPage, error) {
	rows, err := db.QueryContext(ctx, q.SQL, q.Args...)
	if err != nil {
		return MemberPage{}, fmt.Errorf("audience: kişi listesi okunamadı: %w", err)
	}
	defer rows.Close()

	out := MemberPage{Members: []Member{}}
	for rows.Next() {
		var total int
		var id sql.NullInt64
		var name, city, email, phone sql.NullString
		var smsConsent, smsAddress, emailConsent, emailAddress sql.NullBool
		if err := rows.Scan(&total, &id, &name, &city, &email, &phone,
			&smsConsent, &smsAddress, &emailConsent, &emailAddress); err != nil {
			return MemberPage{}, fmt.Errorf("audience: kişi okunamadı: %w", err)
		}
		out.Total = total
		if !id.Valid {
			continue // sayfa boş: yalnızca toplam geldi
		}
		out.Members = append(out.Members, Member{
			ID:          id.Int64,
			FirstName:   present(name, strings.TrimSpace),
			City:        city.String,
			Email:       present(email, trimAddress),
			Phone:       present(phone, trimAddress),
			SMSStatus:   channelStatus(smsConsent.Bool, smsAddress.Bool),
			EmailStatus: channelStatus(emailConsent.Bool, emailAddress.Bool),
		})
	}
	if err := rows.Err(); err != nil {
		return MemberPage{}, fmt.Errorf("audience: kişi listesi okunamadı: %w", err)
	}
	return out, nil
}

// Lister, bir veritabanına bağlı ListMembers çağrısıdır; HTTP katmanı gibi çağıranların küçük
// bir arayüze bağlanmasını sağlar.
type Lister struct {
	db MemberSource
}

// NewLister, verilen veritabanıyla bir Lister oluşturur.
func NewLister(db MemberSource) *Lister { return &Lister{db: db} }

// ListMembers, Lister'ın veritabanında ListMembers çalıştırır.
func (l *Lister) ListMembers(ctx context.Context, seg segment.Segment, now time.Time, page, limit int) (MemberPage, error) {
	return ListMembers(ctx, l.db, seg, now, page, limit)
}
