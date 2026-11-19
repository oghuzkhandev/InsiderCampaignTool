package integration

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// Bu dosya migration 00004'ü (messages.first_name, ADR-026) sınar. Veri 00004'ten
// ÖNCEKİ şemada (sürüm 3) kurulur; yalnızca campaigns_test'te çalışır.

// at00003, şemayı sürüm 3'e indirir (00004 geri alınır) ve test bitince şemayı
// güncele getirir. Test içinde bırakılan mesajlar önce temizlenir: koruma, riskli
// mesaj varken Up'ı bilerek reddeder.
func at00003(t *testing.T) (*sql.DB, *goose.Provider) {
	t.Helper()
	db := testdb.Open(t)
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`TRUNCATE mock_provider_acceptances, messages, campaign_contents, campaigns, user_consents, users RESTART IDENTITY CASCADE`)
		_ = database.Migrate(context.Background(), db)
	})
	p := newGooseProvider(t, db)
	if _, err := p.DownTo(context.Background(), 3); err != nil {
		t.Fatalf("00004 geri alınamadı: %v", err)
	}
	return db, p
}

// insertV3Message, sürüm 3 şemasına (first_name sütunu yok) bir mesaj ekler.
func insertV3Message(t *testing.T, db *sql.DB, campaignID string, userID int64, channel, status string, attempts int) int64 {
	t.Helper()
	recipient := "kisi@example.test"
	if channel == "sms" {
		recipient = "+905550000001"
	}
	var id int64
	err := db.QueryRow(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, idempotency_key, status, attempts,
		                       claim_token, claimed_at, sent_at)
		 VALUES ($1, $2, $3, $4, $5, $6::text, $7,
		         CASE WHEN $6::text = 'sending' THEN gen_random_uuid() END,
		         CASE WHEN $6::text = 'sending' THEN now() END,
		         CASE WHEN $6::text = 'sent' THEN now() END)
		 RETURNING id`,
		campaignID, userID, channel, recipient, idemKey(campaignID, userID, channel), status, attempts,
	).Scan(&id)
	if err != nil {
		t.Fatalf("mesaj eklenemedi (%s, attempts=%d): %v", status, attempts, err)
	}
	return id
}

// namedUser, verilen adla (nil: ad yok) bir kullanıcı ekler.
func namedUser(t *testing.T, db *sql.DB, email string, name *string) int64 {
	t.Helper()
	uid, err := insertUser(db, email, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET first_name = $2 WHERE id = $1`, uid, name); err != nil {
		t.Fatal(err)
	}
	return uid
}

func strp(s string) *string { return &s }

// messageSnapshot, worker ve migration'dan etkilenebilecek mesaj alanlarının sıralı
// özetidir (first_name hariç: sürüm 3'te sütun yok).
func messageSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT id || '|' || status || '|' || attempts || '|' || coalesce(claim_token::text, '') || '|' ||
		        coalesce(failure_reason, '') || '|' || coalesce(last_error, '') || '|' || idempotency_key
		 FROM messages ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func hasFirstNameColumn(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns
	                       WHERE table_name = 'messages' AND column_name = 'first_name'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// Doldurma: mevcut mesajların first_name'i users.first_name'in ŞİMDİKİ değeridir
// (yoksa boş metin). Diğer alanlar ve mesaj sayısı değişmez. Sütun NOT NULL'dur ve
// varsayılanı yoktur.
func TestMigration00004BackfillsFromCurrentUserNamesAndKeepsOtherData(t *testing.T) {
	db, p := at00003(t)
	id := newCampaign(t, db)

	ada := namedUser(t, db, "a@example.test", strp("Ada"))
	empty := namedUser(t, db, "b@example.test", strp(""))
	blank := namedUser(t, db, "c@example.test", strp("   "))
	none := namedUser(t, db, "d@example.test", nil)
	ctrl := namedUser(t, db, "e@example.test", strp("Zeynep\tHanım"))

	want := map[int64]string{}
	for uid, name := range map[int64]string{ada: "Ada", empty: "", blank: "   ", none: "", ctrl: "Zeynep\tHanım"} {
		want[insertV3Message(t, db, id, uid, "email", "queued", 0)] = name
	}
	// Terminal mesajlar (denenmiş olsalar da) migration'ı engellemez ve doldurulur.
	sentUser := namedUser(t, db, "f@example.test", strp("Can"))
	want[insertV3Message(t, db, id, sentUser, "email", "sent", 2)] = "Can"
	failedUser := namedUser(t, db, "g@example.test", strp("Deniz"))
	want[insertV3Message(t, db, id, failedUser, "email", "failed", 5)] = "Deniz"
	skippedUser := namedUser(t, db, "h@example.test", nil)
	want[insertV3Message(t, db, id, skippedUser, "email", "skipped", 1)] = ""

	before := messageSnapshot(t, db)
	if hasFirstNameColumn(t, db) {
		t.Fatal("hazırlık: sürüm 3'te first_name sütunu olmamalı")
	}

	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("00004 uygulanmalı: %v", err)
	}
	if v, err := p.GetDBVersion(context.Background()); err != nil || v != 4 {
		t.Fatalf("sürüm 4 olmalı: %d %v", v, err)
	}
	if got := messageSnapshot(t, db); !slices.Equal(got, before) {
		t.Fatalf("doldurma diğer alanları veya mesaj sayısını değiştirmemeli:\n önce %v\n sonra %v", before, got)
	}

	rows, err := db.Query(`SELECT id, first_name FROM messages`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var mid int64
		var name string
		if err := rows.Scan(&mid, &name); err != nil {
			t.Fatal(err)
		}
		if name != want[mid] {
			t.Errorf("mesaj %d: first_name %q, beklenen %q", mid, name, want[mid])
		}
		n++
	}
	if n != len(want) {
		t.Fatalf("%d mesaj okundu, %d bekleniyordu", n, len(want))
	}

	// NOT NULL ve varsayılansız: ad yazmayan ekleme gürültülü şekilde başarısız olur.
	var nullable string
	var def sql.NullString
	if err := db.QueryRow(`SELECT is_nullable, column_default FROM information_schema.columns
	                       WHERE table_name = 'messages' AND column_name = 'first_name'`).Scan(&nullable, &def); err != nil {
		t.Fatal(err)
	}
	if nullable != "NO" || def.Valid {
		t.Fatalf("first_name NOT NULL ve varsayılansız olmalı: nullable=%s default=%v", nullable, def)
	}
	uid := namedUser(t, db, "yeni@example.test", strp("Yeni"))
	_, err = db.Exec(`INSERT INTO messages (campaign_id, user_id, channel, recipient, idempotency_key)
	                  VALUES ($1, $2, 'email', 'yeni@example.test', $3)`, id, uid, idemKey(id, uid, "email"))
	wantPgError(t, err, pgNotNullViolation, "")
}

// riskyCases, koruma tarafından reddedilmesi gereken durumlardır: queued veya
// sending ve daha önce denenmiş (attempts > 0).
var riskyCases = []struct {
	name     string
	status   string
	attempts int
}{
	{"retry bekleyen (queued, 1 deneme)", "queued", 1},
	{"retry bekleyen (queued, 4 deneme)", "queued", 4},
	{"gönderilmekte (sending)", "sending", 1},
	{"gönderilmekte, çok denenmiş (sending)", "sending", 5},
}

// Riskli (queued veya sending ve attempts > 0) mesaj varsa migration HİÇBİR VERİ
// DEĞİŞTİRMEDEN durur: şema, sürüm ve mesajlar olduğu gibi kalır; otomatik failed
// yapılmaz. Güvenli mesajlar arasında tek bir riskli mesaj bile yeterlidir.
func TestMigration00004RefusesWithoutChangingAnythingWhenAttemptedMessagesArePending(t *testing.T) {
	for _, tc := range riskyCases {
		t.Run(tc.name, func(t *testing.T) {
			db, p := at00003(t)
			id := newCampaign(t, db)

			// Güvenli mesajlar (queued/0, terminal) ve tek bir riskli mesaj.
			for i := 0; i < 3; i++ {
				insertV3Message(t, db, id, namedUser(t, db, fmt.Sprintf("guvenli%d@example.test", i), strp("Ada")), "email", "queued", 0)
			}
			insertV3Message(t, db, id, namedUser(t, db, "gonderildi@example.test", strp("Can")), "email", "sent", 3)
			risky := insertV3Message(t, db, id, namedUser(t, db, "riskli@example.test", strp("Ece")), "email", tc.status, tc.attempts)

			schemaBefore := schemaFingerprint(t, db)
			snapshotBefore := messageSnapshot(t, db)
			countsBefore := tableCounts(t, db)

			_, err := p.Up(context.Background())
			if err == nil {
				t.Fatal("riskli mesaj varken migration reddedilmeli")
			}
			if !strings.Contains(err.Error(), "Migration 00004 reddedildi") || !strings.Contains(err.Error(), "1 mesaj") ||
				!strings.Contains(err.Error(), "Hiçbir veri değiştirilmedi") {
				t.Fatalf("hata nedeni ve sayısı açık olmalı: %v", err)
			}

			if got := schemaFingerprint(t, db); !slices.Equal(got, schemaBefore) {
				t.Fatalf("reddedilen migration şemayı değiştirmemeli: %d → %d satır", len(schemaBefore), len(got))
			}
			if got := messageSnapshot(t, db); !slices.Equal(got, snapshotBefore) {
				t.Fatalf("reddedilen migration hiçbir mesajı değiştirmemeli (otomatik failed yok):\n önce %v\n sonra %v", snapshotBefore, got)
			}
			if got := tableCounts(t, db); !maps.Equal(got, countsBefore) {
				t.Fatalf("veri silinmemeli/eklenmemeli: %v → %v", countsBefore, got)
			}
			if v, err := p.GetDBVersion(context.Background()); err != nil || v != 3 {
				t.Fatalf("sürüm 3 kalmalı: %d %v", v, err)
			}
			if hasFirstNameColumn(t, db) {
				t.Fatal("first_name sütunu eklenmemeli")
			}
			if row := getMessage(t, db, risky); row.Status != tc.status || row.Attempts != tc.attempts || row.FailureReason.Valid {
				t.Fatalf("riskli mesaj olduğu gibi kalmalı: %+v", row)
			}

			// Operatör açıkça karar verip mesajı kapattıktan sonra migration ilerler.
			// (Bu SQL'i migration değil operatör çalıştırır.)
			if _, err := db.Exec(`UPDATE messages SET status = 'failed', failure_reason = 'exhausted',
			                      last_error = 'operatör kararı' WHERE id = $1`, risky); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Up(context.Background()); err != nil {
				t.Fatalf("operatör mesajı kapattıktan sonra migration uygulanmalı: %v", err)
			}
		})
	}
}

// Birden çok riskli mesajın sayısı hata metninde görünür.
func TestMigration00004ReportsHowManyRiskyMessagesThereAre(t *testing.T) {
	db, p := at00003(t)
	id := newCampaign(t, db)
	for i := 0; i < 3; i++ {
		insertV3Message(t, db, id, namedUser(t, db, fmt.Sprintf("r%d@example.test", i), nil), "email", "queued", 2)
	}
	insertV3Message(t, db, id, namedUser(t, db, "s@example.test", nil), "email", "sending", 1)

	_, err := p.Up(context.Background())
	if err == nil || !strings.Contains(err.Error(), "4 mesaj") {
		t.Fatalf("4 riskli mesaj raporlanmalı: %v", err)
	}
}

// Güvenli durumlarda koruma gereğinden fazla engellemez: queued/attempts=0 ve her
// terminal durum (denenmiş olsalar da) migration'ı durdurmaz.
func TestMigration00004IsNotBlockedBySafeMessages(t *testing.T) {
	db, p := at00003(t)
	id := newCampaign(t, db)
	for i := 0; i < 4; i++ {
		insertV3Message(t, db, id, namedUser(t, db, fmt.Sprintf("q%d@example.test", i), strp("Ada")), "email", "queued", 0)
	}
	insertV3Message(t, db, id, namedUser(t, db, "s@example.test", nil), "email", "sent", 2)
	insertV3Message(t, db, id, namedUser(t, db, "f@example.test", nil), "email", "failed", 5)
	insertV3Message(t, db, id, namedUser(t, db, "k@example.test", nil), "email", "skipped", 1)

	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("yalnızca güvenli mesajlar varken migration uygulanmalı: %v", err)
	}
}

// Mesajsız (boş) veritabanında da uygulanır.
func TestMigration00004AppliesToDatabaseWithoutMessages(t *testing.T) {
	_, p := at00003(t)
	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("mesajsız veritabanında migration uygulanmalı: %v", err)
	}
}

// Koruma kontrolden önce messages tablosunu kilitler: uçuştaki (commit edilmemiş)
// bir claim varsa migration onu bekler, commit sonrası görür ve reddeder. (Kilitsiz
// kontrol commit edilmemiş sending'i görmez ve migration riskli mesajın üstünden
// geçerdi.)
func TestMigration00004WaitsForInFlightClaimAndThenRefuses(t *testing.T) {
	db, p := at00003(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()
	id := newCampaign(t, db)
	mid := insertV3Message(t, db, id, namedUser(t, db, "claim@example.test", strp("Ada")), "email", "queued", 0)

	// Uçuştaki claim: mesajı sending yapar ama commit etmez.
	claimer := begin(t, ctx, db)
	if _, err := claimer.ExecContext(ctx,
		`UPDATE messages SET status = 'sending', claim_token = gen_random_uuid(), claimed_at = now(), attempts = attempts + 1
		 WHERE id = $1`, mid); err != nil {
		t.Fatal(err)
	}

	up := make(chan error, 1)
	go func() {
		_, err := p.Up(ctx)
		up <- err
	}()
	waitForLockWaiters(t, ctx, db, "LOCK TABLE", 1)
	notDone(t, up, "migration")

	if err := claimer.Commit(); err != nil {
		t.Fatal(err)
	}
	err := receive(t, up, "migration")
	if err == nil || !strings.Contains(err.Error(), "Migration 00004 reddedildi") {
		t.Fatalf("commit sonrası migration riskli mesajı görüp reddetmeli: %v", err)
	}
	if hasFirstNameColumn(t, db) {
		t.Fatal("first_name sütunu eklenmemeli")
	}
	if row := getMessage(t, db, mid); row.Status != "sending" || row.Attempts != 1 {
		t.Fatalf("mesaj claim'li kalmalı: %+v", row)
	}
}

// 00004 geri alınınca yalnızca first_name sütunu kalkar, veri korunur ve yeniden
// uygulanınca şema birebir aynı olur.
func TestMigration00004DownAndUpAgain(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = db.Exec(`TRUNCATE mock_provider_acceptances, messages, campaign_contents, campaigns, user_consents, users RESTART IDENTITY CASCADE`)
		_ = database.Migrate(ctx, db)
	})
	id := newCampaign(t, db)
	uid := namedUser(t, db, "gidis@example.test", strp("Ada"))
	if _, err := db.Exec(`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key)
	                      VALUES ($1, $2, 'email', 'gidis@example.test', 'Ada', $3)`, id, uid, idemKey(id, uid, "email")); err != nil {
		t.Fatal(err)
	}
	before := schemaFingerprint(t, db)

	p := newGooseProvider(t, db)
	if _, err := p.DownTo(ctx, 3); err != nil {
		t.Fatalf("Down başarısız: %v", err)
	}
	after := schemaFingerprint(t, db)
	removed := 0
	for _, line := range before {
		if !slices.Contains(after, line) {
			removed++
			if !strings.Contains(line, "first_name") || !strings.Contains(line, "messages") {
				t.Fatalf("Down yalnızca messages.first_name'i kaldırmalı: %s", line)
			}
		}
	}
	for _, line := range after {
		if !slices.Contains(before, line) {
			t.Fatalf("Down sonrası şemada Up öncesinde olmayan satır var: %s", line)
		}
	}
	if removed == 0 {
		t.Fatal("Down hiçbir nesneyi kaldırmadı")
	}
	if n := tableCounts(t, db)["messages"]; n != 1 {
		t.Fatalf("Down mesajları silmemeli: %d", n)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up (tekrar) başarısız: %v", err)
	}
	if got := schemaFingerprint(t, db); !slices.Equal(got, before) {
		t.Fatalf("Down+Up sonrası şema farklı: önce %d, sonra %d satır", len(before), len(got))
	}
	var name string
	if err := db.QueryRow(`SELECT first_name FROM messages`).Scan(&name); err != nil || name != "Ada" {
		t.Fatalf("Up sonrası mesaj users'tan doldurulmalı: %q %v", name, err)
	}
}
