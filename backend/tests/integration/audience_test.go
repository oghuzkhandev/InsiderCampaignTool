package integration

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// now, tarih sınırı testlerinde kullanılan sabit referans zamandır.
var now = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

type fixtureUser struct {
	email    string
	city     string
	cartAgo  *time.Duration // nil: hiç sepete eklememiş
	buyAgo   *time.Duration // nil: hiç satın almamış
	orders   int
	consents map[string]bool // yoksa kayıt yok (izin yok)
	noPhone  bool            // true: telefon yok (SMS adresi yok)
	phone    *string         // nil ve noPhone false ise otomatik telefon verilir
	email2   *string         // nil değilse e-posta adresi yerine bu değer yazılır
}

func ago(d time.Duration) *time.Duration { return &d }

const day = 24 * time.Hour

func seedFixture(t *testing.T, db *sql.DB, users []fixtureUser) {
	t.Helper()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	for i, u := range users {
		var cart, buy any
		if u.cartAgo != nil {
			cart = now.Add(-*u.cartAgo)
		}
		if u.buyAgo != nil {
			buy = now.Add(-*u.buyAgo)
		}
		// Kanal uygunluğu izin ve adres ister (ADR-023): aksi belirtilmedikçe her
		// kullanıcıya telefon verilir.
		var phone any
		switch {
		case u.noPhone:
		case u.phone != nil:
			phone = *u.phone
		default:
			phone = fmt.Sprintf("+90555%07d", i)
		}
		email := u.email
		if u.email2 != nil {
			email = *u.email2
		}
		var id int64
		err := db.QueryRow(
			`INSERT INTO users (email, city, last_cart_add_at, last_purchase_at, order_count, phone)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			email, u.city, cart, buy, u.orders, phone,
		).Scan(&id)
		if err != nil {
			t.Fatalf("%s: %v", u.email, err)
		}
		for channel, granted := range u.consents {
			if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, $2, $3)`, id, channel, granted); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func count(t *testing.T, db *sql.DB, raw string) (total, sms, email int) {
	t.Helper()
	seg, issues := segment.Validate(withUnsupported(raw))
	if len(issues) != 0 {
		t.Fatalf("geçerli segment bekleniyordu: %v", issues)
	}
	c, err := audience.Count(t.Context(), db, seg, now)
	if err != nil {
		t.Fatal(err)
	}
	return c.Total, c.SMS, c.Email
}

func TestAudienceCounts(t *testing.T) {
	db := testdb.Open(t)

	both := map[string]bool{"sms": true, "email": true}
	seedFixture(t, db, []fixtureUser{
		// Eşleşenler.
		{email: "iki-kanal", city: "İstanbul", cartAgo: ago(5 * day), consents: both},
		{email: "yalniz-sms", city: "İstanbul", cartAgo: ago(29 * day), buyAgo: ago(40 * day), consents: map[string]bool{"sms": true, "email": false}},
		{email: "sinirda-sepet", city: "İstanbul", cartAgo: ago(30 * day), consents: map[string]bool{"email": true}}, // alt sınır dahil
		{email: "izin-kaydi-yok", city: "İstanbul", cartAgo: ago(3 * day), buyAgo: ago(31 * day), orders: 7},

		// Dışarıda kalanlar.
		{email: "pencere-disi", city: "İstanbul", cartAgo: ago(30*day + time.Second), consents: both},
		{email: "yakin-alisveris", city: "İstanbul", cartAgo: ago(5 * day), buyAgo: ago(20 * day), consents: both},
		{email: "sinirda-alisveris", city: "İstanbul", cartAgo: ago(3 * day), buyAgo: ago(30 * day), consents: both}, // satın alma pencerede sayılır
		{email: "baska-sehir", city: "Ankara", cartAgo: ago(5 * day), consents: both},
		{email: "sepet-yok", city: "İstanbul", consents: both},
	})

	const exampleBrief = `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30},{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`

	t.Run("örnek brief", func(t *testing.T) {
		total, sms, email := count(t, db, exampleBrief)
		if total != 4 || sms != 2 || email != 2 {
			t.Fatalf("toplam/sms/e-posta = %d/%d/%d, beklenen 4/2/2", total, sms, email)
		}
	})

	t.Run("sıfır eşleşme", func(t *testing.T) {
		total, sms, email := count(t, db, `{"all":[{"field":"city","op":"eq","value":"Kars"}]}`)
		if total != 0 || sms != 0 || email != 0 {
			t.Fatalf("0/0/0 bekleniyordu: %d/%d/%d", total, sms, email)
		}
	})

	t.Run("eşleşme var ama uygun alıcı yok", func(t *testing.T) {
		total, sms, email := count(t, db, `{"all":[{"field":"order_count","op":"eq","value":7}]}`)
		if total != 1 || sms != 0 || email != 0 {
			t.Fatalf("1/0/0 bekleniyordu: %d/%d/%d", total, sms, email)
		}
	})

	t.Run("şehir listesi ve sipariş sayısı", func(t *testing.T) {
		total, _, _ := count(t, db, `{"all":[{"field":"city","op":"in","value":["Ankara","Kars"]}]}`)
		if total != 1 {
			t.Fatalf("toplam = %d, beklenen 1", total)
		}
		total, _, _ = count(t, db, `{"all":[{"field":"order_count","op":"gte","value":1},{"field":"order_count","op":"lte","value":7}]}`)
		if total != 1 {
			t.Fatalf("toplam = %d, beklenen 1", total)
		}
	})

	t.Run("hiç alışveriş yapmamış kullanıcı satın almamış sayılır", func(t *testing.T) {
		total, _, _ := count(t, db, `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`)
		// iki-kanal, yalniz-sms, sinirda-sepet, izin-kaydi-yok, pencere-disi, sepet-yok
		if total != 6 {
			t.Fatalf("toplam = %d, beklenen 6", total)
		}
	})

	t.Run("SQL benzeri değer yalnızca değerdir", func(t *testing.T) {
		total, _, _ := count(t, db, `{"all":[{"field":"city","op":"eq","value":"x' OR '1'='1"}]}`)
		if total != 0 {
			t.Fatalf("enjeksiyon denemesi hiçbir kullanıcıyla eşleşmemeli, toplam = %d", total)
		}
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil || n != 9 {
			t.Fatalf("kullanıcı tablosu bozulmamalı: n=%d err=%v", n, err)
		}
	})
}

// matches, segment koşullarını SQL'den bağımsız olarak Go'da değerlendirir
// (oracle). Pencere: alan değeri sınırdan önce değilse "son N gün içinde".
func matches(u seed.User, seg segment.Segment) bool {
	within := func(ts *time.Time, days int) bool {
		return ts != nil && !ts.Before(now.Add(-time.Duration(days)*day))
	}
	for _, c := range seg.Conditions {
		var ok bool
		switch {
		case c.Field == "city" && c.Op == "eq":
			ok = u.City == c.Text
		case c.Field == "city" && c.Op == "in":
			for _, v := range c.Texts {
				ok = ok || u.City == v
			}
		case c.Field == "last_cart_add_at" && c.Op == "within_last_days":
			ok = within(u.LastCartAddAt, c.Number)
		case c.Field == "last_cart_add_at" && c.Op == "not_within_last_days":
			ok = !within(u.LastCartAddAt, c.Number)
		case c.Field == "last_purchase_at" && c.Op == "within_last_days":
			ok = within(u.LastPurchaseAt, c.Number)
		case c.Field == "last_purchase_at" && c.Op == "not_within_last_days":
			ok = !within(u.LastPurchaseAt, c.Number)
		case c.Field == "order_count" && c.Op == "eq":
			ok = u.OrderCount == c.Number
		case c.Field == "order_count" && c.Op == "gte":
			ok = u.OrderCount >= c.Number
		case c.Field == "order_count" && c.Op == "lte":
			ok = u.OrderCount <= c.Number
		}
		if !ok {
			return false
		}
	}
	return true
}

// eligible, kullanıcının kanala uygun olup olmadığını SQL'den bağımsız olarak
// hesaplar (ADR-023): o kanal için açık izin VE adres (telefon/e-posta).
func eligible(u seed.User, channel string) bool {
	consented := false
	for _, c := range u.Consents {
		if c.Channel == channel && c.Granted {
			consented = true
		}
	}
	if !consented {
		return false
	}
	nonBlank := func(s string) bool { return strings.Trim(s, " \t\n\r\f\v") != "" }
	switch channel {
	case "sms":
		return u.Phone != nil && nonBlank(*u.Phone)
	case "email":
		return nonBlank(u.Email)
	}
	return false
}

// TestAudienceMatchesIndependentOracle, seed verisi üzerinde SQL sonuçlarının
// Go'da bağımsız hesaplanan sayılarla eşleştiğini doğrular.
func TestAudienceMatchesIndependentOracle(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	users := seed.Generate(now)

	segments := map[string]string{
		"örnek brief":             `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30},{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`,
		"şehir listesi":           `{"all":[{"field":"city","op":"in","value":["Ankara","İzmir"]},{"field":"last_cart_add_at","op":"within_last_days","value":7}]}`,
		"sipariş aralığı":         `{"all":[{"field":"order_count","op":"gte","value":2},{"field":"order_count","op":"lte","value":4}]}`,
		"alışveriş yapmayanlar":   `{"all":[{"field":"last_purchase_at","op":"not_within_last_days","value":90}]}`,
		"sepet yok sayılanlar":    `{"all":[{"field":"last_cart_add_at","op":"not_within_last_days","value":15},{"field":"city","op":"eq","value":"Bursa"}]}`,
		"eşleşme yok":             `{"all":[{"field":"city","op":"eq","value":"Kars"}]}`,
		"sipariş sayısı tam eşit": `{"all":[{"field":"order_count","op":"eq","value":3}]}`,
	}
	for name, raw := range segments {
		t.Run(name, func(t *testing.T) {
			seg, issues := segment.Validate(withUnsupported(raw))
			if len(issues) != 0 {
				t.Fatalf("geçerli segment bekleniyordu: %v", issues)
			}
			var want audience.Counts
			for _, u := range users {
				if !matches(u, seg) {
					continue
				}
				want.Total++
				if eligible(u, "sms") {
					want.SMS++
				}
				if eligible(u, "email") {
					want.Email++
				}
			}
			got, err := audience.Count(t.Context(), db, seg, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("SQL = %+v, oracle = %+v", got, want)
			}
			if name != "eşleşme yok" && got.Total == 0 {
				t.Fatal("seed verisi bu segment için en az bir kullanıcı üretmeli")
			}
		})
	}
}

// ADR-023 (B): kanala uygunluk açık izin VE adres ister. İzinli ama telefonu
// (veya e-postası) olmayan kullanıcı toplamda sayılır, ilgili kanalda sayılmaz.
func TestAudienceCountsRequireAddressForChannel(t *testing.T) {
	db := testdb.Open(t)

	both := map[string]bool{"sms": true, "email": true}
	blank := "   "
	seedFixture(t, db, []fixtureUser{
		{email: "tam", city: "İstanbul", consents: both},
		{email: "telefonsuz", city: "İstanbul", consents: both, noPhone: true},
		{email: "telefon-bos", city: "İstanbul", consents: both, phone: &blank},
		{email: "eposta-bos", city: "İstanbul", consents: both, email2: &blank},
		{email: "telefonsuz-sadece-eposta", city: "İstanbul", consents: map[string]bool{"email": true}, noPhone: true},
		{email: "telefonlu-izinsiz", city: "İstanbul", consents: map[string]bool{"sms": false, "email": false}},
		{email: "telefonsuz-izinsiz", city: "İstanbul", noPhone: true},
	})

	total, sms, email := count(t, db, `{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}`)

	// Toplam: hepsi. SMS: yalnızca "tam" ve "eposta-bos" (telefonu var ve izinli).
	// E-posta: "tam", "telefonsuz", "telefon-bos", "telefonsuz-sadece-eposta"
	// ("eposta-bos" adressiz, izinsizler dışarıda).
	if total != 7 || sms != 2 || email != 4 {
		t.Fatalf("toplam/sms/e-posta = %d/%d/%d, beklenen 7/2/4", total, sms, email)
	}
}
