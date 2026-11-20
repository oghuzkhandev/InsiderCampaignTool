package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/database"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

func scheduledCampaign(t *testing.T, db *sql.DB) (http.Handler, string) {
	t.Helper()
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	r := apiPost(h, exampleBriefText)
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	id := decodeCampaign(t, r).ID
	r = apiDo(h, "PUT", "/api/campaigns/"+id+"/content", `{"sms_text":"Merhaba {{first_name}}","email_subject":"Konu","email_body":"Gövde"}`)
	if r.Code != 200 {
		t.Fatalf("content: %d %s", r.Code, r.Body)
	}
	return h, id
}

func dbTime(t *testing.T, db *sql.DB) time.Time {
	t.Helper()
	var at time.Time
	if err := db.QueryRow(`SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func approveScheduledBody(at time.Time) string {
	return fmt.Sprintf(`{"content_version":1,"scheduled_at":%q}`, at.Format(time.RFC3339Nano))
}

func TestScheduledDeliveryWaitsThenSendsAndReplaysAfterDueTime(t *testing.T) {
	db := testdb.Open(t)
	h, id := scheduledCampaign(t, db)
	at := dbTime(t, db).Add(2 * time.Second).Truncate(time.Microsecond)
	payload := approveScheduledBody(at)
	r := apiDo(h, "POST", approvePath(id), payload)
	if r.Code != 200 {
		t.Fatalf("approve: %d %s", r.Code, r.Body)
	}
	var original struct {
		ScheduledAt time.Time `json:"scheduled_at"`
		ApprovedAt  time.Time `json:"approved_at"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	if !original.ScheduledAt.Equal(at) {
		t.Fatalf("wrong schedule: %s", r.Body)
	}
	// An equivalent instant with a different UTC offset is the same plan.
	equivalent := apiDo(h, "POST", approvePath(id), approveScheduledBody(at.In(time.FixedZone("offset", 3*60*60))))
	if equivalent.Code != 200 {
		t.Fatalf("equivalent time rejected: %d %s", equivalent.Code, equivalent.Body)
	}
	var total, wrong int
	if err := db.QueryRow(`SELECT count(*),count(*) FILTER (WHERE next_attempt_at <> $2 OR attempts <> 0) FROM messages WHERE campaign_id=$1`, id, at).Scan(&total, &wrong); err != nil {
		t.Fatal(err)
	}
	if total == 0 || wrong != 0 {
		t.Fatalf("queue timestamps: total=%d wrong=%d", total, wrong)
	}
	repo := worker.NewPGRepo(db)
	for _, channel := range []string{"sms", "email"} {
		jobs, err := repo.Claim(t.Context(), channel, 10, 5)
		if err != nil || len(jobs) != 0 {
			t.Fatalf("claimed before due: %v %v", jobs, err)
		}
	}
	if !dbTime(t, db).Before(at) {
		t.Fatal("pre-due assertion did not actually run before due time")
	}
	stored, err := campaign.NewStore(db).Get(t.Context(), id)
	if err != nil || stored.ScheduledAt == nil || !stored.ScheduledAt.Equal(at) {
		t.Fatalf("GET schedule: %+v %v", stored, err)
	}
	snapshot, err := campaign.NewStore(db).DeliverySnapshot(t.Context(), id)
	if err != nil || snapshot.ScheduledAt == nil || snapshot.Messages.Queued != total {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	srv, _ := startProvider(t, db, mockprovider.Config{})
	// Worker starts after its target time, as after a restart/outage. No work may be lost.
	awaitCond(t, "database reached scheduled time", func() bool { return !dbTime(t, db).Before(at) })
	w := startWorker(t, db, httpProvider(srv), fastConfig())
	awaitCampaign(t, db, id, "completed")
	w.stop()
	if n := acceptanceCount(t, db); n != total {
		t.Fatalf("acceptances=%d want=%d", n, total)
	}
	var early int
	if err := db.QueryRow(`SELECT count(*) FROM mock_provider_acceptances WHERE accepted_at < $1`, at).Scan(&early); err != nil {
		t.Fatal(err)
	}
	if early != 0 {
		t.Fatalf("early provider accepts: %d", early)
	}
	replay := apiDo(h, "POST", approvePath(id), payload)
	if replay.Code != 200 {
		t.Fatalf("replay after due: %d %s", replay.Code, replay.Body)
	}
	var again struct {
		ApprovedAt time.Time `json:"approved_at"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if !again.ApprovedAt.Equal(original.ApprovedAt) || acceptanceCount(t, db) != total {
		t.Fatal("replay changed approval/delivery")
	}
	if r := apiDo(h, "POST", approvePath(id), approveScheduledBody(at.Add(time.Hour))); r.Code != 409 {
		t.Fatalf("time conflict: %d", r.Code)
	}
	if r := approvePost(h, id, 1); r.Code != 409 {
		t.Fatalf("immediate replay of scheduled approval: %d", r.Code)
	}
}

func TestScheduledApprovalRejectsPastAndRollsBackMessages(t *testing.T) {
	db := testdb.Open(t)
	h, id := scheduledCampaign(t, db)
	r := apiDo(h, "POST", approvePath(id), approveScheduledBody(dbTime(t, db).Add(-time.Minute)))
	if r.Code != 422 {
		t.Fatalf("past: %d %s", r.Code, r.Body)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 || campaignStatus(t, db, id) != "draft" {
		t.Fatal("invalid schedule changed campaign or queue")
	}
	r = apiDo(h, "POST", approvePath(id), `{"content_version":1,"scheduled_at":"2026-11-20T14:00"}`)
	if r.Code != 400 {
		t.Fatalf("timezone-free request: %d", r.Code)
	}
}

func TestScheduleMigrationRejectsUnsafeDownAndPreservesUnrelatedData(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	t.Cleanup(func() {
		if err := database.Migrate(context.Background(), db); err != nil {
			t.Error(err)
		}
	})
	h, id := scheduledCampaign(t, db)
	at := dbTime(t, db).Add(time.Hour)
	if r := apiDo(h, "POST", approvePath(id), approveScheduledBody(at)); r.Code != 200 {
		t.Fatalf("approve: %s", r.Body)
	}
	provider := newGooseProvider(t, db)
	if _, err := provider.DownTo(ctx, 6); err == nil {
		t.Fatal("Down erased unfinished schedule")
	}
	if _, err := db.Exec(`UPDATE campaigns SET scheduled_at=scheduled_at+interval '1 hour' WHERE id=$1`, id); err == nil {
		t.Fatal("approved schedule was mutable")
	}
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil || n != 3009 {
		t.Fatalf("migration changed users: %d %v", n, err)
	}
}

func TestConcurrentScheduledApprovalCreatesOneQueueAndFailureRollsBackSchedule(t *testing.T) {
	db := testdb.Open(t)
	h, id := scheduledCampaign(t, db)
	at := dbTime(t, db).Add(time.Hour).Truncate(time.Microsecond)
	// Inject a message-write failure: schedule/approval and every message must roll back together.
	if _, err := db.Exec(`CREATE FUNCTION zz_schedule_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test schedule write failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS zz_schedule_fail ON messages`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS zz_schedule_fail()`)
	})
	if _, err := db.Exec(`CREATE TRIGGER zz_schedule_fail BEFORE INSERT ON messages FOR EACH ROW EXECUTE FUNCTION zz_schedule_fail()`); err != nil {
		t.Fatal(err)
	}
	if r := apiDo(h, "POST", approvePath(id), approveScheduledBody(at)); r.Code != 500 {
		t.Fatalf("injected error: %d", r.Code)
	}
	stored, err := campaign.NewStore(db).Get(t.Context(), id)
	if err != nil || stored.Status != "draft" || stored.ScheduledAt != nil {
		t.Fatalf("partial schedule committed: %+v %v", stored, err)
	}
	if _, err := db.Exec(`DROP TRIGGER zz_schedule_fail ON messages`); err != nil {
		t.Fatal(err)
	}
	codes := make(chan int, 8)
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); codes <- apiDo(h, "POST", approvePath(id), approveScheduledBody(at)).Code }()
	}
	group.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent approval: %d", code)
		}
	}
	var total, unique int
	if err := db.QueryRow(`SELECT count(*),count(DISTINCT idempotency_key) FROM messages WHERE campaign_id=$1`, id).Scan(&total, &unique); err != nil {
		t.Fatal(err)
	}
	if total == 0 || total != unique {
		t.Fatalf("duplicate jobs: %d %d", total, unique)
	}
}
