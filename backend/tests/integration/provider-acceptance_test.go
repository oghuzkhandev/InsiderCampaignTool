package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

func hashOf(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func insertAcceptance(db execer, key, channel, recipient string, hash []byte) error {
	_, err := db.Exec(
		`INSERT INTO mock_provider_acceptances (idempotency_key, channel, recipient, content_hash)
		 VALUES ($1, $2, $3, $4)`, key, channel, recipient, hash)
	return err
}

func TestProviderAcceptanceDefaultsAndValidInserts(t *testing.T) {
	db := testdb.Open(t)

	if err := insertAcceptance(db, "k:1:sms", "sms", "+905550000001", hashOf("merhaba")); err != nil {
		t.Fatalf("geçerli kabul kaydı kabul edilmeli: %v", err)
	}
	if err := insertAcceptance(db, "k:1:email", "email", "a@example.test", hashOf("konu\ngövde")); err != nil {
		t.Fatalf("e-posta kabul kaydı kabul edilmeli: %v", err)
	}

	var messageID sql.NullString
	var recent bool
	err := db.QueryRow(
		`SELECT message_id::text, accepted_at <= now() FROM mock_provider_acceptances WHERE idempotency_key = 'k:1:sms'`,
	).Scan(&messageID, &recent)
	if err != nil {
		t.Fatal(err)
	}
	if !messageID.Valid || messageID.String == "" || !recent {
		t.Fatalf("message_id üretilmeli ve accepted_at dolu olmalı: %v %v", messageID, recent)
	}
}

func TestProviderAcceptanceConstraints(t *testing.T) {
	db := testdb.Open(t)
	good := hashOf("içerik")

	if err := insertAcceptance(db, "k:1:sms", "sms", "+905550000001", good); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		err        error
		code       string
		constraint string
	}{
		{"aynı anahtar", insertAcceptance(db, "k:1:sms", "sms", "+905550000001", good), pgUniqueViolation, "mock_provider_acceptances_pkey"},
		{"aynı anahtar, başka alıcı", insertAcceptance(db, "k:1:sms", "sms", "+905550000002", good), pgUniqueViolation, "mock_provider_acceptances_pkey"},
		{"boş anahtar", insertAcceptance(db, "", "sms", "x", good), pgCheckViolation, "mpa_idempotency_key_nonblank"},
		{"boşluk anahtar", insertAcceptance(db, " \t", "sms", "x", good), pgCheckViolation, "mpa_idempotency_key_nonblank"},
		{"bilinmeyen kanal", insertAcceptance(db, "k:2:push", "push", "x", good), pgCheckViolation, "mpa_channel_valid"},
		{"boş alıcı", insertAcceptance(db, "k:3:sms", "sms", "", good), pgCheckViolation, "mpa_recipient_nonblank"},
		{"boşluk alıcı", insertAcceptance(db, "k:3:sms", "sms", " \n ", good), pgCheckViolation, "mpa_recipient_nonblank"},
		{"kısa özet", insertAcceptance(db, "k:4:sms", "sms", "x", good[:31]), pgCheckViolation, "mpa_content_hash_sha256"},
		{"uzun özet", insertAcceptance(db, "k:4:sms", "sms", "x", append(slices.Clone(good), 0)), pgCheckViolation, "mpa_content_hash_sha256"},
		{"boş özet", insertAcceptance(db, "k:4:sms", "sms", "x", []byte{}), pgCheckViolation, "mpa_content_hash_sha256"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantPgError(t, tc.err, tc.code, tc.constraint)
		})
	}

	// Özet zorunludur.
	_, err := db.Exec(`INSERT INTO mock_provider_acceptances (idempotency_key, channel, recipient) VALUES ('k:5:sms', 'sms', 'x')`)
	wantPgError(t, err, pgNotNullViolation, "")

	// message_id benzersizdir.
	var id string
	if err := db.QueryRow(`SELECT message_id::text FROM mock_provider_acceptances WHERE idempotency_key = 'k:1:sms'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		`INSERT INTO mock_provider_acceptances (idempotency_key, channel, recipient, content_hash, message_id)
		 VALUES ('k:6:sms', 'sms', 'x', $1, $2::uuid)`, good, id)
	wantPgError(t, err, pgUniqueViolation, "mpa_message_id_unique")

	// Reddedilen satırların hiçbiri kaydedilmedi.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM mock_provider_acceptances`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("yalnızca ilk geçerli kayıt kalmalı: n=%d err=%v", n, err)
	}
}

// Provider harici bir sistemi temsil eder (ADR-025): kabul kaydı uygulama
// tablolarına yabancı anahtarla bağlanmamalıdır.
func TestProviderAcceptanceHasNoForeignKeys(t *testing.T) {
	db := testdb.Open(t)

	var fks int
	err := db.QueryRow(
		`SELECT count(*) FROM pg_constraint WHERE conrelid = 'mock_provider_acceptances'::regclass AND contype = 'f'`,
	).Scan(&fks)
	if err != nil {
		t.Fatal(err)
	}
	if fks != 0 {
		t.Fatalf("mock_provider_acceptances yabancı anahtar içermemeli, %d var", fks)
	}
	var referencing int
	err = db.QueryRow(
		`SELECT count(*) FROM pg_constraint WHERE confrelid = 'mock_provider_acceptances'::regclass`,
	).Scan(&referencing)
	if err != nil {
		t.Fatal(err)
	}
	if referencing != 0 {
		t.Fatalf("hiçbir tablo mock_provider_acceptances'a referans vermemeli, %d var", referencing)
	}

	// Bağımsızlığın pratik sonucu: karşılığı olmayan bir anahtar kaydedilebilir.
	if err := insertAcceptance(db, "kampanyasiz:anahtar:sms", "sms", "+905550000001", hashOf("x")); err != nil {
		t.Fatalf("messages'ta karşılığı olmayan anahtar kabul edilmeli: %v", err)
	}
}

// INSERT ... ON CONFLICT DO NOTHING eşzamanlı çağrılarda da atomiktir: aynı
// anahtar için tam bir çağrı satırı yazar, diğerleri satır döndürmez ve tek
// kayıt kalır. Mock provider'ın tekilleştirmesi bu davranışa dayanır.
func TestProviderAcceptanceConcurrentInsertsRecordOnce(t *testing.T) {
	db := testdb.Open(t)
	const workers = 20

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		inserted int
		failures []error
	)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var id string
			err := db.QueryRow(
				`INSERT INTO mock_provider_acceptances (idempotency_key, channel, recipient, content_hash)
				 VALUES ('yaris:1:sms', 'sms', '+905550000001', $1)
				 ON CONFLICT (idempotency_key) DO NOTHING
				 RETURNING message_id::text`, hashOf("içerik")).Scan(&id)
			mu.Lock()
			defer mu.Unlock()
			switch err {
			case nil:
				inserted++
			case sql.ErrNoRows: // çakışma: satır yazılmadı
			default:
				failures = append(failures, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(failures) > 0 {
		t.Fatalf("beklenmeyen hata: %v", failures[0])
	}
	if inserted != 1 {
		t.Fatalf("tam bir çağrı yazmalı, yazan: %d", inserted)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM mock_provider_acceptances`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("tek kabul kaydı kalmalı: n=%d err=%v", n, err)
	}
}

// 00003 geri alınınca yalnızca kendi tablosu kalkar; 00002'nin tabloları ve
// verisi korunur. Yeniden uygulanınca şema birebir aynı olur.
func TestMigration00003DownAndUpAgain(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Migrate(ctx, db) })

	uid, err := insertUser(db, "m3@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	if err := insertMessage(db, id, uid, "email", "m3@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := insertAcceptance(db, idemKey(id, uid, "email"), "email", "m3@example.test", hashOf("x")); err != nil {
		t.Fatal(err)
	}
	// 00003'ü yalnızca kendi nesneleri üzerinden sınamak için önce 00004 geri alınır
	// (aksi hâlde DownTo(2) messages.first_name'i de kaldırırdı).
	p := newGooseProvider(t, db)
	if _, err := p.DownTo(ctx, 3); err != nil {
		t.Fatalf("00004 geri alınamadı: %v", err)
	}
	before := schemaFingerprint(t, db)

	if _, err := p.DownTo(ctx, 2); err != nil {
		t.Fatalf("Down başarısız: %v", err)
	}

	after := schemaFingerprint(t, db)
	for _, line := range after {
		if !slices.Contains(before, line) {
			t.Fatalf("Down sonrası şemada Up öncesinde olmayan satır var: %s", line)
		}
	}
	removed := 0
	for _, line := range before {
		if !slices.Contains(after, line) {
			removed++
			if !strings.Contains(line, "mock_provider_acceptances") && !strings.Contains(line, "mpa_") {
				t.Fatalf("Down 00003'ün dışındaki bir nesneyi kaldırdı: %s", line)
			}
		}
	}
	if removed == 0 {
		t.Fatal("Down hiçbir nesneyi kaldırmadı")
	}
	// 00002'nin verisi korunur.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM messages`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("mesajlar korunmalı: n=%d err=%v", n, err)
	}

	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("Up (tekrar) başarısız: %v", err)
	}
	if got := schemaFingerprint(t, db); !slices.Equal(got, before) {
		t.Fatalf("Down+Up sonrası şema farklı: önce %d satır, sonra %d satır", len(before), len(got))
	}
	if err := insertAcceptance(db, "yeniden:1:sms", "sms", "+905550000001", hashOf("x")); err != nil {
		t.Fatalf("Up sonrası kabul kaydı yazılabilmeli: %v", err)
	}
}
