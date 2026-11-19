package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

const deliveryPath = "/api/campaigns/" + campaignID + "/events"

// fakeDeliveries, DeliveryReader'ın taklididir: çağrı sırasına göre görüntü verir
// (sonuncusu tekrar eder) ve istenirse çağrıyı engelleyebilir veya hata verebilir.
type fakeDeliveries struct {
	mu    sync.Mutex
	snaps []campaign.DeliverySnapshot
	calls int
	ids   []string
	// hook, her çağrıda (1'den numaralı) çalışır; hata dönerse çağrı o hatayla biter.
	hook func(call int, ctx context.Context) error
}

func (f *fakeDeliveries) DeliverySnapshot(ctx context.Context, id string) (campaign.DeliverySnapshot, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.ids = append(f.ids, id)
	snap := campaign.DeliverySnapshot{}
	if len(f.snaps) > 0 {
		snap = f.snaps[min(n-1, len(f.snaps)-1)]
	}
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		if err := hook(n, ctx); err != nil {
			return campaign.DeliverySnapshot{}, err
		}
	}
	return snap, nil
}

func (f *fakeDeliveries) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func snapshot(status string, queued, sent int) campaign.DeliverySnapshot {
	return campaign.DeliverySnapshot{
		Status:   status,
		Messages: campaign.DeliveryCounts{Queued: queued, Sent: sent},
	}
}

type streamEvent struct {
	Event string
	Data  string
}

// stream, olay akışını arka planda okuyup olayları kanala verir; akış bitince kanal kapanır.
type stream struct {
	resp   *http.Response
	events chan streamEvent
	cancel context.CancelFunc
}

func openStream(t *testing.T, url string) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("istek başarısız: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	s := &stream{resp: resp, events: make(chan streamEvent, 64), cancel: cancel}
	go func() {
		defer close(s.events)
		sc := bufio.NewScanner(resp.Body)
		var ev streamEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if ev.Event != "" {
					s.events <- ev
				}
				ev = streamEvent{}
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return s
}

// next, bir sonraki olayı bekler; süre içinde gelmezse testi durdurur.
func (s *stream) next(t *testing.T, within time.Duration) streamEvent {
	t.Helper()
	select {
	case ev, ok := <-s.events:
		if !ok {
			t.Fatal("akış beklenenden önce kapandı")
		}
		return ev
	case <-time.After(within):
		t.Fatalf("%s içinde olay gelmedi", within)
		return streamEvent{}
	}
}

// wantClosed, akışın süre içinde (başka olay yollamadan) kapandığını doğrular.
func (s *stream) wantClosed(t *testing.T, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return
			}
			t.Fatalf("akış kapanmalıydı, olay geldi: %+v", ev)
		case <-deadline:
			t.Fatalf("akış %s içinde kapanmadı", within)
		}
	}
}

type deliveryServer struct {
	*httptest.Server
	reader *fakeDeliveries
}

func newDeliveryServer(t *testing.T, reader *fakeDeliveries, mutate func(*Deps)) *deliveryServer {
	t.Helper()
	deps := Deps{
		Deliveries:           reader,
		DeliveryInterval:     10 * time.Millisecond,
		DeliveryQueryTimeout: time.Second,
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&deps)
	}
	srv := httptest.NewServer(New(deps))
	t.Cleanup(srv.Close)
	return &deliveryServer{Server: srv, reader: reader}
}

func (d *deliveryServer) url() string { return d.URL + deliveryPath }

func decode(t *testing.T, ev streamEvent) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(ev.Data), &out); err != nil {
		t.Fatalf("olay verisi JSON değil: %q: %v", ev.Data, err)
	}
	return out
}

func TestDeliveryStreamSendsSnapshotImmediatelyWithSSEHeadersAndContractShape(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{{
		Status:   "sending",
		Messages: campaign.DeliveryCounts{Queued: 3, Retrying: 1, Sending: 1, Sent: 2, Failed: 4, Skipped: 5},
		Failures: campaign.FailureCounts{Exhausted: 1, Rejected: 2},
	}}}
	// Aralık çok uzun: ilk görüntü zamanlayıcıyı beklemeden gelmeli.
	srv := newDeliveryServer(t, reader, func(d *Deps) { d.DeliveryInterval = time.Hour })
	s := openStream(t, srv.url())

	ev := s.next(t, time.Second)
	if ev.Event != "snapshot" {
		t.Fatalf("ilk olay snapshot olmalı: %+v", ev)
	}
	// Sözleşme (ARCHITECTURE §5): bu birebir biçim arayüzün doğruladığı biçimdir.
	want := `{"status":"sending","messages":{"queued":3,"retrying":1,"sending":1,"sent":2,"failed":4,"skipped":5},"failures":{"exhausted":1,"rejected":2}}`
	if ev.Data != want {
		t.Fatalf("veri = %s\nbeklenen %s", ev.Data, want)
	}

	h := s.resp.Header
	if got := h.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Errorf("Content-Type = %q", got)
	}
	if h.Get("Cache-Control") != "no-store" || h.Get("X-Accel-Buffering") != "no" {
		t.Errorf("tamponlamayı kapatan başlıklar yok: %v", h)
	}
	if s.resp.StatusCode != http.StatusOK {
		t.Errorf("durum = %d", s.resp.StatusCode)
	}
	if reader.ids[0] != campaignID {
		t.Errorf("kampanya kimliği iletilmeli: %v", reader.ids)
	}
}

// Kuyrukta hiçbir şey değişmese de görüntüler düzenli gelir: arayüz görüntü
// gelmeyen bağlantıyı takılmış sayar, bu yüzden sağlıklı bir bağlantı sessiz kalmamalı.
func TestDeliveryStreamKeepsSendingSnapshotsEvenWhenNothingChanges(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{snapshot("sending", 5, 0)}}
	srv := newDeliveryServer(t, reader, func(d *Deps) { d.DeliveryInterval = 20 * time.Millisecond })
	s := openStream(t, srv.url())

	first := s.next(t, time.Second)
	start := time.Now()
	for i := 0; i < 5; i++ {
		ev := s.next(t, time.Second)
		if ev.Event != "snapshot" || ev.Data != first.Data {
			t.Fatalf("%d. görüntü ilkiyle aynı olmalı: %+v", i+2, ev)
		}
	}
	// Görüntüler zamanlayıcıyla gelir: 5 görüntü yaklaşık 5 aralık sürer.
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("görüntüler aralıksız (zamanlayıcısız) geldi: %s", elapsed)
	}
	if reader.callCount() < 6 {
		t.Fatalf("her aralıkta durum okunmalı: %d okuma", reader.callCount())
	}
}

func TestDeliveryStreamReplacesCountsWithEachSnapshot(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{
		snapshot("approved", 7, 0),
		snapshot("sending", 4, 3),
		snapshot("sending", 1, 6),
		snapshot("completed", 0, 7),
	}}
	srv := newDeliveryServer(t, reader, nil)
	s := openStream(t, srv.url())

	var sent []float64
	var statuses []string
	for i := 0; i < 4; i++ {
		d := decode(t, s.next(t, time.Second))
		messages, _ := d["messages"].(map[string]any)
		n, _ := messages["sent"].(float64)
		sent = append(sent, n)
		status, _ := d["status"].(string)
		statuses = append(statuses, status)
	}
	if got := strings.Join(statuses, ","); got != "approved,sending,sending,completed" {
		t.Fatalf("durumlar: %s", got)
	}
	if sent[0] != 0 || sent[1] != 3 || sent[2] != 6 || sent[3] != 7 {
		t.Fatalf("gönderilen sayıları: %v", sent)
	}
}

// Kampanya tamamlanınca son görüntü yollanır ve akış biter; bir daha okunmaz.
func TestDeliveryStreamEndsAfterTheCompletedSnapshot(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{snapshot("sending", 1, 1), snapshot("completed", 0, 2)}}
	srv := newDeliveryServer(t, reader, nil)
	s := openStream(t, srv.url())

	s.next(t, time.Second)
	last := decode(t, s.next(t, time.Second))
	if last["status"] != "completed" {
		t.Fatalf("son görüntü completed olmalı: %v", last)
	}
	s.wantClosed(t, time.Second)

	calls := reader.callCount()
	time.Sleep(60 * time.Millisecond)
	if reader.callCount() != calls || calls != 2 {
		t.Fatalf("tamamlandıktan sonra durum okunmamalı: %d okuma (2 bekleniyordu)", reader.callCount())
	}
}

func TestDeliveryStreamOfAlreadyCompletedCampaignSendsOneSnapshotAndEnds(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{snapshot("completed", 0, 9)}}
	srv := newDeliveryServer(t, reader, func(d *Deps) { d.DeliveryInterval = time.Hour })
	s := openStream(t, srv.url())

	if d := decode(t, s.next(t, time.Second)); d["status"] != "completed" {
		t.Fatalf("completed bekleniyordu: %v", d)
	}
	s.wantClosed(t, time.Second)
}

func TestDeliveryStreamUnknownCampaignIsPlainJSON404BeforeAnyStream(t *testing.T) {
	reader := &fakeDeliveries{hook: func(int, context.Context) error { return campaign.ErrNotFound }}
	srv := newDeliveryServer(t, reader, nil)

	resp, err := http.Get(srv.url())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("durum = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("akış değil düz JSON hata bekleniyordu: %q", ct)
	}
	var out struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Error.Code != "CAMPAIGN_NOT_FOUND" {
		t.Fatalf("gövde = %s", body)
	}
}

func TestDeliveryStreamFirstReadFailureIs500WithoutLeakingDetails(t *testing.T) {
	reader := &fakeDeliveries{hook: func(int, context.Context) error { return errors.New("pq: gizli-baglanti-ayrintisi") }}
	srv := newDeliveryServer(t, reader, nil)

	resp, err := http.Get(srv.url())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), `"INTERNAL"`) {
		t.Fatalf("500 INTERNAL bekleniyordu: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "gizli") {
		t.Fatalf("iç hata ayrıntısı istemciye sızmamalı: %s", body)
	}
}

// Akış sırasında okuma hatası: sessiz kalınmaz, `error` olayı yollanır ve akış
// kapanır (arayüz yeniden bağlanır). Ayrıntı istemciye sızmaz.
func TestDeliveryStreamReadFailureMidStreamSendsErrorEventAndCloses(t *testing.T) {
	reader := &fakeDeliveries{
		snaps: []campaign.DeliverySnapshot{snapshot("sending", 3, 0)},
		hook: func(call int, _ context.Context) error {
			if call == 3 {
				return errors.New("pq: gizli-baglanti-ayrintisi")
			}
			return nil
		},
	}
	srv := newDeliveryServer(t, reader, nil)
	s := openStream(t, srv.url())

	s.next(t, time.Second)
	s.next(t, time.Second)
	ev := s.next(t, time.Second)
	if ev.Event != "error" {
		t.Fatalf("error olayı bekleniyordu: %+v", ev)
	}
	d := decode(t, ev)
	if d["code"] != "INTERNAL" || strings.Contains(ev.Data, "gizli") {
		t.Fatalf("INTERNAL kod ve ayrıntısız mesaj bekleniyordu: %s", ev.Data)
	}
	s.wantClosed(t, time.Second)

	calls := reader.callCount()
	time.Sleep(60 * time.Millisecond)
	if reader.callCount() != calls {
		t.Fatal("hatadan sonra okuma sürmemeli")
	}
}

func TestDeliveryStreamCampaignDisappearingMidStreamSendsNotFoundError(t *testing.T) {
	reader := &fakeDeliveries{
		snaps: []campaign.DeliverySnapshot{snapshot("sending", 1, 0)},
		hook: func(call int, _ context.Context) error {
			if call == 2 {
				return campaign.ErrNotFound
			}
			return nil
		},
	}
	srv := newDeliveryServer(t, reader, nil)
	s := openStream(t, srv.url())

	s.next(t, time.Second)
	ev := s.next(t, time.Second)
	if d := decode(t, ev); ev.Event != "error" || d["code"] != "CAMPAIGN_NOT_FOUND" {
		t.Fatalf("CAMPAIGN_NOT_FOUND error olayı bekleniyordu: %+v", ev)
	}
	s.wantClosed(t, time.Second)
}

// Takılan bir okuma sessizliğe dönüşmez: süre sınırı dolunca error olayı yollanır.
func TestDeliveryStreamStuckReadBecomesErrorEventWithinTheQueryTimeout(t *testing.T) {
	reader := &fakeDeliveries{
		snaps: []campaign.DeliverySnapshot{snapshot("sending", 1, 0)},
		hook: func(call int, ctx context.Context) error {
			if call >= 2 {
				<-ctx.Done() // veritabanı yanıt vermiyor
				return ctx.Err()
			}
			return nil
		},
	}
	srv := newDeliveryServer(t, reader, func(d *Deps) { d.DeliveryQueryTimeout = 80 * time.Millisecond })
	s := openStream(t, srv.url())

	s.next(t, time.Second)
	start := time.Now()
	ev := s.next(t, 2*time.Second)
	if ev.Event != "error" || decode(t, ev)["code"] != "INTERNAL" {
		t.Fatalf("INTERNAL error olayı bekleniyordu: %+v", ev)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("takılan okuma %s sonra bildirildi; süre sınırı 80ms", elapsed)
	}
	s.wantClosed(t, time.Second)
}

func TestDeliveryStreamFirstReadIsAlsoBoundedByTheQueryTimeout(t *testing.T) {
	reader := &fakeDeliveries{hook: func(_ int, ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	srv := newDeliveryServer(t, reader, func(d *Deps) { d.DeliveryQueryTimeout = 80 * time.Millisecond })

	start := time.Now()
	resp, err := http.Get(srv.url())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("500 bekleniyordu: %d", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ilk okuma %s sürdü; süre sınırı 80ms", elapsed)
	}
}

// İstemci ayrılınca akış biter: sunucu bekleyen bir goroutine bırakmaz (httptest
// sunucusunun Close'u bütün istek işleyicileri dönene kadar bekler) ve durum okumak sürmez.
func TestDeliveryStreamStopsWhenTheClientDisconnects(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{snapshot("sending", 1, 0)}}
	srv := newDeliveryServer(t, reader, func(d *Deps) { d.DeliveryInterval = 20 * time.Millisecond })
	s := openStream(t, srv.url())
	s.next(t, time.Second)
	s.next(t, time.Second)

	s.cancel() // istemci bağlantıyı kapattı
	time.Sleep(80 * time.Millisecond)
	calls := reader.callCount()
	time.Sleep(120 * time.Millisecond)
	if reader.callCount() != calls {
		t.Fatalf("istemci ayrıldıktan sonra durum okunmaya devam etti: %d → %d", calls, reader.callCount())
	}

	closed := make(chan struct{})
	go func() { srv.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("işleyici dönmedi: akış goroutine'i sızıyor")
	}
}

// Sunucu kapanırken (Deps.Shutdown) akış kendiliğinden biter; düzgün kapanış bu akışı
// sonuna kadar beklemez.
func TestDeliveryStreamEndsWhenTheServerStartsShuttingDown(t *testing.T) {
	shutdown := make(chan struct{})
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{snapshot("sending", 1, 0)}}
	srv := newDeliveryServer(t, reader, func(d *Deps) {
		d.DeliveryInterval = time.Hour // yalnızca kapanış akışı bitirebilir
		d.Shutdown = shutdown
	})
	s := openStream(t, srv.url())
	s.next(t, time.Second)

	close(shutdown)
	s.wantClosed(t, time.Second)
}

// Yazma hatasında (istemci gitti, yazma süresi doldu) akış biter ve hata alındıktan
// sonra durum okunmaz.
func TestDeliveryStreamStopsWhenWritingFails(t *testing.T) {
	reader := &fakeDeliveries{snaps: []campaign.DeliverySnapshot{snapshot("sending", 1, 0)}}
	h := New(Deps{Deliveries: reader, DeliveryInterval: 5 * time.Millisecond, DeliveryQueryTimeout: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	w := newFailingWriter()
	w.failWriteAt = 2 // ilk görüntü yazılır, ikincisinde bağlantı kopar

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, deliveryPath, nil))
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("yazma hatasına rağmen akış sürüyor")
	}
	if calls := reader.callCount(); calls > 3 {
		t.Fatalf("yazma hatasından sonra okuma sürmemeli: %d okuma", calls)
	}
}

func TestDeliveryStreamMethodNotAllowed(t *testing.T) {
	srv := newDeliveryServer(t, &fakeDeliveries{}, nil)
	resp, err := http.Post(srv.url(), "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != http.MethodGet {
		t.Fatalf("405 ve Allow: GET bekleniyordu: %d %q", resp.StatusCode, resp.Header.Get("Allow"))
	}
}
