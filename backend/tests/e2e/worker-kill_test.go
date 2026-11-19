package e2e

import (
	"strings"
	"testing"
	"time"
)

// Worker gönderim sırasında SIGKILL ile öldürülür: iş `sending`'de kalır, lease
// dolunca ikinci bir worker reaper ile işi kuyruğa alır ve aynı anahtarla
// gönderir. Sonunda her mesaj tek kabul kaydıyla `sent` olur.
func TestWorkerKilledMidSendIsRecoveredWithoutDuplicateDelivery(t *testing.T) {
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, 3, "sms")
	port := freePort(t)
	// Gecikme, worker'ın çağrının ortasında öldürülmesini sağlar.
	startProvider(t, dbURL, port, map[string]string{"MOCK_PROVIDER_LATENCY": "1s"})

	a := startProc(t, "worker", workerEnv(dbURL, port, nil))
	awaitCond(t, "3 mesaj gönderim sırasında (sending)", func() bool {
		return statusCounts(t, db, campaignID)["sending"] == 3
	})
	a.kill() // çökme: sonuç yazılmadı, çıkış temizliği yok

	if got := statusCounts(t, db, campaignID); got["sending"] != 3 {
		t.Fatalf("çöken worker'ın işleri sending'de kalmalı: %v", got)
	}

	startProc(t, "worker", workerEnv(dbURL, port, nil)) // kurtarma worker'ı
	awaitCond(t, "kampanya completed", func() bool { return campaignStatus(t, db, campaignID) == "completed" })

	if got := statusCounts(t, db, campaignID); got["sent"] != 3 || len(got) != 1 {
		t.Fatalf("3 mesajın hepsi sent olmalı: %v", got)
	}
	// 1. claim çöken worker'ın, 2. claim kurtarmanın.
	if n := count(t, db, `SELECT count(*) FROM messages WHERE attempts <> 2`); n != 0 {
		t.Fatalf("her mesaj 2 claim ile sonuçlanmalı: %d farklı", n)
	}
	if n := count(t, db, `SELECT count(*) FROM mock_provider_acceptances`); n != 3 {
		t.Fatalf("3 kabul kaydı bekleniyordu: %d", n)
	}
	snap := providerStats(t, port)
	if snap.UniqueAccepts != 3 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("3 kabul, 0 tekrar teslimat: %+v", snap)
	}
}

// SIGTERM: worker yeni iş sahiplenmeyi durdurur, devam eden çağrıları tamamlar ve
// çıkış koduyla temiz kapanır.
func TestWorkerSIGTERMStopsClaimingAndFinishesInFlightCalls(t *testing.T) {
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, 10, "sms")
	port := freePort(t)
	startProvider(t, dbURL, port, map[string]string{"MOCK_PROVIDER_LATENCY": "700ms"})

	w := startProc(t, "worker", workerEnv(dbURL, port, map[string]string{
		"WORKER_CAPACITY":         "2",
		"WORKER_LEASE":            "10s",
		"WORKER_SHUTDOWN_TIMEOUT": "5s",
	}))
	awaitCond(t, "iki mesaj gönderim sırasında", func() bool {
		return statusCounts(t, db, campaignID)["sending"] == 2
	})

	if code := w.terminate(t, 8*time.Second); code != 0 {
		t.Fatalf("temiz kapanış çıkış kodu 0 olmalı: %d\n%s", code, w.out.String())
	}

	got := statusCounts(t, db, campaignID)
	if got["sent"] != 2 || got["queued"] != 8 || len(got) != 2 {
		t.Fatalf("devam eden 2 çağrı tamamlanmalı, kalan 8 iş sahiplenilmemeli: %v", got)
	}
	if n := count(t, db, `SELECT count(*) FROM messages WHERE status = 'queued' AND attempts <> 0`); n != 0 {
		t.Fatalf("kapanıştan sonra yeni claim olmamalı: %d iş denendi", n)
	}
	if got := campaignStatus(t, db, campaignID); got != "sending" {
		t.Fatalf("kuyrukta iş varken kampanya completed olmamalı: %q", got)
	}
	if !strings.Contains(w.out.String(), "worker durdu") {
		t.Fatalf("worker kapanışı loglamalı:\n%s", w.out.String())
	}
}

// SIGTERM'den sonra kapanış süresi dolarsa takılan çağrılar iptal edilir, sonuç
// yazılmaz ve iş `sending`'de kalır; lease dolunca başka bir worker aynı anahtarla
// gönderir.
func TestWorkerSIGTERMTimeoutLeavesHungCallsToLeaseRecovery(t *testing.T) {
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, 2, "sms")
	slowPort, fastPort := freePort(t), freePort(t)
	startProvider(t, dbURL, slowPort, map[string]string{"MOCK_PROVIDER_LATENCY": "30s"}) // yanıt gelmiyor
	startProvider(t, dbURL, fastPort, nil)

	short := map[string]string{"WORKER_LEASE": "2s", "WORKER_SHUTDOWN_TIMEOUT": "300ms"}
	a := startProc(t, "worker", workerEnv(dbURL, slowPort, short))
	awaitCond(t, "iki mesaj gönderim sırasında", func() bool {
		return statusCounts(t, db, campaignID)["sending"] == 2
	})

	begin := time.Now()
	if code := a.terminate(t, 5*time.Second); code != 0 {
		t.Fatalf("çıkış kodu 0 olmalı: %d\n%s", code, a.out.String())
	}
	if d := time.Since(begin); d < 250*time.Millisecond {
		t.Fatalf("worker kapanış süresini (300ms) beklemeden kapandı: %s", d)
	}
	if got := statusCounts(t, db, campaignID); got["sending"] != 2 {
		t.Fatalf("iptal edilen çağrıların sonucu yazılmamalı, işler sending'de kalmalı: %v", got)
	}

	// İkinci worker hızlı provider'a bağlanır; lease dolunca işleri kurtarır.
	startProc(t, "worker", workerEnv(dbURL, fastPort, short))
	awaitCond(t, "kampanya completed", func() bool { return campaignStatus(t, db, campaignID) == "completed" })

	if got := statusCounts(t, db, campaignID); got["sent"] != 2 || len(got) != 1 {
		t.Fatalf("2 mesaj sent olmalı: %v", got)
	}
	if n := count(t, db, `SELECT count(*) FROM messages WHERE attempts <> 2`); n != 0 {
		t.Fatalf("her mesaj 2 claim ile sonuçlanmalı: %d farklı", n)
	}
	if n := count(t, db, `SELECT count(*) FROM mock_provider_acceptances`); n != 2 {
		t.Fatalf("2 kabul kaydı bekleniyordu: %d", n)
	}
}
