package integration

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/httpapi"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// deliverySnapshotBody, `snapshot` olayının sözleşmedeki (ARCHITECTURE §5) gövdesidir.
type deliverySnapshotBody struct {
	Status   string `json:"status"`
	Messages struct {
		Queued, Retrying, Sending, Sent, Failed, Skipped int
	} `json:"messages"`
	Failures struct{ Exhausted, Rejected int } `json:"failures"`
	Channels struct {
		SMS, Email struct {
			Queued, Retrying, Sending, Sent, Failed, Skipped int
		}
	} `json:"channels"`
	Recovered int `json:"recovered"`
}

type streamedEvent struct {
	Event string
	Data  string
	Body  deliverySnapshotBody
}

// openDeliveryStream, gerçek handler'ın dashboard akışını açar ve olayları kanala verir.
// Akış kapanınca kanal kapanır.
func openDeliveryStream(t *testing.T, db *sql.DB, id string, interval time.Duration) (events <-chan streamedEvent, cancel context.CancelFunc) {
	t.Helper()
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Deliveries:       campaign.NewStore(db),
		DeliveryInterval: interval,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); srv.Close() })

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/campaigns/"+id+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("akış açılamadı: %d %s", resp.StatusCode, b)
	}

	out := make(chan streamedEvent, 128)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		var ev streamedEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if ev.Event == "snapshot" {
					_ = json.Unmarshal([]byte(ev.Data), &ev.Body)
				}
				if ev.Event != "" {
					out <- ev
				}
				ev = streamedEvent{}
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return out, cancel
}

// awaitSnapshot, koşulu sağlayan görüntü gelene kadar olayları okur.
func awaitSnapshot(t *testing.T, events <-chan streamedEvent, what string, cond func(deliverySnapshotBody) bool) deliverySnapshotBody {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("akış %s gelmeden kapandı", what)
			}
			if ev.Event == "snapshot" && cond(ev.Body) {
				return ev.Body
			}
		case <-deadline:
			t.Fatalf("zaman aşımı: %s", what)
		}
	}
}

// Veritabanındaki gerçek değişiklikler bir sonraki görüntüye yansır; kampanya
// tamamlanınca son görüntü yollanır ve akış kapanır.
func TestDeliveryStreamReflectsRealDatabaseChangesAndEndsWhenCompleted(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	for i := 0; i < 4; i++ {
		addMessage(t, db, id, "queued", 0, "")
	}
	events, _ := openDeliveryStream(t, db, id, 30*time.Millisecond)

	first := awaitSnapshot(t, events, "ilk görüntü", func(deliverySnapshotBody) bool { return true })
	if first.Status != "sending" || first.Messages.Queued != 4 || first.Messages.Sent != 0 {
		t.Fatalf("ilk görüntü: %+v", first)
	}

	// Bir mesaj gönderildi, biri yeniden denenecek, biri gönderiliyor.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	ids := messageIDs(t, db, id, "email")
	exec(`UPDATE messages SET status = 'sent', sent_at = now(), attempts = 1 WHERE id = $1`, ids[0])
	exec(`UPDATE messages SET attempts = 2 WHERE id = $1`, ids[1]) // queued kalır: yeniden denenecek
	exec(`UPDATE messages SET status = 'sending', attempts = 1, claim_token = gen_random_uuid(), claimed_at = now() WHERE id = $1`, ids[2])

	mid := awaitSnapshot(t, events, "değişikliğin yansıması", func(s deliverySnapshotBody) bool { return s.Messages.Sent == 1 })
	if mid.Messages.Queued != 1 || mid.Messages.Retrying != 1 || mid.Messages.Sending != 1 || mid.Status != "sending" {
		t.Fatalf("1 bekleyen, 1 yeniden denenecek, 1 gönderiliyor, 1 gönderildi bekleniyordu: %+v", mid)
	}

	// Her şey bitti: biri başarısız (deneme sınırı), biri atlandı; kampanya tamamlandı.
	exec(`UPDATE messages SET status = 'sent', sent_at = now(), claim_token = NULL, claimed_at = NULL WHERE id = $1`, ids[2])
	exec(`UPDATE messages SET status = 'failed', failure_reason = 'exhausted' WHERE id = $1`, ids[1])
	exec(`UPDATE messages SET status = 'skipped' WHERE id = $1`, ids[3])
	exec(`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`, id)

	last := awaitSnapshot(t, events, "tamamlanmış görüntü", func(s deliverySnapshotBody) bool { return s.Status == "completed" })
	if last.Messages.Sent != 2 || last.Messages.Failed != 1 || last.Messages.Skipped != 1 || last.Failures.Exhausted != 1 {
		t.Fatalf("tamamlanmış görüntü: %+v", last)
	}

	select {
	case ev, ok := <-events:
		if ok {
			t.Fatalf("tamamlanmış görüntüden sonra olay gelmemeli: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("kampanya tamamlandı ama akış kapanmadı")
	}
}

// Kuyrukta hiçbir şey değişmese de görüntüler düzenli gelir (heartbeat): arayüz bunu
// sağlıklı bağlantı sayar, sessizliği ise takılma.
func TestDeliveryStreamKeepsSendingIdenticalSnapshotsWhileNothingChanges(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "approved")
	addMessage(t, db, id, "queued", 0, "")
	addMessage(t, db, id, "queued", 0, "")
	events, _ := openDeliveryStream(t, db, id, 25*time.Millisecond)

	first := awaitSnapshot(t, events, "ilk görüntü", func(deliverySnapshotBody) bool { return true })
	start := time.Now()
	for i := 0; i < 6; i++ {
		got := awaitSnapshot(t, events, "tekrarlanan görüntü", func(deliverySnapshotBody) bool { return true })
		if got != first {
			t.Fatalf("değişmeyen kuyrukta görüntü ilkiyle aynı olmalı: %+v != %+v", got, first)
		}
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("görüntüler zamanlayıcısız geldi: %s", elapsed)
	}
}

func TestDeliveryStreamUnknownOrInvalidCampaignIs404JSON(t *testing.T) {
	db := testdb.Open(t)
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Deliveries: campaign.NewStore(db)}))
	t.Cleanup(srv.Close)

	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "uuid-degil"} {
		resp, err := http.Get(srv.URL + "/api/campaigns/" + id + "/events")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(body), `"CAMPAIGN_NOT_FOUND"`) {
			t.Errorf("%s: 404 CAMPAIGN_NOT_FOUND bekleniyordu: %d %s", id, resp.StatusCode, body)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: akış değil düz JSON bekleniyordu: %q", id, ct)
		}
	}
}

// Draft kampanyada da akış açılır (sıfır sayılar, durum draft): arayüz onaylanmamış
// kampanyada bağlanmaz ama sunucu bunu hata saymaz.
func TestDeliveryStreamOfDraftCampaignReportsZeros(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	events, _ := openDeliveryStream(t, db, id, time.Hour)

	got := awaitSnapshot(t, events, "ilk görüntü", func(deliverySnapshotBody) bool { return true })
	if got.Status != "draft" || got.Messages != (deliverySnapshotBody{}).Messages {
		t.Fatalf("draft kampanya sıfır sayıyla dönmeli: %+v", got)
	}
	if got.Channels != (deliverySnapshotBody{}).Channels || got.Recovered != 0 {
		t.Fatalf("kanal kırılımı ve toparlanan da sıfır olmalı: %+v", got)
	}
}

// Akışın gerçek görüntüsünde kanal kırılımı ve toparlanan sayısı veritabanındaki gerçek
// değerlerdir; değişiklik sonraki görüntüye yansır.
func TestDeliveryStreamCarriesChannelSplitAndRecoveredFromTheDatabase(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addMessageOn(t, db, id, "sms", "sent", 2, "") // toparlanan
	addMessageOn(t, db, id, "sms", "queued", 0, "")
	addMessageOn(t, db, id, "email", "sent", 1, "")
	events, _ := openDeliveryStream(t, db, id, 20*time.Millisecond)

	first := awaitSnapshot(t, events, "ilk görüntü", func(deliverySnapshotBody) bool { return true })
	if first.Channels.SMS.Sent != 1 || first.Channels.SMS.Queued != 1 || first.Channels.Email.Sent != 1 || first.Recovered != 1 {
		t.Fatalf("ilk görüntü: %+v", first)
	}
	if first.Messages.Sent != first.Channels.SMS.Sent+first.Channels.Email.Sent {
		t.Fatalf("toplam kanalların toplamı olmalı: %+v", first)
	}

	// Bekleyen SMS 2. denemede gönderilir: SMS gönderilen ve toparlanan artar.
	if _, err := db.Exec(`UPDATE messages SET status = 'sent', attempts = 2, sent_at = now()
	                      WHERE campaign_id = $1 AND channel = 'sms' AND status = 'queued'`, id); err != nil {
		t.Fatal(err)
	}
	next := awaitSnapshot(t, events, "güncel görüntü", func(b deliverySnapshotBody) bool { return b.Channels.SMS.Sent == 2 })
	if next.Recovered != 2 || next.Channels.SMS.Queued != 0 {
		t.Fatalf("güncel görüntü: %+v", next)
	}
}
