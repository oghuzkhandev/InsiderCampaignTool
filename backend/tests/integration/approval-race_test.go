package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// Bu dosyadaki testler gerçek onay uç noktasını eşzamanlı isteklerle sınar.
// Sıra, rastgele gecikmeyle değil pg_stat_activity'den gözlenen kilit
// beklemesiyle kurulur.

func raceFixture(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	addRecipients(t, db, []recipientFixture{
		{email: "a@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"},
		{email: "b@example.test", phone: str("+905550000002"), sms: "granted", mail: "granted"},
		{email: "telefonsuz@example.test", sms: "granted", mail: "none"},
	})
	return id
}

// Aynı sürümle eşzamanlı çok sayıda onay: iş yalnızca bir kez oluşur, hepsi 200
// ve aynı sonucu (aynı onay zamanı ve sayılar) görür.
func TestConcurrentApprovalsOfSameVersionQueueOnce(t *testing.T) {
	db := testdb.Open(t)
	db.SetMaxOpenConns(40)
	h := newAPI(db, llm.NewMock())
	id := raceFixture(t, db)

	const callers = 16
	start := make(chan struct{})
	recs := make(chan *httptest.ResponseRecorder, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-start
			recs <- approvePost(h, id, 1)
		}()
	}
	close(start)

	var approvedAt string
	for i := 0; i < callers; i++ {
		rec := receive(t, recs, "onay")
		if rec.Code != http.StatusOK {
			t.Fatalf("hepsi 200 olmalı: %d %s", rec.Code, rec.Body.String())
		}
		var r approveJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r.Queued != (countsJSON{SMS: 2, Email: 2, Total: 4}) || r.ApprovedContentVersion != 1 {
			t.Fatalf("sonuç aynı olmalı: %+v", r)
		}
		if approvedAt == "" {
			approvedAt = r.ApprovedAt
		} else if r.ApprovedAt != approvedAt {
			t.Fatalf("onay zamanı aynı olmalı: %q %q", r.ApprovedAt, approvedAt)
		}
	}
	if n := messageCount(t, db); n != 4 {
		t.Fatalf("iş bir kez oluşmalı: %d mesaj", n)
	}
}

// Farklı sürümlerle eşzamanlı onaylar: yalnızca son sürümle olan geçer, eski
// sürümle olanların hepsi 409 alır; iş bir kez oluşur.
func TestConcurrentApprovalsOfDifferentVersionsOnlyLatestWins(t *testing.T) {
	db := testdb.Open(t)
	db.SetMaxOpenConns(40)
	h := newAPI(db, llm.NewMock())
	id := raceFixture(t, db)
	mustAddContent(t, db, id, 2)

	const each = 8
	start := make(chan struct{})
	type outcome struct {
		version int
		rec     *httptest.ResponseRecorder
	}
	results := make(chan outcome, 2*each)
	for i := 0; i < each; i++ {
		for _, v := range []int{1, 2} {
			go func() {
				<-start
				results <- outcome{v, approvePost(h, id, v)}
			}()
		}
	}
	close(start)

	for i := 0; i < 2*each; i++ {
		o := receive(t, results, "onay")
		switch o.version {
		case 2:
			if o.rec.Code != http.StatusOK {
				t.Fatalf("son sürüm 200 olmalı: %d %s", o.rec.Code, o.rec.Body.String())
			}
		case 1:
			// v2 henüz onaylanmadıysa eski sürüm (STALE), onaylandıysa kampanya başka
			// sürümle onaylıdır (ALREADY_APPROVED). İkisi de 409'dur.
			code, _ := decodeAPIError(t, o.rec)
			if o.rec.Code != http.StatusConflict || (code != "STALE_CONTENT_VERSION" && code != "CAMPAIGN_ALREADY_APPROVED") {
				t.Fatalf("eski sürüm 409 olmalı: %d %s", o.rec.Code, o.rec.Body.String())
			}
		}
	}
	st := approvalState(t, db, id)
	if st.Status != "approved" || st.Version.Int32 != 2 || messageCount(t, db) != 4 {
		t.Fatalf("yalnızca v2 onaylanmalı ve iş bir kez oluşmalı: %+v mesaj=%d", st, messageCount(t, db))
	}
}

// approveLockQuery, onayın kampanya satırını kilitlerken kullandığı sorgunun başıdır.
const approveLockQuery = `SELECT status, approved_content_version, approved_at, segment, scheduled_at FROM campaigns WHERE id = $1 FOR UPDATE`

const slowTriggerSQL = `CREATE FUNCTION zz_slow_messages() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.4); RETURN NULL; END $$`

// installSlowMessagesTrigger, yalnızca campaigns_test'te, mesaj yazan her ifadeyi
// yavaşlatan geçici bir tetikleyici kurar: onay transaction'ı kilidi tutarak
// uykuda kalır ve eşzamanlı isteğin kilitte beklemesi gözlenebilir. Sıra
// zamanlamaya değil, pg_stat_activity'deki PgSleep gözlemine bağlıdır.
func installSlowMessagesTrigger(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		slowTriggerSQL,
		`CREATE TRIGGER zz_slow_messages BEFORE INSERT ON messages FOR EACH STATEMENT EXECUTE FUNCTION zz_slow_messages()`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS zz_slow_messages ON messages`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS zz_slow_messages()`)
	})
}

func waitForPgSleep(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	deadline := time.Now().Add(lockTestTimeout)
	for time.Now().Before(deadline) {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event = 'PgSleep'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("onay transaction'ı mesaj yazma aşamasına gelmedi")
}

// Onay işlemdeyken (kilit tutulurken) gelen içerik düzenlemesi bekler ve onay
// commit olunca 409 alır. Onaylanan sürüm son sürümdür; yeni sürüm eklenmez.
func TestContentEditDuringApprovalIsRejectedAfterCommit(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()
	h := newAPI(db, llm.NewMock())
	id := raceFixture(t, db)
	installSlowMessagesTrigger(t, db)

	approved := make(chan *httptest.ResponseRecorder, 1)
	go func() { approved <- approvePost(h, id, 1) }()
	waitForPgSleep(t, ctx, db) // onay kilidi tutuyor ve mesaj yazıyor

	edited := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		edited <- putEdit(h, id, `{"sms_text":"Yeni {{first_name}}","email_subject":"Konu","email_body":"Gövde"}`)
	}()
	waitForLockWaiters(t, ctx, db, lockQuery, 1) // düzenleme kampanya satırını bekliyor
	notDone(t, edited, "içerik düzenleme")

	if rec := receive(t, approved, "onay"); rec.Code != http.StatusOK {
		t.Fatalf("onay 200 olmalı: %d %s", rec.Code, rec.Body.String())
	}
	wantApproveError(t, receive(t, edited, "içerik düzenleme"), http.StatusConflict, "CAMPAIGN_NOT_DRAFT")

	if n := countContents(t, db, id); n != 1 {
		t.Fatalf("yeni sürüm eklenmemeli: %d", n)
	}
	if st := approvalState(t, db, id); st.Status != "approved" || st.Version.Int32 != 1 {
		t.Fatalf("v1 onaylı kalmalı: %+v", st)
	}
}

// Tersi: içerik yazımı kilidi tutarken gelen onay bekler ve yeni sürümü görüp
// eski sürümle onayı reddeder (409); hiçbir iş oluşmaz.
func TestApprovalOfOlderVersionIsRejectedWhenNewVersionIsBeingWritten(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()
	h := newAPI(db, llm.NewMock())
	id := raceFixture(t, db)

	// Yazım transaction'ı: kilidi alır, v2'yi yazar, commit etmez.
	writer := begin(t, ctx, db)
	var status string
	if err := writer.QueryRowContext(ctx, lockQuery, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := addContent(ctxExecer{ctx, writer}, id, 2, "human", goodSMS, goodSubject, goodBody); err != nil {
		t.Fatal(err)
	}

	approved := make(chan *httptest.ResponseRecorder, 1)
	go func() { approved <- approvePost(h, id, 1) }()
	waitForLockWaiters(t, ctx, db, approveLockQuery, 1)
	notDone(t, approved, "onay")

	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	wantApproveError(t, receive(t, approved, "onay"), http.StatusConflict, "STALE_CONTENT_VERSION")
	wantDraftUntouched(t, db, id)
	if n := countContents(t, db, id); n != 2 {
		t.Fatalf("v2 kayıtlı kalmalı: %d", n)
	}
}

// Onay ile içerik düzenlemesi aynı anda başlatılır. Hangisinin kazandığı
// veritabanına bağlıdır; her iki sonuçta da değişmezler korunmalıdır: tam
// olarak biri başarılıdır, onaylanan sürüm var olan son sürümdür, iş ya hiç
// yoktur (draft) ya da tam bir kez oluşmuştur.
func TestApprovalAndContentEditRaceKeepInvariants(t *testing.T) {
	db := testdb.Open(t)
	db.SetMaxOpenConns(40)
	h := newAPI(db, llm.NewMock())

	var approveWon, editWon int
	for round := 0; round < 25; round++ {
		id := raceFixture2(t, db, round)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var approveRec, editRec *httptest.ResponseRecorder
		wg.Add(2)
		go func() { defer wg.Done(); <-start; approveRec = approvePost(h, id, 1) }()
		go func() {
			defer wg.Done()
			<-start
			editRec = putEdit(h, id, `{"sms_text":"Yeni","email_subject":"Konu","email_body":"Gövde"}`)
		}()
		close(start)
		wg.Wait()

		st := approvalState(t, db, id)
		versions := countContents(t, db, id)
		queued := len(messageRows(t, db, id))
		switch {
		case approveRec.Code == http.StatusOK && editRec.Code == http.StatusConflict:
			approveWon++
			if st.Status != "approved" || st.Version.Int32 != 1 || versions != 1 || queued != 4 {
				t.Fatalf("tur %d (onay kazandı): %+v sürüm=%d mesaj=%d", round, st, versions, queued)
			}
		case approveRec.Code == http.StatusConflict && editRec.Code == http.StatusOK:
			editWon++
			wantApproveError(t, approveRec, http.StatusConflict, "STALE_CONTENT_VERSION")
			if st.Status != "draft" || versions != 2 || queued != 0 {
				t.Fatalf("tur %d (düzenleme kazandı): %+v sürüm=%d mesaj=%d", round, st, versions, queued)
			}
		default:
			t.Fatalf("tur %d: tam biri başarılı olmalı: onay %d, düzenleme %d\n%s\n%s",
				round, approveRec.Code, editRec.Code, approveRec.Body.String(), editRec.Body.String())
		}
	}
	t.Logf("onay kazandı: %d, düzenleme kazandı: %d", approveWon, editWon)
}

func raceFixture2(t *testing.T, db *sql.DB, round int) string {
	t.Helper()
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	if round == 0 {
		addRecipients(t, db, []recipientFixture{
			{email: "a@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"},
			{email: "b@example.test", phone: str("+905550000002"), sms: "granted", mail: "granted"},
		})
	}
	return id
}

// İstemci iptali (bağlantı kopması) commit'ten ÖNCE gelirse onay geri alınır:
// kampanya draft kalır ve hiçbir mesaj oluşmaz. Onay transaction'ı mesajları
// yazarken (yavaş tetikleyicide uykuda) istek bağlamı iptal edilir.
func TestApprovalCanceledBeforeCommitIsRolledBack(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancelTest := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancelTest()
	h := newAPI(db, llm.NewMock())
	id := raceFixture(t, db)
	installSlowMessagesTrigger(t, db)

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, approvePath(id), strings.NewReader(`{"content_version":1}`)).WithContext(reqCtx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		finished <- rec
	}()
	waitForPgSleep(t, ctx, db) // transaction açık: onay ve mesajlar henüz commit edilmedi
	cancel()
	rec := receive(t, finished, "iptal edilen onay")

	// İstemci gittiği için yanıt yazılmaz (ResponseRecorder'ın varsayılan 200'ü
	// başarı değildir); asıl ölçüt veritabanı durumudur.
	if strings.Contains(rec.Body.String(), "approved_content_version") {
		t.Fatalf("iptal edilen onay başarı sonucu döndürmemeli: %s", rec.Body.String())
	}
	waitForNoActiveBackend(t, ctx, db, "INSERT INTO messages") // sunucu tarafı sorgu bitti
	wantDraftUntouched(t, db, id)
}

// Commit edilmiş (yanıtı tamamlanmış) onay, sonradan gelen iptalle geri alınmaz.
func TestApprovalCompletedBeforeCancelIsKept(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := raceFixture(t, db)

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, approvePath(id), strings.NewReader(`{"content_version":1}`)).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("onay = %d %s", rec.Code, rec.Body.String())
	}
	cancel() // yanıt tamamlandıktan sonra iptal

	if st := approvalState(t, db, id); st.Status != "approved" || st.Version.Int32 != 1 {
		t.Fatalf("tamamlanmış onay iptalle geri alınmamalı: %+v", st)
	}
	if n := len(messageRows(t, db, id)); n != 4 {
		t.Fatalf("mesajlar kalmalı: %d", n)
	}
}

// waitForNoActiveBackend, sorgusu prefix ile başlayan etkin bir bağlantı
// kalmayana kadar bekler.
func waitForNoActiveBackend(t *testing.T, ctx context.Context, db *sql.DB, prefix string) {
	t.Helper()
	deadline := time.Now().Add(lockTestTimeout)
	for time.Now().Before(deadline) {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND state = 'active' AND query LIKE $1`, prefix+"%").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%q başlayan sorgu hâlâ etkin", prefix)
}
