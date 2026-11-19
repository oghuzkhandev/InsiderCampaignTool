package e2e

import (
	"testing"
)

// Ana idempotency kanıtı, gerçek süreçlerle: provider her anahtarın ilk kabul
// yanıtını düşürür, worker aynı anahtarla yeniden dener. Her mesaj için 2 HTTP
// isteği, tek kabul kaydı, sıfır tekrar teslimat ve sonunda `sent`.
func TestLostFirstResponsesAreRetriedWithTheSameKeyAcrossProcesses(t *testing.T) {
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, 5, "sms", "email")
	port := freePort(t)
	startProvider(t, dbURL, port, map[string]string{"MOCK_PROVIDER_MODE": "accept_then_drop"})
	startProc(t, "worker", workerEnv(dbURL, port, nil))

	awaitCond(t, "kampanya completed", func() bool { return campaignStatus(t, db, campaignID) == "completed" })

	if got := statusCounts(t, db, campaignID); got["sent"] != 10 || len(got) != 1 {
		t.Fatalf("10 mesajın hepsi sent olmalı: %v", got)
	}
	if n := count(t, db, `SELECT count(*) FROM messages WHERE attempts <> 2`); n != 0 {
		t.Fatalf("her mesaj tam 2 denemede (1 düşen yanıt + 1 yeniden deneme) gitmeli: %d farklı", n)
	}
	snap := providerStats(t, port)
	if snap.TotalRequests != 20 || snap.UniqueAccepts != 10 || snap.DuplicateRequests != 10 ||
		snap.ResponsesDropped != 10 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("20 istek, 10 kabul, 10 tekrar, 10 düşen yanıt, 0 tekrar teslimat: %+v", snap)
	}
	if n := count(t, db, `SELECT count(*) FROM mock_provider_acceptances`); n != 10 {
		t.Fatalf("10 kabul kaydı bekleniyordu: %d", n)
	}
	// Her mesajın anahtarıyla eşleşen tek bir kabul kaydı var.
	if n := count(t, db, `SELECT count(*) FROM messages m JOIN mock_provider_acceptances a
	                      ON a.idempotency_key = m.idempotency_key AND a.recipient = m.recipient`); n != 10 {
		t.Fatalf("her mesaj bir kabul kaydıyla eşleşmeli: %d", n)
	}
}

// İlk kabulden sonra provider SÜRECİ öldürülüp yeniden başlatılır: kabul kaydı
// PostgreSQL'de kalır, aynı anahtarla yeniden deneme ikinci teslimat oluşturmaz.
func TestProviderRestartAfterFirstAcceptanceDoesNotCreateSecondDelivery(t *testing.T) {
	db, dbURL := testDB(t)
	campaignID := seedCampaign(t, db, 1, "sms")
	port := freePort(t)

	// Birinci provider kabul eder ama hiçbir yanıtı iletmez: worker yeniden dener.
	first := startProvider(t, dbURL, port, map[string]string{"MOCK_PROVIDER_MODE": "drop_all_responses"})
	// Yeniden başlatma sırasında bağlantı hataları deneme hakkı harcar; bu test
	// deneme sınırını değil kabul kaydının kalıcılığını sınar.
	startProc(t, "worker", workerEnv(dbURL, port, map[string]string{"WORKER_MAX_ATTEMPTS": "500"}))

	awaitCond(t, "ilk kabul kaydedildi", func() bool {
		return count(t, db, `SELECT count(*) FROM mock_provider_acceptances`) == 1
	})
	var recorded string
	if err := db.QueryRow(`SELECT message_id::text FROM mock_provider_acceptances`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}

	first.kill() // provider çöktü (SIGKILL); bellekteki istatistikleri gitti
	startProvider(t, dbURL, port, nil)

	awaitCond(t, "mesaj sent oldu", func() bool { return statusCounts(t, db, campaignID)["sent"] == 1 })

	if n := count(t, db, `SELECT count(*) FROM mock_provider_acceptances`); n != 1 {
		t.Fatalf("yeniden başlatma ikinci bir kabul oluşturmamalı: %d kayıt", n)
	}
	var after string
	if err := db.QueryRow(`SELECT message_id::text FROM mock_provider_acceptances`).Scan(&after); err != nil || after != recorded {
		t.Fatalf("kabul kaydı korunmalı: %q → %q %v", recorded, after, err)
	}
	if n := count(t, db, `SELECT attempts FROM messages`); n < 2 {
		t.Fatalf("mesaj en az 2 denemede gitmiş olmalı: attempts=%d", n)
	}
	// Yeni süreç kabulü kendi belleğinde "ilk" görmez: kayıtlı sonuç tekrar isteğidir.
	snap := providerStats(t, port)
	if snap.UniqueAccepts != 0 || snap.DuplicateRequests < 1 || snap.DuplicateDeliveries != 0 || snap.ResponsesDropped != 0 {
		t.Fatalf("yeni provider: 0 yeni kabul, en az 1 tekrar isteği, 0 tekrar teslimat: %+v", snap)
	}
}
