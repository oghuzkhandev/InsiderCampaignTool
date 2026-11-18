package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// Bu dosyadaki testler iki veya daha fazla gerçek veritabanı bağlantısıyla
// kilit davranışını sınar: içerik yazımı ile onayın eşzamanlı olduğu durumda
// onaylanan sürümün sonradan değiştirilemediği ve onaydan sonra yeni sürüm
// yazılamadığı (CNT-001). Uygulama kilidi (kampanya satırı FOR UPDATE) ile
// veritabanı tetikleyicileri birlikte sınanır.

const lockTestTimeout = 20 * time.Second

func begin(t *testing.T, ctx context.Context, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("transaction açılamadı: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

// waitForLockWaiter, sorgusu queryPrefix ile başlayan bir bağlantının bir kilidi
// beklediğini görene kadar bekler. Beklemeyi zamanlamaya değil veritabanının
// kendi bildirdiği duruma bağlar.
func waitForLockWaiter(t *testing.T, ctx context.Context, db *sql.DB, queryPrefix string) {
	t.Helper()
	deadline := time.Now().Add(lockTestTimeout)
	for time.Now().Before(deadline) {
		var n int
		err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1`,
			queryPrefix+"%").Scan(&n)
		if err != nil {
			t.Fatalf("pg_stat_activity okunamadı: %v", err)
		}
		if n > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%q başlayan sorgu kilit beklemedi", queryPrefix)
}

func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(lockTestTimeout):
		t.Fatalf("%s zamanında bitmedi", what)
		panic("unreachable")
	}
}

func notDone[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s beklemeliydi ama bitti", what)
	default:
	}
}

const insertContentPrefix = `INSERT INTO campaign_contents`

// Onay kampanya satırını kilitleyip henüz commit etmemişken gelen içerik yazımı
// beklemeli, onay commit olunca onaylanmış durumu görüp reddedilmelidir.
func TestContentInsertWaitsForApprovalThenIsRejected(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()

	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)

	approver := begin(t, ctx, db)
	var status string
	if err := approver.QueryRowContext(ctx, `SELECT status FROM campaigns WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("kilit/durum: %q %v", status, err)
	}
	if _, err := approver.ExecContext(ctx, approveSQL, id, 1); err != nil {
		t.Fatal(err)
	}

	writer := begin(t, ctx, db)
	done := make(chan error, 1)
	go func() {
		_, err := writer.ExecContext(ctx, insertContentPrefix+` (campaign_id, version, source, sms_text, email_subject, email_body)
			VALUES ($1, 2, 'human', $2, $3, $4)`, id, goodSMS, goodSubject, goodBody)
		done <- err
	}()
	waitForLockWaiter(t, ctx, db, insertContentPrefix)
	notDone(t, done, "içerik yazımı")

	if err := approver.Commit(); err != nil {
		t.Fatal(err)
	}
	wantRestrict(t, receive(t, done, "içerik yazımı"), "onaylanmış kampanya")
	_ = writer.Rollback()

	var versions, approved int
	if err := db.QueryRow(`SELECT count(*), max(approved_content_version) FROM campaign_contents ct
		JOIN campaigns c ON c.id = ct.campaign_id WHERE c.id = $1`, id).Scan(&versions, &approved); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || approved != 1 {
		t.Fatalf("yalnızca onaylanan sürüm kalmalı: sürüm sayısı=%d onaylanan=%d", versions, approved)
	}
}

// Tersi: içerik yazımı kampanya satırı üzerinde paylaşımlı kilit tutarken
// onay (FOR UPDATE) beklemeli ve commit sonrası yeni sürümü görmelidir. Böylece
// onay, eski sürümü "son sürüm" sanamaz.
func TestApprovalWaitsForContentWriteAndSeesNewVersion(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()

	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)

	writer := begin(t, ctx, db)
	if _, err := writer.ExecContext(ctx, insertContentPrefix+` (campaign_id, version, source, sms_text, email_subject, email_body)
		VALUES ($1, 2, 'human', $2, $3, $4)`, id, goodSMS, goodSubject, goodBody); err != nil {
		t.Fatal(err)
	}

	type lockResult struct {
		latest int
		err    error
	}
	approver := begin(t, ctx, db)
	done := make(chan lockResult, 1)
	go func() {
		var status string
		if err := approver.QueryRowContext(ctx, `SELECT status FROM campaigns WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
			done <- lockResult{err: err}
			return
		}
		var latest int
		err := approver.QueryRowContext(ctx, `SELECT max(version) FROM campaign_contents WHERE campaign_id = $1`, id).Scan(&latest)
		done <- lockResult{latest: latest, err: err}
	}()
	waitForLockWaiter(t, ctx, db, `SELECT status FROM campaigns`)
	notDone(t, done, "onay kilidi")

	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	res := receive(t, done, "onay kilidi")
	if res.err != nil || res.latest != 2 {
		t.Fatalf("onay kilidi alındığında son sürüm 2 görünmeli: %+v", res)
	}
}

// İki onay isteği farklı sürümle yarışırsa ikincisi, ilki commit olunca
// onaylanan sürümü değiştiremez (uygulama durum kontrolünü atlasa bile).
func TestSecondApprovalWithDifferentVersionIsRejectedByDatabase(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()

	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	mustAddContent(t, db, id, 2)

	first := begin(t, ctx, db)
	if _, err := first.ExecContext(ctx, approveSQL, id, 1); err != nil {
		t.Fatal(err)
	}

	second := begin(t, ctx, db)
	done := make(chan error, 1)
	go func() {
		// Bilerek status = 'draft' koşulu YOK: tek koruma veritabanıdır.
		_, err := second.ExecContext(ctx, approveSQL, id, 2)
		done <- err
	}()
	waitForLockWaiter(t, ctx, db, `UPDATE campaigns SET status = 'approved'`)
	notDone(t, done, "ikinci onay")

	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}
	wantRestrict(t, receive(t, done, "ikinci onay"), "değiştirilemez")
	_ = second.Rollback()

	var approved int
	if err := db.QueryRow(`SELECT approved_content_version FROM campaigns WHERE id = $1`, id).Scan(&approved); err != nil || approved != 1 {
		t.Fatalf("ilk onay geçerli kalmalı: %d %v", approved, err)
	}
}

// Aynı sürümle yinelenen onay yarışı zararsızdır: ikinci istek kilit sonrası
// aynı değeri yazar ve kabul edilir.
func TestSecondApprovalWithSameVersionIsHarmless(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()

	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)

	first := begin(t, ctx, db)
	if _, err := first.ExecContext(ctx, approveSQL, id, 1); err != nil {
		t.Fatal(err)
	}
	second := begin(t, ctx, db)
	done := make(chan error, 1)
	go func() {
		_, err := second.ExecContext(ctx, approveSQL, id, 1)
		done <- err
	}()
	waitForLockWaiter(t, ctx, db, `UPDATE campaigns SET status = 'approved'`)
	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, done, "ikinci onay"); err != nil {
		t.Fatalf("aynı sürümle tekrar kabul edilmeli: %v", err)
	}
	if err := second.Commit(); err != nil {
		t.Fatal(err)
	}
}

// lockQuery, hem yazarların hem onayın aldığı kampanya satırı kilididir.
const lockQuery = `SELECT status FROM campaigns WHERE id = $1 FOR UPDATE`

// waitForLockWaiters, sorgusu queryPrefix ile başlayan tam want bağlantının
// bir kilidi beklediğini görene kadar bekler. Eşzamanlılık senaryoları
// zamanlamaya değil bu gözleme bağlıdır: hiçbir test uyku süresine veya
// rastgele gecikmeye dayanmaz.
func waitForLockWaiters(t *testing.T, ctx context.Context, db *sql.DB, queryPrefix string, want int) {
	t.Helper()
	deadline := time.Now().Add(lockTestTimeout)
	n := -1
	for time.Now().Before(deadline) {
		err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1`,
			queryPrefix+"%").Scan(&n)
		if err != nil {
			t.Fatalf("pg_stat_activity okunamadı: %v", err)
		}
		if n == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%q başlayan sorgulardan %d tanesi kilit bekliyor, beklenen %d", queryPrefix, n, want)
}

// startWriters, n yazarı başlatır ve sonuçlarını döndüren kanalı verir. Her
// yazar uygulamanın kalıbını izler (writeNextVersion).
func startWriters(ctx context.Context, db *sql.DB, campaignID string, n int, start <-chan struct{}) <-chan error {
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			if start != nil {
				<-start
			}
			results <- writeNextVersion(ctx, db, campaignID)
		}()
	}
	return results
}

// collectWrites, n yazma sonucunu toplar: nil başarılı, restrict_violation
// reddedilmiş, başka her hata test hatasıdır.
func collectWrites(t *testing.T, results <-chan error, n int) (written, rejected int) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := receive(t, results, "yazma")
		var pgErr *pgconn.PgError
		switch {
		case err == nil:
			written++
		case errors.As(err, &pgErr) && pgErr.Code == pgRestrictViolation:
			rejected++
		default:
			t.Fatalf("beklenmeyen yazma hatası: %v", err)
		}
	}
	return written, rejected
}

type campaignState struct {
	count, maxVersion int
	approved          sql.NullInt32
	status            string
}

func readState(t *testing.T, db *sql.DB, id string) campaignState {
	t.Helper()
	var st campaignState
	err := db.QueryRow(
		`SELECT (SELECT count(*) FROM campaign_contents WHERE campaign_id = c.id),
		        coalesce((SELECT max(version) FROM campaign_contents WHERE campaign_id = c.id), 0),
		        c.approved_content_version, c.status
		 FROM campaigns c WHERE c.id = $1`, id).Scan(&st.count, &st.maxVersion, &st.approved, &st.status)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// Çok sayıda içerik yazarı ve bir onay yarışırken şunlar doğrulanır: sürümler
// kesintisiz 1..N olur, onay commit olduktan sonra hiçbir yazma başarılı olmaz
// ve onaylanan sürüm var olan son sürümdür. Yazarlar uygulama kilidini alır
// ama durumu bilerek kontrol etmez; onaydan sonraki yazmaları yalnızca
// tetikleyici reddeder.
//
// Üç senaryo da kontrollüdür: sıra, kilit beklemesi pg_stat_activity'den
// gözlenerek kurulur; rastgele gecikme yoktur.
func TestConcurrentWritersAndApprovalKeepApprovedVersionLatest(t *testing.T) {
	// A: Onaydan önce eşzamanlı yazarlar. Kampanya draft olduğundan hepsi
	// başarılıdır ve sürümler çakışmadan 1..N olur.
	t.Run("onaydan önce eşzamanlı yazarlar hep başarılı", func(t *testing.T) {
		db := testdb.Open(t)
		db.SetMaxOpenConns(40)
		ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
		defer cancel()

		const writers = 16
		id := newCampaign(t, db)
		mustAddContent(t, db, id, 1)

		start := make(chan struct{})
		results := startWriters(ctx, db, id, writers, start)
		close(start) // hepsi aynı anda
		written, rejected := collectWrites(t, results, writers)

		if written != writers || rejected != 0 {
			t.Fatalf("draft kampanyada hepsi başarılı olmalı: %d yazıldı, %d reddedildi", written, rejected)
		}
		st := readState(t, db, id)
		if st.count != 1+writers || st.maxVersion != st.count || st.approved.Valid || st.status != "draft" {
			t.Fatalf("sürümler kesintisiz 1..%d olmalı: %+v", 1+writers, st)
		}
	})

	// B: Onay kampanya satırını kilitli tutarken bekleyen yazarlar. Onay
	// commit olunca hepsi onaylanmış durumu görür ve tetikleyici tarafından
	// reddedilir. Sonuç kesindir: hiçbiri yazılamaz.
	t.Run("kilitli onay varken bekleyen yazarların hepsi reddedilir", func(t *testing.T) {
		db := testdb.Open(t)
		db.SetMaxOpenConns(40)
		ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
		defer cancel()

		const writers = 12
		id := newCampaign(t, db)
		for v := 1; v <= 3; v++ {
			mustAddContent(t, db, id, v)
		}

		approver := begin(t, ctx, db)
		var status string
		if err := approver.QueryRowContext(ctx, lockQuery, id).Scan(&status); err != nil || status != "draft" {
			t.Fatalf("onay kilidi: %q %v", status, err)
		}

		results := startWriters(ctx, db, id, writers, nil)
		waitForLockWaiters(t, ctx, db, lockQuery, writers) // hepsi kilitte bekliyor

		var latest int
		if err := approver.QueryRowContext(ctx, `SELECT max(version) FROM campaign_contents WHERE campaign_id = $1`, id).Scan(&latest); err != nil {
			t.Fatal(err)
		}
		if _, err := approver.ExecContext(ctx, approveSQL, id, latest); err != nil {
			t.Fatal(err)
		}
		if err := approver.Commit(); err != nil {
			t.Fatal(err)
		}

		written, rejected := collectWrites(t, results, writers)
		if written != 0 || rejected != writers {
			t.Fatalf("hepsi reddedilmeli: %d yazıldı, %d reddedildi", written, rejected)
		}
		st := readState(t, db, id)
		if st.count != 3 || st.maxVersion != 3 || !st.approved.Valid || st.approved.Int32 != 3 || st.status != "approved" {
			t.Fatalf("yalnızca 3 sürüm ve onaylanan 3 kalmalı: %+v", st)
		}
	})

	// C: Onay, yazarlarla aynı kilit kuyruğunda bekler. Kilidi önce bir yazar
	// tutar; onay ve diğer yazarlar arkasında sıralanır. Kilidin hangi sırayla
	// verileceği veritabanına bağlıdır, bu yüzden bu senaryo sonucu değil
	// değişmezleri doğrular: onay hangi sırada gelirse gelsin onaylanan sürüm son
	// sürümdür, sürümler kesintisizdir ve onaydan sonra hiçbir yazma başarılı olmaz.
	t.Run("kuyrukta sıralanan onay için değişmezler korunur", func(t *testing.T) {
		db := testdb.Open(t)
		db.SetMaxOpenConns(40)
		ctx, cancel := context.WithTimeout(context.Background(), 4*lockTestTimeout)
		defer cancel()

		const writers = 10
		for round := 0; round < 6; round++ {
			id := newCampaign(t, db)
			mustAddContent(t, db, id, 1)

			// Kilidi tutan yazar: kilitler, sürümü yazar, commit etmez.
			holder := begin(t, ctx, db)
			var status string
			if err := holder.QueryRowContext(ctx, lockQuery, id).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if err := addContent(ctxExecer{ctx, holder}, id, 2, "human", goodSMS, goodSubject, goodBody); err != nil {
				t.Fatal(err)
			}

			approved := make(chan error, 1)
			go func() { approved <- approveLatest(ctx, db, id) }()
			waitForLockWaiters(t, ctx, db, lockQuery, 1)
			results := startWriters(ctx, db, id, writers, nil)
			waitForLockWaiters(t, ctx, db, lockQuery, 1+writers)

			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := receive(t, approved, "onay"); err != nil {
				t.Fatalf("tur %d: onay başarısız: %v", round, err)
			}
			written, rejected := collectWrites(t, results, writers)
			if written+rejected != writers {
				t.Fatalf("tur %d: yazma sonuçları eksik: %d yazıldı, %d reddedildi", round, written, rejected)
			}

			st := readState(t, db, id)
			if st.status != "approved" {
				t.Fatalf("tur %d: durum approved olmalı: %+v", round, st)
			}
			if st.count != st.maxVersion {
				t.Fatalf("tur %d: sürümler kesintisiz olmalı: %+v", round, st)
			}
			if st.count != 2+written {
				t.Fatalf("tur %d: satır sayısı başarılı yazmalarla uyuşmalı: %d satır, %d yazma", round, st.count, written)
			}
			if !st.approved.Valid || int(st.approved.Int32) != st.maxVersion {
				t.Fatalf("tur %d: onaylanan sürüm var olan son sürüm olmalı: %+v", round, st)
			}
		}
	})
}

// writeNextVersion, uygulamanın izleyeceği kalıbı uygular: kampanya satırını
// FOR UPDATE kilitle, sıradaki sürümü hesapla, yaz, commit et. Durumu
// kontrol etmez.
func writeNextVersion(ctx context.Context, db *sql.DB, campaignID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM campaigns WHERE id = $1 FOR UPDATE`, campaignID).Scan(&status); err != nil {
		return err
	}
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) + 1 FROM campaign_contents WHERE campaign_id = $1`, campaignID).Scan(&next); err != nil {
		return err
	}
	if err := addContent(ctxExecer{ctx, tx}, campaignID, next, "human", goodSMS, goodSubject, goodBody); err != nil {
		return err
	}
	return tx.Commit()
}

// approveLatest, onay kalıbını uygular: kilitle, son sürümü oku, onayla, commit et.
func approveLatest(ctx context.Context, db *sql.DB, campaignID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM campaigns WHERE id = $1 FOR UPDATE`, campaignID).Scan(&status); err != nil {
		return err
	}
	var latest int
	if err := tx.QueryRowContext(ctx, `SELECT max(version) FROM campaign_contents WHERE campaign_id = $1`, campaignID).Scan(&latest); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, approveSQL, campaignID, latest); err != nil {
		return err
	}
	return tx.Commit()
}

// ctxExecer, *sql.Tx'i bağlamıyla birlikte execer olarak sunar.
type ctxExecer struct {
	ctx context.Context
	tx  *sql.Tx
}

func (c ctxExecer) Exec(query string, args ...any) (sql.Result, error) {
	return c.tx.ExecContext(c.ctx, query, args...)
}
