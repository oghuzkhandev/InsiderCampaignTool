package e2e

import (
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
)

// rateRun, gerçek worker ve provider süreçleriyle users kullanıcılık iki kanallı
// bir kampanyayı gönderir ve provider'ın aldığı her isteğin zaman damgalarını
// döndürür. faults true ise provider 429 ve 500 üretir; böylece retry'lar da
// ölçüme girer.
func rateRun(t *testing.T, users int, workerRate string, faults bool) (mockprovider.Snapshot, map[string]int) {
	t.Helper()
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, users, "sms", "email")
	port := freePort(t)

	providerExtra := map[string]string{}
	if faults {
		providerExtra["MOCK_PROVIDER_429_RATE"] = "0.10"
		providerExtra["MOCK_PROVIDER_500_RATE"] = "0.05"
		providerExtra["MOCK_PROVIDER_RETRY_AFTER"] = "1s"
	}
	startProvider(t, dbURL, port, providerExtra)
	startProc(t, "worker", workerEnv(dbURL, port, map[string]string{
		"WORKER_RATE":         workerRate,
		"WORKER_CAPACITY":     "10", // varsayılan kapasite
		"WORKER_MAX_ATTEMPTS": "12",
	}))

	awaitCondFor(t, 90*time.Second, "kampanya completed", func() bool {
		return campaignStatus(t, db, campaignID) == "completed"
	})
	return providerStats(t, port), statusCounts(t, db, campaignID)
}

// Brief: kanal başına saniyede en fazla 100 mesaj. Provider'a ulaşan bütün
// isteklerin (retry'lar dahil) zaman damgalarıyla, her kanal için kayan herhangi
// bir 1 saniyelik pencerede ≤ 100. Test limite yaklaşacak kadar iş kullanır.
func TestRateLimitNeverExceeds100PerSlidingSecondPerChannelIncludingRetries(t *testing.T) {
	const users = 300 // kanal başına 300 mesaj; 95/s ile en az ~3 sn
	snap, counts := rateRun(t, users, "95", true)

	if counts["sent"] != 2*users || len(counts) != 1 {
		t.Fatalf("%d mesajın hepsi sent olmalı: %v", 2*users, counts)
	}
	if snap.RateLimited == 0 || snap.ServerErrors == 0 {
		t.Fatalf("test retry'ları da ölçmeli; hata enjekte edilmeliydi: 429=%d 500=%d", snap.RateLimited, snap.ServerErrors)
	}
	if snap.DuplicateDeliveries != 0 {
		t.Fatalf("tekrar teslimat olmamalı: %+v", snap)
	}

	for _, ch := range []string{"sms", "email"} {
		times := snap.Attempts[ch]
		if len(times) <= users {
			t.Fatalf("%s: retry'lar dahil istek sayısı mesaj sayısından büyük olmalı: %d <= %d", ch, len(times), users)
		}
		peak := mockprovider.MaxInWindow(times, time.Second)
		span := latest(times).Sub(earliest(times))
		t.Logf("%s: %d istek (retry dahil), %s sürede, kayan 1 sn penceredeki en yüksek: %d", ch, len(times), span.Round(time.Millisecond), peak)

		if peak > 100 {
			t.Fatalf("%s: kayan 1 sn pencerede %d istek; sınır 100", ch, peak)
		}
		// Limite yaklaşıldığını göster: test gerçekten baskı altında koşmuş olmalı.
		if peak < 80 {
			t.Fatalf("%s: en yüksek pencere %d; test limite yaklaşacak kadar iş kullanmalı (>= 80)", ch, peak)
		}
	}
}

// Kontrol koşusu: limiter pratikte devre dışıyken (çok yüksek hız) aynı ölçüm
// sınırın aşıldığını GÖSTERMELİ. Böylece yukarıdaki testin yeşil olması, ölçümün
// ihlali göremediği için değil, limiter sınırı tuttuğu için olur.
func TestRateLimitMeasurementDetectsViolationsWhenLimiterIsEffectivelyOff(t *testing.T) {
	const users = 300
	snap, counts := rateRun(t, users, "100000", false)

	if counts["sent"] != 2*users {
		t.Fatalf("tüm mesajlar sent olmalı: %v", counts)
	}
	exceeded := false
	var slowest time.Duration
	for _, ch := range []string{"sms", "email"} {
		ts := snap.Attempts[ch]
		if peak := mockprovider.MaxInWindow(ts, time.Second); peak > 100 {
			exceeded = true
			t.Logf("%s: limitsiz koşuda kayan 1 sn penceredeki en yüksek: %d", ch, peak)
		}
		slowest = max(slowest, latest(ts).Sub(earliest(ts)))
	}
	if exceeded {
		return
	}
	// Limitsiz koşu bile saniyede 100'ü aşamadıysa iki neden olabilir: ölçüm ihlali
	// göremiyor (gerçek hata) ya da ortam o kadar yavaş ki (örn. CPU açlığı) sistem
	// zaten bu hıza ulaşamıyor ve kontrol anlamsız. İkincisi koşunun uzun sürmesinden
	// anlaşılır; o durumda atlanır, hızlı bir koşuda ise sert başarısız olur.
	if slowest > 2*time.Second {
		t.Skipf("ortam çok yavaş (limitsiz koşu %s sürdü); kontrol koşusu anlamsız, atlandı", slowest.Round(time.Millisecond))
	}
	t.Fatalf("hızlı limitsiz koşuda (%s) en az bir kanal 100'ü aşmalıydı; ölçüm ihlali yakalayamıyor", slowest.Round(time.Millisecond))
}

// pauseRun, 300 SMS'lik bir kampanyayı gönderirken kesinti uygulatır (pause
// çağrılır, kesinti bitince döner) ve provider'ın aldığı isteklerin zaman
// damgalarını döndürür. Worker varsayılan hızla (95/s) ve varsayılan kapasiteyle
// (10) çalışır.
func pauseRun(t *testing.T, viaProxy bool, pause func(t *testing.T, w *proc, proxy *tcpProxy)) mockprovider.Snapshot {
	t.Helper()
	const users = 300
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, users, "sms")
	port := freePort(t)
	startProvider(t, dbURL, port, nil)

	workerDB := dbURL
	var proxy *tcpProxy
	if viaProxy {
		u, err := url.Parse(dbURL)
		if err != nil {
			t.Fatal(err)
		}
		proxy = newProxy(t, u.Host) // dbURL her zaman campaigns_test adresidir
		proxy.up(t)
		u.Host = proxy.addr
		workerDB = u.String()
	}
	w := startProc(t, "worker", workerEnv(workerDB, port, map[string]string{
		"WORKER_RATE":         "95",
		"WORKER_CAPACITY":     "10",
		"WORKER_MAX_ATTEMPTS": "12",
		"WORKER_LEASE":        "10s", // kesinti boyunca lease dolmasın
	}))

	// Akış başladı: kesinti sürekli gönderim sırasında uygulanır.
	awaitCond(t, "gönderim akıyor", func() bool { return providerStats(t, port).TotalRequests >= 100 })
	pause(t, w, proxy)

	awaitCondFor(t, 90*time.Second, "kampanya completed", func() bool {
		return campaignStatus(t, db, campaignID) == "completed"
	})
	if got := statusCounts(t, db, campaignID); got["sent"] != users || len(got) != 1 {
		t.Fatalf("%d mesajın hepsi sent olmalı: %v", users, got)
	}
	return providerStats(t, port)
}

// Süreç duraklayıp (GC, VM dondurma, SIGSTOP) devam ettiğinde uyuyan işlerin
// zamanlayıcıları birlikte uyanır. Bekleyen işlerin birlikte gönderilmesi kayan 1 sn
// pencerede 100'ü aşmamalıdır.
func TestRateLimitHoldsAfterTheWorkerProcessIsPausedAndResumed(t *testing.T) {
	snap := pauseRun(t, false, func(t *testing.T, w *proc, _ *tcpProxy) {
		w.signal(t, syscall.SIGSTOP)
		time.Sleep(1500 * time.Millisecond)
		w.signal(t, syscall.SIGCONT)
	})
	assertWindowAtMost(t, snap, "sms", 100)
}

// Veritabanı beklemesi bitince: limiter'dan geçip ön kontrolde (veritabanı sorgusu)
// takılan işler, veritabanı dönünce birlikte göndermemeli.
func TestRateLimitHoldsAfterADatabaseStallEnds(t *testing.T) {
	snap := pauseRun(t, true, func(t *testing.T, _ *proc, proxy *tcpProxy) {
		proxy.freeze() // bağlantı açık ama sorgular yanıtsız
		time.Sleep(1200 * time.Millisecond)
		proxy.thaw() // bekleyen sorgular birlikte tamamlanır
	})
	assertWindowAtMost(t, snap, "sms", 100)
}

func assertWindowAtMost(t *testing.T, snap mockprovider.Snapshot, channel string, limit int) {
	t.Helper()
	times := snap.Attempts[channel]
	peak := mockprovider.MaxInWindow(times, time.Second)
	t.Logf("%s: %d istek; kayan 1 sn penceredeki en yüksek: %d (sınır %d)", channel, len(times), peak, limit)
	if peak > limit {
		t.Fatalf("%s: kayan 1 sn pencerede %d istek; sınır %d", channel, peak, limit)
	}
}

func earliest(ts []time.Time) time.Time {
	m := ts[0]
	for _, x := range ts {
		if x.Before(m) {
			m = x
		}
	}
	return m
}

func latest(ts []time.Time) time.Time {
	m := ts[0]
	for _, x := range ts {
		if x.After(m) {
			m = x
		}
	}
	return m
}
