package integration

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

// Bu dosyadaki testler gerçek worker, gerçek mock provider handler'ı ve gerçek
// PostgreSQL'i (yalnızca campaigns_test) aynı süreçte birleştirir. Gerçek bir
// mesaj sağlayıcısı veya API anahtarı kullanılmaz. Süreç dışı senaryolar (SIGKILL,
// SIGTERM, provider yeniden başlatma) backend/tests/e2e/ altındadır.

// startProvider, gerçek PGStore'lu bir mock provider sunucusu başlatır.
func startProvider(t *testing.T, db *sql.DB, cfg mockprovider.Config, opts ...mockprovider.Option) (*httptest.Server, *mockprovider.Handler) {
	t.Helper()
	opts = append([]mockprovider.Option{mockprovider.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)
	h, err := mockprovider.New(cfg, mockprovider.NewPGStore(db), opts...)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, h
}

// fastConfig, testler için kısaltılmış süreli bir worker yapılandırmasıdır;
// lease ve kapanış süresi ilişkisi korunur (lease > kapanış), uygulamanın
// varsayılanları değişmez.
func fastConfig() worker.Config {
	return worker.Config{
		Capacity:        5,
		MaxAttempts:     5,
		Lease:           2 * time.Second,
		ReapInterval:    50 * time.Millisecond,
		PollInterval:    5 * time.Millisecond,
		ShutdownTimeout: 500 * time.Millisecond,
		Rate:            2000,
		SendBurst:       2,
		DBBackoffMin:    5 * time.Millisecond,
		DBBackoffMax:    50 * time.Millisecond,
		BackoffBase:     time.Millisecond,
		BackoffCap:      4 * time.Millisecond,
	}
}

type runningWorker struct {
	*worker.Worker
	stop func()
}

// startWorker, worker'ı arka planda çalıştırır; test bitince durdurur.
func startWorker(t *testing.T, db *sql.DB, prov worker.Provider, cfg worker.Config, opts ...worker.Option) *runningWorker {
	t.Helper()
	return startWorkerWithRepo(t, worker.NewPGRepo(db), prov, cfg, opts...)
}

// startWorkerWithRepo, verilen Repository ile worker başlatır (hata enjekte eden
// sarmalayıcılar için).
func startWorkerWithRepo(t *testing.T, repo worker.Repository, prov worker.Provider, cfg worker.Config, opts ...worker.Option) *runningWorker {
	t.Helper()
	opts = append([]worker.Option{
		worker.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		worker.WithBackoff(worker.Backoff{Base: time.Millisecond, Cap: 4 * time.Millisecond}),
	}, opts...)
	w, err := worker.New(cfg, repo, prov, opts...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(ctx) }()

	var once sync.Once
	rw := &runningWorker{Worker: w, stop: func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("worker kapanmadı")
			}
		})
	}}
	t.Cleanup(rw.stop)
	return rw
}

// awaitCond, koşul sağlanana kadar bekler; süre dolarsa testi durdurur.
func awaitCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("zaman aşımı: %s", what)
}

func awaitCampaign(t *testing.T, db *sql.DB, id, want string) {
	t.Helper()
	awaitCond(t, "kampanya "+want+" oldu", func() bool { return campaignStatus(t, db, id) == want })
}

func acceptanceCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM mock_provider_acceptances`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func singleMessage(t *testing.T, db *sql.DB, q queue, channel string) messageState {
	t.Helper()
	ids := messageIDs(t, db, q.campaignID, channel)
	if len(ids) != 1 {
		t.Fatalf("%s için tek mesaj bekleniyordu: %d", channel, len(ids))
	}
	return getMessage(t, db, ids[0])
}

func httpProvider(srv *httptest.Server) *worker.HTTPProvider { return worker.NewHTTPProvider(srv.URL) }

func TestDeliveryCompletesCampaignAndRecordsEveryAcceptanceOnce(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 20, "sms", "email")
	srv, prov := startProvider(t, db, mockprovider.Config{})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	if got := statusCounts(t, db, q.campaignID); got["sent"] != 40 || len(got) != 1 {
		t.Fatalf("40 mesajın hepsi sent olmalı: %v", got)
	}
	var attemptsOverOne int
	if err := db.QueryRow(`SELECT count(*) FROM messages WHERE attempts <> 1`).Scan(&attemptsOverOne); err != nil || attemptsOverOne != 0 {
		t.Fatalf("hatasız provider ile her mesaj tek denemede gitmeli: %d %v", attemptsOverOne, err)
	}
	var completedAt sql.NullTime
	if err := db.QueryRow(`SELECT completed_at FROM campaigns WHERE id = $1`, q.campaignID).Scan(&completedAt); err != nil || !completedAt.Valid {
		t.Fatalf("completed_at yazılmış olmalı: %v %v", completedAt, err)
	}

	// Provider her mesajı tam bir kez kabul etti ve kayıt mesajla eşleşiyor.
	if got := acceptanceCount(t, db); got != 40 {
		t.Fatalf("40 kabul kaydı bekleniyordu: %d", got)
	}
	var matched int
	err := db.QueryRow(
		`SELECT count(*) FROM messages m JOIN mock_provider_acceptances a
		   ON a.idempotency_key = m.idempotency_key AND a.channel = m.channel AND a.recipient = m.recipient`).Scan(&matched)
	if err != nil || matched != 40 {
		t.Fatalf("her mesajın anahtarı, kanalı ve alıcısıyla eşleşen bir kabul kaydı olmalı: %d %v", matched, err)
	}
	snap := prov.Stats().Snapshot()
	if snap.TotalRequests != 40 || snap.UniqueAccepts != 40 || snap.DuplicateRequests != 0 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("40 istek, 40 kabul, 0 tekrar: %+v", snap)
	}
	m := w.Metrics().Snapshot()
	if m.MessagesSent != 40 || m.Attempts[worker.KindAccepted] != 40 || m.StaleResults != 0 {
		t.Fatalf("worker metrikleri: %+v", m)
	}
}

// Ana idempotency kanıtı: provider ilk isteği kabul edip yanıtı düşürür. Worker
// aynı anahtarla yeniden dener; 2 HTTP isteği, tek kabul kaydı, sıfır tekrar
// teslimat ve sonunda `sent` (attempts = 2).
func TestLostFirstResponseIsRetriedWithTheSameKeyAndDeliveredOnce(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms", "email")
	srv, prov := startProvider(t, db, mockprovider.Config{Mode: mockprovider.ModeAcceptThenDrop})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	for _, ch := range []string{"sms", "email"} {
		m := singleMessage(t, db, q, ch)
		if m.Status != "sent" || m.Attempts != 2 || m.FailureReason.Valid {
			t.Fatalf("%s: sent ve attempts=2 bekleniyordu: %+v", ch, m)
		}
	}
	// Her kanal için 2 HTTP isteği → toplam 4; 2 kabul; 2 tekrar isteği; 2 düşen yanıt.
	snap := prov.Stats().Snapshot()
	if snap.TotalRequests != 4 || snap.UniqueAccepts != 2 || snap.DuplicateRequests != 2 ||
		snap.ResponsesDropped != 2 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("4 istek, 2 kabul, 2 tekrar, 2 düşen yanıt, 0 tekrar teslimat: %+v", snap)
	}
	if len(snap.Attempts["sms"]) != 2 || len(snap.Attempts["email"]) != 2 {
		t.Fatalf("her kanalda 2 deneme zaman damgası: %v", snap.Attempts)
	}
	if got := acceptanceCount(t, db); got != 2 {
		t.Fatalf("2 kabul kaydı (her mesaj için bir): %d", got)
	}
	// Gönderim takibinin görüntüsü: iki mesaj da ikinci denemede gönderildi (yeniden denemeyle
	// toparlandı), her kanalda biri.
	view := snapshotOf(t, db, q.campaignID)
	if view.Recovered != 2 || view.Messages.Sent != 2 ||
		view.Channels.SMS.Sent != 1 || view.Channels.Email.Sent != 1 {
		t.Fatalf("toparlanan 2 ve kanal başına 1 gönderilen bekleniyordu: %+v", view)
	}
}

// Sürekli yanıt kaybı ayrı bir senaryodur: provider isteği kabul eder ama hiçbir
// yanıt ulaşmaz. Deneme sınırında mesaj `failed` / `exhausted` olur; kabul yine
// tek kez kaydedilmiştir ve tekrar teslimat yoktur.
func TestEveryResponseLostEndsExhaustedWithSingleAcceptance(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	srv, prov := startProvider(t, db, mockprovider.Config{Mode: mockprovider.ModeDropAllResponses})
	cfg := fastConfig() // azami deneme 5
	w := startWorker(t, db, httpProvider(srv), cfg)

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "sms")
	if m.Status != "failed" || m.FailureReason.String != worker.FailureExhausted || m.Attempts != cfg.MaxAttempts || m.SentAt.Valid {
		t.Fatalf("failed/exhausted ve attempts=%d bekleniyordu: %+v", cfg.MaxAttempts, m)
	}
	if m.LastError.String == "" {
		t.Fatal("sonucu açıklayan last_error yazılmalı")
	}
	snap := prov.Stats().Snapshot()
	if snap.TotalRequests != cfg.MaxAttempts || snap.UniqueAccepts != 1 || snap.DuplicateRequests != cfg.MaxAttempts-1 ||
		snap.ResponsesDropped != cfg.MaxAttempts || snap.DuplicateDeliveries != 0 {
		t.Fatalf("5 istek, 1 kabul, 4 tekrar, 5 düşen yanıt, 0 tekrar teslimat: %+v", snap)
	}
	if got := acceptanceCount(t, db); got != 1 {
		t.Fatalf("tek kabul kaydı bekleniyordu: %d", got)
	}
	// Kampanya yine completed olur ve gerçek dağılım korunur.
	if got := statusCounts(t, db, q.campaignID); got["failed"] != 1 || len(got) != 1 {
		t.Fatalf("dağılım: %v", got)
	}
	// Hiçbir mesaj gönderilmedi: toparlanan yok; başarısızlık SMS kanalında ve nedeni belli.
	view := snapshotOf(t, db, q.campaignID)
	if view.Recovered != 0 || view.Channels.SMS.Failed != 1 || view.Failures.Exhausted != 1 {
		t.Fatalf("toparlanan 0, SMS'te 1 başarısız (exhausted) bekleniyordu: %+v", view)
	}
}

// 429 ayrı bir durumdur: yeniden denenir ve geçerli Retry-After'dan önce yeni istek
// gitmez.
func TestRateLimitedRequestIsRetriedOnlyAfterRetryAfter(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	var rolls atomic.Int32
	srv, prov := startProvider(t, db,
		mockprovider.Config{RateLimitRate: 0.5, RetryAfter: time.Second},
		mockprovider.WithRand(func() float64 {
			if rolls.Add(1) == 1 {
				return 0 // ilk istek 429
			}
			return 0.99
		}))
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "sms")
	if m.Status != "sent" || m.Attempts != 2 {
		t.Fatalf("429 sonrası yeniden denenip sent olmalı: %+v", m)
	}
	snap := prov.Stats().Snapshot()
	times := snap.Attempts["sms"]
	if snap.RateLimited != 1 || len(times) != 2 {
		t.Fatalf("1 adet 429 ve 2 deneme bekleniyordu: %+v", snap)
	}
	if gap := times[1].Sub(times[0]); gap < 990*time.Millisecond {
		t.Fatalf("Retry-After (1 sn) dolmadan yeniden denendi: aralık %s", gap)
	}
	if got := w.Metrics().Snapshot(); got.Attempts[worker.KindRateLimited] != 1 || got.Attempts[worker.KindAccepted] != 1 {
		t.Fatalf("metrikler: %+v", got)
	}
}

// Geçici sunucu hataları aynı anahtarla yeniden denenir ve sonunda başarılı olur.
func TestTransientServerErrorsAreRetriedUntilAccepted(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "email")
	var rolls atomic.Int32
	srv, prov := startProvider(t, db,
		mockprovider.Config{ServerErrorRate: 0.5},
		mockprovider.WithRand(func() float64 {
			if rolls.Add(1) <= 2 {
				return 0 // ilk iki istek 500
			}
			return 0.99
		}))
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "email")
	if m.Status != "sent" || m.Attempts != 3 {
		t.Fatalf("iki 500'den sonra 3. denemede sent olmalı: %+v", m)
	}
	snap := prov.Stats().Snapshot()
	if snap.ServerErrors != 2 || snap.UniqueAccepts != 1 || snap.TotalRequests != 3 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("2 hata, 1 kabul, 3 istek: %+v", snap)
	}
}

// 429 dışındaki 4xx kalıcıdır: retry yok, `failed` / `rejected`.
func TestPermanentRejectionIsNeverRetried(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 2, "sms")
	ids := messageIDs(t, db, q.campaignID, "sms")
	if _, err := db.Exec(`UPDATE messages SET recipient = 'telefon-degil' WHERE id = $1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	srv, prov := startProvider(t, db, mockprovider.Config{})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	bad := getMessage(t, db, ids[0])
	if bad.Status != "failed" || bad.FailureReason.String != worker.FailureRejected || bad.Attempts != 1 ||
		bad.LastError.String != "HTTP 400 INVALID_RECIPIENT" {
		t.Fatalf("geçersiz alıcı tek denemede failed/rejected olmalı: %+v", bad)
	}
	if good := getMessage(t, db, ids[1]); good.Status != "sent" {
		t.Fatalf("diğer mesaj etkilenmemeli: %+v", good)
	}
	if snap := prov.Stats().Snapshot(); snap.TotalRequests != 2 || snap.InvalidRequests != 1 {
		t.Fatalf("geçersiz alıcı için tek istek: %+v", snap)
	}
}

// Aynı anahtar farklı alıcıyla kullanılmışsa provider reddeder ve worker bunu
// kalıcı hata sayar; başka bir mesajın kayıtlı sonucu döndürülmez.
func TestIdempotencyKeyReuseIsRejectedNotReplayed(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	m := singleMessage(t, db, q, "sms")
	// Provider bu anahtarı başka bir alıcı için daha önce kabul etmiş.
	if err := insertAcceptance(db, m.Key, "sms", "+905559999999", hashOf("başka içerik")); err != nil {
		t.Fatal(err)
	}
	srv, _ := startProvider(t, db, mockprovider.Config{})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	got := singleMessage(t, db, q, "sms")
	if got.Status != "failed" || got.FailureReason.String != worker.FailureRejected || got.Attempts != 1 ||
		got.LastError.String != "HTTP 422 IDEMPOTENCY_KEY_REUSED" {
		t.Fatalf("anahtar çakışması failed/rejected olmalı: %+v", got)
	}
	var recipient string
	if err := db.QueryRow(`SELECT recipient FROM mock_provider_acceptances WHERE idempotency_key = $1`, m.Key).Scan(&recipient); err != nil || recipient != "+905559999999" {
		t.Fatalf("özgün kabul kaydı değişmemeli: %q %v", recipient, err)
	}
}

// Onaydan sonra izin iptal edilirse (veya izin kaydı silinirse) mesaj `skipped`
// olur ve provider'a o mesaj için HİÇ istek gitmez.
func TestRevokedOrMissingConsentIsSkippedWithoutAnyProviderRequest(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 3, "sms", "email")
	u := q.userIDs
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE user_consents SET granted = false WHERE user_id = $1 AND channel = 'sms'`, u[0])
	exec(`DELETE FROM user_consents WHERE user_id = $1 AND channel = 'sms'`, u[1])
	exec(`UPDATE user_consents SET granted = false WHERE user_id = $1 AND channel = 'email'`, u[2])

	srv, prov := startProvider(t, db, mockprovider.Config{})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	if got := statusCounts(t, db, q.campaignID); got["skipped"] != 3 || got["sent"] != 3 || len(got) != 2 {
		t.Fatalf("3 atlanan, 3 gönderilen bekleniyordu: %v", got)
	}
	var skippedAccepted int
	err := db.QueryRow(
		`SELECT count(*) FROM messages m JOIN mock_provider_acceptances a ON a.idempotency_key = m.idempotency_key
		 WHERE m.status = 'skipped'`).Scan(&skippedAccepted)
	if err != nil || skippedAccepted != 0 {
		t.Fatalf("atlanan mesaj için provider kabul kaydı olmamalı: %d %v", skippedAccepted, err)
	}
	if snap := prov.Stats().Snapshot(); snap.TotalRequests != 3 {
		t.Fatalf("provider'a yalnızca izinli 3 mesaj için istek gitmeli: %+v", snap)
	}
	var reason string
	if err := db.QueryRow(`SELECT last_error FROM messages WHERE status = 'skipped' LIMIT 1`).Scan(&reason); err != nil || reason != "kanal izni yok" {
		t.Fatalf("atlama nedeni yazılmalı: %q %v", reason, err)
	}
}

// Gönderim sırasında yeniden render edilen içerik, alıcının adıyla ve onaylı
// sürümle sağlayıcıya gider.
func TestProviderReceivesRenderedApprovedContent(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 2, "sms")
	// İkinci kullanıcının adı ONAY ANINDA yoktu: mesajdaki kayıtlı ad boştur ve
	// fallback kullanılmalı (ad artık onayda mesaja kopyalanır, ADR-026; users'taki
	// sonradan değişiklik mesajı etkilemez).
	if _, err := db.Exec(`UPDATE messages SET first_name = '' WHERE user_id = $1`, q.userIDs[1]); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := map[string]string{}
	rec := &recordingProvider{inner: nil, onSend: func(j worker.Job, c content.Content) {
		mu.Lock()
		got[j.Recipient] = c.SMSText
		mu.Unlock()
	}}
	srv, _ := startProvider(t, db, mockprovider.Config{})
	rec.inner = httpProvider(srv)
	w := startWorker(t, db, rec, fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	mu.Lock()
	defer mu.Unlock()
	if got["+905550000001"] != "Merhaba Ad1, %30 indirim seni bekliyor!" ||
		got["+905550000002"] != "Merhaba "+content.FallbackName+", %30 indirim seni bekliyor!" {
		t.Fatalf("render edilmiş SMS metinleri: %v", got)
	}
}

type recordingProvider struct {
	inner  worker.Provider
	onSend func(worker.Job, content.Content)
}

func (p *recordingProvider) Send(ctx context.Context, j worker.Job, c content.Content) worker.SendResult {
	p.onSend(j, c)
	return p.inner.Send(ctx, j, c)
}

// hookProvider, ilk gönderimden önce bir kancayı bir kez çalıştırır.
type hookProvider struct {
	inner worker.Provider
	once  sync.Once
	hook  func(ctx context.Context, j worker.Job)
}

func (p *hookProvider) Send(ctx context.Context, j worker.Job, c content.Content) worker.SendResult {
	p.once.Do(func() { p.hook(ctx, j) })
	return p.inner.Send(ctx, j, c)
}

// Eski claim ile sonuç yazılamaz: worker bir mesajı gönderirken lease dolar, reaper
// işi kuyruğa alır ve başka bir worker sahiplenir. İlk worker'ın geç gelen sonucu
// uygulanmaz, `stale_result_total` artar ve yeni sahibin durumu bozulmaz.
func TestStaleResultFromWorkerWithExpiredLeaseIsIgnored(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	srv, _ := startProvider(t, db, mockprovider.Config{})
	repo := worker.NewPGRepo(db)

	var (
		mu       sync.Mutex
		newOwner worker.Job
	)
	hook := &hookProvider{inner: httpProvider(srv), hook: func(ctx context.Context, j worker.Job) {
		// İlk worker'ın lease'i doldu; reaper işi kuyruğa aldı ve başka bir worker
		// sahiplendi. Test içindeki worker'ın kendi claim döngüsü kuyruktaki işi
		// "ikinci worker"dan önce kapabileceği için reaper + ikinci claim tek
		// atomik adımda yapılır: satır `sending` kalır, token ve attempts yenilenir.
		// (Reaper'ın kendisi ayrıca TestStaleClaimResultNeverOverwritesNewOwner ve
		// çöken worker testlerinde sınanır.)
		var token string
		var attempts int
		err := db.QueryRowContext(ctx,
			`UPDATE messages SET claim_token = gen_random_uuid(), claimed_at = now(), attempts = attempts + 1
			 WHERE id = $1 RETURNING claim_token::text, attempts`, j.ID).Scan(&token, &attempts)
		if err != nil {
			t.Errorf("ikinci worker işi sahiplenemedi: %v", err)
			return
		}
		mu.Lock()
		newOwner = j
		newOwner.ClaimToken, newOwner.Attempts = token, attempts
		mu.Unlock()
	}}
	cfg := fastConfig()
	cfg.ReapInterval = time.Hour // yalnızca kanca reaper'ı çalıştırır
	w := startWorker(t, db, hook, cfg)

	awaitCond(t, "eski sonuç yok sayıldı", func() bool { return w.Metrics().Snapshot().StaleResults == 1 })
	w.stop()

	mu.Lock()
	owner := newOwner
	mu.Unlock()
	row := singleMessage(t, db, q, "sms")
	if row.Status != "sending" || row.ClaimToken.String != owner.ClaimToken || row.Attempts != 2 || row.SentAt.Valid {
		t.Fatalf("yeni sahibin durumu bozulmamalı: %+v (yeni token %s)", row, owner.ClaimToken)
	}
	if got := campaignStatus(t, db, q.campaignID); got != "sending" {
		t.Fatalf("kampanya tamamlanmamalı: %q", got)
	}
	if m := w.Metrics().Snapshot(); m.MessagesSent != 0 {
		t.Fatalf("eski sonuç gönderim sayılmamalı: %+v", m)
	}

	// Yeni sahip sonucunu yazabilir ve kampanya tamamlanır.
	if ok, err := repo.MarkSent(context.Background(), owner); err != nil || !ok {
		t.Fatalf("güncel sahibin sonucu uygulanmalı: %v %v", ok, err)
	}
	if ok, err := repo.CompleteCampaign(context.Background(), q.campaignID); err != nil || !ok {
		t.Fatalf("kampanya tamamlanmalı: %v %v", ok, err)
	}
}

// Çöken bir worker'ın `sending`'de bıraktığı iş, lease dolunca reaper tarafından
// yeniden kuyruğa alınır ve aynı anahtarla gönderilir.
func TestJobAbandonedByCrashedWorkerIsRecoveredAndSent(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	repo := worker.NewPGRepo(db)

	// "Çöken worker": işi sahiplenir ve hiçbir sonuç yazmadan kaybolur.
	dead := mustClaim(t, repo, "sms", 1)
	if len(dead) != 1 {
		t.Fatal("hazırlık: iş sahiplenilmedi")
	}
	ageLease(t, db, dead[0].ID)

	srv, prov := startProvider(t, db, mockprovider.Config{})
	w := startWorker(t, db, httpProvider(srv), fastConfig())

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "sms")
	if m.Status != "sent" || m.Attempts != 2 || m.ClaimToken.String == dead[0].ClaimToken {
		t.Fatalf("iş kurtarılıp yeni token ile gönderilmeli (attempts=2): %+v", m)
	}
	if got := w.Metrics().Snapshot(); got.ReaperRequeued != 1 || got.MessagesSent != 1 {
		t.Fatalf("metrikler: %+v", got)
	}
	if snap := prov.Stats().Snapshot(); snap.TotalRequests != 1 || snap.UniqueAccepts != 1 {
		t.Fatalf("çöken worker provider'a ulaşmamıştı: tek istek, tek kabul: %+v", snap)
	}
}

// Çöken worker'ın işi deneme hakkını tüketmişse yeniden denenmez: failed/exhausted
// olur ve kampanya tamamlanır.
func TestJobAbandonedWithNoAttemptsLeftIsClosedAsExhausted(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	repo := worker.NewPGRepo(db)
	cfg := fastConfig()

	dead := mustClaim(t, repo, "sms", 1)
	if _, err := db.Exec(`UPDATE messages SET attempts = $2 WHERE id = $1`, dead[0].ID, cfg.MaxAttempts); err != nil {
		t.Fatal(err)
	}
	ageLease(t, db, dead[0].ID)

	srv, prov := startProvider(t, db, mockprovider.Config{})
	w := startWorker(t, db, httpProvider(srv), cfg)

	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()

	m := singleMessage(t, db, q, "sms")
	if m.Status != "failed" || m.FailureReason.String != worker.FailureExhausted || m.Attempts != cfg.MaxAttempts {
		t.Fatalf("failed/exhausted bekleniyordu: %+v", m)
	}
	if snap := prov.Stats().Snapshot(); snap.TotalRequests != 0 {
		t.Fatalf("hakkı biten iş için provider çağrılmamalı: %+v", snap)
	}
}
