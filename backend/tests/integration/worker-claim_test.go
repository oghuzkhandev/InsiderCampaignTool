package integration

import (
	"context"
	"database/sql"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

const (
	testMaxAttempts = 5
	testLease       = time.Minute
)

// queue, onaylı bir kampanya ve kuyruğa yazılmış mesajlarını tanımlar.
type queue struct {
	campaignID string
	userIDs    []int64
}

// seedQueue, onaylı bir kampanya (sürüm 1) ve n kullanıcı yazar. Kullanıcıların adı,
// telefonu, e-postası ve her seçili kanal için açık izni vardır; her kanal için
// `queued` mesaj satırı oluşturulur. Yalnızca campaigns_test'te çalışır.
func seedQueue(t *testing.T, db *sql.DB, n int, channels ...string) queue {
	t.Helper()
	q := queue{campaignID: newCampaign(t, db)}
	mustAddContent(t, db, q.campaignID, 1)
	mustApprove(t, db, q.campaignID, 1)

	rows, err := db.Query(
		`INSERT INTO users (email, city, first_name, phone)
		 SELECT 'kullanici' || g || '@example.test', 'İstanbul', 'Ad' || g, '+9055500' || lpad(g::text, 5, '0')
		 FROM generate_series(1, $1) g RETURNING id`, n)
	if err != nil {
		t.Fatalf("kullanıcılar eklenemedi: %v", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		q.userIDs = append(q.userIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()

	for _, ch := range channels {
		if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) SELECT id, $1, true FROM users`, ch); err != nil {
			t.Fatalf("izinler eklenemedi: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key)
			 SELECT $1::uuid, u.id, $2::text, CASE WHEN $2::text = 'sms' THEN u.phone ELSE u.email END,
			        coalesce(u.first_name, ''), -- onayın yaptığı gibi: adın ham kopyası
			        $3::text || ':' || u.id::text || ':' || $2::text
			 FROM users u`, q.campaignID, ch, q.campaignID); err != nil {
			t.Fatalf("mesajlar eklenemedi: %v", err)
		}
	}
	return q
}

// messageState, bir mesaj satırının worker'la ilgili alanlarıdır.
type messageState struct {
	ID            int64
	Channel       string
	Status        string
	Attempts      int
	Key           string
	FailureReason sql.NullString
	LastError     sql.NullString
	ClaimToken    sql.NullString
	ClaimedAt     sql.NullTime
	SentAt        sql.NullTime
	NextAttemptAt time.Time
}

func getMessage(t *testing.T, db *sql.DB, id int64) messageState {
	t.Helper()
	var m messageState
	err := db.QueryRow(
		`SELECT id, channel, status, attempts, idempotency_key, failure_reason, last_error,
		        claim_token::text, claimed_at, sent_at, next_attempt_at
		 FROM messages WHERE id = $1`, id,
	).Scan(&m.ID, &m.Channel, &m.Status, &m.Attempts, &m.Key, &m.FailureReason, &m.LastError,
		&m.ClaimToken, &m.ClaimedAt, &m.SentAt, &m.NextAttemptAt)
	if err != nil {
		t.Fatalf("mesaj %d okunamadı: %v", id, err)
	}
	return m
}

func messageIDs(t *testing.T, db *sql.DB, campaignID, channel string) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM messages WHERE campaign_id = $1 AND channel = $2 ORDER BY id`, campaignID, channel)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func statusCounts(t *testing.T, db *sql.DB, campaignID string) map[string]int {
	t.Helper()
	rows, err := db.Query(`SELECT status, count(*) FROM messages WHERE campaign_id = $1 GROUP BY status`, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			t.Fatal(err)
		}
		out[s] = n
	}
	return out
}

func campaignStatus(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT status FROM campaigns WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// ageLease, bir mesajın lease'ini dolmuş gösterir.
func ageLease(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	if _, err := db.Exec(`UPDATE messages SET claimed_at = now() - interval '10 minutes' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func mustClaim(t *testing.T, repo *worker.PGRepo, channel string, limit int) []worker.Job {
	t.Helper()
	jobs, err := repo.Claim(context.Background(), channel, limit, testMaxAttempts)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return jobs
}

func TestClaimTakesDueMessagesAndRecordsOwnership(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 3, "sms", "email")
	repo := worker.NewPGRepo(db)

	jobs := mustClaim(t, repo, "sms", 10)
	if len(jobs) != 3 {
		t.Fatalf("3 SMS işi bekleniyordu: %d", len(jobs))
	}
	tokens := map[string]bool{}
	for _, j := range jobs {
		if j.Channel != "sms" || j.Attempts != 1 || j.ClaimToken == "" || j.IdempotencyKey != idemKey(q.campaignID, j.UserID, "sms") {
			t.Fatalf("iş alanları: %+v", j)
		}
		if j.FirstName == "" || j.Recipient == "" || j.Content.SMSText != goodSMS || j.Content.EmailSubject != goodSubject {
			t.Fatalf("render girdileri okunmalı (ad, alıcı, onaylı içerik): %+v", j)
		}
		tokens[j.ClaimToken] = true

		row := getMessage(t, db, j.ID)
		if row.Status != "sending" || row.Attempts != 1 || !row.ClaimedAt.Valid ||
			!row.ClaimToken.Valid || row.ClaimToken.String != j.ClaimToken {
			t.Fatalf("satır sahiplenilmiş olmalı: %+v", row)
		}
	}
	if len(tokens) != 3 {
		t.Fatal("her iş ayrı bir claim token almalı")
	}

	// Diğer kanal dokunulmadı; kampanya ilk sahiplenmede sending oldu.
	for _, id := range messageIDs(t, db, q.campaignID, "email") {
		if row := getMessage(t, db, id); row.Status != "queued" || row.Attempts != 0 || row.ClaimToken.Valid {
			t.Fatalf("e-posta mesajı etkilenmemeli: %+v", row)
		}
	}
	if got := campaignStatus(t, db, q.campaignID); got != "sending" {
		t.Fatalf("kampanya sending olmalı: %q", got)
	}
	// Sahiplenilen işler tekrar verilmez.
	if again := mustClaim(t, repo, "sms", 10); len(again) != 0 {
		t.Fatalf("sahiplenilmiş iş tekrar claim edilmemeli: %d", len(again))
	}
}

// Her yeniden sahiplenmede attempts artar ve claim token yenilenir.
func TestClaimIncrementsAttemptsAndRotatesToken(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	repo := worker.NewPGRepo(db)

	first := mustClaim(t, repo, "sms", 1)[0]
	if _, err := repo.Requeue(context.Background(), first, 0, "deneme"); err != nil {
		t.Fatal(err)
	}
	second := mustClaim(t, repo, "sms", 1)[0]

	if second.ID != first.ID || first.Attempts != 1 || second.Attempts != 2 || second.ClaimToken == first.ClaimToken {
		t.Fatalf("attempts 1→2 ve yeni token bekleniyordu: %+v → %+v", first, second)
	}
	if row := getMessage(t, db, second.ID); row.Attempts != 2 || row.Key != idemKey(q.campaignID, first.UserID, "sms") {
		t.Fatalf("idempotency anahtarı sabit kalmalı: %+v", row)
	}
}

func TestClaimRespectsLimitDueTimeAndAttemptBudget(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 6, "sms")
	repo := worker.NewPGRepo(db)
	ids := messageIDs(t, db, q.campaignID, "sms")

	// ids[0]: vadesi gelecekte. ids[1]: deneme hakkı bitmiş. ids[2]: terminal.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE messages SET next_attempt_at = now() + interval '1 hour' WHERE id = $1`, ids[0])
	exec(`UPDATE messages SET attempts = $2 WHERE id = $1`, ids[1], testMaxAttempts)
	exec(`UPDATE messages SET status = 'sent', sent_at = now() WHERE id = $1`, ids[2])

	if jobs := mustClaim(t, repo, "sms", 0); len(jobs) != 0 {
		t.Fatalf("limit 0 iş vermemeli: %d", len(jobs))
	}
	first := mustClaim(t, repo, "sms", 2)
	if len(first) != 2 {
		t.Fatalf("limit 2 → 2 iş: %d", len(first))
	}
	rest := mustClaim(t, repo, "sms", 10)
	got := []int64{}
	for _, j := range append(first, rest...) {
		got = append(got, j.ID)
	}
	slices.Sort(got)
	if want := []int64{ids[3], ids[4], ids[5]}; !slices.Equal(got, want) {
		t.Fatalf("yalnızca vadesi gelen, hakkı olan, queued işler: %v, beklenen %v", got, want)
	}
	for _, id := range []int64{ids[0], ids[1]} {
		if row := getMessage(t, db, id); row.Status != "queued" || row.ClaimToken.Valid {
			t.Fatalf("claim edilmemeli: %+v", row)
		}
	}
}

// Eşzamanlı claim girişimlerinde her işin yalnızca bir sahibi olur.
func TestClaimConcurrentClaimersNeverShareAJob(t *testing.T) {
	db := testdb.Open(t)
	const total = 120
	seedQueue(t, db, total, "sms")
	repo := worker.NewPGRepo(db)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = map[int64]int{}
	)
	start := make(chan struct{})
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				jobs, err := repo.Claim(context.Background(), "sms", 7, testMaxAttempts)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}
				mu.Lock()
				for _, j := range jobs {
					seen[j.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(seen) != total {
		t.Fatalf("her iş sahiplenilmeli: %d/%d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("iş %d %d kez sahiplenildi", id, n)
		}
	}
}

// Kampanya yalnızca approved ise sending'e geçer; terminal bir kampanya geri açılmaz.
func TestClaimOnlyMovesApprovedCampaignToSending(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	repo := worker.NewPGRepo(db)
	if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, q.campaignID); err != nil {
		t.Fatal(err)
	}
	mustClaim(t, repo, "sms", 1)
	if got := campaignStatus(t, db, q.campaignID); got != "sending" {
		t.Fatalf("durum sending kalmalı: %q", got)
	}
}

func TestPrecheckReportsOwnershipLeaseAndConsent(t *testing.T) {
	db := testdb.Open(t)
	seedQueue(t, db, 1, "sms")
	repo := worker.NewPGRepo(db)
	ctx := context.Background()
	job := mustClaim(t, repo, "sms", 1)[0]

	check := func(j worker.Job) worker.Precheck {
		t.Helper()
		p, err := repo.Precheck(ctx, j, testLease)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	if p := check(job); !p.Owned || !p.Consent {
		t.Fatalf("sahip ve izinli olmalı: %+v", p)
	}

	// Başka bir claim token'ı sahip değildir.
	stranger := job
	stranger.ClaimToken = "00000000-0000-0000-0000-000000000000"
	if p := check(stranger); p.Owned || !p.Consent {
		t.Fatalf("yabancı token sahip olmamalı: %+v", p)
	}

	// İzin geri çekilince.
	if _, err := db.Exec(`UPDATE user_consents SET granted = false WHERE user_id = $1 AND channel = 'sms'`, job.UserID); err != nil {
		t.Fatal(err)
	}
	if p := check(job); !p.Owned || p.Consent {
		t.Fatalf("izin iptali: sahip kalır, izin yok: %+v", p)
	}
	// İzin kaydı hiç yoksa izin yoktur.
	if _, err := db.Exec(`DELETE FROM user_consents WHERE user_id = $1 AND channel = 'sms'`, job.UserID); err != nil {
		t.Fatal(err)
	}
	if p := check(job); p.Consent {
		t.Fatalf("kayıt yoksa izin yok: %+v", p)
	}
	// Başka kanaldaki izin SMS için geçerli değildir.
	if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, 'email', true)`, job.UserID); err != nil {
		t.Fatal(err)
	}
	if p := check(job); p.Consent {
		t.Fatalf("e-posta izni SMS'e geçmemeli: %+v", p)
	}

	// Lease dolunca sahiplik geçersizdir.
	ageLease(t, db, job.ID)
	if p := check(job); p.Owned {
		t.Fatalf("lease dolunca sahiplik yok: %+v", p)
	}
}

// Eski claim'in sonucu yeni sahibin durumunu değiştirmez: lease dolar, reaper
// işi kuyruğa alır, başka bir worker sahiplenir; ilk worker'ın geç sonucu
// uygulanmaz.
func TestStaleClaimResultNeverOverwritesNewOwner(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	repo := worker.NewPGRepo(db)
	ctx := context.Background()

	old := mustClaim(t, repo, "sms", 1)[0]
	ageLease(t, db, old.ID)
	res, err := repo.Reap(ctx, testLease, testMaxAttempts)
	if err != nil || res.Requeued != 1 || res.Exhausted != 0 {
		t.Fatalf("reaper işi yeniden kuyruğa almalı: %+v %v", res, err)
	}
	fresh := mustClaim(t, repo, "sms", 1)[0]
	if fresh.ClaimToken == old.ClaimToken || fresh.Attempts != 2 {
		t.Fatalf("yeni sahip yeni token ve attempts=2 almalı: %+v", fresh)
	}

	// Eski sahibin geç gelen her sonucu yok sayılır.
	for name, write := range map[string]func() (bool, error){
		"sent":    func() (bool, error) { return repo.MarkSent(ctx, old) },
		"failed":  func() (bool, error) { return repo.MarkFailed(ctx, old, worker.FailureRejected, "x") },
		"skipped": func() (bool, error) { return repo.MarkSkipped(ctx, old, "x") },
		"requeue": func() (bool, error) { return repo.Requeue(ctx, old, 0, "x") },
	} {
		applied, err := write()
		if err != nil || applied {
			t.Fatalf("%s: eski claim sonucu uygulanmamalı: applied=%v err=%v", name, applied, err)
		}
	}
	row := getMessage(t, db, fresh.ID)
	if row.Status != "sending" || row.ClaimToken.String != fresh.ClaimToken || row.SentAt.Valid || row.FailureReason.Valid {
		t.Fatalf("yeni sahibin durumu bozulmamalı: %+v", row)
	}
	if got := campaignStatus(t, db, q.campaignID); got != "sending" {
		t.Fatalf("kampanya sending kalmalı: %q", got)
	}

	// Güncel sahibin sonucu uygulanır.
	if applied, err := repo.MarkSent(ctx, fresh); err != nil || !applied {
		t.Fatalf("güncel sahibin sonucu uygulanmalı: %v %v", applied, err)
	}
	// Terminal mesaja ikinci sonuç yazılamaz.
	if applied, _ := repo.MarkFailed(ctx, fresh, worker.FailureExhausted, "x"); applied {
		t.Fatal("terminal mesaj değişmemeli")
	}
}

func TestResultWritesRecordTheirFields(t *testing.T) {
	db := testdb.Open(t)
	seedQueue(t, db, 4, "sms")
	repo := worker.NewPGRepo(db)
	ctx := context.Background()
	jobs := mustClaim(t, repo, "sms", 4)

	if ok, err := repo.MarkSent(ctx, jobs[0]); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := repo.MarkFailed(ctx, jobs[1], worker.FailureRejected, "HTTP 400 INVALID_RECIPIENT"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := repo.MarkSkipped(ctx, jobs[2], "kanal izni yok"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := repo.Requeue(ctx, jobs[3], 90*time.Second, "HTTP 500"); err != nil || !ok {
		t.Fatal(ok, err)
	}

	sent := getMessage(t, db, jobs[0].ID)
	if sent.Status != "sent" || !sent.SentAt.Valid || sent.FailureReason.Valid {
		t.Fatalf("sent: %+v", sent)
	}
	failed := getMessage(t, db, jobs[1].ID)
	if failed.Status != "failed" || failed.FailureReason.String != "rejected" || failed.LastError.String != "HTTP 400 INVALID_RECIPIENT" || failed.SentAt.Valid {
		t.Fatalf("failed: %+v", failed)
	}
	skipped := getMessage(t, db, jobs[2].ID)
	if skipped.Status != "skipped" || skipped.LastError.String != "kanal izni yok" || skipped.FailureReason.Valid {
		t.Fatalf("skipped: %+v", skipped)
	}
	retry := getMessage(t, db, jobs[3].ID)
	var secondsAhead float64
	if err := db.QueryRow(`SELECT extract(epoch FROM next_attempt_at - now()) FROM messages WHERE id = $1`, jobs[3].ID).Scan(&secondsAhead); err != nil {
		t.Fatal(err)
	}
	if retry.Status != "queued" || retry.LastError.String != "HTTP 500" || secondsAhead < 85 || secondsAhead > 91 {
		t.Fatalf("requeue: %+v (vade %.1f sn sonra)", retry, secondsAhead)
	}
	// Vadesi gelmemiş iş claim edilmez.
	if again := mustClaim(t, repo, "sms", 10); len(again) != 0 {
		t.Fatalf("vadesi gelmeyen retry claim edilmemeli: %d", len(again))
	}
}

func TestReapRequeuesExpiredLeasesAndClosesSpentOnes(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 5, "sms")
	repo := worker.NewPGRepo(db)
	ctx := context.Background()
	ids := messageIDs(t, db, q.campaignID, "sms")

	jobs := mustClaim(t, repo, "sms", 4) // ids[0..3] sending
	byID := map[int64]worker.Job{}
	for _, j := range jobs {
		byID[j.ID] = j
	}
	// ids[0]: lease doldu, hakkı var → queued. ids[1]: lease doldu, hak bitti →
	// failed/exhausted. ids[2]: lease geçerli → dokunulmaz.
	// ids[3]: lease doldu ama başka kampanyasız; ids[4]: queued ve hakkı bitmiş.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE messages SET claimed_at = now() - interval '10 minutes' WHERE id = ANY($1)`, []int64{ids[0], ids[1], ids[3]})
	exec(`UPDATE messages SET attempts = $2 WHERE id = $1`, ids[1], testMaxAttempts)
	exec(`UPDATE messages SET attempts = $2 WHERE id = $1`, ids[4], testMaxAttempts)

	res, err := repo.Reap(ctx, testLease, testMaxAttempts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Requeued != 2 || res.Exhausted != 2 || len(res.Campaigns) != 1 || res.Campaigns[0] != q.campaignID {
		t.Fatalf("2 yeniden kuyruğa, 2 deneme sınırı, 1 kampanya bekleniyordu: %+v", res)
	}

	for _, id := range []int64{ids[0], ids[3]} {
		if row := getMessage(t, db, id); row.Status != "queued" || row.LastError.String == "" {
			t.Fatalf("lease'i dolan iş yeniden kuyruğa alınmalı: %+v", row)
		}
	}
	for _, id := range []int64{ids[1], ids[4]} {
		if row := getMessage(t, db, id); row.Status != "failed" || row.FailureReason.String != "exhausted" {
			t.Fatalf("hakkı biten iş failed/exhausted olmalı: %+v", row)
		}
	}
	if row := getMessage(t, db, ids[2]); row.Status != "sending" || row.ClaimToken.String != byID[ids[2]].ClaimToken {
		t.Fatalf("lease'i geçerli iş dokunulmaz: %+v", row)
	}

	// Reaper'ın yeniden kuyruğa aldığı iş hemen yeniden claim edilebilir.
	if again := mustClaim(t, repo, "sms", 10); len(again) != 2 {
		t.Fatalf("yeniden kuyruğa alınan 2 iş claim edilebilmeli: %d", len(again))
	}
	// İkinci tur yapılacak iş bulmaz.
	if res, err := repo.Reap(ctx, testLease, testMaxAttempts); err != nil || res.Requeued != 0 || res.Exhausted != 0 {
		t.Fatalf("ikinci tur boş olmalı: %+v %v", res, err)
	}
}

// Kampanya tamamlama kuralı (ARCHITECTURE §7, TRK-001).
func TestCompleteCampaignOnlyWhenEveryMessageIsTerminal(t *testing.T) {
	complete := func(t *testing.T, db *sql.DB, id string) bool {
		t.Helper()
		ok, err := worker.NewPGRepo(db).CompleteCampaign(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	setStatuses := func(t *testing.T, db *sql.DB, q queue, statuses ...string) {
		t.Helper()
		ids := messageIDs(t, db, q.campaignID, "sms")
		for i, st := range statuses {
			var query string
			switch st {
			case "sent":
				query = `UPDATE messages SET status = 'sent', sent_at = now() WHERE id = $1`
			case "failed":
				query = `UPDATE messages SET status = 'failed', failure_reason = 'rejected' WHERE id = $1`
			case "skipped":
				query = `UPDATE messages SET status = 'skipped' WHERE id = $1`
			case "sending":
				query = `UPDATE messages SET status = 'sending', claim_token = gen_random_uuid(), claimed_at = now() WHERE id = $1`
			case "retry": // queued, vadesi gelecekte
				query = `UPDATE messages SET attempts = 1, next_attempt_at = now() + interval '1 hour' WHERE id = $1`
			case "queued":
				continue
			}
			if _, err := db.Exec(query, ids[i]); err != nil {
				t.Fatal(err)
			}
		}
	}

	tests := []struct {
		name     string
		statuses []string
		want     bool
	}{
		{"hepsi gönderildi", []string{"sent", "sent", "sent"}, true},
		{"gönderilen, başarısız ve atlanan birlikte", []string{"sent", "failed", "skipped"}, true},
		{"hepsi başarısız", []string{"failed", "failed", "failed"}, true},
		{"hepsi atlandı", []string{"skipped", "skipped", "skipped"}, true},
		{"gönderilmekte olan iş var", []string{"sent", "sending", "failed"}, false},
		{"retry bekleyen iş var (vadesi gelecekte)", []string{"sent", "retry", "failed"}, false},
		{"hiç denenmemiş queued iş var", []string{"sent", "queued", "sent"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := testdb.Open(t)
			q := seedQueue(t, db, len(tc.statuses), "sms")
			setStatuses(t, db, q, tc.statuses...)

			if got := complete(t, db, q.campaignID); got != tc.want {
				t.Fatalf("CompleteCampaign = %v, beklenen %v", got, tc.want)
			}
			status := campaignStatus(t, db, q.campaignID)
			if tc.want && status != "completed" || !tc.want && status == "completed" {
				t.Fatalf("kampanya durumu %q", status)
			}
			if tc.want {
				var at sql.NullTime
				if err := db.QueryRow(`SELECT completed_at FROM campaigns WHERE id = $1`, q.campaignID).Scan(&at); err != nil || !at.Valid {
					t.Fatalf("completed_at yazılmalı: %v %v", at, err)
				}
				if complete(t, db, q.campaignID) {
					t.Fatal("tamamlanmış kampanya ikinci kez tamamlanmamalı")
				}
			}
		})
	}
}

// CompleteIdleCampaigns, yalnızca bütün mesajları terminal olan ve en az bir
// mesajı bulunan `approved`/`sending` kampanyaları tamamlar; diğerlerine dokunmaz.
func TestCompleteIdleCampaignsOnlyTouchesFinishedCampaigns(t *testing.T) {
	db := testdb.Open(t)
	repo := worker.NewPGRepo(db)

	// Her kampanya kendi kullanıcılarıyla: seedQueue e-postaları sabit olduğu için
	// kampanyaları ayrı ayrı hazırlayıp önceki kullanıcıları silmeden tek tek yazarız.
	newFinished := func(statuses ...string) string {
		t.Helper()
		id := newCampaign(t, db)
		mustAddContent(t, db, id, 1)
		mustApprove(t, db, id, 1)
		for _, st := range statuses {
			uid := mustUser(t, db)
			if _, err := db.Exec(
				`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key, status, sent_at,
				                       claim_token, claimed_at)
				 VALUES ($1, $2, 'email', 'x@example.test', '', $3, $4::text,
				         CASE WHEN $4::text = 'sent' THEN now() END,
				         CASE WHEN $4::text = 'sending' THEN gen_random_uuid() END,
				         CASE WHEN $4::text = 'sending' THEN now() END)`,
				id, uid, idemKey(id, uid, "email"), st); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}

	finished := newFinished("sent", "failed", "skipped")
	allFailed := newFinished("failed", "failed")
	withQueued := newFinished("sent", "queued")
	withSending := newFinished("sent", "sending")
	noMessages := newFinished() // onaylı ama mesajsız

	draft := newCampaign(t, db) // draft: dokunulmaz
	alreadyDone := newFinished("sent")
	if _, err := db.Exec(`UPDATE campaigns SET status = 'completed', completed_at = now() - interval '1 day' WHERE id = $1`, alreadyDone); err != nil {
		t.Fatal(err)
	}

	n, err := repo.CompleteIdleCampaigns(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("yalnızca 2 kampanya tamamlanmalı: n=%d err=%v", n, err)
	}
	for id, want := range map[string]string{
		finished: "completed", allFailed: "completed",
		withQueued: "approved", withSending: "approved", noMessages: "approved",
		draft: "draft", alreadyDone: "completed",
	} {
		if got := campaignStatus(t, db, id); got != want {
			t.Errorf("kampanya %s: durum %q, beklenen %q", id[:8], got, want)
		}
	}
	// Daha önce tamamlanmış kampanyanın completed_at değeri ezilmedi.
	var old bool
	if err := db.QueryRow(`SELECT completed_at < now() - interval '1 hour' FROM campaigns WHERE id = $1`, alreadyDone).Scan(&old); err != nil || !old {
		t.Fatalf("tamamlanmış kampanyaya dokunulmamalı: %v %v", old, err)
	}
	// İkinci tur yapılacak bir şey bulmaz.
	if n, err := repo.CompleteIdleCampaigns(context.Background()); err != nil || n != 0 {
		t.Fatalf("ikinci tur boş olmalı: n=%d err=%v", n, err)
	}
}

// mustUser, benzersiz e-postalı bir kullanıcı ekler ve kimliğini döndürür.
func mustUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO users (email, city) VALUES ('u' || nextval('users_id_seq') || '@example.test', 'İstanbul') RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// 99 gönderildi + 1 başarısız: kampanya completed olur ve dağılım korunur.
func TestCompleteCampaignKeepsRealDistribution(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 100, "sms")
	if _, err := db.Exec(`UPDATE messages SET status = 'sent', sent_at = now() WHERE campaign_id = $1 AND id <> (SELECT min(id) FROM messages WHERE campaign_id = $1)`, q.campaignID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE messages SET status = 'failed', failure_reason = 'rejected' WHERE campaign_id = $1 AND status = 'queued'`, q.campaignID); err != nil {
		t.Fatal(err)
	}
	if ok, err := worker.NewPGRepo(db).CompleteCampaign(context.Background(), q.campaignID); err != nil || !ok {
		t.Fatalf("kampanya tamamlanmalı: %v %v", ok, err)
	}
	if got := statusCounts(t, db, q.campaignID); got["sent"] != 99 || got["failed"] != 1 || len(got) != 2 {
		t.Fatalf("dağılım 99 sent / 1 failed olmalı: %v", got)
	}
}
