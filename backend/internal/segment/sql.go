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

const countSelect = `SELECT
  count(*) AS total,
  count(*) FILTER (WHERE EXISTS (SELECT 1 FROM user_consents c WHERE c.user_id = u.id AND c.channel = 'sms' AND c.granted)) AS sms_eligible,
  count(*) FILTER (WHERE EXISTS (SELECT 1 FROM user_consents c WHERE c.user_id = u.id AND c.channel = 'email' AND c.granted)) AS email_eligible
FROM users u
WHERE `

// BuildCount, segment için toplam, SMS'e uygun ve e-postaya uygun kişi
// sayılarını döndüren sorguyu üretir. now referans zamandır; testte sabitlenir.
//
// Sorgu metni yalnızca sabit alan ve operatör eşlemelerinden oluşur. Segment
// koşulları sabit kataloğa karşı tekrar kontrol edilir (savunma katmanı).
func BuildCount(seg Segment, now time.Time) (Query, error) {
	if len(seg.Conditions) == 0 {
		return Query{}, ErrEmptySegment
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
			return Query{}, fmt.Errorf("segment: desteklenmeyen alan %q", c.Field)
		}
		kind, ok := catalog[c.Field][c.Op]
		if !ok {
			return Query{}, fmt.Errorf("segment: desteklenmeyen operatör %q", c.Op)
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
			return Query{}, fmt.Errorf("segment: işlenemeyen koşul %s %s", c.Field, c.Op)
		}
	}

	return Query{SQL: countSelect + strings.Join(parts, " AND "), Args: args}, nil
}
