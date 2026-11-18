package integration

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/migrations"
)

// schemaFingerprint, public şemasının sütunlarını, kısıtlarını, indekslerini,
// tetikleyicilerini ve fonksiyon adlarını sıralı satırlar olarak döndürür.
// Goose'un kendi tablosu hariçtir.
func schemaFingerprint(t *testing.T, db *sql.DB) []string {
	t.Helper()
	queries := []string{
		`SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type || ' null=' || is_nullable
		        || ' default=' || coalesce(column_default, '')
		 FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name NOT LIKE 'goose%'`,
		`SELECT 'constraint ' || conrelid::regclass::text || ' ' || conname || ' ' || pg_get_constraintdef(oid)
		 FROM pg_constraint WHERE connamespace = 'public'::regnamespace AND conrelid::regclass::text NOT LIKE 'goose%'`,
		`SELECT 'index ' || indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename NOT LIKE 'goose%'`,
		`SELECT 'trigger ' || tgrelid::regclass::text || ' ' || tgname FROM pg_trigger WHERE NOT tgisinternal`,
		`SELECT 'function ' || proname FROM pg_proc WHERE pronamespace = 'public'::regnamespace`,
	}
	var lines []string
	for _, q := range queries {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatalf("şema okunamadı: %v\n%s", err, q)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	slices.Sort(lines)
	return lines
}

func newGooseProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// 00002 geri alınınca yalnızca kendi nesneleri kalkar, 00001'in verisi korunur
// ve yeniden uygulanınca şema birebir aynı olur. Yalnızca campaigns_test'te
// çalışır; bu pakette başka test eşzamanlı çalışmaz ve test bitince şema
// her durumda yeniden ileri alınır.
func TestMigration00002DownAndUpAgain(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	// Test başarısız olsa bile şemayı güncel hale getir.
	t.Cleanup(func() { _ = database.Migrate(ctx, db) })

	before := schemaFingerprint(t, db)

	// Down öncesi 00001 biçiminde veri: kullanıcı, draft kampanya.
	uid, err := insertUser(db, "down@example.test", 3)
	if err != nil {
		t.Fatal(err)
	}
	id := newCampaign(t, db)
	// 00002 nesneleri doluyken de geri alma çalışmalı.
	mustAddContent(t, db, id, 1)
	if err := insertMessage(db, id, uid, "email", "down@example.test"); err != nil {
		t.Fatal(err)
	}

	p := newGooseProvider(t, db)
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("Down başarısız: %v", err)
	}

	// Yeni nesneler gitti.
	after := schemaFingerprint(t, db)
	removed := 0
	for _, line := range before {
		if !slices.Contains(after, line) {
			removed++
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
	for _, gone := range []string{
		"campaign_contents", "messages", "approved_content_version", "approved_at", "completed_at",
		"content_text_valid", "campaign_contents_block_update", "campaign_contents_require_draft",
		"campaigns_keep_approved_version", "campaigns_approved_content_fk",
	} {
		for _, line := range after {
			if strings.Contains(line, gone) {
				t.Fatalf("Down sonrası %q kalmamalı: %s", gone, line)
			}
		}
	}

	// 00001'in verisi ve kuralları korunur.
	var email, status string
	if err := db.QueryRow(`SELECT u.email, c.status FROM users u, campaigns c WHERE u.id = $1 AND c.id = $2`, uid, id).Scan(&email, &status); err != nil {
		t.Fatalf("00001 verisi korunmalı: %v", err)
	}
	if email != "down@example.test" || status != "draft" {
		t.Fatalf("veri değişmiş: %q %q", email, status)
	}
	wantPgError(t, insertCampaign(db, "brief", validSegment, "failed", 0, 0, 0), pgCheckViolation, "campaigns_status_valid")

	// Yeniden ileri alma: şema birebir aynı ve 00001'den kalan kampanya sağlam.
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up (tekrar) başarısız: %v", err)
	}
	if got := schemaFingerprint(t, db); !slices.Equal(got, before) {
		t.Fatalf("Down+Up sonrası şema farklı:\n önce %d satır, sonra %d satır", len(before), len(got))
	}
	var v sql.NullInt32
	if err := db.QueryRow(`SELECT approved_content_version FROM campaigns WHERE id = $1`, id).Scan(&v); err != nil || v.Valid {
		t.Fatalf("var olan draft kampanya yeni sütunlarla boş kalmalı: %v %v", v, err)
	}
	// Yeniden uygulanan şema çalışır: yeni sürüm ve mesaj yazılabilir.
	mustAddContent(t, db, id, 1)
	if err := insertMessage(db, id, uid, "sms", "+905550000009"); err != nil {
		t.Fatalf("Up sonrası mesaj yazılabilmeli: %v", err)
	}
}

// 00002, 00001 sonrası dolu bir veritabanına uygulanabilir: var olan draft
// kampanyalar ve kullanıcılar bozulmaz, yeni onay sütunları boş kalır.
func TestMigration00002AppliesToPopulatedDatabase(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = database.Migrate(ctx, db) })

	p := newGooseProvider(t, db)
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("Down başarısız: %v", err)
	}

	// 00001 şemasında veri yaz (onay sütunları yok).
	uid, err := insertUser(db, "dolu@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < 3; i++ {
		var id string
		if err := db.QueryRow(`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email)
			VALUES ('brief', $1::jsonb, 4, 3, 2) RETURNING id`, validSegment).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("dolu veritabanına Up başarısız: %v", err)
	}

	for _, id := range ids {
		var status string
		var total, sms, email int
		var v sql.NullInt32
		var a, c sql.NullTime
		err := db.QueryRow(`SELECT status, audience_total, audience_sms, audience_email,
			approved_content_version, approved_at, completed_at FROM campaigns WHERE id = $1`, id).
			Scan(&status, &total, &sms, &email, &v, &a, &c)
		if err != nil {
			t.Fatal(err)
		}
		if status != "draft" || total != 4 || sms != 3 || email != 2 || v.Valid || a.Valid || c.Valid {
			t.Fatalf("var olan kampanya bozuldu: %q %d %d %d %v %v %v", status, total, sms, email, v, a, c)
		}
	}
	// Var olan kampanyalar yeni özellikleri kullanabilir.
	mustAddContent(t, db, ids[0], 1)
	mustApprove(t, db, ids[0], 1)
	if err := insertMessage(db, ids[0], uid, "email", "dolu@example.test"); err != nil {
		t.Fatal(err)
	}
}

// tableCounts, 00002'nin sahip olduğu tabloların satır sayılarını döndürür.
func tableCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"campaigns", "campaign_contents", "messages", "users"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("%s sayılamadı: %v", table, err)
		}
		out[table] = n
	}
	return out
}

// Draft olmayan kampanya varken Down, hiçbir veri silmeden reddedilmelidir.
func TestMigration00002DownIsRefusedWhileNonDraftCampaignsExist(t *testing.T) {
	for _, state := range []string{"approved", "sending", "completed"} {
		t.Run(state, func(t *testing.T) {
			db := testdb.Open(t)
			ctx := context.Background()
			t.Cleanup(func() { _ = database.Migrate(ctx, db) })

			uid, err := insertUser(db, "korumali@example.test", 0)
			if err != nil {
				t.Fatal(err)
			}
			draftID := newCampaign(t, db)
			mustAddContent(t, db, draftID, 1)
			lockedID := newCampaign(t, db)
			mustAddContent(t, db, lockedID, 1)
			mustApprove(t, db, lockedID, 1)
			switch state {
			case "sending":
				if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, lockedID); err != nil {
					t.Fatal(err)
				}
			case "completed":
				if _, err := db.Exec(`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`, lockedID); err != nil {
					t.Fatal(err)
				}
			}
			if err := insertMessage(db, lockedID, uid, "email", "korumali@example.test"); err != nil {
				t.Fatal(err)
			}

			p := newGooseProvider(t, db)
			schemaBefore := schemaFingerprint(t, db)
			countsBefore := tableCounts(t, db)

			_, err = p.DownTo(ctx, 1)
			if err == nil {
				t.Fatal("draft olmayan kampanya varken Down reddedilmeli")
			}
			if !strings.Contains(err.Error(), "draft olmayan 1 kampanya") {
				t.Fatalf("hata nedeni açık olmalı: %v", err)
			}

			// Hiçbir şey silinmedi ve şema ile sürüm olduğu gibi duruyor.
			if got := schemaFingerprint(t, db); !slices.Equal(got, schemaBefore) {
				t.Fatalf("reddedilen Down şemayı değiştirmemeli: %d satır → %d satır", len(schemaBefore), len(got))
			}
			if got := tableCounts(t, db); !maps.Equal(got, countsBefore) {
				t.Fatalf("reddedilen Down veri silmemeli: %v → %v", countsBefore, got)
			}
			if v, err := p.GetDBVersion(ctx); err != nil || v < 2 {
				t.Fatalf("sürüm 2 veya üstü kalmalı: %d %v", v, err)
			}
			var version sql.NullInt32
			var status string
			if err := db.QueryRow(`SELECT approved_content_version, status FROM campaigns WHERE id = $1`, lockedID).Scan(&version, &status); err != nil || !version.Valid || version.Int32 != 1 || status != state {
				t.Fatalf("onay verisi bozulmamalı: %v %q %v", version, status, err)
			}
			var sms string
			if err := db.QueryRow(`SELECT sms_text FROM campaign_contents WHERE campaign_id = $1 AND version = 1`, draftID).Scan(&sms); err != nil || sms != goodSMS {
				t.Fatalf("draft kampanyanın sürümü de silinmemeli: %q %v", sms, err)
			}

			// Draft olmayan kampanya bilinçli olarak silinince Down çalışır.
			if _, err := db.Exec(`DELETE FROM campaigns WHERE id = $1`, lockedID); err != nil {
				t.Fatalf("kampanya silinemedi: %v", err)
			}
			if _, err := p.DownTo(ctx, 1); err != nil {
				t.Fatalf("yalnızca draft kalınca Down çalışmalı: %v", err)
			}
		})
	}
}

// Yalnızca draft kampanyalar varken (boş veritabanı dahil) Down reddedilmez;
// bu, korumanın gereğinden fazla engellemediğini gösterir.
func TestMigration00002DownIsAllowedWithOnlyDraftCampaigns(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = database.Migrate(ctx, db) })

	newCampaign(t, db)
	newCampaign(t, db)

	p := newGooseProvider(t, db)
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("yalnızca draft kampanya varken Down çalışmalı: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM campaigns`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("draft kampanyalar korunmalı: %d %v", n, err)
	}
}

// Down koruması kontrolden önce tabloları kilitler: kontrol ile DROP arasında
// onay commit edilemez. Uçuştaki bir onay varsa Down onu bekler, sonra onaylı
// kampanyayı görüp hiçbir veri silmeden reddeder. (Kilitsiz korumada kontrol,
// henüz commit edilmemiş onayı görmez; DROP onay commit olunca çalışır ve
// onay verisini siler.)
func TestMigration00002DownWaitsForInFlightApprovalAndThenRefuses(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()
	t.Cleanup(func() { _ = database.Migrate(context.Background(), db) })

	uid, err := insertUser(db, "yaris@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	if err := insertMessage(db, id, uid, "email", "yaris@example.test"); err != nil {
		t.Fatal(err)
	}
	p := newGooseProvider(t, db)
	schemaBefore := schemaFingerprint(t, db)

	// Uçuştaki onay: kampanyayı kilitler ve onaylar ama commit etmez.
	approver := begin(t, ctx, db)
	var status string
	if err := approver.QueryRowContext(ctx, lockQuery, id).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("onay kilidi: %q %v", status, err)
	}
	if _, err := approver.ExecContext(ctx, approveSQL, id, 1); err != nil {
		t.Fatal(err)
	}

	down := make(chan error, 1)
	go func() {
		_, err := p.DownTo(ctx, 1)
		down <- err
	}()
	// Down, kilidi beklemeli: koruma tabloyu kilitlemeden kontrol etmemeli.
	waitForLockWaiters(t, ctx, db, "LOCK TABLE", 1)
	notDone(t, down, "Down")

	if err := approver.Commit(); err != nil {
		t.Fatal(err)
	}
	err = receive(t, down, "Down")
	if err == nil || !strings.Contains(err.Error(), "draft olmayan 1 kampanya") {
		t.Fatalf("onay commit olunca Down onaylı kampanyayı görüp reddetmeli: %v", err)
	}

	// Hiçbir şey silinmedi.
	if got := schemaFingerprint(t, db); !slices.Equal(got, schemaBefore) {
		t.Fatal("reddedilen Down şemayı değiştirmemeli")
	}
	var approved sql.NullInt32
	var st string
	if err := db.QueryRow(`SELECT approved_content_version, status FROM campaigns WHERE id = $1`, id).Scan(&approved, &st); err != nil || !approved.Valid || approved.Int32 != 1 || st != "approved" {
		t.Fatalf("onay verisi korunmalı: %v %q %v", approved, st, err)
	}
	if n := tableCounts(t, db)["messages"]; n != 1 {
		t.Fatalf("mesajlar korunmalı: %d", n)
	}
}
