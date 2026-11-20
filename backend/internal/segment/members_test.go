package segment

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// ADR-031: üye listesi sorgusu. Gerçek veritabanı davranışı
// tests/integration/audience-members_test.go'dadır; burada sorgunun yapısı sınanır.

func TestBuildMembersUsesSameConditionsAsCount(t *testing.T) {
	seg := mustValidate(t, exampleBriefJSON)
	count, err := BuildCount(seg, referenceNow)
	if err != nil {
		t.Fatal(err)
	}
	members, err := BuildMembers(seg, referenceNow, 10, 20)
	if err != nil {
		t.Fatal(err)
	}

	// Koşul parametreleri aynıdır; sonuna yalnızca limit ve offset eklenir.
	want := append(append([]any{}, count.Args...), int64(10), int64(20))
	if !reflect.DeepEqual(members.Args, want) {
		t.Fatalf("argümanlar sayı sorgusunun koşulları + limit + offset olmalı:\n alınan  %#v\n beklenen %#v", members.Args, want)
	}
	// Koşul metni sayı sorgusundakiyle birebir aynıdır ve iki kez (toplam ve sayfa) kullanılır.
	conditions := count.SQL[strings.LastIndex(count.SQL, "WHERE ")+len("WHERE "):]
	if got := strings.Count(members.SQL, "WHERE "+conditions); got != 2 {
		t.Fatalf("koşullar toplam ve sayfa için iki kez bulunmalı, %d kez bulundu:\n%s", got, members.SQL)
	}
}

func TestBuildMembersShape(t *testing.T) {
	q, err := BuildMembers(mustValidate(t, exampleBriefJSON), referenceNow, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ORDER BY u.id",    // kararlı sıra: birincil anahtar
		"ORDER BY page.id", // birleşim sonrası da aynı sıra
		"LIMIT $4 OFFSET $5",
		"count(*) AS n FROM users u", // toplam kanal iznine bakmadan sayılır
		"LEFT JOIN page ON true",     // sayfa boşsa bile toplam gelir
	} {
		if !strings.Contains(q.SQL, want) {
			t.Errorf("sorgu %q içermeli:\n%s", want, q.SQL)
		}
	}
	// İzin filtresi yoktur: izinsiz eşleşenler listede görünmelidir (BuildRecipients'ten farkı).
	for _, banned := range []string{"sms_ok OR", "WHERE r."} {
		if strings.Contains(q.SQL, banned) {
			t.Errorf("sorgu kanal iznine göre elememeli (%q bulundu):\n%s", banned, q.SQL)
		}
	}
}

// Kanal uygunluğu iki parçadan (izin, adres) ayrı ayrı döner ve birleşik ifade bu
// parçalardan oluşur: "SMS'e uygun" sayısı ile liste aynı kuralı kullanır (ADR-023).
func TestBuildMembersExposesConsentAndAddressSeparately(t *testing.T) {
	q, err := BuildMembers(mustValidate(t, exampleBriefJSON), referenceNow, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, part := range map[string]string{
		"SMS izni": smsConsent, "SMS adresi": smsAddress,
		"e-posta izni": emailConsent, "e-posta adresi": emailAddress,
	} {
		if !strings.Contains(q.SQL, part) {
			t.Errorf("%s ifadesi sorguda olmalı:\n%s", name, q.SQL)
		}
	}
	if smsEligible != smsConsent+"\n    AND "+smsAddress || emailEligible != emailConsent+"\n    AND "+emailAddress {
		t.Fatal("birleşik uygunluk ifadeleri izin ve adres parçalarından oluşmalı")
	}
	for _, col := range []string{"AS sms_consent", "AS sms_address", "AS email_consent", "AS email_address"} {
		if !strings.Contains(q.SQL, col) {
			t.Errorf("sorgu %q sütununu döndürmeli", col)
		}
	}
}

// Değerler SQL metnine girmez; yalnızca parametre olarak bağlanır.
func TestBuildMembersNeverInlinesValues(t *testing.T) {
	seg := mustValidate(t, `{"all":[{"field":"city","op":"eq","value":"x' OR '1'='1"}],"unsupported":[]}`)
	q, err := BuildMembers(seg, referenceNow, 7, 14)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.SQL, "OR '1'='1") || strings.Contains(q.SQL, "x'") {
		t.Fatalf("değer SQL metnine eklenmemeli:\n%s", q.SQL)
	}
	if q.Args[0] != "x' OR '1'='1" {
		t.Fatalf("değer parametre olarak taşınmalı: %#v", q.Args)
	}
}

func TestBuildMembersRejectsEmptySegmentAndInvalidPaging(t *testing.T) {
	if _, err := BuildMembers(Segment{}, referenceNow, 10, 0); !errors.Is(err, ErrEmptySegment) {
		t.Fatalf("koşulsuz segment ErrEmptySegment vermeli: %v", err)
	}
	seg := mustValidate(t, exampleBriefJSON)
	for name, c := range map[string]struct{ limit, offset int }{
		"sıfır limit": {0, 0}, "negatif limit": {-1, 0}, "negatif kaydırma": {10, -1},
	} {
		if _, err := BuildMembers(seg, referenceNow, c.limit, c.offset); !errors.Is(err, ErrInvalidPage) {
			t.Errorf("%s: ErrInvalidPage bekleniyordu: %v", name, err)
		}
	}
}
