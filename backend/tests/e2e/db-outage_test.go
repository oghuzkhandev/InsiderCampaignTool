package e2e

import (
	"database/sql"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// tcpProxy, worker ile PostgreSQL arasındaki bağlantıyı kesip geri getirmeye
// yarayan küçük bir TCP aktarıcısıdır. Gerçek veritabanı servisini durdurmadan
// "erişilemeyen veritabanı"nı taklit eder; geliştirme veritabanına dokunulmaz.
type tcpProxy struct {
	target string
	addr   string

	mu     sync.Mutex
	ln     net.Listener
	conns  map[net.Conn]struct{}
	thawed chan struct{} // nil: dondurulmamış; dolu: kapanana kadar veri aktarılmaz
}

// freeze, bağlantıları KESMEDEN veri aktarımını durdurur: sorgular yanıtsız
// bekler (yavaş/takılı veritabanı). thaw, bekleyenleri birlikte serbest bırakır.
func (p *tcpProxy) freeze() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.thawed == nil {
		p.thawed = make(chan struct{})
	}
}

func (p *tcpProxy) thaw() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.thawed != nil {
		close(p.thawed)
		p.thawed = nil
	}
}

// pipe, src'den dst'ye veri aktarır; proxy donmuşsa yazmadan önce bekler.
func (p *tcpProxy) pipe(dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			p.mu.Lock()
			wait := p.thawed
			p.mu.Unlock()
			if wait != nil {
				<-wait
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	_ = dst.Close()
	_ = src.Close()
}

func newProxy(t *testing.T, target string) *tcpProxy {
	t.Helper()
	p := &tcpProxy{target: target, addr: "127.0.0.1:" + freePort(t), conns: map[net.Conn]struct{}{}}
	t.Cleanup(p.cut)
	return p
}

// up, bağlantıları kabul etmeye başlar.
func (p *tcpProxy) up(t *testing.T) {
	t.Helper()
	var ln net.Listener
	var err error
	for i := 0; i < 50; i++ { // kesintiden hemen sonra port boşalana kadar dene
		if ln, err = net.Listen("tcp", p.addr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("proxy dinlemeye başlayamadı: %v", err)
	}
	p.mu.Lock()
	p.ln = ln
	p.mu.Unlock()
	go p.accept(ln)
}

// cut, yeni bağlantıları reddeder ve var olan bütün bağlantıları keser.
func (p *tcpProxy) cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ln != nil {
		_ = p.ln.Close()
		p.ln = nil
	}
	for c := range p.conns {
		_ = c.Close()
		delete(p.conns, c)
	}
}

func (p *tcpProxy) track(c net.Conn) {
	p.mu.Lock()
	p.conns[c] = struct{}{}
	p.mu.Unlock()
}

func (p *tcpProxy) accept(ln net.Listener) {
	for {
		client, err := ln.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", p.target)
		if err != nil {
			_ = client.Close()
			continue
		}
		p.track(client)
		p.track(server)
		go p.pipe(server, client)
		go p.pipe(client, server)
	}
}

// Veritabanına erişilemediğinde worker yeni iş sahiplenmez ve süreçte kalır;
// erişim dönünce kuyruğu kendiliğinden işler. Kesinti tekrarlanınca da aynı.
func TestWorkerStaysAliveDuringDatabaseOutageAndResumesWhenItReturns(t *testing.T) {
	db, dbURL := testDB(t)
	first := seedCampaign(t, db, 6, "sms")

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newProxy(t, u.Host) // dbURL her zaman campaigns_test adresidir
	u.Host = proxy.addr
	viaProxy := u.String()

	port := freePort(t)
	startProvider(t, dbURL, port, nil) // provider doğrudan bağlanır; yalnızca worker'ın erişimi kesilir
	w := startProc(t, "worker", workerEnv(viaProxy, port, nil))

	// Erişim yokken: worker süreçte kalır, hiçbir iş sahiplenilmez, provider'a istek gitmez.
	time.Sleep(1500 * time.Millisecond)
	assertIdleDuringOutage(t, w, db, port, first, 6, 0)
	if !strings.Contains(w.out.String(), "kuyruk okunamadı") {
		t.Fatalf("worker erişim sorununu loglamalı:\n%s", w.out.String())
	}

	// Erişim döndü: kuyruk kendiliğinden işlenir.
	proxy.up(t)
	awaitCond(t, "ilk kampanya completed", func() bool { return campaignStatus(t, db, first) == "completed" })
	if got := statusCounts(t, db, first); got["sent"] != 6 || len(got) != 1 {
		t.Fatalf("6 mesajın hepsi sent olmalı: %v", got)
	}
	if !w.running() {
		t.Fatalf("worker kesinti boyunca sonlanmamalı:\n%s", w.out.String())
	}
	if !strings.Contains(w.out.String(), "kuyruk erişimi geri geldi") {
		t.Fatalf("worker erişimin dönüşünü loglamalı:\n%s", w.out.String())
	}

	// İkinci kesinti: yeni bir kampanya yine bekler ve erişim dönünce işlenir.
	proxy.cut()
	second := seedCampaign(t, db, 4, "sms")
	time.Sleep(1 * time.Second)
	assertIdleDuringOutage(t, w, db, port, second, 4, 6) // ilk kampanyanın 6 isteği

	proxy.up(t)
	awaitCond(t, "ikinci kampanya completed", func() bool { return campaignStatus(t, db, second) == "completed" })
	if got := statusCounts(t, db, second); got["sent"] != 4 || len(got) != 1 {
		t.Fatalf("4 mesajın hepsi sent olmalı: %v", got)
	}
	if !w.running() {
		t.Fatalf("worker sonlanmamalı:\n%s", w.out.String())
	}
}

// assertIdleDuringOutage, erişim yokken worker'ın canlı kaldığını ve kampanyanın
// mesajlarına hiç dokunmadığını doğrular.
func assertIdleDuringOutage(t *testing.T, w *proc, db *sql.DB, providerPort, campaignID string, queued, providerRequestsSoFar int) {
	t.Helper()
	if !w.running() {
		t.Fatalf("worker veritabanı kesintisinde sonlanmamalı:\n%s", w.out.String())
	}
	if got := statusCounts(t, db, campaignID); got["queued"] != queued || len(got) != 1 {
		t.Fatalf("erişim yokken hiçbir iş sahiplenilmemeli (%d queued): %v", queued, got)
	}
	if n := count(t, db, `SELECT count(*) FROM messages WHERE campaign_id = $1 AND attempts <> 0`, campaignID); n != 0 {
		t.Fatalf("erişim yokken hiçbir iş denenmemeli: %d iş denendi", n)
	}
	if snap := providerStats(t, providerPort); snap.TotalRequests != providerRequestsSoFar {
		t.Fatalf("erişim yokken provider'a yeni istek gitmemeli (şimdiye kadar %d): %+v", providerRequestsSoFar, snap)
	}
}
