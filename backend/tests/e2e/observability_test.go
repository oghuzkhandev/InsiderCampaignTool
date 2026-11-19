package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// startAPI, gerçek API ikilisini başlatır: ana port (apiPort) ve ayrı iç metrics
// dinleyicisi (metricsPort). Yalnızca verilen veritabanı adresine bağlanır.
func startAPI(t *testing.T, dbURL, apiPort, metricsPort string) *proc {
	t.Helper()
	p := startProc(t, "api", map[string]string{
		"DATABASE_URL": dbURL,
		"PORT":         apiPort,
		"LLM_PROVIDER": "mock",
		"METRICS_ADDR": "127.0.0.1:" + metricsPort,
	})
	awaitCond(t, "api hazır", func() bool {
		if !p.running() {
			t.Fatalf("api erken kapandı:\n%s", p.out.String())
		}
		code, _ := httpGet("http://127.0.0.1:" + apiPort + "/healthz")
		return code == http.StatusOK
	})
	return p
}

// httpGet, kısa süre sınırıyla bir GET yapar; bağlantı hatasında kod 0 döner.
func httpGet(u string) (int, string) {
	c := http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(u)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// scrape, bir /metrics ucunu okur; 200 değilse testi durdurur.
func scrape(t *testing.T, port string) string {
	t.Helper()
	code, body := httpGet("http://127.0.0.1:" + port + "/metrics")
	if code != http.StatusOK {
		t.Fatalf("/metrics %s: %d\n%s", port, code, body)
	}
	return body
}

// metricValue, `seri değer` satırının değerini döndürür; seri yoksa testi durdurur.
func metricValue(t *testing.T, body, series string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if rest, ok := strings.CutPrefix(line, series+" "); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				t.Fatalf("%s değeri okunamadı: %q", series, line)
			}
			return v
		}
	}
	t.Fatalf("seri bulunamadı: %s\n%s", series, body)
	return 0
}

func readyz(apiPort string) int {
	code, _ := httpGet("http://127.0.0.1:" + apiPort + "/readyz")
	return code
}

// Gerçek API süreci: veritabanı erişimi kesilince /readyz 503 olur, /healthz 200
// kalır ve süreç ayakta kalır; erişim dönünce /readyz yeniden 200 olur. Takılan
// (yanıtsız) veritabanında da probe asılı kalmaz, süre sınırında 503 olur.
func TestAPIReadyzFollowsDatabaseOutageWhileHealthzStaysUp(t *testing.T) {
	_, dbURL := testDB(t)
	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newProxy(t, u.Host)
	u.Host = proxy.addr
	proxy.up(t) // API başlarken veritabanına ping atar: proxy önce açık olmalı

	apiPort := freePort(t)
	api := startAPI(t, u.String(), apiPort, freePort(t))
	healthz := func() int { code, _ := httpGet("http://127.0.0.1:" + apiPort + "/healthz"); return code }

	if got := readyz(apiPort); got != http.StatusOK {
		t.Fatalf("veritabanı ayakta: /readyz 200 bekleniyordu: %d", got)
	}

	// 1) Bağlantı kesildi (erişilemeyen veritabanı).
	proxy.cut()
	awaitCond(t, "/readyz 503", func() bool { return readyz(apiPort) == http.StatusServiceUnavailable })
	if got := healthz(); got != http.StatusOK {
		t.Fatalf("kesintide /healthz 200 kalmalı: %d", got)
	}
	if !api.running() {
		t.Fatalf("api veritabanı kesintisinde sonlanmamalı:\n%s", api.out.String())
	}

	// 2) Erişim döndü.
	proxy.up(t)
	awaitCond(t, "/readyz yeniden 200", func() bool { return readyz(apiPort) == http.StatusOK })

	// 3) Veritabanı yanıtsız (bağlantı açık ama takılı): probe süre sınırında 503 olur.
	proxy.freeze()
	begin := time.Now()
	awaitCond(t, "takılı veritabanında /readyz 503", func() bool { return readyz(apiPort) == http.StatusServiceUnavailable })
	if elapsed := time.Since(begin); elapsed > 8*time.Second {
		t.Fatalf("takılı veritabanı probe'u asmamalı: %s", elapsed)
	}
	if got := healthz(); got != http.StatusOK {
		t.Fatalf("takılı veritabanında /healthz 200 kalmalı: %d", got)
	}
	proxy.thaw()
	awaitCond(t, "çözülünce /readyz 200", func() bool { return readyz(apiPort) == http.StatusOK })

	if !api.running() {
		t.Fatalf("api sonlanmamalı:\n%s", api.out.String())
	}
	if out := api.out.String(); !strings.Contains(out, "veritabanına erişilemiyor") || !strings.Contains(out, "yeniden 200") {
		t.Fatalf("api durum değişimlerini loglamalı:\n%s", out)
	}
}

// queue_depth veritabanından hesaplanır: worker HİÇ çalışmıyorken de kuyruk birikimi
// görünür; worker başlayınca birikim kendiliğinden erir. API ana portu /metrics
// sunmaz (yalnızca iç dinleyici sunar).
func TestAPIQueueDepthShowsBacklogWhileNoWorkerRunsAndDrainsWhenItStarts(t *testing.T) {
	db, dbURL := testDB(t)
	sms := seedCampaign(t, db, 5, "sms")
	email := seedCampaign(t, db, 2, "email")

	apiPort, metricsPort := freePort(t), freePort(t)
	startAPI(t, dbURL, apiPort, metricsPort)

	body := scrape(t, metricsPort)
	if got := metricValue(t, body, `queue_depth{channel="sms"}`); got != 5 {
		t.Fatalf("worker yokken sms birikimi 5 olmalı: %v\n%s", got, body)
	}
	if got := metricValue(t, body, `queue_depth{channel="email"}`); got != 2 {
		t.Fatalf("worker yokken email birikimi 2 olmalı: %v", got)
	}

	// Ana port /metrics sunmaz: metrikler yalnızca iç dinleyicidedir (ADR-016).
	if code, _ := httpGet("http://127.0.0.1:" + apiPort + "/metrics"); code != http.StatusNotFound {
		t.Fatalf("API ana portu /metrics sunmamalı: %d", code)
	}
	// İç dinleyici başka hiçbir şey sunmaz.
	if code, _ := httpGet("http://127.0.0.1:" + metricsPort + "/api/campaigns/" + sms); code != http.StatusNotFound {
		t.Fatalf("iç dinleyici yalnızca /metrics sunmalı: %d", code)
	}

	// Worker başlar: birikim erir.
	providerPort := freePort(t)
	startProvider(t, dbURL, providerPort, nil)
	startProc(t, "worker", workerEnv(dbURL, providerPort, nil))
	awaitCond(t, "iki kampanya completed", func() bool {
		return campaignStatus(t, db, sms) == "completed" && campaignStatus(t, db, email) == "completed"
	})
	body = scrape(t, metricsPort)
	if metricValue(t, body, `queue_depth{channel="sms"}`) != 0 || metricValue(t, body, `queue_depth{channel="email"}`) != 0 {
		t.Fatalf("işlenince kuyruk derinliği 0 olmalı:\n%s", body)
	}
}

// Bilinen sayıda gönderimden sonra worker sayaçları beklenen değerlere ulaşır ve
// yalnızca worker'ın /metrics ucundadır; API'nin çıktısında bulunmaz (ADR-016).
func TestWorkerCountersReachExpectedValuesAndAreAbsentFromTheAPI(t *testing.T) {
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, 5, "sms", "email")
	providerPort, workerMetrics, apiPort, apiMetrics := freePort(t), freePort(t), freePort(t), freePort(t)
	startProvider(t, dbURL, providerPort, map[string]string{"MOCK_PROVIDER_MODE": "accept_then_drop"})
	env := workerEnv(dbURL, providerPort, nil)
	env["METRICS_ADDR"] = "127.0.0.1:" + workerMetrics
	startProc(t, "worker", env)
	startAPI(t, dbURL, apiPort, apiMetrics)

	awaitCond(t, "kampanya completed", func() bool { return campaignStatus(t, db, campaignID) == "completed" })

	// 10 mesaj, her biri 2 denemede: 1 düşen yanıt (ağ hatası) + 1 kabul.
	body := scrape(t, workerMetrics)
	for series, want := range map[string]float64{
		"messages_sent_total":                             10,
		`send_attempts_total{outcome="accepted"}`:         10,
		`send_attempts_total{outcome="network_error"}`:    10,
		`send_attempts_total{outcome="rate_limited"}`:     0,
		`send_attempts_total{outcome="server_error"}`:     0,
		`send_attempts_total{outcome="rejected"}`:         0,
		`send_attempts_total{outcome="invalid_response"}`: 0,
		"stale_result_total":                              0,
		"reaper_requeued_total":                           0,
		"ratelimit_wait_seconds_count":                    20, // her deneme bir gözlem
		`ratelimit_wait_seconds_bucket{le="+Inf"}`:        20,
	} {
		if got := metricValue(t, body, series); got != want {
			t.Errorf("%s = %v, beklenen %v", series, got, want)
		}
	}
	if sum := metricValue(t, body, "ratelimit_wait_seconds_sum"); sum < 0 {
		t.Errorf("ratelimit_wait_seconds_sum negatif olamaz: %v", sum)
	}

	// API çıktısında worker sayaçları yok; kendi metrikleri var.
	apiBody := scrape(t, apiMetrics)
	for _, name := range []string{"messages_sent_total", "send_attempts_total", "stale_result_total", "reaper_requeued_total", "ratelimit_wait_seconds"} {
		if strings.Contains(apiBody, name) {
			t.Errorf("API çıktısında worker metriği %q olmamalı", name)
		}
	}
	if !strings.Contains(apiBody, "llm_validation_failures_total 0\n") || !strings.Contains(apiBody, "queue_depth{") {
		t.Errorf("API kendi metriklerini sunmalı:\n%s", apiBody)
	}
}
