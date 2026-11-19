package integration

import (
	"context"
	"database/sql"
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

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

func providerRequest(key, channel, recipient, content string) mockprovider.Request {
	return mockprovider.Request{Key: key, Channel: channel, Recipient: recipient, ContentHash: [32]byte(hashOf(content))}
}

func TestProviderStoreRecordsOnceAndReplaysRecordedResult(t *testing.T) {
	db := testdb.Open(t)
	store := mockprovider.NewPGStore(db)
	ctx := context.Background()
	req := providerRequest("k:1:sms", "sms", "+905550000001", "merhaba")

	first, err := store.Accept(ctx, req)
	if err != nil || !first.Created || first.MessageID == "" {
		t.Fatalf("ilk kabul kaydedilmeli: %+v %v", first, err)
	}
	again, err := store.Accept(ctx, req)
	if err != nil || again.Created || again.MessageID != first.MessageID {
		t.Fatalf("tekrar istek kayıtlı sonucu almalı (Created=false): %+v %v", again, err)
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM mock_provider_acceptances`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("tek kayıt bekleniyordu: %d %v", n, err)
	}
}

func TestProviderStoreRejectsKeyReuseWithDifferentRecipientChannelOrContent(t *testing.T) {
	db := testdb.Open(t)
	store := mockprovider.NewPGStore(db)
	ctx := context.Background()
	orig := providerRequest("k:1:sms", "sms", "+905550000001", "merhaba")
	first, err := store.Accept(ctx, orig)
	if err != nil {
		t.Fatal(err)
	}

	for name, req := range map[string]mockprovider.Request{
		"farklı alıcı":  providerRequest("k:1:sms", "sms", "+905550000002", "merhaba"),
		"farklı kanal":  providerRequest("k:1:sms", "email", "+905550000001", "merhaba"),
		"farklı içerik": providerRequest("k:1:sms", "sms", "+905550000001", "başka"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := store.Accept(ctx, req)
			if !errors.Is(err, mockprovider.ErrKeyReused) || got.MessageID != "" {
				t.Fatalf("ErrKeyReused bekleniyordu, kayıtlı sonuç dönmemeli: %+v %v", got, err)
			}
		})
	}

	// Kayıt değişmedi.
	var id, recipient string
	if err := db.QueryRow(`SELECT message_id::text, recipient FROM mock_provider_acceptances WHERE idempotency_key = 'k:1:sms'`).Scan(&id, &recipient); err != nil {
		t.Fatal(err)
	}
	if id != first.MessageID || recipient != "+905550000001" {
		t.Fatalf("özgün kayıt korunmalı: %s %s", id, recipient)
	}
}

// Aynı anahtarla eşzamanlı gelen aynı istekler: tam bir kabul yazılır, hepsi aynı
// message_id'yi alır, hata yoktur.
func TestProviderStoreConcurrentSameRequestRecordsOnce(t *testing.T) {
	db := testdb.Open(t)
	store := mockprovider.NewPGStore(db)
	const workers = 25
	req := providerRequest("yaris:1:sms", "sms", "+905550000001", "içerik")

	type result struct {
		acc mockprovider.Acceptance
		err error
	}
	results := make(chan result, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			acc, err := store.Accept(context.Background(), req)
			results <- result{acc, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	created, ids := 0, map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Fatalf("beklenmeyen hata: %v", r.err)
		}
		if r.acc.Created {
			created++
		}
		ids[r.acc.MessageID] = true
	}
	if created != 1 || len(ids) != 1 {
		t.Fatalf("tam bir kabul ve tek message_id bekleniyordu: created=%d ids=%v", created, ids)
	}
}

// Aynı anahtarla eşzamanlı gelen FARKLI istekler: tam bir kazanan, kalanlar
// ErrKeyReused. Hiçbiri kazananın sonucunu yanlışlıkla almaz.
func TestProviderStoreConcurrentDifferentRecipientsOnlyOneWins(t *testing.T) {
	db := testdb.Open(t)
	store := mockprovider.NewPGStore(db)
	const workers = 20

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []string
		reused  int
	)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		recipient := "+90555000" + strings.Repeat("0", 3) + string(rune('0'+i%10)) + string(rune('0'+i/10))
		go func() {
			defer wg.Done()
			<-start
			acc, err := store.Accept(context.Background(), providerRequest("yaris:2:sms", "sms", recipient, "içerik"))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && acc.Created:
				winners = append(winners, recipient)
			case errors.Is(err, mockprovider.ErrKeyReused):
				reused++
			default:
				t.Errorf("beklenmeyen sonuç: %+v %v", acc, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(winners) != 1 || reused != workers-1 {
		t.Fatalf("1 kazanan ve %d çakışma bekleniyordu: kazananlar=%v çakışma=%d", workers-1, winners, reused)
	}
	var recipient string
	if err := db.QueryRow(`SELECT recipient FROM mock_provider_acceptances WHERE idempotency_key = 'yaris:2:sms'`).Scan(&recipient); err != nil || recipient != winners[0] {
		t.Fatalf("kayıt kazananın olmalı: %q %v", recipient, err)
	}
}

// newProviderServer, gerçek PGStore ile bir mock provider sunucusu başlatır.
func newProviderServer(t *testing.T, db *sql.DB, cfg mockprovider.Config) (*httptest.Server, *mockprovider.Handler) {
	t.Helper()
	h, err := mockprovider.New(cfg, mockprovider.NewPGStore(db),
		mockprovider.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, h
}

var noKeepAlive = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}

func providerPost(srv *httptest.Server, key, body string) (status int, messageID string, err error) {
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/sms", strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Idempotency-Key", key)
	resp, err := noKeepAlive.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var out struct {
		MessageID string `json:"message_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.MessageID, nil
}

const providerSMS = `{"to":"+905550000001","text":"Merhaba Ada"}`

// Provider yeniden başlasa da kabul kaydı korunur: yeni bir Handler (boş bellek
// istatistikleri) aynı veritabanıyla aynı anahtara kayıtlı sonucu döndürür ve
// ikinci bir kabul oluşturmaz. İstatistikler ise sıfırlanır (ADR-025).
func TestProviderRestartKeepsAcceptanceRecords(t *testing.T) {
	db := testdb.Open(t)
	srv1, h1 := newProviderServer(t, db, mockprovider.Config{Mode: mockprovider.ModeAcceptThenDrop})

	// İlk kabulün yanıtı düşer; kayıt commit edilmiştir.
	if _, _, err := providerPost(srv1, "k:1:sms", providerSMS); err == nil {
		t.Fatal("ilk yanıt düşmeli")
	}
	var recorded string
	if err := db.QueryRow(`SELECT message_id::text FROM mock_provider_acceptances WHERE idempotency_key = 'k:1:sms'`).Scan(&recorded); err != nil {
		t.Fatalf("yanıt düşse de kabul kaydedilmiş olmalı: %v", err)
	}
	srv1.Close() // "provider yeniden başlıyor"

	srv2, h2 := newProviderServer(t, db, mockprovider.Config{})
	status, id, err := providerPost(srv2, "k:1:sms", providerSMS)
	if err != nil || status != 200 || id != recorded {
		t.Fatalf("yeniden başlayan provider kayıtlı sonucu döndürmeli: %d %q (kayıtlı %q) %v", status, id, recorded, err)
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM mock_provider_acceptances`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("hâlâ tek kabul kaydı olmalı: %d %v", n, err)
	}
	s1, s2 := h1.Stats().Snapshot(), h2.Stats().Snapshot()
	if s1.UniqueAccepts != 1 || s1.ResponsesDropped != 1 {
		t.Fatalf("ilk süreç: 1 kabul, 1 düşen yanıt: %+v", s1)
	}
	// Yeni süreç kabulü kendi belleğinde "ilk" görmez; kayıtlı sonuç tekrar isteğidir.
	if s2.UniqueAccepts != 0 || s2.DuplicateRequests != 1 || s2.DuplicateDeliveries != 0 {
		t.Fatalf("yeni süreç: kabul 0, tekrar isteği 1, tekrar teslimat 0: %+v", s2)
	}
}

// Anahtar çakışması gerçek veritabanıyla da reddedilir ve kayıt değişmez.
func TestProviderRejectsKeyReuseAgainstRealDatabase(t *testing.T) {
	db := testdb.Open(t)
	srv, h := newProviderServer(t, db, mockprovider.Config{})

	status, first, err := providerPost(srv, "k:1:sms", providerSMS)
	if err != nil || status != 200 {
		t.Fatalf("hazırlık: %d %v", status, err)
	}
	status, id, err := providerPost(srv, "k:1:sms", `{"to":"+905550000002","text":"Merhaba Ada"}`)
	if err != nil || status != http.StatusUnprocessableEntity || id != "" {
		t.Fatalf("farklı alıcıyla aynı anahtar 422 almalı: %d %q %v", status, id, err)
	}
	status, id, err = providerPost(srv, "k:1:sms", providerSMS)
	if err != nil || status != 200 || id != first {
		t.Fatalf("özgün istek kayıtlı sonucu almalı: %d %q %v", status, id, err)
	}
	if got := h.Stats().Snapshot().KeyReuseRejected; got != 1 {
		t.Fatalf("bir çakışma sayılmalı: %d", got)
	}
}
