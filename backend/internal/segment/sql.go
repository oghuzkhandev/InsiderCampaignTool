package segment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrEmptySegment, koşulsuz bir segment için sorgu üretilmeye çalışıldığını
// gösterir. Filtresiz kitle sorgusu hiçbir zaman çalıştırılmaz.
var ErrEmptySegment = errors.New("segment: koşul yok")

// Query, parametreli bir SQL sorgusudur. Model çıktısı SQL metnine girmez;
// değerler yalnızca Args içinde taşınır.
type Query struct {
	SQL  string
	Args []any
}

// Kanal uygunluğu (ADR-023): bir kullanıcı bir kanala uygundur, ancak o kanal için
// açıkça izin vermişse VE o kanalın adresi (telefon veya e-posta) varsa. Kitle
// sayıları ve onaydaki alıcı listesi aynı ifadeleri kullanır; önizleme ile kuyruk
// bu yüzden aynı kümeyi sayar. Adres "ASCII boşluk dışında en az bir karakter"
// demektir (migration'daki messages.recipient koruması ile aynı sınıf).
//
// Uygunluk iki parçadan oluşur: açık izin ve adres. Parçalar ayrı sabitlerdir, çünkü
// üye listesi (BuildMembers) kişinin bir kanala neden uygun olmadığını (izin mi, adres mi)
// söylemek zorundadır. Birleşik ifadeler parçalardan türetilir; kitle sayısı ve alıcı
// sorgusundaki metin bu yüzden değişmez.
const (
	smsConsent   = `EXISTS (SELECT 1 FROM user_consents c WHERE c.user_id = u.id AND c.channel = 'sms' AND c.granted)`
	smsAddress   = `COALESCE(u.phone ~ '[^ \t\n\r\f\v]', false)`
	emailConsent = `EXISTS (SELECT 1 FROM user_consents c WHERE c.user_id = u.id AND c.channel = 'email' AND c.granted)`
	emailAddress = `COALESCE(u.email ~ '[^ \t\n\r\f\v]', false)`

	smsEligible   = smsConsent + "\n    AND " + smsAddress
	emailEligible = emailConsent + "\n    AND " + emailAddress
)

const countSelect = `SELECT
  count(*) AS total,
  count(*) FILTER (WHERE ` + smsEligible + `) AS sms_eligible,
  count(*) FILTER (WHERE ` + emailEligible + `) AS email_eligible
FROM users u
WHERE `

// recipientsSelect, segmente uyan ve en az bir kanala uygun (izin ve adres)
// kullanıcıları, kanal uygunluklarıyla birlikte tek geçişte okur. Onay, kanal
// bazında alıcı listesini bu tek sorgudan bir kez seçer.
const recipientsSelect = `SELECT r.id, r.first_name, r.phone, r.email, r.sms_ok, r.email_ok
FROM (
  SELECT u.id, u.first_name, u.phone, u.email,
    (` + smsEligible + `) AS sms_ok,
    (` + emailEligible + `) AS email_ok
  FROM users u
  WHERE `

// buildWhere, segment koşullarını parametreli bir WHERE ifadesine çevirir.
// Sorgu metni yalnızca sabit alan ve operatör eşlemelerinden oluşur. Koşullar
// sabit kataloğa karşı tekrar kontrol edilir (savunma katmanı). Hem kitle
// sayıları hem alıcı listesi aynı koşulları kullanır.
func buildWhere(seg Segment, now time.Time) (string, []any, error) {
	if len(seg.Conditions) == 0 {
		return "", nil, ErrEmptySegment
	}

	var (
		parts []string
		args  []any
	)
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	for _, c := range seg.Conditions {
		col, ok := columns[c.Field]
		if !ok {
			return "", nil, fmt.Errorf("segment: desteklenmeyen alan %q", c.Field)
		}
		kind, ok := catalog[c.Field][c.Op]
		if !ok {
			return "", nil, fmt.Errorf("segment: desteklenmeyen operatör %q", c.Op)
		}

		switch {
		case c.Field == FieldCity && c.Op == OpEq:
			parts = append(parts, col+" = "+next(c.Text))
		case c.Field == FieldCity && c.Op == OpIn:
			parts = append(parts, col+" = ANY("+next(c.Texts)+")")
		case kind == kindDays:
			boundary := now.UTC().Add(-time.Duration(c.Number) * 24 * time.Hour)
			p := next(boundary)
			if c.Op == OpWithinLastDays {
				parts = append(parts, col+" >= "+p)
			} else {
				parts = append(parts, "("+col+" IS NULL OR "+col+" < "+p+")")
			}
		case c.Field == FieldOrderCount:
			op := map[string]string{OpEq: "=", OpGte: ">=", OpLte: "<="}[c.Op]
			parts = append(parts, col+" "+op+" "+next(c.Number))
		default:
			return "", nil, fmt.Errorf("segment: işlenemeyen koşul %s %s", c.Field, c.Op)
		}
	}
	return strings.Join(parts, " AND "), args, nil
}

// BuildCount, segment için toplam, SMS'e uygun ve e-postaya uygun kişi
// sayılarını döndüren sorguyu üretir. now referans zamandır; testte sabitlenir.
func BuildCount(seg Segment, now time.Time) (Query, error) {
	where, args, err := buildWhere(seg, now)
	if err != nil {
		return Query{}, err
	}
	return Query{SQL: countSelect + where, Args: args}, nil
}

// BuildRecipients, segmente uyan ve en az bir kanala uygun (açık izin ve adres)
// kullanıcıları (kimlik, ad, telefon, e-posta, kanal uygunlukları) kimliğe göre
// sıralı döndüren sorguyu üretir. Koşullar BuildCount ile aynıdır. Koşulsuz
// segment için ErrEmptySegment döner; filtresiz sorgu hiçbir zaman çalıştırılmaz.
func BuildRecipients(seg Segment, now time.Time) (Query, error) {
	where, args, err := buildWhere(seg, now)
	if err != nil {
		return Query{}, err
	}
	return Query{SQL: recipientsSelect + where + "\n) r\nWHERE r.sms_ok OR r.email_ok\nORDER BY r.id", Args: args}, nil
}

// ErrInvalidPage, üye listesi için geçersiz bir sınır veya kaydırma verildiğini gösterir.
var ErrInvalidPage = errors.New("segment: sayfa sınırı geçersiz")

// BuildMembers, segmente uyan **bütün** kişilerden (kanal iznine bakılmadan) kimliğe göre
// sıralı bir sayfa ve toplam eşleşen sayısını TEK ifadede döndüren sorguyu üretir. Toplam ile
// sayfa aynı anlık görüntüden gelir. Koşullar BuildCount ile aynıdır, yani toplam kitle
// önizlemesindeki "toplam eşleşen kişi" ile aynı tanımdır. BuildRecipients'ten farkı: iki
// kanala da uygun olmayan kişiler elenmez; kanal bazında izin ve adres ayrı sütunlar olarak
// döner, böylece "neden uygun değil" söylenebilir.
//
// Sorgu her zaman en az bir satır döner: sayfa boşsa (kaydırma son kayıttan sonraysa veya
// kimse eşleşmediyse) tek satır, kimlik sütunları NULL ve toplam dolu olarak gelir.
// Sütunlar: total, id, first_name, city, email, phone, sms_consent, sms_address,
// email_consent, email_address. limit en az 1, offset en az 0 olmalıdır; koşulsuz segment için
// ErrEmptySegment döner ve sorgu üretilmez.
func BuildMembers(seg Segment, now time.Time, limit, offset int) (Query, error) {
	if limit < 1 || offset < 0 {
		return Query{}, ErrInvalidPage
	}
	where, args, err := buildWhere(seg, now)
	if err != nil {
		return Query{}, err
	}
	return buildMemberPage(where, args, limit, offset), nil
}

// ErrInvalidFilter, kişi dizini için tanınmayan bir kanal uygunluğu filtresi verildiğini gösterir.
var ErrInvalidFilter = errors.New("segment: filtre değeri geçersiz")

// DirectoryFilter, kampanyadan bağımsız kişi dizininin süzgeçleridir. Boş alan "süzme" demektir;
// dolu alanlar AND ile birleşir.
type DirectoryFilter struct {
	// Search, ad, e-posta ve telefonda büyük/küçük harf gözetmeden aranan metindir. Şehir bu
	// aramanın dışındadır (şehir için ayrı süzgeç vardır): "Ada" araması Adana'daki herkesi getirmez.
	Search string
	// City, şehirle tam eşleşir (segmentteki `city eq` ile aynı kural).
	City string
	// SMS ve Email, ilgili kanalın uygunluk durumudur: "eligible", "no_consent" veya "no_address"
	// (ADR-023 ve ADR-031 ile aynı tanım). Boş: süzme yok.
	SMS   string
	Email string
}

// channelFilter, kanalın izin ve adres ifadelerinden istenen durumu seçen SQL koşulunu üretir.
func channelFilter(status, consent, address string) (string, error) {
	switch status {
	case "eligible":
		return "(" + consent + " AND " + address + ")", nil
	case "no_consent":
		return "(NOT " + consent + ")", nil
	case "no_address":
		return "(" + consent + " AND NOT " + address + ")", nil
	}
	return "", ErrInvalidFilter
}

// BuildDirectory, kampanyadan bağımsız, salt okunur kişi dizinini (tüm kullanıcılar) süzgeçlerle
// sayfalı döndürür. Segmentle ilgisi yoktur ve bir kampanyayı onaylamak için KULLANILMAZ;
// BuildMembers koşulsuz segmenti hâlâ reddeder. Değerler yalnızca parametre olarak bağlanır.
// Sütunlar ve sayfa davranışı BuildMembers ile aynıdır.
func BuildDirectory(f DirectoryFilter, limit, offset int) (Query, error) {
	if limit < 1 || offset < 0 {
		return Query{}, ErrInvalidPage
	}
	var (
		parts []string
		args  []any
	)
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Search != "" {
		parts = append(parts, "strpos(lower(coalesce(u.first_name, '') || ' ' || u.email || ' ' || coalesce(u.phone, '')), lower("+next(f.Search)+")) > 0")
	}
	if f.City != "" {
		parts = append(parts, "u.city = "+next(f.City))
	}
	for _, c := range []struct{ status, consent, address string }{
		{f.SMS, smsConsent, smsAddress},
		{f.Email, emailConsent, emailAddress},
	} {
		if c.status == "" {
			continue
		}
		cond, err := channelFilter(c.status, c.consent, c.address)
		if err != nil {
			return Query{}, err
		}
		parts = append(parts, cond)
	}
	where := "true"
	if len(parts) > 0 {
		where = strings.Join(parts, " AND ")
	}
	return buildMemberPage(where, args, limit, offset), nil
}

// BuildCities, kayıtlı kullanıcıların farklı şehirlerini (süzgeç seçenekleri için) döndürür.
func BuildCities() Query {
	return Query{SQL: "SELECT DISTINCT city FROM users ORDER BY city"}
}

func buildMemberPage(where string, args []any, limit, offset int) Query {
	args = append(args, int64(limit), int64(offset))
	limitArg, offsetArg := fmt.Sprintf("$%d", len(args)-1), fmt.Sprintf("$%d", len(args))

	// Koşul metni iki kez kullanılır; yer tutucular aynı parametrelere bağlanır, değerler
	// SQL metnine girmez. Uygunluk ifadeleri yalnızca sayfadaki satırlar için çalışır.
	q := "WITH total AS (\n" +
		"  SELECT count(*) AS n FROM users u\n" +
		"  WHERE " + where + "\n" +
		"), page AS (\n" +
		"  SELECT u.id, u.first_name, u.city, u.email, u.phone,\n" +
		"    (" + smsConsent + ") AS sms_consent,\n" +
		"    (" + smsAddress + ") AS sms_address,\n" +
		"    (" + emailConsent + ") AS email_consent,\n" +
		"    (" + emailAddress + ") AS email_address\n" +
		"  FROM users u\n" +
		"  WHERE " + where + "\n" +
		"  ORDER BY u.id\n" +
		"  LIMIT " + limitArg + " OFFSET " + offsetArg + "\n" +
		")\n" +
		"SELECT total.n, page.id, page.first_name, page.city, page.email, page.phone,\n" +
		"  page.sms_consent, page.sms_address, page.email_consent, page.email_address\n" +
		"FROM total LEFT JOIN page ON true\n" +
		"ORDER BY page.id"
	return Query{SQL: q, Args: args}
}
