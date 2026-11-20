package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

// addMessage, belirli durumda tek bir e-posta mesajı ekler. Her mesaj kendi kullanıcısıdır.
func addMessage(t *testing.T, db *sql.DB, campaignID, status string, attempts int, failureReason string) {
	t.Helper()
	addMessageOn(t, db, campaignID, "email", status, attempts, failureReason)
}

// addMessageOn, belirli kanalda ve durumda tek bir mesaj ekler.
func addMessageOn(t *testing.T, db *sql.DB, campaignID, channel, status string, attempts int, failureReason string) {
	t.Helper()
	uid := mustUser(t, db)
	recipient := "x@example.test"
	if channel == "sms" {
		recipient = "+905550000000"
	}
	var reason any
	if failureReason != "" {
		reason = failureReason
	}
	_, err := db.Exec(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key,
		                       status, attempts, failure_reason, sent_at, claim_token, claimed_at)
		 VALUES ($1, $2, $7, $8, '', $3, $4::text, $5, $6,
		         CASE WHEN $4::text = 'sent' THEN now() END,
		         CASE WHEN $4::text = 'sending' THEN gen_random_uuid() END,
		         CASE WHEN $4::text = 'sending' THEN now() END)`,
		campaignID, uid, idemKey(campaignID, uid, channel), status, attempts, reason, channel, recipient)
	if err != nil {
		t.Fatalf("mesaj eklenemedi (%s, %s): %v", channel, status, err)
	}
}

func snapshotOf(t *testing.T, db *sql.DB, id string) campaign.DeliverySnapshot {
	t.Helper()
	snap, err := campaign.NewStore(db).DeliverySnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snap
}

func approvedCampaignWithStatus(t *testing.T, db *sql.DB, status string) string {
	t.Helper()
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	mustApprove(t, db, id, 1)
	if status != "approved" {
		if _, err := db.Exec(`UPDATE campaigns SET status = $2 WHERE id = $1`, id, status); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// Her mesaj durumu doğru kovaya sayılır. Sabit veri kümesiyle kesin sayılar.
func TestDeliverySnapshotCountsEveryMessageState(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")

	for i := 0; i < 2; i++ {
		addMessage(t, db, id, "queued", 0, "") // hiç denenmemiş
	}
	addMessage(t, db, id, "queued", 2, "") // denenmiş, yeniden sırada
	addMessage(t, db, id, "sending", 1, "")
	for i := 0; i < 3; i++ {
		addMessage(t, db, id, "sent", 1, "")
	}
	addMessage(t, db, id, "failed", 5, worker.FailureExhausted)
	addMessage(t, db, id, "failed", 1, worker.FailureRejected)
	addMessage(t, db, id, "failed", 3, "") // nedeni kaydedilmemiş: yalnızca failed'e sayılır
	addMessage(t, db, id, "skipped", 1, "")

	got := snapshotOf(t, db, id)
	counts := campaign.DeliveryCounts{Queued: 2, Retrying: 1, Sending: 1, Sent: 3, Failed: 3, Skipped: 1}
	want := campaign.DeliverySnapshot{
		Status:   "sending",
		Messages: counts,
		Failures: campaign.FailureCounts{Exhausted: 1, Rejected: 1},
		// Bu testin mesajları yalnızca e-postadır: kırılım e-postada toplamdır, SMS sıfır.
		Channels: campaign.ChannelDelivery{Email: counts},
		// Gönderilenlerin hepsi ilk denemede (attempts = 1) gitti: toparlanan yok.
		Recovered: 0,
	}
	if got != want {
		t.Fatalf("snapshot = %+v, beklenen %+v", got, want)
	}
	if got.Failures.Exhausted+got.Failures.Rejected > got.Messages.Failed {
		t.Fatal("neden sayılarının toplamı başarısız sayısını aşamaz")
	}
}

// Yeniden denenecekler (queued, attempts > 0) bekleyenlere ikinci kez eklenmez.
func TestDeliverySnapshotDoesNotCountRetriesAsQueuedToo(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addMessage(t, db, id, "queued", 0, "")
	addMessage(t, db, id, "queued", 1, "")
	addMessage(t, db, id, "queued", 4, "")

	got := snapshotOf(t, db, id).Messages
	if got.Queued != 1 || got.Retrying != 2 {
		t.Fatalf("1 bekleyen ve 2 yeniden denenecek bekleniyordu: %+v", got)
	}
	if total := got.Queued + got.Retrying + got.Sending + got.Sent + got.Failed + got.Skipped; total != 3 {
		t.Fatalf("toplam 3 mesaj olmalı (çift sayım yok): %d", total)
	}
}

// Başka kampanyanın mesajları karışmaz.
func TestDeliverySnapshotCountsOnlyTheRequestedCampaign(t *testing.T) {
	db := testdb.Open(t)
	a := approvedCampaignWithStatus(t, db, "sending")
	b := approvedCampaignWithStatus(t, db, "sending")
	addMessage(t, db, a, "sent", 1, "")
	addMessage(t, db, a, "sent", 1, "")
	addMessage(t, db, b, "failed", 5, worker.FailureExhausted)

	if got := snapshotOf(t, db, a); got.Messages.Sent != 2 || got.Messages.Failed != 0 || got.Failures.Exhausted != 0 {
		t.Fatalf("A: %+v", got)
	}
	if got := snapshotOf(t, db, b); got.Messages.Sent != 0 || got.Messages.Failed != 1 || got.Failures.Exhausted != 1 {
		t.Fatalf("B: %+v", got)
	}
}

// Mesajı olmayan kampanya (draft) sıfır sayılarla ve gerçek durumuyla döner.
func TestDeliverySnapshotOfCampaignWithoutMessagesIsAllZeros(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db) // draft

	got := snapshotOf(t, db, id)
	if got != (campaign.DeliverySnapshot{Status: "draft"}) {
		t.Fatalf("draft kampanya sıfır sayılarla dönmeli: %+v", got)
	}
}

func TestDeliverySnapshotReportsCampaignStatus(t *testing.T) {
	db := testdb.Open(t)
	for _, status := range []string{"approved", "sending"} {
		id := approvedCampaignWithStatus(t, db, status)
		if got := snapshotOf(t, db, id).Status; got != status {
			t.Errorf("durum %q bekleniyordu: %q", status, got)
		}
	}

	id := approvedCampaignWithStatus(t, db, "approved")
	addMessage(t, db, id, "sent", 1, "")
	if _, err := db.Exec(`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if got := snapshotOf(t, db, id); got.Status != "completed" || got.Messages.Sent != 1 {
		t.Fatalf("tamamlanmış kampanya: %+v", got)
	}
}

func TestDeliverySnapshotUnknownOrInvalidIDIsNotFound(t *testing.T) {
	db := testdb.Open(t)
	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "uuid-degil", ""} {
		_, err := campaign.NewStore(db).DeliverySnapshot(context.Background(), id)
		if !errors.Is(err, campaign.ErrNotFound) {
			t.Errorf("%q: ErrNotFound bekleniyordu: %v", id, err)
		}
	}
}

// Sorgudaki neden değerleri, worker'ın yazdığı değerlerle aynı kalmalıdır. İki
// paket birbirine bağımlı olmadığı için bu eşitlik yalnızca testle korunur.
func TestDeliverySnapshotFailureReasonsMatchWorkerValues(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addMessage(t, db, id, "failed", 5, worker.FailureExhausted)
	addMessage(t, db, id, "failed", 1, worker.FailureRejected)

	got := snapshotOf(t, db, id).Failures
	if got.Exhausted != 1 || got.Rejected != 1 {
		t.Fatalf("worker'ın yazdığı nedenler doğru sayılmalı: %+v", got)
	}
}

// ---- Kanal kırılımı ve yeniden denemeyle toparlananlar (TRK-001) ----

// addMixed, iki kanala, her durumdan farklı sayıda mesaj ekler ve beklenen kırılımı döndürür.
func addMixed(t *testing.T, db *sql.DB, id string) (sms, email campaign.DeliveryCounts) {
	t.Helper()
	// SMS: 2 bekleyen, 1 yeniden denenecek, 3 gönderildi, 1 başarısız, 2 atlandı.
	for i := 0; i < 2; i++ {
		addMessageOn(t, db, id, "sms", "queued", 0, "")
	}
	addMessageOn(t, db, id, "sms", "queued", 2, "")
	for i := 0; i < 3; i++ {
		addMessageOn(t, db, id, "sms", "sent", 1, "")
	}
	addMessageOn(t, db, id, "sms", "failed", 5, worker.FailureExhausted)
	for i := 0; i < 2; i++ {
		addMessageOn(t, db, id, "sms", "skipped", 1, "")
	}
	sms = campaign.DeliveryCounts{Queued: 2, Retrying: 1, Sent: 3, Failed: 1, Skipped: 2}

	// E-posta: 1 bekleyen, 2 gönderiliyor, 4 gönderildi, 2 başarısız.
	addMessageOn(t, db, id, "email", "queued", 0, "")
	for i := 0; i < 2; i++ {
		addMessageOn(t, db, id, "email", "sending", 1, "")
	}
	for i := 0; i < 4; i++ {
		addMessageOn(t, db, id, "email", "sent", 1, "")
	}
	addMessageOn(t, db, id, "email", "failed", 1, worker.FailureRejected)
	addMessageOn(t, db, id, "email", "failed", 5, worker.FailureExhausted)
	email = campaign.DeliveryCounts{Queued: 1, Sending: 2, Sent: 4, Failed: 2}
	return sms, email
}

func TestDeliverySnapshotSplitsCountsByChannel(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	wantSMS, wantEmail := addMixed(t, db, id)

	got := snapshotOf(t, db, id)

	if got.Channels.SMS != wantSMS {
		t.Fatalf("SMS = %+v, beklenen %+v", got.Channels.SMS, wantSMS)
	}
	if got.Channels.Email != wantEmail {
		t.Fatalf("e-posta = %+v, beklenen %+v", got.Channels.Email, wantEmail)
	}
}

// Her durum için iki kanalın toplamı kampanyanın toplamına eşittir (tek sorgu, tutarlı görüntü).
func TestDeliverySnapshotChannelTotalsEqualCampaignTotals(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addMixed(t, db, id)

	got := snapshotOf(t, db, id)

	sum := campaign.DeliveryCounts{
		Queued:   got.Channels.SMS.Queued + got.Channels.Email.Queued,
		Retrying: got.Channels.SMS.Retrying + got.Channels.Email.Retrying,
		Sending:  got.Channels.SMS.Sending + got.Channels.Email.Sending,
		Sent:     got.Channels.SMS.Sent + got.Channels.Email.Sent,
		Failed:   got.Channels.SMS.Failed + got.Channels.Email.Failed,
		Skipped:  got.Channels.SMS.Skipped + got.Channels.Email.Skipped,
	}
	if sum != got.Messages {
		t.Fatalf("kanalların toplamı %+v, kampanya toplamı %+v", sum, got.Messages)
	}
	want := campaign.DeliveryCounts{Queued: 3, Retrying: 1, Sending: 2, Sent: 7, Failed: 3, Skipped: 2}
	if got.Messages != want {
		t.Fatalf("toplam = %+v, beklenen %+v", got.Messages, want)
	}
}

// Kampanya yalnızca bir kanala mesaj yazdıysa diğer kanal sıfırdır (satır yok ≠ hata).
func TestDeliverySnapshotChannelWithoutMessagesIsZero(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addMessageOn(t, db, id, "sms", "sent", 1, "")
	addMessageOn(t, db, id, "sms", "queued", 0, "")

	got := snapshotOf(t, db, id)

	if got.Channels.Email != (campaign.DeliveryCounts{}) {
		t.Fatalf("e-postada mesaj yok, sıfır bekleniyordu: %+v", got.Channels.Email)
	}
	if got.Channels.SMS != (campaign.DeliveryCounts{Queued: 1, Sent: 1}) || got.Messages != got.Channels.SMS {
		t.Fatalf("SMS toplamdır: %+v / %+v", got.Channels.SMS, got.Messages)
	}
}

// Başka kampanyanın mesajları kanal kırılımına karışmaz.
func TestDeliverySnapshotChannelSplitIsPerCampaign(t *testing.T) {
	db := testdb.Open(t)
	a := approvedCampaignWithStatus(t, db, "sending")
	b := approvedCampaignWithStatus(t, db, "sending")
	addMessageOn(t, db, a, "sms", "sent", 1, "")
	addMessageOn(t, db, b, "sms", "failed", 5, worker.FailureExhausted)
	addMessageOn(t, db, b, "email", "sent", 1, "")

	gotA, gotB := snapshotOf(t, db, a), snapshotOf(t, db, b)

	if gotA.Channels.SMS.Sent != 1 || gotA.Channels.SMS.Failed != 0 || gotA.Channels.Email != (campaign.DeliveryCounts{}) {
		t.Fatalf("A: %+v", gotA.Channels)
	}
	if gotB.Channels.SMS.Failed != 1 || gotB.Channels.Email.Sent != 1 || gotB.Channels.SMS.Sent != 0 {
		t.Fatalf("B: %+v", gotB.Channels)
	}
}

// Yalnızca gönderilmiş ve ikinci veya sonraki denemede gönderilmiş mesajlar "toparlanan"dır.
func TestDeliverySnapshotRecoveredCountsOnlySentMessagesThatNeededRetries(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	addMessageOn(t, db, id, "sms", "sent", 1, "")   // ilk denemede: toparlanan değil
	addMessageOn(t, db, id, "sms", "sent", 2, "")   // toparlanan
	addMessageOn(t, db, id, "email", "sent", 3, "") // toparlanan
	addMessageOn(t, db, id, "email", "sent", 5, "") // toparlanan (son deneme)
	// Başka durumlar attempts > 1 olsa da toparlanan değildir.
	addMessageOn(t, db, id, "email", "failed", 5, worker.FailureExhausted)
	addMessageOn(t, db, id, "email", "queued", 3, "")
	addMessageOn(t, db, id, "sms", "sending", 2, "")
	addMessageOn(t, db, id, "sms", "skipped", 2, "")

	got := snapshotOf(t, db, id)

	if got.Recovered != 3 {
		t.Fatalf("3 toparlanan bekleniyordu: %d", got.Recovered)
	}
	if got.Recovered > got.Messages.Sent {
		t.Fatalf("toparlanan (%d) gönderilenden (%d) fazla olamaz", got.Recovered, got.Messages.Sent)
	}
}

func TestDeliverySnapshotRecoveredIsZeroWhenEverythingWentThroughOnTheFirstAttempt(t *testing.T) {
	db := testdb.Open(t)
	id := approvedCampaignWithStatus(t, db, "sending")
	for i := 0; i < 3; i++ {
		addMessageOn(t, db, id, "sms", "sent", 1, "")
	}

	if got := snapshotOf(t, db, id).Recovered; got != 0 {
		t.Fatalf("toparlanan 0 olmalı: %d", got)
	}
}

func TestDeliverySnapshotRecoveredIsPerCampaign(t *testing.T) {
	db := testdb.Open(t)
	a := approvedCampaignWithStatus(t, db, "sending")
	b := approvedCampaignWithStatus(t, db, "sending")
	addMessageOn(t, db, a, "sms", "sent", 2, "")
	addMessageOn(t, db, a, "email", "sent", 4, "")
	addMessageOn(t, db, b, "sms", "sent", 1, "")

	if got := snapshotOf(t, db, a).Recovered; got != 2 {
		t.Fatalf("A: 2 toparlanan bekleniyordu: %d", got)
	}
	if got := snapshotOf(t, db, b).Recovered; got != 0 {
		t.Fatalf("B: 0 toparlanan bekleniyordu: %d", got)
	}
}
