package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
)

// Channels, worker'ın işlediği kanallardır. Her kanalın ayrı bir limiter'ı ve
// claim döngüsü vardır.
var Channels = []string{ChannelSMS, ChannelEmail}

// Config, worker'ın zamanlama ve kapasite ayarlarıdır (ARCHITECTURE §8,
// ADR-005, ADR-006).
type Config struct {
	// Capacity, kanal başına aynı anda sahiplenilmiş (limiter bekleyen veya
	// gönderilen) iş sayısı üst sınırıdır. Limiter beklemesi lease'e dahil
	// olduğundan küçük tutulur.
	Capacity int
	// MaxAttempts, azami işleme denemesidir (claim sayısı).
	MaxAttempts int
	// Lease, claim geçerlilik süresidir. Provider zaman aşımından ve kapanış
	// bütçesinden uzun olmalıdır.
	Lease time.Duration
	// ReapInterval, reaper turlarının aralığıdır.
	ReapInterval time.Duration
	// PollInterval, kuyruk boşken claim denemeleri arasındaki beklemedir.
	PollInterval time.Duration
	// ShutdownTimeout, SIGTERM sonrası devam eden çağrılara tanınan süredir.
	ShutdownTimeout time.Duration
	// Rate, kanal başına saniyedeki hedef provider isteği sayısıdır (retry'lar
	// dahil). Brief sınırı 100'dür; hedef 95.
	Rate float64
	// SendBurst, gönderim anı kapısının (TokenGate) kova kapasitesidir: duraklama
	// veya veritabanı beklemesinden sonra aynı anda geçebilecek en çok iş sayısı.
	// Herhangi bir kayan 1 sn pencerede en çok Rate + SendBurst gönderim çıkar
	// (varsayılan 95 + 2 = 97; brief sınırı 100'ün altında, ağ ve zamanlayıcı
	// kaymaları için pay bırakır).
	SendBurst int
	// DBBackoffMin ve DBBackoffMax, veritabanına erişilemediğinde claim
	// denemeleri arasındaki üstel beklemenin sınırlarıdır.
	DBBackoffMin, DBBackoffMax time.Duration
	// BackoffBase ve BackoffCap, geçici provider hatalarından sonraki üstel (tam
	// jitter'lı) retry beklemesinin tabanı ve tavanıdır.
	BackoffBase, BackoffCap time.Duration
}

// DefaultConfig, ARCHITECTURE §8'deki varsayılan değerlerdir.
func DefaultConfig() Config {
	return Config{
		Capacity:        10,
		MaxAttempts:     5,
		Lease:           60 * time.Second,
		ReapInterval:    10 * time.Second,
		PollInterval:    200 * time.Millisecond,
		ShutdownTimeout: 15 * time.Second,
		Rate:            95,
		SendBurst:       2,
		DBBackoffMin:    500 * time.Millisecond,
		DBBackoffMax:    30 * time.Second,
		BackoffBase:     2 * time.Second,
		BackoffCap:      60 * time.Second,
	}
}

// Validate, ayarların tutarlı olduğunu doğrular.
func (c Config) Validate() error {
	switch {
	case c.Capacity < 1:
		return errors.New("worker: kapasite en az 1 olmalı")
	case c.MaxAttempts < 1:
		return errors.New("worker: azami deneme en az 1 olmalı")
	case c.Lease <= 0 || c.ReapInterval <= 0 || c.PollInterval <= 0 || c.ShutdownTimeout <= 0:
		return errors.New("worker: süre ayarları pozitif olmalı")
	case c.Rate <= 0:
		return errors.New("worker: hız sınırı pozitif olmalı")
	case c.SendBurst < 1:
		return errors.New("worker: gönderim burst değeri en az 1 olmalı")
	case c.DBBackoffMin <= 0 || c.DBBackoffMax < c.DBBackoffMin:
		return errors.New("worker: veritabanı backoff sınırları geçersiz")
	case c.BackoffBase <= 0 || c.BackoffCap < c.BackoffBase:
		return errors.New("worker: retry backoff sınırları geçersiz")
	case c.Lease <= c.ShutdownTimeout:
		return fmt.Errorf("worker: lease (%s) kapanış süresinden (%s) uzun olmalı", c.Lease, c.ShutdownTimeout)
	}
	return nil
}

// Worker, kuyruktaki mesajları provider'a gönderir.
type Worker struct {
	cfg      Config
	repo     Repository
	provider Provider
	limiters map[string]Limiter
	gates    map[string]Gate
	backoff  Backoff
	metrics  *Metrics
	log      *slog.Logger
	sleep    func(ctx context.Context, d time.Duration) error
}

// Option, Worker'ı özelleştirir.
type Option func(*Worker)

// WithLimiter, bir kanalın limiter'ını değiştirir.
func WithLimiter(channel string, l Limiter) Option {
	return func(w *Worker) { w.limiters[channel] = l }
}

// WithGate, bir kanalın gönderim anı hız kapısını değiştirir.
func WithGate(channel string, g Gate) Option {
	return func(w *Worker) { w.gates[channel] = g }
}

// WithBackoff, retry beklemesini değiştirir.
func WithBackoff(b Backoff) Option { return func(w *Worker) { w.backoff = b } }

// WithLogger, günlükçüyü değiştirir.
func WithLogger(l *slog.Logger) Option { return func(w *Worker) { w.log = l } }

// WithMetrics, sayaçları dışarıdan verilen nesneye yazdırır.
func WithMetrics(m *Metrics) Option { return func(w *Worker) { w.metrics = m } }

// New, bir Worker oluşturur. Her kanal için cfg.Rate ile bir IntervalLimiter
// kurulur; WithLimiter ile değiştirilebilir.
func New(cfg Config, repo Repository, provider Provider, opts ...Option) (*Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	w := &Worker{
		cfg:      cfg,
		repo:     repo,
		provider: provider,
		limiters: map[string]Limiter{},
		gates:    map[string]Gate{},
		backoff:  Backoff{Base: cfg.BackoffBase, Cap: cfg.BackoffCap, Rand: rand.Float64},
		metrics:  NewMetrics(),
		log:      slog.Default(),
		sleep:    sleepContext,
	}
	for _, ch := range Channels {
		w.limiters[ch] = NewIntervalLimiter(cfg.Rate)
		w.gates[ch] = NewTokenGate(cfg.Rate, cfg.SendBurst)
	}
	for _, o := range opts {
		o(w)
	}
	return w, nil
}

// Metrics, worker sayaçlarını döndürür.
func (w *Worker) Metrics() *Metrics { return w.metrics }

// Run, ctx iptal edilene kadar kuyruğu işler.
//
// ctx iptal edilince (SIGTERM) yeni iş sahiplenmeyi bırakır; devam eden
// provider çağrılarına ve sonuç yazımlarına ShutdownTimeout kadar süre tanır. Süre
// dolarsa çağrılar iptal edilir ve tamamlanmayan işler `sending`'de kalır: lease
// dolunca reaper yeniden kuyruğa alır veya hakkı bittiyse kapatır. Veritabanına
// erişilemese bile Run sonlanmaz; erişim dönünce kuyruğu işlemeye kendiliğinden
// devam eder.
func (w *Worker) Run(ctx context.Context) error {
	// İş bağlamı ctx'ten bağımsızdır: ctx iptali yalnızca yeni claim'leri durdurur.
	workCtx, cancelWork := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelWork()

	var wg sync.WaitGroup
	for _, ch := range Channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.claimLoop(ctx, workCtx, ch, &wg)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.reapLoop(ctx)
	}()

	<-ctx.Done()
	w.log.Info("worker kapanıyor: yeni iş sahiplenilmiyor", "bekleme", w.cfg.ShutdownTimeout)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	timer := time.NewTimer(w.cfg.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		w.log.Warn("kapanış süresi doldu: devam eden çağrılar iptal ediliyor; işler lease ile kurtarılacak")
		cancelWork()
		<-done
	}
	return nil
}

// claimLoop, bir kanalın işlerini sahiplenir ve her birini ayrı bir goroutine'de
// işler. Aynı anda sahiplenilmiş iş sayısı cfg.Capacity ile sınırlıdır.
func (w *Worker) claimLoop(ctx, workCtx context.Context, channel string, wg *sync.WaitGroup) {
	slots := make(chan struct{}, w.cfg.Capacity)
	dbBackoff := w.cfg.DBBackoffMin
	failing := false

	for ctx.Err() == nil {
		free := cap(slots) - len(slots)
		if free == 0 {
			_ = w.sleep(ctx, w.cfg.PollInterval)
			continue
		}
		jobs, err := w.repo.Claim(ctx, channel, free, w.cfg.MaxAttempts)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Veritabanı yok: yeni iş sahiplenilmez, backoff ile beklenir ve süreç
			// sonlanmaz (SND-001).
			if !failing {
				w.log.Error("kuyruk okunamadı; backoff ile tekrar denenecek", "kanal", channel, "error", err)
				failing = true
			}
			_ = w.sleep(ctx, dbBackoff)
			dbBackoff = min(dbBackoff*2, w.cfg.DBBackoffMax)
			continue
		}
		if failing {
			w.log.Info("kuyruk erişimi geri geldi", "kanal", channel)
			failing = false
		}
		dbBackoff = w.cfg.DBBackoffMin

		if len(jobs) == 0 {
			_ = w.sleep(ctx, w.cfg.PollInterval)
			continue
		}
		for _, job := range jobs {
			slots <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				w.process(workCtx, job)
			}()
		}
	}
}

// reapLoop, her ReapInterval'de lease'i dolan işleri ele alır.
func (w *Worker) reapLoop(ctx context.Context) {
	for ctx.Err() == nil {
		if err := w.sleep(ctx, w.cfg.ReapInterval); err != nil {
			return
		}
		w.Reap(ctx)
	}
}

// Reap, bir reaper turu çalıştırır: lease'i dolan işleri ele alır, sonra
// mesajları zaten terminal olduğu halde takılı kalmış kampanyaları tamamlar. İki
// adım birbirinden bağımsızdır; biri hata verirse diğeri yine çalışır. Hatalar
// loglanır ve sonraki tur denenir.
func (w *Worker) Reap(ctx context.Context) {
	w.reapExpired(ctx)
	w.completeIdleCampaigns(ctx)
}

// completeIdleCampaigns, sonuç yazıldıktan sonra tamamlama kontrolü başarısız olan
// (örneğin geçici veritabanı hatası) ve bu yüzden `sending`'de kalan kampanyaları
// tamamlar. Başka mesaj kalmadığı için o kampanyayı kimse tekrar kontrol etmezdi.
func (w *Worker) completeIdleCampaigns(ctx context.Context) {
	n, err := w.repo.CompleteIdleCampaigns(ctx)
	if err != nil {
		if ctx.Err() == nil {
			w.log.Error("takılı kampanyalar tamamlanamadı", "error", err)
		}
		return
	}
	if n > 0 {
		w.log.Warn("takılı kalmış kampanyalar tamamlandı", "adet", n)
	}
}

// reapExpired, lease'i dolan işleri yeniden kuyruğa alır veya kapatır.
func (w *Worker) reapExpired(ctx context.Context) {
	res, err := w.repo.Reap(ctx, w.cfg.Lease, w.cfg.MaxAttempts)
	if err != nil {
		if ctx.Err() == nil {
			w.log.Error("reaper çalışmadı", "error", err)
		}
		return
	}
	if res.Requeued > 0 || res.Exhausted > 0 {
		w.metrics.ReaperRequeued(res.Requeued)
		w.log.Warn("lease'i dolan işler ele alındı", "yeniden_kuyruğa", res.Requeued, "deneme_sınırı", res.Exhausted)
	}
	for _, id := range res.Campaigns {
		if _, err := w.repo.CompleteCampaign(ctx, id); err != nil {
			w.log.Error("kampanya tamamlanamadı", "kampanya", id, "error", err)
		}
	}
}

// process, sahiplenilmiş tek bir işi gönderir. Sıra (ADR-005): limiter beklemesi →
// sahiplik ve kanal izni kontrolü → render → provider çağrısı → token'lı sonuç
// yazımı. Sonucu bilinmeyen veya yazılamayan iş `sending`'de bırakılır; lease
// dolunca kurtarılır ve aynı idempotency anahtarıyla güvenle yeniden denenir.
func (w *Worker) process(ctx context.Context, job Job) {
	log := w.log.With("mesaj", job.ID, "kanal", job.Channel, "deneme", job.Attempts)

	limiterStart := time.Now()
	if err := w.limiters[job.Channel].Wait(ctx); err != nil {
		return // kapanış: lease kurtarır
	}
	waited := time.Since(limiterStart)

	// Limiter beklemesinden SONRA, provider çağrısına en yakın noktada: sahiplik,
	// lease ve kanal izni; ardından gönderim anı hız kapısı.
	ok, gated := w.clearToSend(ctx, log, job)
	// Hız sınırı yüzünden toplam bekleme: limiter + kapı. Kapanışta veya izin
	// yokluğunda da gözlenir; bekleme gerçekten yaşandı.
	w.metrics.RateLimitWait(waited + gated)
	if !ok {
		return
	}

	rendered := render(job)
	res := w.provider.Send(ctx, job, rendered)
	if ctx.Err() != nil {
		// Kapanış süresi doldu: çağrının sonucu bilinmiyor. Provider isteği
		// kabul etmiş olabilir; iş lease ile kurtarılır ve aynı anahtarla yeniden
		// denenir.
		return
	}
	w.metrics.Attempt(res.Kind)

	d := Decide(res, job.Attempts, w.cfg.MaxAttempts, w.backoff.Delay)
	switch d.Action {
	case ActionSent:
		if w.finish(ctx, log, job, func() (bool, error) { return w.repo.MarkSent(ctx, job) }) {
			w.metrics.MessageSent()
		}
	case ActionFail:
		log.Warn(failureMessage(d.FailureReason), "sebep", d.FailureReason, "tür", string(res.Kind), "hata", d.LastError)
		w.finish(ctx, log, job, func() (bool, error) { return w.repo.MarkFailed(ctx, job, d.FailureReason, d.LastError) })
	case ActionRequeue:
		level, msg := retryMessage(res.Kind)
		log.Log(ctx, level, msg, "tür", string(res.Kind), "bekleme", d.Delay, "hata", d.LastError)
		w.write(log, func() (bool, error) { return w.repo.Requeue(ctx, job, d.Delay, d.LastError) })
	}
}

// retryMessage, yeniden denenecek bir sonucun gerçek türünü anlatan log mesajını
// ve düzeyini verir. Tek bir "geçici hata" etiketi dört farklı durumu karıştırırdı:
// 429 ve 5xx provider'ın açık yanıtıdır; kayıp ve geçersiz yanıtta ise sonuç
// BELİRSİZDİR (provider isteği kabul etmiş olabilir), bu yüzden uyarı düzeyindedir.
func retryMessage(kind Kind) (slog.Level, string) {
	switch kind {
	case KindRateLimited:
		return slog.LevelInfo, "provider hız sınırı uyguladı (429); yeniden denenecek"
	case KindServerError:
		return slog.LevelInfo, "provider sunucu hatası verdi; yeniden denenecek"
	case KindNetworkError:
		return slog.LevelWarn, "provider'dan yanıt alınamadı; sonuç belirsiz (istek kabul edilmiş olabilir), aynı idempotency anahtarıyla yeniden denenecek"
	case KindInvalidResponse:
		return slog.LevelWarn, "provider yanıtı geçersiz (2xx ama message_id yok veya okunamadı); kabul doğrulanamadı, aynı idempotency anahtarıyla yeniden denenecek"
	default:
		return slog.LevelInfo, "geçici hata; yeniden denenecek"
	}
}

// failureMessage, mesajın neden başarısız olduğunu anlatır (ADR-006): kalıcı red ile
// deneme sınırının dolması aynı şey değildir.
func failureMessage(reason string) string {
	switch reason {
	case FailureRejected:
		return "provider isteği kalıcı olarak reddetti (4xx); yeniden denenmeyecek"
	case FailureExhausted:
		return "deneme sınırı doldu; mesajın gönderilip gönderilmediği doğrulanamadı (sonuç doğrulanamadı)"
	default:
		return "mesaj başarısız"
	}
}

// clearToSend, işin ŞİMDİ gönderilip gönderilemeyeceğini belirler. true: gönder.
//
// Sıra: ön kontrol (sahiplik, lease, kanal izni; tek sorgu) → gönderim anı hız
// kapısı. Limiter işleri ön kontrolden ÖNCE aralıklar, ama ön kontrolde veritabanı
// beklemesi veya süreç duraklaması olursa işler birikip birlikte serbest kalabilir;
// kapı hızı gerçek gönderim anında zorlar (ADR-005). Kapı bekletirse bekleme
// sırasında izin iptal olmuş, lease dolmuş veya iş başka claim'e geçmiş olabilir:
// ön kontrol yenilenir. Kapı en son adımdır, kendisiyle gönderim arasında hiçbir
// engelleyici işlem yoktur.
//
// İkinci dönüş değeri, kapıda beklenen toplam süredir (ratelimit_wait_seconds).
func (w *Worker) clearToSend(ctx context.Context, log *slog.Logger, job Job) (ok bool, gated time.Duration) {
	for {
		pre, err := w.repo.Precheck(ctx, job, w.cfg.Lease)
		if err != nil {
			if ctx.Err() == nil {
				log.Error("gönderim öncesi kontrol okunamadı; iş lease ile kurtarılacak", "error", err)
			}
			return false, gated
		}
		if !pre.Owned {
			// Lease doldu veya iş başka bir claim'e geçti: bu worker çağrı yapmaz.
			log.Warn("sahiplik veya lease geçersiz; provider çağrılmadı")
			return false, gated
		}
		if !pre.Consent {
			w.finish(ctx, log, job, func() (bool, error) { return w.repo.MarkSkipped(ctx, job, "kanal izni yok") })
			return false, gated
		}

		wait := w.gates[job.Channel].Take()
		if wait <= 0 {
			return true, gated
		}
		if err := w.sleep(ctx, wait); err != nil {
			return false, gated // kapanış: iş lease ile kurtarılır
		}
		gated += wait
	}
}

// finish, terminal bir sonucu yazar ve uygulandıysa kampanyanın tamamlanıp
// tamamlanmadığını kontrol eder. Sonuç uygulandıysa true döner.
func (w *Worker) finish(ctx context.Context, log *slog.Logger, job Job, write func() (bool, error)) bool {
	if !w.write(log, write) {
		return false
	}
	if _, err := w.repo.CompleteCampaign(ctx, job.CampaignID); err != nil && ctx.Err() == nil {
		log.Error("kampanya tamamlama kontrolü başarısız", "kampanya", job.CampaignID, "error", err)
	}
	return true
}

// write, token'lı bir sonuç yazımını çalıştırır. Eski claim sonucu (0 satır)
// yok sayılır ve sayılır; veritabanı hatası loglanır (iş lease ile kurtarılır).
func (w *Worker) write(log *slog.Logger, write func() (bool, error)) bool {
	applied, err := write()
	switch {
	case err != nil:
		log.Error("sonuç yazılamadı; iş lease ile kurtarılacak", "error", err)
		return false
	case !applied:
		w.metrics.StaleResult()
		log.Warn("eski claim sonucu yok sayıldı")
		return false
	}
	return true
}

// render, işin kanalının metnini alıcının adıyla render eder.
func render(job Job) content.Content {
	return content.Render(job.Content, job.FirstName)
}
