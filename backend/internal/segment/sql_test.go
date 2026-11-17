package segment

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// referenceNow, tarih sınırı testlerinde kullanılan sabit referans zamandır.
var referenceNow = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func mustValidate(t *testing.T, raw string) Segment {
	t.Helper()
	seg, issues := Validate([]byte(raw))
	if len(issues) != 0 {
		t.Fatalf("geçerli segment bekleniyordu: %v", issues)
	}
	return seg
}

func TestBuildCountExampleBrief(t *testing.T) {
	q, err := BuildCount(mustValidate(t, exampleBriefJSON), referenceNow)
	if err != nil {
		t.Fatal(err)
	}

	wantWhere := "WHERE u.city = $1 AND u.last_cart_add_at >= $2 AND (u.last_purchase_at IS NULL OR u.last_purchase_at < $3)"
	if !strings.HasSuffix(q.SQL, wantWhere) {
		t.Fatalf("WHERE beklenen gibi değil:\n%s", q.SQL)
	}
	boundary := referenceNow.Add(-30 * 24 * time.Hour)
	wantArgs := []any{"İstanbul", boundary, boundary}
	if !reflect.DeepEqual(q.Args, wantArgs) {
		t.Fatalf("args = %#v, beklenen %#v", q.Args, wantArgs)
	}
	for _, want := range []string{"count(*) AS total", "AS sms_eligible", "AS email_eligible", "c.granted"} {
		if !strings.Contains(q.SQL, want) {
			t.Fatalf("sorguda %q yok:\n%s", want, q.SQL)
		}
	}
}

func TestBuildCountOtherOperators(t *testing.T) {
	seg := mustValidate(t, `{"all":[
		{"field":"city","op":"in","value":["İstanbul","Ankara"]},
		{"field":"last_cart_add_at","op":"not_within_last_days","value":7},
		{"field":"last_purchase_at","op":"within_last_days","value":10},
		{"field":"order_count","op":"gte","value":2},
		{"field":"order_count","op":"lte","value":9},
		{"field":"order_count","op":"eq","value":5}
	],"unsupported":[]}`)
	q, err := BuildCount(seg, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE u.city = ANY($1) AND (u.last_cart_add_at IS NULL OR u.last_cart_add_at < $2) AND u.last_purchase_at >= $3 AND u.order_count >= $4 AND u.order_count <= $5 AND u.order_count = $6"
	if !strings.HasSuffix(q.SQL, want) {
		t.Fatalf("WHERE beklenen gibi değil:\n%s", q.SQL)
	}
	if len(q.Args) != 6 {
		t.Fatalf("args sayısı = %d", len(q.Args))
	}
	if got, ok := q.Args[0].([]string); !ok || !reflect.DeepEqual(got, []string{"İstanbul", "Ankara"}) {
		t.Fatalf("ilk arg = %#v", q.Args[0])
	}
}

func TestBuildCountWindowBoundaryUsesReferenceTime(t *testing.T) {
	seg := mustValidate(t, `{"all":[{"field":"last_cart_add_at","op":"within_last_days","value":30}],"unsupported":[]}`)
	// Yerel saat dilimi farkı sınırı değiştirmemeli: referans zaman UTC'ye çevrilir.
	istanbul := time.FixedZone("TRT", 3*3600)
	q1, _ := BuildCount(seg, referenceNow)
	q2, _ := BuildCount(seg, referenceNow.In(istanbul))
	if !reflect.DeepEqual(q1.Args, q2.Args) {
		t.Fatalf("saat dilimi sınırı değiştirdi: %v != %v", q1.Args, q2.Args)
	}
	want := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if got := q1.Args[0].(time.Time); !got.Equal(want) {
		t.Fatalf("sınır = %v, beklenen %v", got, want)
	}
}

func TestBuildCountSQLLikeValueDoesNotChangeQueryText(t *testing.T) {
	benign := mustValidate(t, `{"all":[{"field":"city","op":"eq","value":"İstanbul"}],"unsupported":[]}`)
	hostile := mustValidate(t, `{"all":[{"field":"city","op":"eq","value":"x' OR '1'='1'; DROP TABLE users; --"}],"unsupported":[]}`)

	qb, err := BuildCount(benign, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	qh, err := BuildCount(hostile, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	if qb.SQL != qh.SQL {
		t.Fatalf("sorgu metni değişti:\n%s\n---\n%s", qb.SQL, qh.SQL)
	}
	if qh.Args[0] != "x' OR '1'='1'; DROP TABLE users; --" {
		t.Fatalf("değer yalnızca argüman olmalı: %#v", qh.Args)
	}
	if strings.Contains(qh.SQL, "DROP") {
		t.Fatal("değer SQL metnine girmemeli")
	}
}

func TestBuildCountRefusesUnvalidatedSegments(t *testing.T) {
	if _, err := BuildCount(Segment{}, referenceNow); !errors.Is(err, ErrEmptySegment) {
		t.Fatalf("filtresiz segment için ErrEmptySegment bekleniyordu: %v", err)
	}
	bad := []Segment{
		{Conditions: []Condition{{Field: "email", Op: "eq", Text: "a@b.c"}}},
		{Conditions: []Condition{{Field: "city", Op: "like", Text: "x"}}},
		{Conditions: []Condition{{Field: "city", Op: "within_last_days", Number: 3}}},
	}
	for _, seg := range bad {
		if _, err := BuildCount(seg, referenceNow); err == nil {
			t.Fatalf("doğrulanmamış segment reddedilmeli: %#v", seg)
		}
	}
}

// FuzzBuildCount, doğrulamadan geçen her çıktı için sorgu metninin yalnızca
// alan ve operatör yapısına bağlı olduğunu denetler: değerler değişince metin
// değişmez ve placeholder sayısı argüman sayısına eşittir.
func FuzzBuildCount(f *testing.F) {
	f.Add([]byte(exampleBriefJSON))
	f.Add([]byte(`{"all":[{"field":"city","op":"eq","value":"x'; DROP TABLE users; --"}],"unsupported":[]}`))
	f.Add([]byte(`{"all":[{"field":"city","op":"in","value":["a","b"]},{"field":"order_count","op":"gte","value":1}],"unsupported":[]}`))
	f.Add([]byte(`{"all":[{"field":"city\u0000","op":"eq","value":"x"}],"unsupported":[]}`))
	f.Add([]byte(`{"any":[],"all":[],"unsupported":["x"]}`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		seg, issues := Validate(raw)
		if len(issues) != 0 {
			if len(seg.Conditions) != 0 {
				t.Fatal("sorunlu çıktıda koşul olmamalı")
			}
			return
		}
		q, err := BuildCount(seg, referenceNow)
		if err != nil {
			t.Fatalf("doğrulanmış segment için sorgu üretilmeli: %v", err)
		}
		if got := strings.Count(q.SQL, "$"); got != len(q.Args) {
			t.Fatalf("placeholder sayısı %d, argüman sayısı %d", got, len(q.Args))
		}

		skeleton := Segment{}
		for _, c := range seg.Conditions {
			s := Condition{Field: c.Field, Op: c.Op}
			switch catalog[c.Field][c.Op] {
			case kindText:
				s.Text = "x"
			case kindTextList:
				s.Texts = []string{"x"}
			default:
				s.Number = 1
			}
			skeleton.Conditions = append(skeleton.Conditions, s)
		}
		qs, err := BuildCount(skeleton, referenceNow)
		if err != nil {
			t.Fatal(err)
		}
		if q.SQL != qs.SQL {
			t.Fatalf("sorgu metni değerlere bağlı olmamalı:\n%s\n---\n%s", q.SQL, qs.SQL)
		}
	})
}
