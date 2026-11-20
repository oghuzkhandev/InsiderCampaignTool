package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// ADR-031: segmente uyan kişiler listesi, gerçek PostgreSQL ile. Yalnızca campaigns_test.

const istanbulCartSegment = `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30}]}`

func mustSegment(t *testing.T, raw string) segment.Segment {
	t.Helper()
	seg, issues := segment.Validate(withUnsupported(raw))
	if len(issues) != 0 {
		t.Fatalf("geçerli segment bekleniyordu: %v", issues)
	}
	return seg
}

type memberFixture struct {
	name     *string
	phone    *string
	email    string
	city     string
	cartAgo  time.Duration
	consents map[string]bool // yoksa kayıt yok (izin yok)
}

func sp(s string) *string { return &s }

// insertMember, istenen ad, telefon ve e-postayla (NULL, boş veya dolu) bir kullanıcı yazar.
func insertMember(t *testing.T, db *sql.DB, f memberFixture) int64 {
	t.Helper()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (first_name, email, phone, city, last_cart_add_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		f.name, f.email, f.phone, f.city, now.Add(-f.cartAgo),
	).Scan(&id)
	if err != nil {
		t.Fatalf("%s: %v", f.email, err)
	}
	for channel, granted := range f.consents {
		if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, $2, $3)`, id, channel, granted); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func listAll(t *testing.T, db *sql.DB, seg segment.Segment, limit int) (members []audience.Member, total int) {
	t.Helper()
	for page := 1; ; page++ {
		got, err := audience.ListMembers(t.Context(), db, seg, now, page, limit)
		if err != nil {
			t.Fatal(err)
		}
		if page > 1 && got.Total != total {
			t.Fatalf("sayfa %d: toplam değişti (%d → %d)", page, total, got.Total)
		}
		total = got.Total
		if len(got.Members) == 0 {
			return members, total
		}
		members = append(members, got.Members...)
	}
}

// Eşleşen herkes listelenir: izni olmayan, adresi olmayan ve adı olmayanlar dahil. Eksik bilgi
// nil, uygunluk durum + sebeptir.
func TestAudienceMembersListsEveryMatchedPersonWithExplicitStatus(t *testing.T) {
	db := testdb.Open(t)
	both := map[string]bool{"sms": true, "email": true}

	a := insertMember(t, db, memberFixture{name: sp("Ayşe"), phone: sp("+905550000001"), email: "a@example.test", city: "İstanbul", cartAgo: 5 * day, consents: both})
	b := insertMember(t, db, memberFixture{city: "İstanbul", email: "b@example.test", cartAgo: 5 * day, consents: both}) // ad ve telefon NULL
	c := insertMember(t, db, memberFixture{name: sp("   "), phone: sp("  "), email: "c@example.test", city: "İstanbul", cartAgo: 5 * day,
		consents: map[string]bool{"sms": false, "email": true}}) // boş ad ve telefon; SMS izni reddedilmiş
	// d: hiç izin kaydı yok. e: e-posta adresi boş.
	d := insertMember(t, db, memberFixture{name: sp("Deniz"), phone: sp("+905550000004"), email: "d@example.test", city: "İstanbul", cartAgo: 5 * day})
	e := insertMember(t, db, memberFixture{name: sp("Ece"), phone: sp("+905550000005"), email: "   ", city: "İstanbul", cartAgo: 5 * day, consents: both})
	// Eşleşmeyenler: başka şehir ve sepet penceresi dışı.
	insertMember(t, db, memberFixture{name: sp("Hande"), phone: sp("+905550000006"), email: "h@example.test", city: "Ankara", cartAgo: 5 * day, consents: both})
	insertMember(t, db, memberFixture{name: sp("Ilgın"), phone: sp("+905550000007"), email: "i@example.test", city: "İstanbul", cartAgo: 40 * day, consents: both})

	page, err := audience.ListMembers(t.Context(), db, mustSegment(t, istanbulCartSegment), now, 1, 10)
	if err != nil {
		t.Fatal(err)
	}

	if page.Total != 5 || len(page.Members) != 5 {
		t.Fatalf("5 kişi bekleniyordu (izinsizler dahil): toplam %d, sayfa %d", page.Total, len(page.Members))
	}
	wantIDs := []int64{a, b, c, d, e}
	for i, m := range page.Members {
		if m.ID != wantIDs[i] {
			t.Fatalf("sıra kimliğe göre artan olmalı: %d. kişi %d, beklenen %d", i, m.ID, wantIDs[i])
		}
	}

	type want struct {
		name, phone, email *string
		sms, email_        audience.ChannelStatus
	}
	for i, w := range []want{
		{sp("Ayşe"), sp("+905550000001"), sp("a@example.test"), audience.ChannelEligible, audience.ChannelEligible},
		{nil, nil, sp("b@example.test"), audience.ChannelNoAddress, audience.ChannelEligible},
		// SMS izni reddedilmiş ve telefon da boş: izin yokluğu öncelikli sebeptir.
		{nil, nil, sp("c@example.test"), audience.ChannelNoConsent, audience.ChannelEligible},
		// İzin kaydı hiç yok: iki kanalda da uygun değil ama listede.
		{sp("Deniz"), sp("+905550000004"), sp("d@example.test"), audience.ChannelNoConsent, audience.ChannelNoConsent},
		{sp("Ece"), sp("+905550000005"), nil, audience.ChannelEligible, audience.ChannelNoAddress},
	} {
		m := page.Members[i]
		if !samePtr(m.FirstName, w.name) || !samePtr(m.Phone, w.phone) || !samePtr(m.Email, w.email) {
			t.Errorf("%d. kişi ad/telefon/e-posta = %v/%v/%v, beklenen %v/%v/%v", i, deref(m.FirstName), deref(m.Phone), deref(m.Email), deref(w.name), deref(w.phone), deref(w.email))
		}
		if m.SMSStatus != w.sms || m.EmailStatus != w.email_ {
			t.Errorf("%d. kişi sms/e-posta durumu = %s/%s, beklenen %s/%s", i, m.SMSStatus, m.EmailStatus, w.sms, w.email_)
		}
		if m.City != "İstanbul" {
			t.Errorf("%d. kişi şehri = %q", i, m.City)
		}
	}

	// Listedeki uygunluk, kitle sayılarıyla aynı kuraldır (ADR-023).
	counts, err := audience.Count(t.Context(), db, mustSegment(t, istanbulCartSegment), now)
	if err != nil {
		t.Fatal(err)
	}
	var sms, mail int
	for _, m := range page.Members {
		if m.SMSStatus == audience.ChannelEligible {
			sms++
		}
		if m.EmailStatus == audience.ChannelEligible {
			mail++
		}
	}
	if counts.Total != page.Total || counts.SMS != sms || counts.Email != mail {
		t.Fatalf("liste sayıları kitle sayılarıyla aynı olmalı: liste %d/%d/%d, sayılar %d/%d/%d",
			page.Total, sms, mail, counts.Total, counts.SMS, counts.Email)
	}
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// Sayfalar kararlı sırayla, çakışmadan ve boşluksuz birbirini izler; toplam her sayfada aynıdır.
func TestAudienceMembersPagesAreStableDisjointAndComplete(t *testing.T) {
	db := testdb.Open(t)
	var want []int64
	for i := range 25 {
		want = append(want, insertMember(t, db, memberFixture{
			name: sp(fmt.Sprintf("Kişi %02d", i)), phone: sp(fmt.Sprintf("+90555%07d", i)),
			email: fmt.Sprintf("k%02d@example.test", i), city: "İstanbul", cartAgo: day,
			consents: map[string]bool{"sms": i%2 == 0},
		}))
	}
	insertMember(t, db, memberFixture{email: "baska@example.test", city: "Ankara", cartAgo: day}) // eşleşmez
	seg := mustSegment(t, istanbulCartSegment)

	var seen []int64
	for page, wantLen := range []int{10, 10, 5, 0, 0} {
		got, err := audience.ListMembers(t.Context(), db, seg, now, page+1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != 25 {
			t.Fatalf("sayfa %d: toplam %d, beklenen 25", page+1, got.Total)
		}
		if len(got.Members) != wantLen {
			t.Fatalf("sayfa %d: %d kişi, beklenen %d", page+1, len(got.Members), wantLen)
		}
		if got.Members == nil {
			t.Fatalf("sayfa %d: boş sayfa nil değil boş liste olmalı", page+1)
		}
		for _, m := range got.Members {
			seen = append(seen, m.ID)
		}
	}
	if !slices.Equal(seen, want) {
		t.Fatalf("sayfaların birleşimi tüm kişiler olmalı, artan sırada:\n alınan  %v\n beklenen %v", seen, want)
	}

	// Aynı sorgu tekrar çalışınca aynı sayfa gelir (kararlı sıra).
	first, _ := audience.ListMembers(t.Context(), db, seg, now, 2, 10)
	again, _ := audience.ListMembers(t.Context(), db, seg, now, 2, 10)
	if !slices.Equal(ids(first.Members), ids(again.Members)) {
		t.Fatal("aynı sayfa iki okumada aynı olmalı")
	}

	// Farklı sayfa boyutları aynı kişileri aynı sırayla verir.
	for _, limit := range []int{1, 7, 25, 50} {
		all, total := listAll(t, db, seg, limit)
		if total != 25 || !slices.Equal(ids(all), want) {
			t.Fatalf("limit %d: toplam %d, kişiler %v", limit, total, ids(all))
		}
	}
}

func ids(ms []audience.Member) []int64 {
	out := make([]int64, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func TestAudienceMembersNoMatchGivesZeroTotalAndEmptyPage(t *testing.T) {
	db := testdb.Open(t)
	insertMember(t, db, memberFixture{name: sp("Ada"), email: "ada@example.test", city: "İstanbul", cartAgo: day})

	got, err := audience.ListMembers(t.Context(), db, mustSegment(t, `{"all":[{"field":"city","op":"eq","value":"Kars"}]}`), now, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 || got.Members == nil || len(got.Members) != 0 {
		t.Fatalf("sıfır eşleşme: toplam %d, sayfa %v", got.Total, got.Members)
	}
}

// SQL benzeri değer yalnızca değerdir; tablolar bozulmaz.
func TestAudienceMembersTreatsSQLLikeValuesAsData(t *testing.T) {
	db := testdb.Open(t)
	insertMember(t, db, memberFixture{name: sp("Ada"), email: "ada@example.test", city: "İstanbul", cartAgo: day})

	got, err := audience.ListMembers(t.Context(), db, mustSegment(t, `{"all":[{"field":"city","op":"eq","value":"x' OR '1'='1"}]}`), now, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 || len(got.Members) != 0 {
		t.Fatalf("enjeksiyon denemesi hiçbir kullanıcıyla eşleşmemeli: %+v", got)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("kullanıcı tablosu bozulmamalı: n=%d err=%v", n, err)
	}
}

// Seed verisi üzerinde, SQL'den bağımsız Go oracle'ıyla: eşleşen herkes listelenir, her kişinin
// ad, telefon ve durumları doğrudur ve liste sayıları kitle sayılarıyla (ADR-023) birebir aynıdır.
func TestAudienceMembersMatchIndependentOracleOnSeedData(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	users := seed.Generate(now)
	byEmail := make(map[string]seed.User, len(users))
	for _, u := range users {
		byEmail[u.Email] = u
	}

	wantStatus := func(u seed.User, channel string) audience.ChannelStatus {
		consented := false
		for _, c := range u.Consents {
			if c.Channel == channel && c.Granted {
				consented = true
			}
		}
		switch {
		case !consented:
			return audience.ChannelNoConsent
		case !eligible(u, channel):
			return audience.ChannelNoAddress
		default:
			return audience.ChannelEligible
		}
	}
	present := func(s *string, trim func(string) string) bool { return s != nil && trim(*s) != "" }
	ascii := func(s string) string { return strings.Trim(s, " \t\n\r\f\v") }

	segments := map[string]string{
		"örnek brief":     `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30},{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`,
		"şehir listesi":   `{"all":[{"field":"city","op":"in","value":["Ankara","İzmir"]},{"field":"last_cart_add_at","op":"within_last_days","value":7}]}`,
		"sipariş aralığı": `{"all":[{"field":"order_count","op":"gte","value":2},{"field":"order_count","op":"lte","value":4}]}`,
		"eşleşme yok":     `{"all":[{"field":"city","op":"eq","value":"Kars"}]}`,
	}
	for name, raw := range segments {
		t.Run(name, func(t *testing.T) {
			seg := mustSegment(t, raw)
			members, total := listAll(t, db, seg, 50)

			var wantTotal, wantSMS, wantEmail int
			for _, u := range users {
				if !matches(u, seg) {
					continue
				}
				wantTotal++
				if eligible(u, "sms") {
					wantSMS++
				}
				if eligible(u, "email") {
					wantEmail++
				}
			}
			if total != wantTotal || len(members) != wantTotal {
				t.Fatalf("toplam %d, listelenen %d, beklenen %d", total, len(members), wantTotal)
			}

			var sms, mail int
			var prev int64
			for _, m := range members {
				if m.ID <= prev {
					t.Fatalf("sıra kimliğe göre kesin artan olmalı: %d ardından %d", prev, m.ID)
				}
				prev = m.ID
				if m.Email == nil {
					t.Fatalf("seed'de adresi boş kullanıcı yok, e-posta dolu olmalı: %+v", m)
				}
				u, ok := byEmail[*m.Email]
				if !ok || !matches(u, seg) {
					t.Fatalf("listedeki kişi segmente uymalı: %+v", m)
				}
				if got, want := m.FirstName != nil, present(u.FirstName, strings.TrimSpace); got != want {
					t.Fatalf("%s: ad var mı = %v, beklenen %v", u.Email, got, want)
				}
				if got, want := m.Phone != nil, present(u.Phone, ascii); got != want {
					t.Fatalf("%s: telefon var mı = %v, beklenen %v", u.Email, got, want)
				}
				if m.City != u.City {
					t.Fatalf("%s: şehir %q, beklenen %q", u.Email, m.City, u.City)
				}
				if got, want := m.SMSStatus, wantStatus(u, "sms"); got != want {
					t.Fatalf("%s: sms durumu %s, beklenen %s", u.Email, got, want)
				}
				if got, want := m.EmailStatus, wantStatus(u, "email"); got != want {
					t.Fatalf("%s: e-posta durumu %s, beklenen %s", u.Email, got, want)
				}
				if m.SMSStatus == audience.ChannelEligible {
					sms++
				}
				if m.EmailStatus == audience.ChannelEligible {
					mail++
				}
			}
			if sms != wantSMS || mail != wantEmail {
				t.Fatalf("uygun sayıları %d/%d, beklenen %d/%d", sms, mail, wantSMS, wantEmail)
			}

			// Kitle önizlemesindeki üç sayı ile birebir aynıdır.
			counts, err := audience.Count(t.Context(), db, seg, now)
			if err != nil {
				t.Fatal(err)
			}
			if counts.Total != total || counts.SMS != sms || counts.Email != mail {
				t.Fatalf("liste %d/%d/%d, kitle sayıları %d/%d/%d", total, sms, mail, counts.Total, counts.SMS, counts.Email)
			}
		})
	}
}

type memberPageJSON struct {
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

func sizeOfTables(t *testing.T, db *sql.DB) [4]int {
	t.Helper()
	var out [4]int
	for i, table := range []string{"users", "user_consents", "campaigns", "messages"} {
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// Gerçek yığın: örnek brief'ten kampanya oluşturulur, liste kampanyanın kayıtlı segmentiyle
// okunur; sayfalama, hata kodları ve salt okunurluk doğrulanır.
func TestAudienceMembersEndpointEndToEnd(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	both := map[string]bool{"sms": true, "email": true}
	for i := range 13 { // örnek brief'e uyan 13 kişi: iki sayfa
		consents := both
		if i == 12 {
			consents = nil // izni olmayan kişi de listelenir
		}
		insertMember(t, db, memberFixture{name: sp(fmt.Sprintf("Kişi %02d", i)), phone: sp(fmt.Sprintf("+90555%07d", i)),
			email: fmt.Sprintf("k%02d@example.test", i), city: "İstanbul", cartAgo: 3 * day, consents: consents})
	}
	insertMember(t, db, memberFixture{name: sp("Uzak"), email: "uzak@example.test", city: "Ankara", cartAgo: 3 * day, consents: both})

	created := apiPost(h, exampleBriefText)
	if created.Code != http.StatusCreated {
		t.Fatalf("kampanya oluşmadı: %d %s", created.Code, created.Body.String())
	}
	c := decodeCampaign(t, created)
	if c.Audience.Total != 13 {
		t.Fatalf("kitle toplamı %d, beklenen 13", c.Audience.Total)
	}
	before := sizeOfTables(t, db)

	get := func(query string) (*memberPageJSON, int) {
		rec := apiDo(h, http.MethodGet, "/api/campaigns/"+c.ID+"/audience"+query, "")
		var body memberPageJSON
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("JSON değil: %v\n%s", err, rec.Body.String())
			}
		}
		return &body, rec.Code
	}

	first, status := get("")
	if status != http.StatusOK || first.Page != 1 || first.Limit != 10 || first.Total != 13 || len(first.Items) != 10 {
		t.Fatalf("ilk sayfa 10 kişi ve toplam 13 olmalı: durum %d, %+v", status, first)
	}
	second, _ := get("?page=2")
	if second.Total != 13 || len(second.Items) != 3 {
		t.Fatalf("ikinci sayfa 3 kişi olmalı: %+v", second)
	}
	// İzin kaydı olmayan son kişi listelenir ve kanal için uygun değildir.
	last := second.Items[2]
	if last.SMSStatus != "no_consent" || last.EmailStatus != "no_consent" {
		t.Fatalf("izinsiz kişi durumları: %+v", last)
	}

	// Kitle önizlemesindeki sayılarla tutarlı: uygun sayıları liste durumlarından türetilir.
	var sms, mail int
	for _, page := range []*memberPageJSON{first, second} {
		for _, it := range page.Items {
			if it.SMSStatus == "eligible" {
				sms++
			}
			if it.EmailStatus == "eligible" {
				mail++
			}
		}
	}
	if sms != c.Audience.SMS || mail != c.Audience.Email {
		t.Fatalf("liste uygunluğu kampanya sayılarıyla aynı olmalı: liste %d/%d, kampanya %d/%d", sms, mail, c.Audience.SMS, c.Audience.Email)
	}

	beyond, status := get("?page=5")
	if status != http.StatusOK || beyond.Total != 13 || len(beyond.Items) != 0 {
		t.Fatalf("son sayfanın ötesi boş liste ve doğru toplam olmalı: %d %+v", status, beyond)
	}
	if capped, _ := get("?limit=500"); capped.Limit != 50 || len(capped.Items) != 13 {
		t.Fatalf("limit 50'ye indirilmeli: %+v", capped)
	}

	for _, bad := range []string{"?page=0", "?page=x", "?limit=0", "?limit=-1", "?page=100001"} {
		if _, status := get(bad); status != http.StatusBadRequest {
			t.Errorf("%s: durum %d, beklenen 400", bad, status)
		}
	}
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		rec := apiDo(h, http.MethodGet, "/api/campaigns/"+id+"/audience", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: durum %d, beklenen 404", id, rec.Code)
		}
	}
	if rec := apiDo(h, http.MethodPost, "/api/campaigns/"+c.ID+"/audience", `{}`); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: durum %d, beklenen 405", rec.Code)
	}

	if after := sizeOfTables(t, db); after != before {
		t.Fatalf("liste hiçbir kayıt yazmamalı: önce %v, sonra %v", before, after)
	}
}
