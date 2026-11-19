// Package e2e, gerçek worker ve mock-provider ikililerini alt süreç olarak
// başlatır ve süreç düzeyindeki senaryoları sınar: SIGKILL, SIGTERM, provider
// yeniden başlatma ve veritabanı kesintisi (ADR-015, SND-001). Yalnızca
// campaigns_test veritabanına bağlanır; gerçek API anahtarı veya mesaj
// sağlayıcısı kullanılmaz.
package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// binDir, TestMain'de derlenen ikililerin dizinidir.
var binDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "campaign-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "geçici dizin:", err)
		os.Exit(1)
	}
	binDir = dir
	code := func() int {
		for _, name := range []string{"worker", "mock-provider", "api"} {
			cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, name), "../../cmd/"+name)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "%s derlenemedi: %v\n%s", name, err, out)
				return 1
			}
		}
		return m.Run()
	}()
	_ = os.RemoveAll(binDir)
	os.Exit(code)
}

// syncBuffer, alt sürecin çıktısını güvenle biriktirir.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// proc, bir alt süreçtir.
type proc struct {
	name string
	cmd  *exec.Cmd
	out  *syncBuffer
	done chan struct{}
	exit int
}

// startProc, ikiliyi YALNIZCA verilen ortamla başlatır. Geliştirici makinesindeki
// DATABASE_URL gibi değişkenler alt sürece geçmez; veritabanı adresi çağıranın
// verdiği campaigns_test adresidir.
func startProc(t *testing.T, name string, env map[string]string) *proc {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, name))
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s başlatılamadı: %v", name, err)
	}
	p := &proc{name: name, cmd: cmd, out: out, done: make(chan struct{}), exit: -1}
	go func() {
		_ = cmd.Wait()
		p.exit = cmd.ProcessState.ExitCode()
		close(p.done)
	}()
	t.Cleanup(func() {
		// Durdurulmuş (SIGSTOP) bir süreç SIGKILL'i de işleyemez gibi görünebilir;
		// önce devam ettirilir.
		if p.running() {
			_ = p.cmd.Process.Signal(syscall.SIGCONT)
		}
		p.kill()
		if t.Failed() {
			t.Logf("--- %s çıktısı ---\n%s", name, out.String())
		}
	})
	return p
}

func (p *proc) running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// signal, çalışan sürece bir sinyal yollar (örn. SIGSTOP/SIGCONT ile süreç
// duraklaması taklidi).
func (p *proc) signal(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatalf("%s'a %v gönderilemedi: %v", p.name, sig, err)
	}
}

// kill, süreci SIGKILL ile sonlandırır ve bitmesini bekler (çökme taklidi).
func (p *proc) kill() {
	if p.running() {
		_ = p.cmd.Process.Signal(syscall.SIGKILL)
	}
	<-p.done
}

// terminate, SIGTERM yollar ve çıkış kodunu en geç timeout içinde döndürür.
func (p *proc) terminate(t *testing.T, timeout time.Duration) int {
	t.Helper()
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("%s'a SIGTERM gönderilemedi: %v", p.name, err)
	}
	select {
	case <-p.done:
		return p.exit
	case <-time.After(timeout):
		t.Fatalf("%s SIGTERM'den sonra %s içinde kapanmadı", p.name, timeout)
		return -1
	}
}

// freePort, kullanılabilir bir yerel port döndürür.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

// providerEnv, mock-provider'ın ortamıdır.
func providerEnv(t *testing.T, dbURL, port string, extra map[string]string) map[string]string {
	t.Helper()
	env := map[string]string{"DATABASE_URL": dbURL, "PORT": port}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// workerEnv, test için süreleri kısaltılmış bir worker ortamıdır. Uygulamanın
// varsayılanları değişmez; lease kapanış süresinden uzun kalır.
func workerEnv(dbURL, providerPort string, extra map[string]string) map[string]string {
	env := map[string]string{
		"DATABASE_URL":            dbURL,
		"PROVIDER_URL":            "http://127.0.0.1:" + providerPort,
		"WORKER_CAPACITY":         "5",
		"WORKER_MAX_ATTEMPTS":     "5",
		"WORKER_LEASE":            "3s",
		"WORKER_REAP_INTERVAL":    "200ms",
		"WORKER_POLL_INTERVAL":    "10ms",
		"WORKER_SHUTDOWN_TIMEOUT": "1s",
		"WORKER_RATE":             "2000",
		"WORKER_BACKOFF_BASE":     "10ms",
		"WORKER_BACKOFF_CAP":      "50ms",
		"WORKER_DB_BACKOFF_MIN":   "20ms",
		"WORKER_DB_BACKOFF_MAX":   "200ms",
	}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// startProvider, mock-provider'ı başlatır ve hazır olmasını bekler.
func startProvider(t *testing.T, dbURL, port string, extra map[string]string) *proc {
	t.Helper()
	p := startProc(t, "mock-provider", providerEnv(t, dbURL, port, extra))
	awaitCond(t, "mock-provider hazır", func() bool {
		if !p.running() {
			t.Fatalf("mock-provider erken kapandı:\n%s", p.out.String())
		}
		resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
	return p
}

// providerStats, sağlayıcının bellek istatistiklerini okur.
func providerStats(t *testing.T, port string) mockprovider.Snapshot {
	t.Helper()
	resp, err := http.Get("http://127.0.0.1:" + port + "/admin/stats")
	if err != nil {
		t.Fatalf("istatistikler okunamadı: %v", err)
	}
	defer resp.Body.Close()
	var snap mockprovider.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

// testDB, campaigns_test'i hazırlar (migration, temizlik) ve alt süreçler için
// adresini döndürür.
func testDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	db := testdb.Open(t)
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	url, err := testdb.URL()
	if err != nil {
		t.Fatal(err)
	}
	return db, url
}

func awaitCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	awaitCondFor(t, 20*time.Second, what, cond)
}

func awaitCondFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("zaman aşımı: %s", what)
}

// seedCampaign, onaylı bir kampanya (sürüm 1) ve n kullanıcı için her seçili
// kanalda açık izinli, `queued` mesaj satırları yazar. Yalnızca campaigns_test.
func seedCampaign(t *testing.T, db *sql.DB, n int, channels ...string) string {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, query)
		}
	}
	var id string
	if err := db.QueryRow(
		`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email)
		 VALUES ('e2e', '{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}'::jsonb, $1, $1, $1) RETURNING id`, n,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO campaign_contents (campaign_id, version, source, sms_text, email_subject, email_body)
	      VALUES ($1, 1, 'llm', 'Merhaba {{first_name}}, %30 indirim!', 'Sepetin seni bekliyor', 'Merhaba {{first_name}}, ürünlerin burada.')`, id)
	exec(`UPDATE campaigns SET status = 'approved', approved_content_version = 1, approved_at = now() WHERE id = $1`, id)
	// Kullanıcıların e-postası kampanyaya özgüdür; bir testte birden çok kampanya
	// yazılabilir ve izin ile mesajlar yalnızca bu kampanyanın kullanıcılarına eklenir.
	rows, err := db.Query(
		`INSERT INTO users (email, city, first_name, phone)
		 SELECT 'k' || $2::text || '-' || g || '@example.test', 'İstanbul', 'Ad' || g, '+9055500' || lpad(g::text, 5, '0')
		 FROM generate_series(1, $1) g RETURNING id`, n, id[:8])
	if err != nil {
		t.Fatalf("seed: kullanıcılar eklenemedi: %v", err)
	}
	var userIDs []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, uid)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()

	for _, ch := range channels {
		exec(`INSERT INTO user_consents (user_id, channel, granted) SELECT unnest($2::bigint[]), $1::text, true`, ch, userIDs)
		exec(`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key)
		      SELECT $1::uuid, u.id, $2::text, CASE WHEN $2::text = 'sms' THEN u.phone ELSE u.email END,
		             coalesce(u.first_name, ''), -- onayın yaptığı gibi: adın ham kopyası
		             $3::text || ':' || u.id::text || ':' || $2::text
		      FROM users u WHERE u.id = ANY($4::bigint[])`, id, ch, id, userIDs)
	}
	return id
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

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, query)
	}
	return n
}
