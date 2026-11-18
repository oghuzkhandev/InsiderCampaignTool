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
const (
	smsEligible = `EXISTS (SELECT 1 FROM user_consents c WHERE c.user_id = u.id AND c.channel = 'sms' AND c.granted)
    AND COALESCE(u.phone ~ '[^ \t\n\r\f\v]', false)`
	emailEligible = `EXISTS (SELECT 1 FROM user_consents c WHERE c.user_id = u.id AND c.channel = 'email' AND c.granted)
    AND COALESCE(u.email ~ '[^ \t\n\r\f\v]', false)`
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
