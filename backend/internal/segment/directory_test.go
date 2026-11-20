package segment

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Kampanyadan bağımsız kişi dizini sorgusu (BuildDirectory). Gerçek veritabanı davranışı
// tests/integration/audience-directory_test.go'dadır; burada sorgunun yapısı sınanır.

func TestBuildDirectoryWithoutFiltersReadsEveryone(t *testing.T) {
	q, err := BuildDirectory(DirectoryFilter{}, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.SQL, "WHERE true") {
		t.Fatalf("süzgeçsiz dizin koşulsuz okunmalı:\n%s", q.SQL)
	}
	if want := []any{int64(10), int64(20)}; !reflect.DeepEqual(q.Args, want) {
		t.Fatalf("yalnızca limit ve kaydırma bağlanmalı: %#v", q.Args)
	}
}

func TestBuildDirectorySearchCoversNameEmailAndPhoneButNotCity(t *testing.T) {
	q, err := BuildDirectory(DirectoryFilter{Search: "Ada"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(q.SQL, "strpos(")
	end := strings.Index(q.SQL, ") > 0")
	if start < 0 || end < start {
		t.Fatalf("arama ifadesi bulunamadı:\n%s", q.SQL)
	}
	expr := q.SQL[start:end]
	for _, want := range []string{"u.first_name", "u.email", "u.phone", "lower($1)"} {
		if !strings.Contains(expr, want) {
			t.Errorf("arama %q içermeli: %s", want, expr)
		}
	}
	// "Ada" araması Adana'daki herkesi getirmemeli: şehir ayrı bir süzgeçtir.
	if strings.Contains(expr, "u.city") {
		t.Errorf("arama şehri kapsamamalı: %s", expr)
	}
	if q.Args[0] != "Ada" {
		t.Fatalf("arama metni parametre olmalı: %#v", q.Args)
	}
}

func TestBuildDirectoryCombinesFiltersWithANDAndNumbersPlaceholders(t *testing.T) {
	q, err := BuildDirectory(DirectoryFilter{Search: "ayşe", City: "İzmir", SMS: "eligible", Email: "no_consent"}, 10, 30)
	if err != nil {
		t.Fatal(err)
	}
	// İki metin parametresi, ardından limit ve kaydırma.
	if want := []any{"ayşe", "İzmir", int64(10), int64(30)}; !reflect.DeepEqual(q.Args, want) {
		t.Fatalf("argümanlar:\n alınan  %#v\n beklenen %#v", q.Args, want)
	}
	for _, want := range []string{
		"u.city = $2",
		"LIMIT $3 OFFSET $4",
		"(" + smsConsent + " AND " + smsAddress + ")", // sms: eligible
		"(NOT " + emailConsent + ")",                  // e-posta: no_consent
	} {
		if !strings.Contains(q.SQL, want) {
			t.Errorf("sorgu %q içermeli:\n%s", want, q.SQL)
		}
	}
	// Koşullar toplam ve sayfa sorgularında aynıdır (iki kez).
	if got := strings.Count(q.SQL, "u.city = $2"); got != 2 {
		t.Errorf("şehir koşulu toplam ve sayfa için iki kez olmalı, %d", got)
	}
}

func TestBuildDirectoryChannelStatuses(t *testing.T) {
	for status, want := range map[string]string{
		"eligible":   "(" + smsConsent + " AND " + smsAddress + ")",
		"no_consent": "(NOT " + smsConsent + ")",
		"no_address": "(" + smsConsent + " AND NOT " + smsAddress + ")",
	} {
		q, err := BuildDirectory(DirectoryFilter{SMS: status}, 10, 0)
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if !strings.Contains(q.SQL, "WHERE "+want) {
			t.Errorf("%s: koşul %q olmalı:\n%s", status, want, q.SQL)
		}
	}
}

// Değerler SQL metnine girmez; yalnızca parametre olarak bağlanır.
func TestBuildDirectoryNeverInlinesValues(t *testing.T) {
	f := DirectoryFilter{Search: "x' OR '1'='1", City: "'; DROP TABLE users; --"}
	q, err := BuildDirectory(f, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"OR '1'='1", "DROP TABLE"} {
		if strings.Contains(q.SQL, bad) {
			t.Fatalf("değer SQL metnine eklenmemeli (%q):\n%s", bad, q.SQL)
		}
	}
	if q.Args[0] != f.Search || q.Args[1] != f.City {
		t.Fatalf("değerler parametre olmalı: %#v", q.Args)
	}
}

func TestBuildDirectoryRejectsUnknownStatusesAndInvalidPaging(t *testing.T) {
	for name, f := range map[string]DirectoryFilter{
		"sms":    {SMS: "maybe"},
		"email":  {Email: "ELIGIBLE"},
		"boşluk": {SMS: " eligible"},
	} {
		if _, err := BuildDirectory(f, 10, 0); !errors.Is(err, ErrInvalidFilter) {
			t.Errorf("%s: ErrInvalidFilter bekleniyordu: %v", name, err)
		}
	}
	for name, c := range map[string]struct{ limit, offset int }{
		"sıfır limit": {0, 0}, "negatif limit": {-1, 0}, "negatif kaydırma": {10, -1},
	} {
		if _, err := BuildDirectory(DirectoryFilter{}, c.limit, c.offset); !errors.Is(err, ErrInvalidPage) {
			t.Errorf("%s: ErrInvalidPage bekleniyordu: %v", name, err)
		}
	}
}

func TestBuildCities(t *testing.T) {
	q := BuildCities()
	if !strings.Contains(q.SQL, "DISTINCT city") || !strings.Contains(q.SQL, "ORDER BY city") || len(q.Args) != 0 {
		t.Fatalf("şehir sorgusu: %+v", q)
	}
}
