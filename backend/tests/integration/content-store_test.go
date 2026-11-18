package integration

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

var goodContent = content.Content{SMSText: goodSMS, EmailSubject: goodSubject, EmailBody: goodBody}

func countContents(t *testing.T, db *sql.DB, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_contents WHERE campaign_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestContentStoreSavesNumberedVersionsAndReadsLatest(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewContentStore(db)
	ctx := context.Background()
	id := newCampaign(t, db)

	if _, found, err := store.Latest(ctx, id); err != nil || found {
		t.Fatalf("sürüm yokken (zero, false, nil) beklenir: %v %v", found, err)
	}

	sources := []string{campaign.SourceLLM, campaign.SourceLLM, campaign.SourceHuman}
	for i, src := range sources {
		c := content.Content{SMSText: "sms " + src, EmailSubject: "konu", EmailBody: "gövde ü😀"}
		c.SMSText += string(rune('a' + i))
		v, err := store.Save(ctx, id, src, c)
		if err != nil {
			t.Fatal(err)
		}
		if v.Version != i+1 || v.Source != src || v.Content != c || v.CreatedAt.IsZero() {
			t.Fatalf("sürüm %d: %+v", i+1, v)
		}
	}

	latest, found, err := store.Latest(ctx, id)
	if err != nil || !found {
		t.Fatalf("son sürüm bulunmalı: %v %v", found, err)
	}
	if latest.Version != 3 || latest.Source != campaign.SourceHuman || latest.Content.EmailBody != "gövde ü😀" {
		t.Fatalf("son sürüm = %+v", latest)
	}

	// Başka kampanyanın sürümleri ayrı numaralanır.
	other := newCampaign(t, db)
	if v, err := store.Save(ctx, other, campaign.SourceLLM, goodContent); err != nil || v.Version != 1 {
		t.Fatalf("başka kampanya 1'den başlamalı: %+v %v", v, err)
	}
}

func TestContentStoreUnknownCampaign(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewContentStore(db)
	ctx := context.Background()

	for _, id := range []string{
		"00000000-0000-0000-0000-000000000000",
		"abc",
		"1; DROP TABLE campaign_contents",
		"",
	} {
		if _, err := store.Save(ctx, id, campaign.SourceLLM, goodContent); !errors.Is(err, campaign.ErrNotFound) {
			t.Fatalf("%q için ErrNotFound bekleniyordu: %v", id, err)
		}
		if _, found, err := store.Latest(ctx, id); err != nil || found {
			t.Fatalf("%q için Latest (false, nil) beklenir: %v %v", id, found, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_contents`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("tablo boş kalmalı ve bozulmamalı: %d %v", n, err)
	}
}

func TestContentStoreRefusesNonDraftCampaigns(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewContentStore(db)
	ctx := context.Background()

	for _, state := range []string{"approved", "sending", "completed"} {
		t.Run(state, func(t *testing.T) {
			id := newCampaign(t, db)
			mustAddContent(t, db, id, 1)
			mustApprove(t, db, id, 1)
			switch state {
			case "sending":
				if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, id); err != nil {
					t.Fatal(err)
				}
			case "completed":
				if _, err := db.Exec(`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
					t.Fatal(err)
				}
			}
			for _, src := range []string{campaign.SourceLLM, campaign.SourceHuman} {
				if _, err := store.Save(ctx, id, src, goodContent); !errors.Is(err, campaign.ErrNotDraft) {
					t.Fatalf("%s/%s: ErrNotDraft bekleniyordu: %v", state, src, err)
				}
			}
			if n := countContents(t, db, id); n != 1 {
				t.Fatalf("yeni sürüm yazılmamalı: %d", n)
			}
		})
	}
}

func TestContentStoreRejectsUnknownSourceAndDatabaseLevelInvalidContent(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewContentStore(db)
	ctx := context.Background()
	id := newCampaign(t, db)

	if _, err := store.Save(ctx, id, "bot", goodContent); err == nil || errors.Is(err, campaign.ErrNotDraft) || errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("bilinmeyen kaynak genel hata olmalı: %v", err)
	}
	// Doğrulayıcıdan geçmemiş içerik veritabanının ikinci savunmasına takılır ve
	// ErrNotDraft gibi yanlış bir duruma çevrilmez.
	bad := content.Content{SMSText: "{{credit_card}}", EmailSubject: "k", EmailBody: "g"}
	if _, err := store.Save(ctx, id, campaign.SourceLLM, bad); err == nil || errors.Is(err, campaign.ErrNotDraft) {
		t.Fatalf("DB kısıtı hatası bekleniyordu: %v", err)
	}
	if n := countContents(t, db, id); n != 0 {
		t.Fatalf("hiçbir sürüm yazılmamalı: %d", n)
	}
	// Başarısız denemeler sürüm numarası tüketmez.
	if v, err := store.Save(ctx, id, campaign.SourceLLM, goodContent); err != nil || v.Version != 1 {
		t.Fatalf("ilk başarılı sürüm 1 olmalı: %+v %v", v, err)
	}
}

// Eşzamanlı yazımlar kilit altında kesintisiz ve benzersiz sürüm alır.
func TestContentStoreConcurrentSavesGetContiguousVersions(t *testing.T) {
	db := testdb.Open(t)
	db.SetMaxOpenConns(40)
	store := campaign.NewContentStore(db)
	ctx := context.Background()
	id := newCampaign(t, db)

	const writers = 24
	start := make(chan struct{})
	versions := make(chan int, writers)
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := store.Save(ctx, id, campaign.SourceHuman, goodContent)
			if err != nil {
				errs <- err
				return
			}
			versions <- v.Version
		}()
	}
	close(start)
	wg.Wait()
	close(versions)
	close(errs)

	for err := range errs {
		t.Fatalf("eşzamanlı yazım başarısız: %v", err)
	}
	var got []int
	for v := range versions {
		got = append(got, v)
	}
	slices.Sort(got)
	for i, v := range got {
		if v != i+1 {
			t.Fatalf("sürümler 1..%d kesintisiz ve benzersiz olmalı: %v", writers, got)
		}
	}
	if len(got) != writers {
		t.Fatalf("%d sürüm bekleniyordu: %v", writers, got)
	}
}

// Onay kampanya satırını kilitlemişken gelen Save beklemeli, onay commit olunca
// ErrNotDraft dönmelidir (uygulama kilidi).
func TestContentStoreSaveWaitsForApproval(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewContentStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTestTimeout)
	defer cancel()

	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)

	approver := begin(t, ctx, db)
	var status string
	if err := approver.QueryRowContext(ctx, lockQuery, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if _, err := approver.ExecContext(ctx, approveSQL, id, 1); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := store.Save(ctx, id, campaign.SourceHuman, goodContent)
		done <- err
	}()
	waitForLockWaiters(t, ctx, db, lockQuery, 1)
	notDone(t, done, "Save")

	if err := approver.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, done, "Save"); !errors.Is(err, campaign.ErrNotDraft) {
		t.Fatalf("onaydan sonra ErrNotDraft bekleniyordu: %v", err)
	}
	if n := countContents(t, db, id); n != 1 {
		t.Fatalf("yalnızca onaylanan sürüm kalmalı: %d", n)
	}
}

func TestContentStoreMaxFirstNameRunes(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewContentStore(db)
	ctx := context.Background()

	if n, err := store.MaxFirstNameRunes(ctx); err != nil || n != 0 {
		t.Fatalf("kullanıcı yokken 0 beklenir: %d %v", n, err)
	}

	add := func(email string, name any) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO users (email, city, first_name) VALUES ($1, 'İstanbul', $2)`, email, name); err != nil {
			t.Fatal(err)
		}
	}
	add("a@example.test", nil)
	add("b@example.test", "   ")
	if n, _ := store.MaxFirstNameRunes(ctx); n != 0 {
		t.Fatalf("ad yoksa veya boşluksa 0 beklenir: %d", n)
	}

	add("c@example.test", "Ali")
	add("d@example.test", "Çağşü")    // 5 kod noktası, 10 bayt
	add("e@example.test", " Zeynep ") // kenar boşlukları sayılmaz: 6
	if n, _ := store.MaxFirstNameRunes(ctx); n != 6 {
		t.Fatalf("en uzun ad 6 kod nokta olmalı: %d", n)
	}
	add("f@example.test", "😀😀😀😀😀😀😀") // 7 kod noktası, 28 bayt
	if n, _ := store.MaxFirstNameRunes(ctx); n != 7 {
		t.Fatalf("bayt değil kod noktası sayılmalı: %d", n)
	}
}
