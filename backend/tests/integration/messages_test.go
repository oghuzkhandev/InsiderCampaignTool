package integration

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

func idemKey(campaignID string, userID int64, channel string) string {
	return campaignID + ":" + strconv.FormatInt(userID, 10) + ":" + channel
}

func insertMessage(db execer, campaignID string, userID int64, channel, recipient string) error {
	_, err := db.Exec(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5)`,
		campaignID, userID, channel, recipient, idemKey(campaignID, userID, channel),
	)
	return err
}

func TestMessagesDefaultsAndValidInserts(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	uid, err := insertUser(db, "a@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := insertMessage(db, id, uid, "sms", "+905550000001"); err != nil {
		t.Fatalf("geçerli mesaj kabul edilmeli: %v", err)
	}
	// Aynı kullanıcıya diğer kanaldan ayrı iş.
	if err := insertMessage(db, id, uid, "email", "a@example.test"); err != nil {
		t.Fatalf("ikinci kanal kabul edilmeli: %v", err)
	}

	var (
		status          string
		attempts        int
		dueNow          bool
		claim, claimed  sql.NullString
		sentAt, failure sql.NullString
	)
	err = db.QueryRow(
		`SELECT status, attempts, next_attempt_at <= now(), claim_token::text, claimed_at::text, sent_at::text, failure_reason
		 FROM messages WHERE campaign_id = $1 AND channel = 'sms'`, id,
	).Scan(&status, &attempts, &dueNow, &claim, &claimed, &sentAt, &failure)
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" || attempts != 0 || !dueNow || claim.Valid || claimed.Valid || sentAt.Valid || failure.Valid {
		t.Fatalf("varsayılanlar: status=%q attempts=%d due=%v claim=%v claimed=%v sent=%v failure=%v",
			status, attempts, dueNow, claim, claimed, sentAt, failure)
	}
}

func TestMessagesConstraints(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	uid, err := insertUser(db, "a@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	key := idemKey(id, uid, "sms")

	insert := func(channel, recipient, status string, attempts int, idem string) error {
		_, err := db.Exec(
			`INSERT INTO messages (campaign_id, user_id, channel, recipient, status, attempts, idempotency_key)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, uid, channel, recipient, status, attempts, idem)
		return err
	}

	tests := []struct {
		name       string
		err        error
		code       string
		constraint string
	}{
		{"bilinmeyen kanal", insert("push", "x", "queued", 0, idemKey(id, uid, "push")), pgCheckViolation, "messages_channel_valid"},
		{"bilinmeyen durum", insert("sms", "x", "retrying", 0, key), pgCheckViolation, "messages_status_valid"},
		{"negatif deneme", insert("sms", "x", "queued", -1, key), pgCheckViolation, "messages_attempts_nonneg"},
		{"boş alıcı", insert("sms", "", "queued", 0, key), pgCheckViolation, "messages_recipient_nonblank"},
		{"boşluk alıcı", insert("sms", " \t ", "queued", 0, key), pgCheckViolation, "messages_recipient_nonblank"},
		{"yanlış idempotency anahtarı", insert("sms", "x", "queued", 0, "başka"), pgCheckViolation, "messages_idempotency_key_valid"},
		{"başka kanalın anahtarı", insert("sms", "x", "queued", 0, idemKey(id, uid, "email")), pgCheckViolation, "messages_idempotency_key_valid"},
		{"claim'siz sending", insert("sms", "x", "sending", 1, key), pgCheckViolation, "messages_sending_has_claim"},
		{"sent_at'siz sent", insert("sms", "x", "sent", 1, key), pgCheckViolation, "messages_sent_at_matches"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantPgError(t, tt.err, tt.code, tt.constraint)
		})
	}

	// Zorunlu alanlar.
	_, err = db.Exec(`INSERT INTO messages (campaign_id, user_id, channel, status, idempotency_key) VALUES ($1, $2, 'sms', 'queued', $3)`, id, uid, key)
	wantPgError(t, err, pgNotNullViolation, "") // recipient
	_, err = db.Exec(`INSERT INTO messages (campaign_id, user_id, channel, recipient) VALUES ($1, $2, 'sms', 'x')`, id, uid)
	wantPgError(t, err, pgNotNullViolation, "") // idempotency_key

	// Yabancı anahtarlar.
	missingCampaign := "00000000-0000-0000-0000-000000000000"
	err = insertMessage(db, missingCampaign, uid, "sms", "x")
	wantPgError(t, err, pgFKViolation, "messages_campaign_id_fkey")
	err = insertMessage(db, id, 999999, "sms", "x")
	wantPgError(t, err, pgFKViolation, "messages_user_id_fkey")

	// Geçerli durum örnekleri.
	if _, err := db.Exec(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, status, attempts, claim_token, claimed_at, idempotency_key)
		 VALUES ($1, $2, 'sms', 'x', 'sending', 1, gen_random_uuid(), now(), $3)`, id, uid, key); err != nil {
		t.Fatalf("claim'li sending kabul edilmeli: %v", err)
	}
	if _, err := db.Exec(`UPDATE messages SET status = 'sent', sent_at = now() WHERE campaign_id = $1`, id); err != nil {
		t.Fatalf("sent_at'li sent kabul edilmeli: %v", err)
	}
	// Sonuç sonrası sent dışına dönüşte sent_at temizlenmeden kalamaz.
	_, err = db.Exec(`UPDATE messages SET status = 'failed', failure_reason = 'x' WHERE campaign_id = $1`, id)
	wantPgError(t, err, pgCheckViolation, "messages_sent_at_matches")
}

func TestMessagesAreUniquePerCampaignUserChannel(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	other := newCampaign(t, db)
	uid, _ := insertUser(db, "a@example.test", 0)

	if err := insertMessage(db, id, uid, "sms", "x"); err != nil {
		t.Fatal(err)
	}
	wantPgError(t, insertMessage(db, id, uid, "sms", "y"), pgUniqueViolation, "messages_unique_per_recipient")

	// Farklı kampanya aynı alıcıya ayrı iş oluşturabilir.
	if err := insertMessage(db, other, uid, "sms", "x"); err != nil {
		t.Fatalf("başka kampanya için kabul edilmeli: %v", err)
	}

	// Onay fan-out'u ON CONFLICT DO NOTHING kullanır: ikinci yazım satır eklemez.
	res, err := db.Exec(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, idempotency_key)
		 VALUES ($1, $2, 'sms', 'x', $3) ON CONFLICT DO NOTHING`, id, uid, idemKey(id, uid, "sms"))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("çakışmada 0 satır beklenir: %d", n)
	}
}

func TestMessagesFollowCampaignAndProtectUsers(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	uid, _ := insertUser(db, "a@example.test", 0)
	if err := insertMessage(db, id, uid, "email", "a@example.test"); err != nil {
		t.Fatal(err)
	}

	// Mesajı olan kullanıcı silinemez.
	_, err := db.Exec(`DELETE FROM users WHERE id = $1`, uid)
	wantPgError(t, err, pgFKViolation, "messages_user_id_fkey")

	// Kampanya silinince mesajlar da silinir.
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM messages`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("mesajlar kampanyayla silinmeli: n=%d err=%v", n, err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, uid); err != nil {
		t.Fatalf("mesajı kalmayan kullanıcı silinebilmeli: %v", err)
	}
}

// TestMessagesIndexes, ARCHITECTURE §4'teki üç indeksin ve kısmi koşullarının
// var olduğunu doğrular.
func TestMessagesIndexes(t *testing.T) {
	db := testdb.Open(t)

	want := map[string][]string{
		"messages_claim_idx":    {"(channel, next_attempt_at)", "WHERE (status = 'queued'"},
		"messages_reaper_idx":   {"(claimed_at)", "WHERE (status = 'sending'"},
		"messages_campaign_idx": {"(campaign_id, status)"},
	}
	for name, parts := range want {
		var def string
		if err := db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'messages' AND indexname = $1`, name).Scan(&def); err != nil {
			t.Fatalf("indeks %s bulunamadı: %v", name, err)
		}
		for _, p := range parts {
			if !strings.Contains(def, p) {
				t.Fatalf("indeks %s %q içermeli: %s", name, p, def)
			}
		}
	}
	// campaign_id, status indeksi kısmi değildir.
	var def string
	_ = db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE indexname = 'messages_campaign_idx'`).Scan(&def)
	if strings.Contains(def, "WHERE") {
		t.Fatalf("messages_campaign_idx kısmi olmamalı: %s", def)
	}
}
