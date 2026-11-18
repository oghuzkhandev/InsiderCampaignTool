package integration

import (
	"database/sql"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// fingerprint, kullanıcıların ve izinlerin içeriğinin özetini döndürür
// (updated_at ve created_at hariç).
func fingerprint(t *testing.T, db *sql.DB) (users, consents string, nUsers, nConsents int) {
	t.Helper()
	err := db.QueryRow(`SELECT coalesce(md5(string_agg(concat_ws('|', email, first_name, phone, city, language, last_cart_add_at, last_purchase_at, order_count), ',' ORDER BY email)), ''), count(*) FROM users`).Scan(&users, &nUsers)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`SELECT coalesce(md5(string_agg(concat_ws('|', u.email, c.channel, c.granted), ',' ORDER BY u.email, c.channel)), ''), count(*)
		FROM user_consents c JOIN users u ON u.id = c.user_id`).Scan(&consents, &nConsents)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestSeedIsIdempotent(t *testing.T) {
	db := testdb.Open(t)
	generated := seed.Generate(now)
	wantConsents := 0
	for _, u := range generated {
		wantConsents += len(u.Consents)
	}

	res, err := seed.Run(t.Context(), db, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != len(generated) || res.Skipped != 0 {
		t.Fatalf("ilk çalıştırma = %+v, beklenen %d eklenen", res, len(generated))
	}
	u1, c1, nUsers, nConsents := fingerprint(t, db)
	if nUsers != len(generated) || nConsents != wantConsents {
		t.Fatalf("kullanıcı/izin sayısı = %d/%d, beklenen %d/%d", nUsers, nConsents, len(generated), wantConsents)
	}

	// Tekrar çalıştırma, daha sonraki bir referans zamanla bile, hiçbir şeyi değiştirmez.
	for _, later := range []time.Time{now, now.Add(30 * day), now.Add(400 * day)} {
		res, err := seed.Run(t.Context(), db, later)
		if err != nil {
			t.Fatal(err)
		}
		if res.Inserted != 0 || res.Skipped != len(generated) {
			t.Fatalf("tekrar çalıştırma = %+v, beklenen hepsi atlanmış", res)
		}
		u2, c2, n2, m2 := fingerprint(t, db)
		if u2 != u1 || c2 != c1 || n2 != nUsers || m2 != nConsents {
			t.Fatal("tekrar çalıştırma kullanıcı veya izin verisini değiştirmemeli")
		}
	}
}

func TestSeedDoesNotTouchExistingUsers(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}

	// Elle yapılan değişiklikler: tarih, izin değeri, silinmiş izin kaydı.
	custom := now.Add(-2 * day)
	if _, err := db.Exec(`UPDATE users SET last_cart_add_at = $1, city = 'Kars' WHERE email = 'senaryo-iki-kanal@example.test'`, custom); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE user_consents SET granted = false
		WHERE channel = 'sms' AND user_id = (SELECT id FROM users WHERE email = 'senaryo-iki-kanal@example.test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM user_consents
		WHERE channel = 'email' AND user_id = (SELECT id FROM users WHERE email = 'senaryo-iki-kanal@example.test')`); err != nil {
		t.Fatal(err)
	}
	u1, c1, _, _ := fingerprint(t, db)

	res, err := seed.Run(t.Context(), db, now.Add(45*day))
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 0 {
		t.Fatalf("var olan kullanıcı yeniden eklenmemeli: %+v", res)
	}
	u2, c2, _, _ := fingerprint(t, db)
	if u1 != u2 || c1 != c2 {
		t.Fatal("elle değiştirilmiş kullanıcı ve izinler olduğu gibi kalmalı")
	}

	var cart time.Time
	var city string
	var smsGranted bool
	var emailRows int
	if err := db.QueryRow(`SELECT last_cart_add_at, city FROM users WHERE email = 'senaryo-iki-kanal@example.test'`).Scan(&cart, &city); err != nil {
		t.Fatal(err)
	}
	if !cart.Equal(custom) || city != "Kars" {
		t.Fatalf("tarih ve şehir korunmalı: %v %s", cart, city)
	}
	if err := db.QueryRow(`SELECT granted FROM user_consents WHERE channel = 'sms' AND user_id = (SELECT id FROM users WHERE email = 'senaryo-iki-kanal@example.test')`).Scan(&smsGranted); err != nil || smsGranted {
		t.Fatalf("değiştirilen SMS izni korunmalı: granted=%v err=%v", smsGranted, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM user_consents WHERE channel = 'email' AND user_id = (SELECT id FROM users WHERE email = 'senaryo-iki-kanal@example.test')`).Scan(&emailRows); err != nil || emailRows != 0 {
		t.Fatalf("silinen izin kaydı geri eklenmemeli: n=%d err=%v", emailRows, err)
	}
}

func TestSeedRecreatesMissingUserRelativeToNewReferenceTime(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE email = 'senaryo-iki-kanal@example.test'`); err != nil {
		t.Fatal(err)
	}

	later := now.Add(10 * day)
	res, err := seed.Run(t.Context(), db, later)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 1 {
		t.Fatalf("yalnızca silinen kullanıcı yeniden oluşmalı: %+v", res)
	}

	var cart time.Time
	var consents int
	if err := db.QueryRow(`SELECT last_cart_add_at FROM users WHERE email = 'senaryo-iki-kanal@example.test'`).Scan(&cart); err != nil {
		t.Fatal(err)
	}
	if want := later.Add(-5 * day); !cart.Equal(want) {
		t.Fatalf("yeni kullanıcının tarihi oluşturulma anındaki referans zamana göre olmalı: %v != %v", cart, want)
	}
	if err := db.QueryRow(`SELECT count(*) FROM user_consents WHERE granted AND user_id = (SELECT id FROM users WHERE email = 'senaryo-iki-kanal@example.test')`).Scan(&consents); err != nil || consents != 2 {
		t.Fatalf("yeni kullanıcının izinleri yazılmalı: n=%d err=%v", consents, err)
	}
}

// E2E veri kümesi (cmd/seed SEED_DATASET=e2e) gerçek PostgreSQL'de, kitle sayısı
// SQL'iyle de belgelenmiş kesin sayıları verir. Frontend e2e testleri bu sayılara
// güvenir (seed.E2EExpected).
func TestE2EDatasetGivesTheDocumentedCountsThroughSQL(t *testing.T) {
	db := testdb.Open(t)

	res, err := seed.RunUsers(t.Context(), db, seed.GenerateE2E(now))
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 10 || res.Skipped != 0 {
		t.Fatalf("10 kullanıcı yazılmalı: %+v", res)
	}

	total, sms, email := count(t, db, `{"all":[
		{"field":"city","op":"eq","value":"İstanbul"},
		{"field":"last_cart_add_at","op":"within_last_days","value":30},
		{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`)
	want := seed.E2EExpected
	if total != want.Total || sms != want.SMS || email != want.Email {
		t.Fatalf("toplam/sms/e-posta = %d/%d/%d, beklenen %d/%d/%d", total, sms, email, want.Total, want.SMS, want.Email)
	}

	// Tekrar çalıştırmak hiçbir şeyi değiştirmez.
	again, err := seed.RunUsers(t.Context(), db, seed.GenerateE2E(now.Add(time.Hour)))
	if err != nil || again.Inserted != 0 || again.Skipped != 10 {
		t.Fatalf("tekrar çalıştırma: %+v %v", again, err)
	}
	if t2, s2, e2 := count(t, db, `{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}`); t2 != 9 || s2 == 0 || e2 == 0 {
		t.Fatalf("beklenmeyen kullanıcı sayısı: %d/%d/%d", t2, s2, e2)
	}
}
