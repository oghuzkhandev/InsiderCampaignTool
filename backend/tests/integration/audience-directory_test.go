package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// Kampanyadan bağımsız kişi dizini süzgeçleri (GET /api/audience, /api/audience/cities), gerçek
// PostgreSQL ile. Yalnızca campaigns_test.

type directoryJSON struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Items []struct {
		ID          int64   `json:"id"`
		FirstName   *string `json:"first_name"`
		City        string  `json:"city"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		SMSStatus   string  `json:"sms_status"`
		EmailStatus string  `json:"email_status"`
	} `json:"items"`
}

func directoryAPI(db *sql.DB) http.Handler {
	return httpapi.New(httpapi.Deps{Directory: audience.NewLister(db)})
}

func getDirectory(t *testing.T, h http.Handler, query string) directoryJSON {
	t.Helper()
	rec := apiDo(h, http.MethodGet, "/api/audience"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: durum %d: %s", query, rec.Code, rec.Body.String())
	}
	var out directoryJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: JSON değil: %v", query, err)
	}
	return out
}

func memberNames(d directoryJSON) []string {
	out := make([]string, 0, len(d.Items))
	for _, it := range d.Items {
		if it.FirstName == nil {
			out = append(out, "<adsız>")
			continue
		}
		out = append(out, *it.FirstName)
	}
	return out
}

func directoryFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	both := map[string]bool{"sms": true, "email": true}
	phone := func(n int) *string { return sp(fmt.Sprintf("+9055500000%02d", n)) }
	for _, f := range []memberFixture{
		{name: sp("Ada"), phone: phone(1), email: "ada@example.test", city: "Adana", consents: both},
		{name: sp("Berk"), phone: phone(2), email: "berk@example.test", city: "İstanbul", consents: map[string]bool{"sms": true}},
		// Telefonu yok, iki izin var: SMS için adres yok.
		{name: sp("Ceren"), email: "ceren@example.test", city: "Adana", consents: both},
		// Hiç izin kaydı yok.
		{name: sp("Deniz"), phone: phone(4), email: "deniz@example.test", city: "İzmir"},
		// Adı yok; "ada" yalnızca e-postasında geçer, şehri Ankara. Açıkça reddedilmiş SMS izni.
		{phone: phone(5), email: "ada.kaya@example.test", city: "Ankara", consents: map[string]bool{"sms": false, "email": true}},
		// E-posta adresi boş: izin var ama adres yok.
		{name: sp("Fatih"), phone: phone(6), email: "   ", city: "Ankara", consents: both},
	} {
		f.cartAgo = day
		insertMember(t, db, f)
	}
}

// Arama ad, e-posta ve telefonda yapılır, büyük/küçük harf gözetmez ve ŞEHRİ KAPSAMAZ:
// "ada" Adana'daki herkesi değil, yalnızca adında veya e-postasında "ada" geçenleri getirir.
func TestDirectorySearchMatchesNameEmailAndPhoneButNotCity(t *testing.T) {
	db := testdb.Open(t)
	directoryFixture(t, db)
	h := directoryAPI(db)

	got := getDirectory(t, h, "?search=ada")
	if got.Total != 2 || strings.Join(memberNames(got), ",") != "Ada,<adsız>" {
		t.Fatalf("'ada': toplam %d, kişiler %v (Ceren yalnızca şehri Adana olduğu için gelmemeli)", got.Total, memberNames(got))
	}
	if upper := getDirectory(t, h, "?search=ADA"); upper.Total != 2 {
		t.Fatalf("büyük harf aramada da 2 kişi gelmeli: %d", upper.Total)
	}
	if byPhone := getDirectory(t, h, "?search=0000004"); byPhone.Total != 1 || memberNames(byPhone)[0] != "Deniz" {
		t.Fatalf("telefon parçasıyla arama: %+v", memberNames(byPhone))
	}
	if byMail := getDirectory(t, h, "?search=ceren@example"); byMail.Total != 1 {
		t.Fatalf("e-posta parçasıyla arama: %d", byMail.Total)
	}
	if none := getDirectory(t, h, "?search=zzzz"); none.Total != 0 || len(none.Items) != 0 {
		t.Fatalf("eşleşmeyen arama boş olmalı: %+v", none)
	}
	// Joker karakterler ve SQL benzeri metin yalnızca metindir.
	for _, q := range []string{"%25", "_", "%27%20OR%201%3D1--"} {
		if got := getDirectory(t, h, "?search="+q); got.Total != 0 {
			t.Fatalf("%q joker veya SQL gibi davranmamalı: %d", q, got.Total)
		}
	}
}

func TestDirectoryCityFilterIsExact(t *testing.T) {
	db := testdb.Open(t)
	directoryFixture(t, db)
	h := directoryAPI(db)

	if got := getDirectory(t, h, "?city=Adana"); got.Total != 2 || strings.Join(memberNames(got), ",") != "Ada,Ceren" {
		t.Fatalf("Adana: %d %v", got.Total, memberNames(got))
	}
	if got := getDirectory(t, h, "?city="+url.QueryEscape("İstanbul")); got.Total != 1 {
		t.Fatalf("İstanbul: %d", got.Total)
	}
	// Tam eşleşme: küçük harf veya parça eşleşmez.
	for _, city := range []string{"adana", "Adan", "Ankar"} {
		if got := getDirectory(t, h, "?city="+city); got.Total != 0 {
			t.Fatalf("%q hiçbir şehirle eşleşmemeli: %d", city, got.Total)
		}
	}
}

func TestDirectoryChannelFilters(t *testing.T) {
	db := testdb.Open(t)
	directoryFixture(t, db)
	h := directoryAPI(db)
	names := func(query string) string { return strings.Join(memberNames(getDirectory(t, h, query)), ",") }

	for query, want := range map[string]string{
		"?sms=eligible":     "Ada,Berk,Fatih",
		"?sms=no_consent":   "Deniz,<adsız>",
		"?sms=no_address":   "Ceren",
		"?email=eligible":   "Ada,Ceren,<adsız>",
		"?email=no_consent": "Berk,Deniz",
		"?email=no_address": "Fatih",
	} {
		if got := names(query); got != want {
			t.Errorf("%s: %q, beklenen %q", query, got, want)
		}
	}
	// Süzgeçler AND ile birleşir.
	for query, want := range map[string]string{
		"?sms=eligible&email=eligible":               "Ada",
		"?sms=eligible&email=no_consent":             "Berk",
		"?city=Adana&sms=no_address":                 "Ceren",
		"?city=Ankara&email=eligible&search=ada":     "<adsız>",
		"?city=Ankara&sms=eligible&email=no_address": "Fatih",
		"?city=Adana&sms=no_consent":                 "",
	} {
		if got := names(query); got != want {
			t.Errorf("%s: %q, beklenen %q", query, got, want)
		}
	}
	// Toplam süzgeçlere uyan kişi sayısıdır (yalnızca sayfadakiler değil).
	if got := getDirectory(t, h, "?sms=eligible&limit=1"); got.Total != 3 || len(got.Items) != 1 {
		t.Fatalf("limit 1: toplam %d, sayfa %d", got.Total, len(got.Items))
	}
}

func TestDirectoryFiltersPaginateStably(t *testing.T) {
	db := testdb.Open(t)
	both := map[string]bool{"sms": true, "email": true}
	var want []int64
	for i := range 25 {
		want = append(want, insertMember(t, db, memberFixture{name: sp(fmt.Sprintf("Konya %02d", i)), phone: sp(fmt.Sprintf("+90555%07d", i)),
			email: fmt.Sprintf("k%02d@example.test", i), city: "Konya", cartAgo: day, consents: both}))
	}
	insertMember(t, db, memberFixture{name: sp("Diğer"), email: "diger@example.test", city: "Bursa", cartAgo: day, consents: both})
	h := directoryAPI(db)

	var seen []int64
	for page, wantLen := range []int{10, 10, 5, 0} {
		got := getDirectory(t, h, fmt.Sprintf("?city=Konya&sms=eligible&page=%d&limit=10", page+1))
		if got.Total != 25 || len(got.Items) != wantLen {
			t.Fatalf("sayfa %d: toplam %d, kişi %d, beklenen %d", page+1, got.Total, len(got.Items), wantLen)
		}
		for _, it := range got.Items {
			seen = append(seen, it.ID)
		}
	}
	if !slices.Equal(seen, want) {
		t.Fatalf("süzgeçli sayfalar birleşince tüm kişiler artan sırada gelmeli:\n %v\n %v", seen, want)
	}
}

func TestDirectoryRejectsInvalidFilters(t *testing.T) {
	db := testdb.Open(t)
	h := directoryAPI(db)
	for _, q := range []string{"?sms=maybe", "?email=ELIGIBLE", "?sms=eligible,no_consent", "?city=" + strings.Repeat("a", 65)} {
		if rec := apiDo(h, http.MethodGet, "/api/audience"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: durum %d, beklenen 400", q, rec.Code)
		}
	}
}

func TestDirectoryCitiesAreDistinctAndComplete(t *testing.T) {
	db := testdb.Open(t)
	directoryFixture(t, db)
	rec := apiDo(directoryAPI(db), http.MethodGet, "/api/audience/cities", "")
	var body struct {
		Cities []string `json:"cities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("durum %d: %s", rec.Code, rec.Body.String())
	}
	got := slices.Clone(body.Cities)
	slices.Sort(got)
	want := []string{"Adana", "Ankara", "İstanbul", "İzmir"}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("şehirler %v, beklenen %v", got, want)
	}
	// Boş veritabanında null değil boş liste.
	empty := testdb.Open(t)
	rec = apiDo(directoryAPI(empty), http.MethodGet, "/api/audience/cities", "")
	if !strings.Contains(rec.Body.String(), `"cities":[]`) {
		t.Fatalf("boş liste [] olmalı: %s", rec.Body.String())
	}
}

// Seed verisi üzerinde, SQL'den bağımsız Go oracle'ıyla: her kanal süzgeci kombinasyonu için
// toplam, listedeki her kişinin durumu ve sıra doğrudur.
func TestDirectoryFiltersMatchIndependentOracleOnSeedData(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	users := seed.Generate(now)
	h := directoryAPI(db)

	status := func(u seed.User, channel string) string {
		consented := false
		for _, c := range u.Consents {
			if c.Channel == channel && c.Granted {
				consented = true
			}
		}
		switch {
		case !consented:
			return "no_consent"
		case !eligible(u, channel):
			return "no_address"
		}
		return "eligible"
	}

	for _, city := range []string{"", users[0].City} {
		for _, sms := range []string{"", "eligible", "no_consent", "no_address"} {
			for _, mail := range []string{"", "eligible", "no_consent", "no_address"} {
				want := 0
				for _, u := range users {
					if (city == "" || u.City == city) &&
						(sms == "" || status(u, "sms") == sms) &&
						(mail == "" || status(u, "email") == mail) {
						want++
					}
				}
				q := "?limit=20&sms=" + sms + "&email=" + mail + "&city=" + url.QueryEscape(city)
				first := getDirectory(t, h, q)
				if first.Total != want {
					t.Fatalf("city=%q sms=%q email=%q: toplam %d, beklenen %d", city, sms, mail, first.Total, want)
				}
				var prev int64
				for _, it := range first.Items {
					if it.ID <= prev {
						t.Fatalf("sıra kesin artan olmalı: %d ardından %d", prev, it.ID)
					}
					prev = it.ID
					if (sms != "" && it.SMSStatus != sms) || (mail != "" && it.EmailStatus != mail) || (city != "" && it.City != city) {
						t.Fatalf("listedeki kişi süzgece uymuyor: %+v (city=%q sms=%q email=%q)", it, city, sms, mail)
					}
				}
			}
		}
	}

	// ASCII bir parçayla arama: e-postada geçenler (büyük/küçük harf farkı Go ile PostgreSQL arasında
	// değişmesin diye yalnızca ASCII).
	term := "kullanici-00"
	want := 0
	for _, u := range users {
		if strings.Contains(strings.ToLower(u.Email), term) {
			want++
		}
	}
	if want == 0 {
		t.Fatal("oracle: seed'de eşleşen e-posta olmalı")
	}
	if got := getDirectory(t, h, "?search="+term); got.Total != want {
		t.Fatalf("arama toplamı %d, beklenen %d", got.Total, want)
	}
}
