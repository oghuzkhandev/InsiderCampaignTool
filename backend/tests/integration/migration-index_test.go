package integration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// Bu dosya migration 00005'i (campaigns_recent_idx, CMP-001) sınar; yalnızca campaigns_test'te
// çalışır.

const recentIndexName = "campaigns_recent_idx"

func indexDef(t *testing.T, db *sql.DB) (def string, exists bool) {
	t.Helper()
	err := db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE tablename = 'campaigns' AND indexname = $1`,
		recentIndexName).Scan(&def)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return def, true
}

func TestMigration00005CreatesTheRecentCampaignsIndexInListOrder(t *testing.T) {
	db := testdb.Open(t)

	def, ok := indexDef(t, db)

	if !ok {
		t.Fatal("campaigns_recent_idx oluşmalı")
	}
	// Sıralama sorgusuyla aynı: (created_at DESC, id DESC).
	if !strings.Contains(def, "(created_at DESC, id DESC)") {
		t.Fatalf("indeks sıralamayla aynı olmalı: %s", def)
	}
}

// Liste sorgusu (campaign.Store.List ile birebir aynı metin) indeksle karşılanabilir: indeks
// sorgunun sıralamasıyla aynı olduğundan ayrıca Sort adımı gerekmez. Bu test planlayıcının
// demo ölçeğinde her zaman indeksi SEÇTİĞİNİ kanıtlamaz (maliyete göre karar verir, çok küçük
// tabloda tarama + sıralama seçebilir); yalnızca indeksin bu sorgu için kullanılabilir ve sırasının
// doğru olduğunu gösterir.
func TestRecentCampaignsQueryCanBeServedByTheIndexWithoutASortStep(t *testing.T) {
	db := testdb.Open(t)
	base := now.Truncate(time.Second)
	for i := 0; i < 30; i++ {
		insertDraft(t, db, "Brief", base.Add(time.Duration(i)*time.Minute))
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	// Az satırlı tabloda planlayıcı tarama seçebilir; indeksin sorgu için kullanılabilir olduğunu
	// görmek için yalnızca bu transaction'da sıralı taramayı kapat.
	if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(`EXPLAIN SELECT id, brief, status, created_at
		 FROM campaigns
		 ORDER BY created_at DESC, id DESC
		 LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(plan.String(), recentIndexName) {
		t.Fatalf("tarama kapalıyken liste sorgusu indeksle karşılanmalı:\n%s", plan.String())
	}
	if strings.Contains(plan.String(), "Sort") {
		t.Fatalf("indeks sırası sorgu sırasıyla aynıdır, ayrıca Sort adımı olmamalı:\n%s", plan.String())
	}
}

// 00005 geri alınınca yalnızca indeks kalkar, kampanya verisi korunur; yeniden uygulanınca
// indeks aynı tanımla geri gelir.
func TestMigration00005DownRemovesOnlyTheIndexAndUpRecreatesIt(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Migrate(ctx, db) }) // başarısız olsa da şema güncel kalsın

	want, _ := indexDef(t, db)
	id := insertDraft(t, db, "Korunacak brief", now.Truncate(time.Second))
	p := newGooseProvider(t, db)

	if _, err := p.DownTo(ctx, 4); err != nil {
		t.Fatalf("00005 geri alınamadı: %v", err)
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != 4 {
		t.Fatalf("sürüm 4 olmalı: %d %v", v, err)
	}
	if _, ok := indexDef(t, db); ok {
		t.Fatal("Down indeksi kaldırmalı")
	}
	var brief string
	if err := db.QueryRow(`SELECT brief FROM campaigns WHERE id = $1`, id).Scan(&brief); err != nil || brief != "Korunacak brief" {
		t.Fatalf("Down kampanya verisine dokunmamalı: %q %v", brief, err)
	}

	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("00005 yeniden uygulanamadı: %v", err)
	}
	got, ok := indexDef(t, db)
	if !ok || got != want {
		t.Fatalf("indeks aynı tanımla geri gelmeli:\n önce  %s\n sonra %s", want, got)
	}
}

// Indeks varken de liste doğru ve aynı sırayla döner (indeks yalnızca hızlandırır).
func TestCampaignListGivesTheSameResultWithAndWithoutTheIndex(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Migrate(ctx, db) })
	h := newAPI(db, nil)
	base := now.Truncate(time.Second)
	for i := 0; i < 12; i++ {
		insertDraft(t, db, "Brief "+string(rune('A'+i)), base.Add(time.Duration(i%4)*time.Minute)) // eşit zamanlar da var
	}

	_, withIndex := apiList(t, h, "?limit=20")
	if _, err := newGooseProvider(t, db).DownTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	_, withoutIndex := apiList(t, h, "?limit=20")

	if len(withIndex.Campaigns) != 12 || len(withoutIndex.Campaigns) != 12 {
		t.Fatalf("12 kampanya bekleniyordu: %d / %d", len(withIndex.Campaigns), len(withoutIndex.Campaigns))
	}
	for i := range withIndex.Campaigns {
		if withIndex.Campaigns[i].ID != withoutIndex.Campaigns[i].ID {
			t.Fatalf("%d. satır indeksle ve indekssiz farklı", i)
		}
	}
}
