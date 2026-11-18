package segment

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Alıcı sorgusu, kitle sayısı sorgusuyla AYNI koşulları kullanmalıdır; aksi halde
// onayda kuyruğa alınan kişiler önizlemedeki kişilerden farklı bir kural
// uygulamış olur.
func TestBuildRecipientsUsesSameConditionsAsCount(t *testing.T) {
	for name, raw := range map[string]string{
		"örnek brief": exampleBriefJSON,
		"diğer operatörler": `{"all":[
			{"field":"city","op":"in","value":["İstanbul","Ankara"]},
			{"field":"last_cart_add_at","op":"not_within_last_days","value":7},
			{"field":"last_purchase_at","op":"within_last_days","value":10},
			{"field":"order_count","op":"gte","value":2},
			{"field":"order_count","op":"lte","value":9},
			{"field":"order_count","op":"eq","value":5}],"unsupported":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			seg := mustValidate(t, raw)
			count, err := BuildCount(seg, referenceNow)
			if err != nil {
				t.Fatal(err)
			}
			rec, err := BuildRecipients(seg, referenceNow)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rec.Args, count.Args) {
				t.Fatalf("argümanlar aynı olmalı:\n alıcı %#v\n sayı  %#v", rec.Args, count.Args)
			}
			// Koşullar iki sorguda da son WHERE'den sonra gelir (alt sorgulardaki
			// WHERE'ler koşul içermez).
			conditions := count.SQL[strings.LastIndex(count.SQL, "WHERE ")+len("WHERE "):]
			if !strings.Contains(rec.SQL, "WHERE "+conditions) {
				t.Fatalf("alıcı sorgusu aynı koşulları içermeli:\n%s\nkoşullar:\n%s", rec.SQL, conditions)
			}
		})
	}
}

func TestBuildRecipientsShape(t *testing.T) {
	q, err := BuildRecipients(mustValidate(t, exampleBriefJSON), referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"r.sms_ok OR r.email_ok", // en az bir kanalda açık izin
		"ORDER BY r.id",          // kararlı sıra
		"c.channel = 'sms' AND c.granted",
		"c.channel = 'email' AND c.granted",
		"FROM users u",
		"u.phone ~ '[^ \\t\\n\\r\\f\\v]'", // SMS: izin VE telefon (ADR-023)
		"u.email ~ '[^ \\t\\n\\r\\f\\v]'", // e-posta: izin VE adres
	} {
		if !strings.Contains(q.SQL, want) {
			t.Fatalf("sorguda %q yok:\n%s", want, q.SQL)
		}
	}
}

func TestBuildRecipientsSQLLikeValueStaysInArgs(t *testing.T) {
	evil := "x'; DROP TABLE users; --"
	seg := mustValidate(t, `{"all":[{"field":"city","op":"eq","value":"`+evil+`"}],"unsupported":[]}`)
	q, err := BuildRecipients(seg, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.SQL, "DROP") || strings.Contains(q.SQL, evil) {
		t.Fatalf("değer sorgu metnine girmemeli:\n%s", q.SQL)
	}
	if !reflect.DeepEqual(q.Args, []any{evil}) {
		t.Fatalf("değer yalnızca argüman olmalı: %#v", q.Args)
	}
}

// Filtresiz alıcı sorgusu hiçbir zaman üretilmez.
func TestBuildRecipientsRefusesEmptyAndUnvalidatedSegments(t *testing.T) {
	if _, err := BuildRecipients(Segment{}, referenceNow); !errors.Is(err, ErrEmptySegment) {
		t.Fatalf("ErrEmptySegment bekleniyordu: %v", err)
	}
	for _, c := range []Condition{
		{Field: "favorite_color", Op: OpEq, Text: "mavi"},
		{Field: FieldCity, Op: "like", Text: "İst%"},
	} {
		if _, err := BuildRecipients(Segment{Conditions: []Condition{c}}, time.Now()); err == nil {
			t.Fatalf("doğrulanmamış koşul reddedilmeli: %+v", c)
		}
	}
}

// Kitle sayısı ve alıcı sorgusu AYNI kanal uygunluğu ifadesini kullanır
// (ADR-023): önizleme ile kuyruk aynı kümeyi sayar.
func TestCountAndRecipientsShareChannelEligibilityExpressions(t *testing.T) {
	seg := mustValidate(t, exampleBriefJSON)
	count, err := BuildCount(seg, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := BuildRecipients(seg, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	for name, expr := range map[string]string{"SMS": smsEligible, "e-posta": emailEligible} {
		if !strings.Contains(count.SQL, expr) {
			t.Fatalf("%s uygunluğu kitle sayısı sorgusunda olmalı:\n%s", name, count.SQL)
		}
		if !strings.Contains(rec.SQL, expr) {
			t.Fatalf("%s uygunluğu alıcı sorgusunda olmalı:\n%s", name, rec.SQL)
		}
	}
}
