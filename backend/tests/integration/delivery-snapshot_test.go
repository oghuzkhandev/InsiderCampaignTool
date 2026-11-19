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

// addMessage, belirli durumda tek bir mesaj ekler. Her mesaj kendi kullanıcısıdır.
func addMessage(t *testing.T, db *sql.DB, campaignID, status string, attempts int, failureReason string) {
	t.Helper()
	uid := mustUser(t, db)
	var reason any
	if failureReason != "" {
		reason = failureReason
	}
	_, err := db.Exec(
		`INSERT INTO messages (campaign_id, user_id, channel, recipient, first_name, idempotency_key,
		                       status, attempts, failure_reason, sent_at, claim_token, claimed_at)
		 VALUES ($1, $2, 'email', 'x@example.test', '', $3, $4::text, $5, $6,
		         CASE WHEN $4::text = 'sent' THEN now() END,
		         CASE WHEN $4::text = 'sending' THEN gen_random_uuid() END,
		         CASE WHEN $4::text = 'sending' THEN now() END)`,
		campaignID, uid, idemKey(campaignID, uid, "email"), status, attempts, reason)
	if err != nil {
		t.Fatalf("mesaj eklenemedi (%s): %v", status, err)
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
	want := campaign.DeliverySnapshot{
		Status:   "sending",
		Messages: campaign.DeliveryCounts{Queued: 2, Retrying: 1, Sending: 1, Sent: 3, Failed: 3, Skipped: 1},
		Failures: campaign.FailureCounts{Exhausted: 1, Rejected: 1},
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
