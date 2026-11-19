package integration

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"context"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

// flakyCompletionRepo, kampanya tamamlama sorgusunun ilk failures çağrısını
// başarısız yapar; mesaj sonucu yazımı ve diğer işlemler gerçek PostgreSQL'e gider.
type flakyCompletionRepo struct {
	*worker.PGRepo
	failures atomic.Int32 // kalan başarısız çağrı sayısı
	calls    atomic.Int32
}

func (r *flakyCompletionRepo) CompleteCampaign(ctx context.Context, id string) (bool, error) {
	r.calls.Add(1)
	if r.failures.Add(-1) >= 0 {
		return false, errors.New("kampanya tamamlama sorgusu başarısız (test)")
	}
	return r.PGRepo.CompleteCampaign(ctx, id)
}

// Mesaj sonucu yazıldıktan sonra kampanya tamamlama sorgusu başarısız olursa
// kampanya `sending`'de kalmamalı: sonraki bir turda yeniden kontrol edilip
// `completed` yapılmalıdır. (Tamamlama kontrolü yalnızca sonuç yazan worker'ın tek
// seferlik çağrısına bağlıysa, başka mesaj kalmadığı için kimse tekrar denemez.)
func TestCampaignIsCompletedLaterWhenCompletionCheckFailsAfterResultWrite(t *testing.T) {
	db := testdb.Open(t)
	q := seedQueue(t, db, 1, "sms")
	srv, _ := startProvider(t, db, mockprovider.Config{})

	repo := &flakyCompletionRepo{PGRepo: worker.NewPGRepo(db)}
	repo.failures.Store(1) // ilk tamamlama kontrolü başarısız
	cfg := fastConfig()
	cfg.ReapInterval = time.Second // ara durumu gözlemlemek için
	w := startWorkerWithRepo(t, repo, httpProvider(srv), cfg)

	awaitCond(t, "mesaj sent oldu", func() bool { return statusCounts(t, db, q.campaignID)["sent"] == 1 })
	if repo.calls.Load() < 1 {
		t.Fatal("tamamlama kontrolü denenmiş olmalı")
	}
	// Kontrol başarısız oldu: kampanya şimdilik sending.
	if got := campaignStatus(t, db, q.campaignID); got != "sending" {
		t.Fatalf("başarısız kontrolün hemen ardından kampanya sending olmalı: %q", got)
	}

	// Bütün mesajlar terminal: sonraki bir turda kampanya completed olmalı.
	awaitCampaign(t, db, q.campaignID, "completed")
	w.stop()
	if got := statusCounts(t, db, q.campaignID); got["sent"] != 1 || len(got) != 1 {
		t.Fatalf("mesaj durumu değişmemeli: %v", got)
	}
}
