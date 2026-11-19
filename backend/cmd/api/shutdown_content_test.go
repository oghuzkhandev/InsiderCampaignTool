package main

import (
	"bufio"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

const holdingContentJSON = `{"sms_text":"Merhaba {{first_name}}, %30 indirim!","email_subject":"Sepetin seni bekliyor","email_body":"Merhaba {{first_name}}, ürünlerin burada."}`

// holdingLLM, içerik akışını ortasında tutar: önce bir parça yollar (akış başlar ve
// istemci `delta` görür), sonra hold süresi boyunca bekler, sonra kalanı yollar.
// Bağlam iptal olursa beklemeden döner ve bunu cancelled kanalıyla bildirir.
type holdingLLM struct {
	llm.Provider // segment çağrıları kullanılmaz; yalnızca arayüzü tamamlar

	hold      time.Duration
	started   chan struct{} // ilk parça yollandı
	cancelled chan struct{} // bekleme bağlam iptaliyle kesildi
	once      sync.Once
}

func newHoldingLLM(hold time.Duration) *holdingLLM {
	return &holdingLLM{
		Provider:  llm.NewMock(),
		hold:      hold,
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
}

func (h *holdingLLM) GenerateContent(ctx context.Context, _ llm.ContentRequest, onChunk func(string)) (string, error) {
	cut := len(holdingContentJSON) / 2
	onChunk(holdingContentJSON[:cut])
	h.once.Do(func() { close(h.started) })

	select {
	case <-time.After(h.hold):
	case <-ctx.Done():
		close(h.cancelled)
		return "", ctx.Err()
	}
	onChunk(holdingContentJSON[cut:])
	return holdingContentJSON, nil
}

// contentShutdownEnv, gerçek bağlamayı (newServer + newHandler + serve), gerçek
// veritabanını ve yavaş bir LLM'i kurar. Sunucu timeout kapanış süresiyle çalışır.
type contentShutdownEnv struct {
	addr       string
	campaignID string
	db         *sql.DB
	llm        *holdingLLM
	cancel     context.CancelFunc // SIGTERM
	done       <-chan error
}

func startContentShutdownEnv(t *testing.T, hold, shutdownTimeout time.Duration) *contentShutdownEnv {
	t.Helper()
	db := testdb.Open(t)
	var id string
	if err := db.QueryRow(
		`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email)
		 VALUES ('sepet hatırlatması', '{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}'::jsonb, 3, 2, 2) RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}

	fake := newHoldingLLM(hold)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var handlerErr error
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(ln.Addr().String(), func(shutdown <-chan struct{}) http.Handler {
		h, err := newHandler(db, fake, shutdown, &metrics.Counter{}, logger)
		handlerErr = err
		return h
	})
	if handlerErr != nil {
		t.Fatal(handlerErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, ln, shutdownTimeout) }()
	return &contentShutdownEnv{addr: ln.Addr().String(), campaignID: id, db: db, llm: fake, cancel: cancel, done: done}
}

// openContentStream, içerik üretimini başlatır ve ilk `delta` olayını görene kadar
// bekler: üretim gerçekten akış ortasındadır.
func (e *contentShutdownEnv) openContentStream(t *testing.T) *bufio.Reader {
	t.Helper()
	resp, err := http.Post("http://"+e.addr+"/api/campaigns/"+e.campaignID+"/content", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("içerik akışı açılamadı: %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "event: delta") {
		t.Fatalf("ilk olay delta olmalı: %q %v", line, err)
	}
	select {
	case <-e.llm.started:
	case <-time.After(3 * time.Second):
		t.Fatal("LLM akışı başlamadı")
	}
	return br
}

func (e *contentShutdownEnv) savedVersions(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT count(*) FROM campaign_contents WHERE campaign_id = $1`, e.campaignID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Kapanış penceresi içinde biten bir içerik üretimi SIGTERM'e rağmen tamamlanır:
// istemci `validated` olayını alır, sürüm kaydedilir, sunucu pencereyi (burada 10 sn)
// beklemeden temiz döner ve yeni bağlantı kabul etmez.
func TestSIGTERMLetsAnActiveContentGenerationFinishWithinTheGraceWindow(t *testing.T) {
	env := startContentShutdownEnv(t, 500*time.Millisecond, 10*time.Second)
	stream := env.openContentStream(t)

	begin := time.Now()
	env.cancel() // SIGTERM, üretim akış ortasında

	// Yeni bağlantı kabul edilmez (dinleyici hemen kapanır).
	awaitNoNewConnections(t, env.addr)

	rest, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("akışın kalanı okunamadı: %v", err)
	}
	if !strings.Contains(string(rest), "event: validated") {
		t.Fatalf("pencere içinde biten üretim validated ile tamamlanmalı:\n%s", rest)
	}
	if err := wantReturn(t, env.done, 5*time.Second); err != nil {
		t.Fatalf("temiz kapanış bekleniyordu: %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Fatalf("sunucu üretim bitince kapanmalı, 10 sn pencereyi beklememeli: %s", elapsed)
	}
	if n := env.savedVersions(t); n != 1 {
		t.Fatalf("tamamlanan üretim bir sürüm olarak kaydedilmeli: %d", n)
	}
}

// Kapanış penceresini aşan üretim kesilir: sunucu pencere dolunca bağlantıyı kapatır,
// LLM çağrısının bağlamı iptal olur (arkada çalışmaya devam etmez) ve hiçbir sürüm
// kaydedilmez; istemci `validated` almaz.
func TestSIGTERMCancelsAContentGenerationThatOutlastsTheGraceWindowAndSavesNothing(t *testing.T) {
	env := startContentShutdownEnv(t, 30*time.Second, 300*time.Millisecond)
	stream := env.openContentStream(t)

	begin := time.Now()
	env.cancel() // SIGTERM

	err := wantReturn(t, env.done, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "kapanış tamamlanamadı") {
		t.Fatalf("süre aşımı kapanış hatası olarak bildirilmeli: %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 3*time.Second {
		t.Fatalf("kapanış süresi (300 ms) sonrasında hemen dönmeli: %s", elapsed)
	}

	// Üretim arkada sürmez: LLM bağlamı iptal oldu.
	select {
	case <-env.llm.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("kapanış sonrası LLM çağrısı iptal edilmedi (arkada çalışmaya devam ediyor)")
	}

	// İstemci akışın kesildiğini görür, validated almaz.
	rest, _ := io.ReadAll(stream) // kesik bağlantıda okuma hatası beklenir; içerik önemli
	if strings.Contains(string(rest), "event: validated") {
		t.Fatalf("kesilen üretim validated göndermemeli:\n%s", rest)
	}
	if n := env.savedVersions(t); n != 0 {
		t.Fatalf("kesilen üretim sürüm kaydetmemeli: %d", n)
	}
}

// awaitNoNewConnections, SIGTERM sonrası dinleyicinin kapandığını doğrular.
func awaitNoNewConnections(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return
		}
		_ = c.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("SIGTERM sonrası sunucu hâlâ yeni bağlantı kabul ediyor")
}
