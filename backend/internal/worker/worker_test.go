package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
)

// fakeRepo, Repository'nin bellekteki taklididir. Çağrıları sırayla kaydeder.
type fakeRepo struct {
	mu    sync.Mutex
	calls []string

	claim func(ctx context.Context, channel string, limit int) ([]Job, error)

	pre        Precheck
	preErr     error
	applied    bool // sonuç yazımları bu değeri döndürür (varsayılan true)
	writeErr   error
	reap       ReapResult
	reapErr    error
	completeCh chan string

	sweepCompleted int
	sweepErr       error

	// preHook, her Precheck çağrısında çalışır; engelleyerek yavaş veya takılı bir
	// veritabanını taklit eder.
	preHook func()

	requeueDelay time.Duration
	failReason   string
	failErr      string
	skipReason   string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{pre: Precheck{Owned: true, Consent: true}, applied: true, completeCh: make(chan string, 100)}
}

func (r *fakeRepo) record(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *fakeRepo) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func (r *fakeRepo) count(call string) int {
	n := 0
	for _, c := range r.Calls() {
		if c == call {
			n++
		}
	}
	return n
}

func (r *fakeRepo) Claim(ctx context.Context, channel string, limit, _ int) ([]Job, error) {
	r.record("claim:" + channel)
	if r.claim == nil {
		return nil, nil
	}
	return r.claim(ctx, channel, limit)
}

func (r *fakeRepo) Precheck(context.Context, Job, time.Duration) (Precheck, error) {
	r.record("precheck")
	if r.preHook != nil {
		r.preHook() // veritabanı beklemesi taklidi (engelleyebilir)
	}
	return r.pre, r.preErr
}

func (r *fakeRepo) MarkSent(context.Context, Job) (bool, error) {
	r.record("sent")
	return r.applied, r.writeErr
}

func (r *fakeRepo) MarkFailed(_ context.Context, _ Job, reason, lastError string) (bool, error) {
	r.record("failed")
	r.mu.Lock()
	r.failReason, r.failErr = reason, lastError
	r.mu.Unlock()
	return r.applied, r.writeErr
}

func (r *fakeRepo) MarkSkipped(_ context.Context, _ Job, reason string) (bool, error) {
	r.record("skipped")
	r.mu.Lock()
	r.skipReason = reason
	r.mu.Unlock()
	return r.applied, r.writeErr
}

func (r *fakeRepo) Requeue(_ context.Context, _ Job, delay time.Duration, _ string) (bool, error) {
	r.record("requeue")
	r.mu.Lock()
	r.requeueDelay = delay
	r.mu.Unlock()
	return r.applied, r.writeErr
}

func (r *fakeRepo) CompleteCampaign(_ context.Context, id string) (bool, error) {
	r.record("complete")
	select {
	case r.completeCh <- id:
	default:
	}
	return true, nil
}

func (r *fakeRepo) CompleteIdleCampaigns(context.Context) (int, error) {
	r.record("sweep")
	return r.sweepCompleted, r.sweepErr
}

func (r *fakeRepo) Reap(context.Context, time.Duration, int) (ReapResult, error) {
	r.record("reap")
	return r.reap, r.reapErr
}

// fakeProvider, Provider'ın taklididir.
type fakeProvider struct {
	mu       sync.Mutex
	calls    []Job
	rendered []content.Content
	fn       func(ctx context.Context, job Job) SendResult
	onCall   func(name string)
}

func (p *fakeProvider) Send(ctx context.Context, job Job, c content.Content) SendResult {
	if p.onCall != nil {
		p.onCall("send")
	}
	p.mu.Lock()
	p.calls = append(p.calls, job)
	p.rendered = append(p.rendered, c)
	fn := p.fn
	p.mu.Unlock()
	if fn == nil {
		return SendResult{Kind: KindAccepted}
	}
	return fn(ctx, job)
}

func (p *fakeProvider) Calls() []Job {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

// recordingLimiter, Wait çağrılarını sıraya yazar.
type recordingLimiter struct{ onWait func() }

func (l recordingLimiter) Wait(context.Context) error {
	if l.onWait != nil {
		l.onWait()
	}
	return nil
}

func testConfig() Config {
	return Config{
		Capacity:        2,
		MaxAttempts:     3,
		Lease:           time.Second,
		ReapInterval:    5 * time.Millisecond,
		PollInterval:    time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
		Rate:            1000,
		SendBurst:       2,
		DBBackoffMin:    time.Millisecond,
		DBBackoffMax:    5 * time.Millisecond,
		BackoffBase:     time.Millisecond,
		BackoffCap:      4 * time.Millisecond,
	}
}

func newTestWorker(t *testing.T, cfg Config, repo Repository, prov Provider, opts ...Option) *Worker {
	t.Helper()
	opts = append([]Option{
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		WithBackoff(Backoff{Base: time.Second, Cap: time.Minute, Rand: func() float64 { return 0.5 }}),
	}, opts...)
	w, err := New(cfg, repo, prov, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func testJob() Job {
	return Job{
		ID: 1, CampaignID: "c-1", UserID: 10, Channel: ChannelSMS, Recipient: "+905550000001",
		ClaimToken: "tok-1", Attempts: 1, IdempotencyKey: "c-1:10:sms", FirstName: "Ada",
		Content: content.Content{SMSText: "Merhaba {{first_name}}", EmailSubject: "Konu {{first_name}}", EmailBody: "Gövde"},
	}
}

func TestProcessSendsThenWritesSentAndChecksCompletion(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 1 {
		t.Fatalf("tek provider çağrısı bekleniyordu: %d", len(prov.Calls()))
	}
	if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "sent", "complete"}) {
		t.Fatalf("çağrı sırası: %v", got)
	}
	snap := w.Metrics().Snapshot()
	if snap.MessagesSent != 1 || snap.Attempts[KindAccepted] != 1 || snap.StaleResults != 0 {
		t.Fatalf("metrikler: %+v", snap)
	}
}

// Sıra (ADR-005): limiter beklemesi → izin/sahiplik kontrolü → provider çağrısı.
func TestProcessChecksConsentAfterLimiterWaitAndBeforeSending(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	add := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }

	repo := newFakeRepo()
	repo.pre = Precheck{Owned: true, Consent: true}
	prov := &fakeProvider{onCall: add}
	w := newTestWorker(t, testConfig(), &orderRepo{fakeRepo: repo, add: add}, prov,
		WithLimiter(ChannelSMS, recordingLimiter{onWait: func() { add("wait") }}))

	w.process(context.Background(), testJob())

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(order, []string{"wait", "precheck", "send"}) {
		t.Fatalf("sıra limiter → kontrol → gönderim olmalı: %v", order)
	}
}

// orderRepo, Precheck çağrısını ortak sıra kaydına da yazar.
type orderRepo struct {
	*fakeRepo
	add func(string)
}

func (r *orderRepo) Precheck(ctx context.Context, j Job, l time.Duration) (Precheck, error) {
	r.add("precheck")
	return r.fakeRepo.Precheck(ctx, j, l)
}

// İzin yoksa (iptal edilmiş veya kayıt hiç yok) mesaj `skipped` olur ve
// provider'a hiç istek gitmez.
func TestProcessSkipsWithoutConsentAndNeverCallsProvider(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	repo.pre = Precheck{Owned: true, Consent: false}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 0 {
		t.Fatalf("izinsiz mesaj için provider çağrılmamalı: %d çağrı", len(prov.Calls()))
	}
	if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "skipped", "complete"}) {
		t.Fatalf("çağrı sırası: %v", got)
	}
	if snap := w.Metrics().Snapshot(); snap.MessagesSent != 0 || snap.Attempts[KindAccepted] != 0 {
		t.Fatalf("atlanan mesaj gönderim sayılmamalı: %+v", snap)
	}
}

// Sahiplik veya lease geçersizse (örn. limiter beklerken lease doldu) çağrı
// yapılmaz ve hiçbir sonuç yazılmaz.
func TestProcessDoesNothingWhenOwnershipOrLeaseIsLost(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	repo.pre = Precheck{Owned: false, Consent: true}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 0 {
		t.Fatal("sahipliği kaybeden iş için provider çağrılmamalı")
	}
	if got := repo.Calls(); !slices.Equal(got, []string{"precheck"}) {
		t.Fatalf("hiçbir sonuç yazılmamalı: %v", got)
	}
}

// Kapı yalnızca beklenecekse ön kontrol yenilenir; beklemeyen normal gönderimde
// ön kontrol tek seferdir.
func TestGateWithoutWaitDoesNotRepeatThePrecheck(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	gate := &fakeGate{}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))

	w.process(context.Background(), testJob())

	if repo.count("precheck") != 1 || gate.calls != 1 || len(prov.Calls()) != 1 {
		t.Fatalf("1 ön kontrol, 1 kapı, 1 gönderim bekleniyordu: precheck=%d kapı=%d gönderim=%d",
			repo.count("precheck"), gate.calls, len(prov.Calls()))
	}
}

// Kapı beklettiğinde bekleme sırasında kanal izni iptal olmuş olabilir: ön kontrol
// yenilenir ve izin yoksa provider hiç çağrılmaz.
func TestGateWaitRefreshesPrecheckSoConsentRevokedWhileWaitingSkipsTheMessage(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	var calls int
	repo.preHook = func() {
		calls++
		if calls == 2 { // bekleme sırasında izin geri çekildi
			repo.pre = Precheck{Owned: true, Consent: false}
		}
	}
	gate := &fakeGate{waits: []time.Duration{2 * time.Millisecond}}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 0 {
		t.Fatal("bekleme sırasında izin iptal edildi: provider çağrılmamalı")
	}
	if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "precheck", "skipped", "complete"}) {
		t.Fatalf("çağrılar: %v", got)
	}
}

// Kapı beklerken lease dolmuş veya iş başka claim'e geçmiş olabilir: yenilenen ön
// kontrol bunu yakalar, çağrı yapılmaz ve sonuç yazılmaz.
func TestGateWaitRefreshesPrecheckSoLostOwnershipWhileWaitingSendsNothing(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	var calls int
	repo.preHook = func() {
		calls++
		if calls == 2 {
			repo.pre = Precheck{Owned: false, Consent: true}
		}
	}
	gate := &fakeGate{waits: []time.Duration{2 * time.Millisecond}}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 0 {
		t.Fatal("sahiplik kaybedildi: provider çağrılmamalı")
	}
	if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "precheck"}) {
		t.Fatalf("hiçbir sonuç yazılmamalı: %v", got)
	}
}

// Kapıyı birden çok kez bekleyen iş, kapı izin verene kadar döner ve sonunda tam bir
// kez gönderir.
func TestGateWaitsAsLongAsNeededThenSendsExactlyOnce(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	gate := &fakeGate{waits: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 1 || repo.count("precheck") != 4 || gate.calls != 4 {
		t.Fatalf("4 deneme, 1 gönderim bekleniyordu: gönderim=%d precheck=%d kapı=%d",
			len(prov.Calls()), repo.count("precheck"), gate.calls)
	}
}

// Kapı beklerken kapanış gelirse iş gönderilmez ve sonuç yazılmaz (lease kurtarır).
func TestGateWaitIsInterruptedByShutdownWithoutSending(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	gate := &fakeGate{waits: []time.Duration{time.Hour}}
	w := newTestWorker(t, testConfig(), repo, prov, WithGate(ChannelSMS, gate))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.process(ctx, testJob()) }()
	waitFor(t, "kapı beklemede", func() bool { return gate.count() >= 1 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("iptal edilen bekleme hemen dönmeli")
	}
	if len(prov.Calls()) != 0 || repo.count("sent")+repo.count("failed")+repo.count("requeue") != 0 {
		t.Fatalf("iptalden sonra gönderim veya sonuç olmamalı: %v", repo.Calls())
	}
}

func TestProcessSendsRenderedContentForTheChannel(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov)

	job := testJob()
	w.process(context.Background(), job)
	job.FirstName = "  " // adı olmayan alıcı: fallback
	job.ID = 2
	w.process(context.Background(), job)

	prov.mu.Lock()
	defer prov.mu.Unlock()
	if prov.rendered[0].SMSText != "Merhaba Ada" {
		t.Fatalf("ad yerleştirilmeli: %q", prov.rendered[0].SMSText)
	}
	if prov.rendered[1].SMSText != "Merhaba "+content.FallbackName {
		t.Fatalf("ad yoksa fallback kullanılmalı: %q", prov.rendered[1].SMSText)
	}
}

func TestProcessRetriesTransientFailuresWithBackoffOrRetryAfter(t *testing.T) {
	tests := []struct {
		name      string
		res       SendResult
		wantDelay time.Duration
	}{
		{"429 Retry-After büyük", SendResult{Kind: KindRateLimited, RetryAfter: 30 * time.Second, Err: "HTTP 429"}, 30 * time.Second},
		// Backoff: Rand 0.5, deneme 1 → tavan 1 sn → 500 ms.
		{"500", SendResult{Kind: KindServerError, Err: "HTTP 500"}, 500 * time.Millisecond},
		{"yanıt kaybı", SendResult{Kind: KindNetworkError, Err: "bağlantı hatası"}, 500 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			prov := &fakeProvider{fn: func(context.Context, Job) SendResult { return tc.res }}
			w := newTestWorker(t, testConfig(), repo, prov)

			w.process(context.Background(), testJob())

			if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "requeue"}) {
				t.Fatalf("çağrılar: %v (retry kampanyayı tamamlamamalı)", got)
			}
			if repo.requeueDelay != tc.wantDelay {
				t.Fatalf("bekleme = %s, beklenen %s", repo.requeueDelay, tc.wantDelay)
			}
			if got := w.Metrics().Snapshot(); got.MessagesSent != 0 || got.Attempts[tc.res.Kind] != 1 {
				t.Fatalf("metrikler: %+v", got)
			}
		})
	}
}

func TestProcessFailsPermanentRejectionWithoutRetry(t *testing.T) {
	repo := newFakeRepo()
	prov := &fakeProvider{fn: func(context.Context, Job) SendResult {
		return SendResult{Kind: KindRejected, Err: "HTTP 400 INVALID_RECIPIENT"}
	}}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob()) // ilk deneme

	if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "failed", "complete"}) {
		t.Fatalf("çağrılar: %v", got)
	}
	if repo.failReason != FailureRejected || repo.failErr != "HTTP 400 INVALID_RECIPIENT" {
		t.Fatalf("sebep: %q %q", repo.failReason, repo.failErr)
	}
}

func TestProcessExhaustsWhenAttemptLimitReached(t *testing.T) {
	repo := newFakeRepo()
	prov := &fakeProvider{fn: func(context.Context, Job) SendResult {
		return SendResult{Kind: KindNetworkError, Err: "bağlantı hatası"}
	}}
	w := newTestWorker(t, testConfig(), repo, prov)

	job := testJob()
	job.Attempts = testConfig().MaxAttempts
	w.process(context.Background(), job)

	if got := repo.Calls(); !slices.Equal(got, []string{"precheck", "failed", "complete"}) {
		t.Fatalf("son denemede geçici hata kuyruğa dönmemeli: %v", got)
	}
	if repo.failReason != FailureExhausted {
		t.Fatalf("failure_reason = %q", repo.failReason)
	}
}

// Eski claim'in sonucu (0 satır) yok sayılır, sayılır ve kampanya tamamlanmaz.
func TestProcessIgnoresAndCountsStaleResults(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  SendResult
	}{
		{"kabul", SendResult{Kind: KindAccepted}},
		{"kalıcı hata", SendResult{Kind: KindRejected, Err: "HTTP 400"}},
		{"geçici hata", SendResult{Kind: KindServerError, Err: "HTTP 500"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.applied = false
			prov := &fakeProvider{fn: func(context.Context, Job) SendResult { return tc.res }}
			w := newTestWorker(t, testConfig(), repo, prov)

			w.process(context.Background(), testJob())

			if repo.count("complete") != 0 {
				t.Fatal("uygulanmayan sonuç kampanya tamamlama kontrolü tetiklememeli")
			}
			snap := w.Metrics().Snapshot()
			if snap.StaleResults != 1 || snap.MessagesSent != 0 {
				t.Fatalf("1 eski sonuç sayılmalı, gönderim sayılmamalı: %+v", snap)
			}
		})
	}
}

func TestProcessLeavesJobToLeaseWhenResultCannotBeWritten(t *testing.T) {
	repo := newFakeRepo()
	repo.writeErr = errors.New("veritabanı yok")
	w := newTestWorker(t, testConfig(), repo, &fakeProvider{})

	w.process(context.Background(), testJob())

	if repo.count("complete") != 0 {
		t.Fatal("yazılamayan sonuçtan sonra tamamlama kontrolü yapılmamalı")
	}
	if snap := w.Metrics().Snapshot(); snap.MessagesSent != 0 || snap.StaleResults != 0 {
		t.Fatalf("yazılamayan sonuç gönderim veya eski sonuç sayılmamalı: %+v", snap)
	}
}

func TestProcessAbandonsJobWhenPrecheckFails(t *testing.T) {
	repo, prov := newFakeRepo(), &fakeProvider{}
	repo.preErr = errors.New("veritabanı yok")
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(context.Background(), testJob())

	if len(prov.Calls()) != 0 || len(repo.Calls()) != 1 {
		t.Fatalf("ön kontrol okunamazsa gönderilmemeli: çağrı=%d %v", len(prov.Calls()), repo.Calls())
	}
}

// Bağlam iptal edildiyse (kapanış süresi doldu) çağrının sonucu bilinmez:
// hiçbir sonuç yazılmaz, iş lease ile kurtarılır.
func TestProcessWritesNothingWhenContextCancelledDuringSend(t *testing.T) {
	repo := newFakeRepo()
	ctx, cancel := context.WithCancel(context.Background())
	prov := &fakeProvider{fn: func(context.Context, Job) SendResult {
		cancel()
		return SendResult{Kind: KindAccepted} // yanıt gelmiş olsa bile bağlam iptal
	}}
	w := newTestWorker(t, testConfig(), repo, prov)

	w.process(ctx, testJob())

	if got := repo.Calls(); !slices.Equal(got, []string{"precheck"}) {
		t.Fatalf("iptalden sonra sonuç yazılmamalı: %v", got)
	}
	if snap := w.Metrics().Snapshot(); snap.MessagesSent != 0 || snap.Attempts[KindAccepted] != 0 {
		t.Fatalf("metrikler: %+v", snap)
	}
}

// poolClaim, kanal için havuzdaki işleri en çok limit kadar verir ve her
// çağrının limitini kaydeder.
type poolClaim struct {
	mu     sync.Mutex
	pool   []Job
	limits []int
}

func (p *poolClaim) fn(_ context.Context, channel string, limit int) ([]Job, error) {
	if channel != ChannelSMS {
		return nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limits = append(p.limits, limit)
	n := min(limit, len(p.pool))
	out := slices.Clone(p.pool[:n])
	p.pool = p.pool[n:]
	return out, nil
}

func (p *poolClaim) remaining() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pool)
}

func smsJobs(n int) []Job {
	jobs := make([]Job, n)
	for i := range jobs {
		jobs[i] = testJob()
		jobs[i].ID = int64(i + 1)
	}
	return jobs
}

func runAsync(t *testing.T, w *Worker) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("Run kapanmadı")
			return nil
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("zaman aşımı: %s", what)
}

// Brief: kanal başına saniyede en fazla 100 istek. Hız, limiter çıkışında değil
// GERÇEK GÖNDERİM ANINDA da bağlı olmalıdır: işler limiter'dan geçip ön kontrolde
// (veritabanı) takılırsa, veritabanı döndüğünde hepsi birlikte göndermemeli. Aksi
// hâlde kayan 1 sn pencerede "sürekli hız + birikmiş iş sayısı" kadar istek çıkar.
//
// Ölçek küçültülmüştür (hız 40/s, kapasite 20) ama oran gerçektir: düzeltme yokken
// pencere ≈ 40 + 20 = 60; kapıyla ≤ 40 + burst.
func TestSendRateStaysBoundedWhenJobsPileUpBehindAStalledPrecheck(t *testing.T) {
	cfg := testConfig()
	cfg.Capacity, cfg.Rate = 20, 40

	pool := &poolClaim{pool: smsJobs(5000)}
	repo := newFakeRepo()
	repo.claim = pool.fn
	release := make(chan struct{})
	var stalled atomic.Int32
	repo.preHook = func() {
		if stalled.Add(1) <= int32(cfg.Capacity) {
			<-release // ilk 20 iş limiter'dan geçti, "veritabanı" takılı
		}
	}

	var mu sync.Mutex
	var times []time.Time
	prov := &fakeProvider{onCall: func(string) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
	}}
	w := newTestWorker(t, cfg, repo, prov)
	stop := runAsync(t, w)

	waitFor(t, "20 iş ön kontrolde takıldı", func() bool { return stalled.Load() >= int32(cfg.Capacity) })
	time.Sleep(600 * time.Millisecond) // beklerken limiter slotları geçmişte kaldı
	close(release)                     // veritabanı döndü: bekleyen işler birlikte serbest
	time.Sleep(1800 * time.Millisecond)
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	peak := mockprovider.MaxInWindow(times, time.Second)
	t.Logf("%d gönderim; kayan 1 sn pencerede en yüksek: %d (hız %v/s)", len(times), peak, cfg.Rate)
	// Üst sınır: hız + izin verilen küçük burst (2) + ölçüm payı (1).
	if limit := int(cfg.Rate) + 3; peak > limit {
		t.Fatalf("kayan 1 sn pencerede %d gönderim; hız %v/s ve küçük burst ile en çok %d olmalı", peak, cfg.Rate, limit)
	}
}

// Worker yalnızca boş kapasitesi kadar iş sahiplenir; sahiplenilmiş ve işlenen
// iş sayısı Capacity'yi aşmaz.
func TestRunClaimsOnlyFreeCapacity(t *testing.T) {
	cfg := testConfig() // kapasite 2
	pool := &poolClaim{pool: smsJobs(6)}
	repo := newFakeRepo()
	repo.claim = pool.fn

	release := make(chan struct{})
	var inflight, maxInflight atomic.Int32
	prov := &fakeProvider{fn: func(ctx context.Context, _ Job) SendResult {
		n := inflight.Add(1)
		for {
			m := maxInflight.Load()
			if n <= m || maxInflight.CompareAndSwap(m, n) {
				break
			}
		}
		defer inflight.Add(-1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return SendResult{Kind: KindAccepted}
	}}
	w := newTestWorker(t, cfg, repo, prov)
	stop := runAsync(t, w)

	waitFor(t, "iki iş eşzamanlı gönderiliyor", func() bool { return inflight.Load() == 2 })
	time.Sleep(50 * time.Millisecond) // kapasite doluyken yeni claim olmamalı
	if got := pool.remaining(); got != 4 {
		t.Fatalf("kapasite doluyken ek iş sahiplenilmemeli: havuzda %d iş kaldı, beklenen 4", got)
	}

	close(release)
	waitFor(t, "6 iş gönderildi", func() bool { return w.Metrics().Snapshot().MessagesSent == 6 })
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	if maxInflight.Load() > int32(cfg.Capacity) {
		t.Fatalf("eşzamanlı gönderim kapasiteyi aştı: %d > %d", maxInflight.Load(), cfg.Capacity)
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, l := range pool.limits {
		if l < 1 || l > cfg.Capacity {
			t.Fatalf("claim limiti 1..%d aralığında olmalı: %v", cfg.Capacity, pool.limits)
		}
	}
}

// Veritabanına erişilemediğinde worker yeni iş sahiplenmez, süreç sonlanmaz ve
// erişim dönünce kuyruğu kendiliğinden işler (SND-001).
func TestRunSurvivesDatabaseOutageAndResumes(t *testing.T) {
	pool := &poolClaim{pool: smsJobs(3)}
	var down atomic.Bool
	down.Store(true)
	var failedClaims atomic.Int32

	repo := newFakeRepo()
	repo.claim = func(ctx context.Context, ch string, limit int) ([]Job, error) {
		if down.Load() {
			failedClaims.Add(1)
			return nil, errors.New("veritabanına erişilemiyor")
		}
		return pool.fn(ctx, ch, limit)
	}
	prov := &fakeProvider{}
	w := newTestWorker(t, testConfig(), repo, prov)
	stop := runAsync(t, w)

	waitFor(t, "claim backoff ile tekrar tekrar deneniyor", func() bool { return failedClaims.Load() >= 6 })
	if len(prov.Calls()) != 0 || pool.remaining() != 3 {
		t.Fatalf("erişim yokken iş sahiplenilmemeli: çağrı=%d havuz=%d", len(prov.Calls()), pool.remaining())
	}

	down.Store(false) // erişim döndü
	waitFor(t, "kuyruk kendiliğinden işlendi", func() bool { return w.Metrics().Snapshot().MessagesSent == 3 })
	if err := stop(); err != nil {
		t.Fatalf("Run hatasız dönmeli: %v", err)
	}
}

// SIGTERM: yeni claim durur, devam eden çağrı tamamlanır ve sonucu yazılır.
func TestRunStopsClaimingOnShutdownButLetsInFlightFinish(t *testing.T) {
	pool := &poolClaim{pool: smsJobs(1)}
	repo := newFakeRepo()
	repo.claim = pool.fn

	started, release := make(chan struct{}), make(chan struct{})
	prov := &fakeProvider{fn: func(context.Context, Job) SendResult {
		close(started)
		<-release
		return SendResult{Kind: KindAccepted}
	}}
	cfg := testConfig()
	cfg.ShutdownTimeout = 2 * time.Second
	cfg.Lease = 5 * time.Second
	w := newTestWorker(t, cfg, repo, prov)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	<-started
	cancel() // SIGTERM
	time.Sleep(30 * time.Millisecond)
	claimsAtShutdown := repo.count("claim:sms")
	time.Sleep(50 * time.Millisecond)
	if got := repo.count("claim:sms"); got != claimsAtShutdown {
		t.Fatalf("kapanıştan sonra yeni claim yapılmamalı: %d → %d", claimsAtShutdown, got)
	}

	select {
	case <-done:
		t.Fatal("Run, devam eden çağrı bitmeden dönmemeli")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("çağrı bitince Run dönmeli")
	}
	if repo.count("sent") != 1 || w.Metrics().Snapshot().MessagesSent != 1 {
		t.Fatalf("devam eden çağrının sonucu yazılmalı: %v", repo.Calls())
	}
}

// Kapanış süresi dolunca tamamlanmayan çağrı iptal edilir ve sonuç YAZILMAZ: iş
// `sending`'de kalır, lease dolunca kurtarılır.
func TestRunCancelsHungCallAfterShutdownTimeoutAndWritesNoResult(t *testing.T) {
	pool := &poolClaim{pool: smsJobs(1)}
	repo := newFakeRepo()
	repo.claim = pool.fn

	started := make(chan struct{})
	var cancelled atomic.Bool
	prov := &fakeProvider{fn: func(ctx context.Context, _ Job) SendResult {
		close(started)
		<-ctx.Done() // yanıt hiç gelmiyor
		cancelled.Store(true)
		return SendResult{Kind: KindNetworkError, Err: "iptal edildi"}
	}}
	cfg := testConfig() // kapanış süresi 200 ms
	w := newTestWorker(t, cfg, repo, prov)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	<-started
	begin := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("kapanış süresi dolunca Run dönmeli")
	}
	if elapsed := time.Since(begin); elapsed < cfg.ShutdownTimeout-20*time.Millisecond {
		t.Fatalf("Run kapanış süresinden önce döndü: %s < %s", elapsed, cfg.ShutdownTimeout)
	}
	if !cancelled.Load() {
		t.Fatal("takılan çağrının bağlamı iptal edilmeli")
	}
	if repo.count("sent")+repo.count("failed")+repo.count("requeue") != 0 {
		t.Fatalf("iptal edilen çağrı sonuç yazmamalı: %v", repo.Calls())
	}
}

func TestReapRequeuesAndChecksCampaignCompletion(t *testing.T) {
	repo := newFakeRepo()
	repo.reap = ReapResult{Requeued: 2, Exhausted: 1, Campaigns: []string{"c-1", "c-2"}}
	w := newTestWorker(t, testConfig(), repo, &fakeProvider{})

	w.Reap(context.Background())

	if got := w.Metrics().Snapshot().ReaperRequeued; got != 2 {
		t.Fatalf("yeniden kuyruğa alınan 2 mesaj sayılmalı: %d", got)
	}
	var completed []string
	for len(completed) < 2 {
		select {
		case id := <-repo.completeCh:
			completed = append(completed, id)
		case <-time.After(time.Second):
			t.Fatalf("her etkilenen kampanya için tamamlama kontrolü yapılmalı: %v", completed)
		}
	}
	slices.Sort(completed)
	if !slices.Equal(completed, []string{"c-1", "c-2"}) {
		t.Fatalf("tamamlama kontrolleri: %v", completed)
	}
}

// Her reaper turu, kendi etkilemediği (sonuç yazıldıktan sonra tamamlama kontrolü
// başarısız olmuş) takılı kampanyaları da tamamlamayı dener.
func TestReapSweepsStuckCampaignsEveryRound(t *testing.T) {
	repo := newFakeRepo()
	repo.sweepCompleted = 1
	w := newTestWorker(t, testConfig(), repo, &fakeProvider{})

	w.Reap(context.Background())

	if got := repo.Calls(); !slices.Equal(got, []string{"reap", "sweep"}) {
		t.Fatalf("reaper turu lease taramasından sonra takılı kampanyaları da taramalı: %v", got)
	}
}

// Takılı kampanya taraması ile lease taraması birbirinden bağımsızdır: biri hata
// verse de diğeri çalışır ve hiçbiri süreci durdurmaz.
func TestReapStepsAreIndependentWhenOneFails(t *testing.T) {
	t.Run("lease taraması başarısız, takılı kampanya taraması yine çalışır", func(t *testing.T) {
		repo := newFakeRepo()
		repo.reapErr = errors.New("veritabanı yok")
		w := newTestWorker(t, testConfig(), repo, &fakeProvider{})
		w.Reap(context.Background())
		if got := repo.Calls(); !slices.Equal(got, []string{"reap", "sweep"}) {
			t.Fatalf("çağrılar: %v", got)
		}
	})
	t.Run("takılı kampanya taraması başarısız, lease taraması etkilenmez", func(t *testing.T) {
		repo := newFakeRepo()
		repo.sweepErr = errors.New("veritabanı yok")
		repo.reap = ReapResult{Requeued: 1, Campaigns: []string{"c-1"}}
		w := newTestWorker(t, testConfig(), repo, &fakeProvider{})
		w.Reap(context.Background())
		if got := w.Metrics().Snapshot().ReaperRequeued; got != 1 {
			t.Fatalf("lease taraması sonucu sayılmalı: %d", got)
		}
		if repo.count("complete") != 1 {
			t.Fatalf("etkilenen kampanya yine kontrol edilmeli: %v", repo.Calls())
		}
	})
}

func TestRunSweepsStuckCampaignsPeriodically(t *testing.T) {
	repo := newFakeRepo()
	w := newTestWorker(t, testConfig(), repo, &fakeProvider{})
	stop := runAsync(t, w)

	waitFor(t, "takılı kampanya taraması birden çok tur çalıştı", func() bool { return repo.count("sweep") >= 3 })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestReapErrorIsNotFatal(t *testing.T) {
	repo := newFakeRepo()
	repo.reapErr = errors.New("veritabanı yok")
	w := newTestWorker(t, testConfig(), repo, &fakeProvider{})

	w.Reap(context.Background()) // panik veya takılma olmamalı

	if repo.count("complete") != 0 || w.Metrics().Snapshot().ReaperRequeued != 0 {
		t.Fatal("hata sonrası hiçbir şey sayılmamalı veya tamamlanmamalı")
	}
}

func TestRunRunsReaperPeriodically(t *testing.T) {
	repo := newFakeRepo()
	w := newTestWorker(t, testConfig(), repo, &fakeProvider{})
	stop := runAsync(t, w)

	waitFor(t, "reaper birden çok tur çalıştı", func() bool { return repo.count("reap") >= 3 })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigFromEnvOverridesOnlyWhatIsSet(t *testing.T) {
	got, err := ConfigFromEnv()
	if err != nil || got != DefaultConfig() {
		t.Fatalf("hiçbir değişken yokken varsayılanlar: %+v %v", got, err)
	}

	t.Setenv(EnvCapacity, "3")
	t.Setenv(EnvMaxAttempts, "4")
	t.Setenv(EnvLease, "2s")
	t.Setenv(EnvReapInterval, "100ms")
	t.Setenv(EnvPollInterval, "10ms")
	t.Setenv(EnvShutdownTimeout, "500ms")
	t.Setenv(EnvRate, "50")
	t.Setenv(EnvBackoffBase, "1ms")
	t.Setenv(EnvBackoffCap, "8ms")
	t.Setenv(EnvDBBackoffMin, "20ms")
	t.Setenv(EnvDBBackoffMax, "200ms")
	t.Setenv(EnvSendBurst, "3")
	got, err = ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.SendBurst = 3
	want.DBBackoffMin, want.DBBackoffMax = 20*time.Millisecond, 200*time.Millisecond
	want.Capacity, want.MaxAttempts, want.Rate = 3, 4, 50
	want.Lease, want.ReapInterval, want.PollInterval, want.ShutdownTimeout = 2*time.Second, 100*time.Millisecond, 10*time.Millisecond, 500*time.Millisecond
	want.BackoffBase, want.BackoffCap = time.Millisecond, 8*time.Millisecond
	if got != want {
		t.Fatalf("ConfigFromEnv = %+v, beklenen %+v", got, want)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	for name, kv := range map[string][2]string{
		"geçersiz süre":         {EnvLease, "uzun"},
		"geçersiz sayı":         {EnvCapacity, "on"},
		"geçersiz oran":         {EnvRate, "çok"},
		"kapasite 0":            {EnvCapacity, "0"},
		"burst 0":               {EnvSendBurst, "0"},
		"lease kapanıştan kısa": {EnvLease, "1s"}, // kapanış 15 sn
		"retry tavanı < taban":  {EnvBackoffCap, "1ms"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(kv[0], kv[1])
			if _, err := ConfigFromEnv(); err == nil {
				t.Fatalf("%s=%s reddedilmeli", kv[0], kv[1])
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("varsayılan yapılandırma geçerli olmalı: %v", err)
	}
	d := DefaultConfig()
	if d.Capacity != 10 || d.MaxAttempts != 5 || d.Lease != 60*time.Second || d.ReapInterval != 10*time.Second ||
		d.ShutdownTimeout != 15*time.Second || d.Rate != 95 || d.PollInterval != 200*time.Millisecond ||
		d.BackoffBase != 2*time.Second || d.BackoffCap != 60*time.Second || d.SendBurst != 2 {
		t.Fatalf("varsayılanlar ARCHITECTURE §8 ve ADR-005 ile uyuşmalı: %+v", d)
	}

	for name, mutate := range map[string]func(*Config){
		"kapasite 0":              func(c *Config) { c.Capacity = 0 },
		"deneme 0":                func(c *Config) { c.MaxAttempts = 0 },
		"lease 0":                 func(c *Config) { c.Lease = 0 },
		"hız 0":                   func(c *Config) { c.Rate = 0 },
		"gönderim burst 0":        func(c *Config) { c.SendBurst = 0 },
		"lease kapanıştan kısa":   func(c *Config) { c.Lease = c.ShutdownTimeout },
		"backoff aralığı ters":    func(c *Config) { c.DBBackoffMax = c.DBBackoffMin / 2 },
		"yoklama aralığı negatif": func(c *Config) { c.PollInterval = -1 },
		"retry tabanı 0":          func(c *Config) { c.BackoffBase = 0 },
		"retry tavanı < taban":    func(c *Config) { c.BackoffCap = c.BackoffBase / 2 },
	} {
		c := DefaultConfig()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: geçersiz yapılandırma reddedilmeli", name)
		}
	}
}
