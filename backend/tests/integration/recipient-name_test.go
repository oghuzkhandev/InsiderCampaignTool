package integration

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/mockprovider"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/worker"
)

// Bu dosya ADR-026'yı sınar: onayda render için kullanılan ad mesaja kopyalanır;
// worker retry dahil her denemede bu kayıtlı adı kullanır. Kullanıcının adı onaydan
// sonra değişse bile aynı idempotency anahtarıyla aynı içerik gider.

// approveRecipients, kullanıcıları ve bir kampanyayı yazar ve onayı GERÇEK onay
// koduyla (API → Approver) yapar; mesajlar ve adlar onayın yazdığı hâlidir.
func approveRecipients(t *testing.T, db *sql.DB, rs []recipientFixture) string {
	t.Helper()
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1) // SMS: "Merhaba {{first_name}}, %30 indirim seni bekliyor!"
	addRecipients(t, db, rs)
	mustApproveOK(t, h, id, 1)
	return id
}

func messageFirstName(t *testing.T, db *sql.DB, email, channel string) string {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT m.first_name FROM messages m JOIN users u ON u.id = m.user_id
	                    WHERE u.email = $1 AND m.channel = $2`, email, channel).Scan(&name)
	if err != nil {
		t.Fatalf("%s/%s mesajı okunamadı: %v", email, channel, err)
	}
	return name
}

// Onay, render için kullanılan adı HAM olarak (boşluk ve kontrol karakterleri
// dahil; ad yoksa boş metin) her kanalın mesajına kopyalar. Sonraki ad değişimi
// kopyayı etkilemez.
func TestApproveCopiesRawRecipientNameIntoEveryMessage(t *testing.T) {
	db := testdb.Open(t)
	id := approveRecipients(t, db, []recipientFixture{
		{email: "ayse@example.test", name: str("Ayşe"), phone: str("+905550000001"), sms: "granted", mail: "granted"},
		{email: "bosluk@example.test", name: str("   "), phone: str("+905550000002"), sms: "granted", mail: "granted"},
		{email: "bos@example.test", name: str(""), phone: str("+905550000003"), sms: "granted", mail: "none"},
		{email: "yok@example.test", phone: str("+905550000004"), sms: "granted", mail: "granted"},
		{email: "kontrol@example.test", name: str("Ali\tVeli"), phone: str("+905550000005"), sms: "granted", mail: "none"},
	})

	for _, tc := range []struct{ email, channel, want string }{
		{"ayse@example.test", "sms", "Ayşe"},
		{"ayse@example.test", "email", "Ayşe"},
		{"bosluk@example.test", "sms", "   "}, // ham kopya: fallback render'da uygulanır
		{"bosluk@example.test", "email", "   "},
		{"bos@example.test", "sms", ""},
		{"yok@example.test", "sms", ""}, // NULL → boş metin
		{"yok@example.test", "email", ""},
		{"kontrol@example.test", "sms", "Ali\tVeli"},
	} {
		if got := messageFirstName(t, db, tc.email, tc.channel); got != tc.want {
			t.Errorf("%s/%s: first_name %q, beklenen %q", tc.email, tc.channel, got, tc.want)
		}
	}

	// Onaydan sonra kullanıcı adı değişir: mesajdaki kopya değişmez.
	if _, err := db.Exec(`UPDATE users SET first_name = 'Değişti'`); err != nil {
		t.Fatal(err)
	}
	if got := messageFirstName(t, db, "ayse@example.test", "sms"); got != "Ayşe" {
		t.Fatalf("kopya kullanıcı adından bağımsız kalmalı: %q", got)
	}
	if n := messageCount(t, db); n == 0 || id == "" {
		t.Fatal("hazırlık: mesajlar oluşmalı")
	}
}

// Worker adı users'tan değil, mesajın kendi kaydından okur: kullanıcı adı onaydan
// sonra değişse de claim edilen işin adı onay anındakidir.
func TestClaimReadsFirstNameFromTheMessageNotFromUsers(t *testing.T) {
	db := testdb.Open(t)
	approveRecipients(t, db, []recipientFixture{
		{email: "ayse@example.test", name: str("Ayşe"), phone: str("+905550000001"), sms: "granted", mail: "none"},
	})
	if _, err := db.Exec(`UPDATE users SET first_name = 'Sonradan Değişen'`); err != nil {
		t.Fatal(err)
	}

	jobs := mustClaim(t, worker.NewPGRepo(db), "sms", 10)
	if len(jobs) != 1 || jobs[0].FirstName != "Ayşe" {
		t.Fatalf("iş onay anındaki adı taşımalı: %+v", jobs)
	}
}

// sentRecord, provider'a giden bir denemenin özetidir.
type sentRecord struct {
	key      string
	smsText  string
	attempts int
}

// observingProvider, her denemeyi kaydeder; renameAfterFirst doluysa ilk denemeden
// SONRA kullanıcının adını değiştirir (retry arasında ad değişimi taklidi).
type observingProvider struct {
	inner worker.Provider
	db    *sql.DB

	renameAfterFirst string
	once             sync.Once

	mu      sync.Mutex
	records map[string][]sentRecord // alıcı → denemeler
}

func (p *observingProvider) Send(ctx context.Context, j worker.Job, c content.Content) worker.SendResult {
	p.mu.Lock()
	if p.records == nil {
		p.records = map[string][]sentRecord{}
	}
	p.records[j.Recipient] = append(p.records[j.Recipient], sentRecord{j.IdempotencyKey, c.SMSText, j.Attempts})
	p.mu.Unlock()

	res := p.inner.Send(ctx, j, c)
	if p.renameAfterFirst != "" {
		p.once.Do(func() {
			_, _ = p.db.ExecContext(ctx, `UPDATE users SET first_name = $1`, p.renameAfterFirst)
		})
	}
	return res
}

func (p *observingProvider) forRecipient(recipient string) []sentRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]sentRecord(nil), p.records[recipient]...)
}

// Onaydan sonra kullanıcının adı değişse de provider'a onay anındaki ad gider.
func TestProviderReceivesTheNameFromApprovalTimeEvenIfUserIsRenamedLater(t *testing.T) {
	db := testdb.Open(t)
	id := approveRecipients(t, db, []recipientFixture{
		{email: "ayse@example.test", name: str("Ayşe"), phone: str("+905550000001"), sms: "granted", mail: "none"},
	})
	if _, err := db.Exec(`UPDATE users SET first_name = 'Yeni Ad'`); err != nil {
		t.Fatal(err)
	}
	srv, _ := startProvider(t, db, mockprovider.Config{})
	obs := &observingProvider{inner: httpProvider(srv), db: db}
	w := startWorker(t, db, obs, fastConfig())

	awaitCampaign(t, db, id, "completed")
	w.stop()

	got := obs.forRecipient("+905550000001")
	if len(got) != 1 || got[0].smsText != "Merhaba Ayşe, %30 indirim seni bekliyor!" {
		t.Fatalf("onay anındaki ad gönderilmeli: %+v", got)
	}
}

// Asıl risk: provider ilk isteği KABUL EDER ama yanıtı kaybolur; ad değişir; retry
// aynı idempotency anahtarıyla gelir. İçerik değişseydi provider 422 ile reddeder ve
// kabul edilmiş mesaj yanlışlıkla failed/rejected olurdu. Kayıtlı ad sayesinde
// retry birebir aynı içerikle gider, provider kayıtlı sonucu döndürür ve mesaj sent olur.
func TestRetryAfterLostResponseSendsSameContentEvenIfUserIsRenamedBetweenAttempts(t *testing.T) {
	db := testdb.Open(t)
	id := approveRecipients(t, db, []recipientFixture{
		{email: "ayse@example.test", name: str("Ayşe"), phone: str("+905550000001"), sms: "granted", mail: "none"},
	})
	srv, prov := startProvider(t, db, mockprovider.Config{Mode: mockprovider.ModeAcceptThenDrop})
	obs := &observingProvider{inner: httpProvider(srv), db: db, renameAfterFirst: "Başka Ad"}
	w := startWorker(t, db, obs, fastConfig())

	awaitCampaign(t, db, id, "completed")
	w.stop()

	got := obs.forRecipient("+905550000001")
	if len(got) != 2 {
		t.Fatalf("2 deneme bekleniyordu (ilk yanıt kayboldu): %+v", got)
	}
	if got[0].key != got[1].key || got[0].smsText != got[1].smsText {
		t.Fatalf("retry aynı anahtar ve aynı içerikle gitmeli:\n 1. %+v\n 2. %+v", got[0], got[1])
	}
	if got[1].smsText != "Merhaba Ayşe, %30 indirim seni bekliyor!" {
		t.Fatalf("retry kayıtlı adı kullanmalı, kullanıcının yeni adını değil: %q", got[1].smsText)
	}

	var status, reason string
	var attempts int
	if err := db.QueryRow(`SELECT status, coalesce(failure_reason, ''), attempts FROM messages`).Scan(&status, &reason, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || reason != "" || attempts != 2 {
		t.Fatalf("kabul edilmiş mesaj sent olmalı (rejected değil): status=%s reason=%q attempts=%d", status, reason, attempts)
	}
	snap := prov.Stats().Snapshot()
	if snap.KeyReuseRejected != 0 || snap.UniqueAccepts != 1 || snap.DuplicateRequests != 1 || snap.DuplicateDeliveries != 0 {
		t.Fatalf("anahtar çakışması olmamalı; 1 kabul, 1 tekrar isteği: %+v", snap)
	}
	if got := acceptanceCount(t, db); got != 1 {
		t.Fatalf("tek kabul kaydı bekleniyordu: %d", got)
	}
}

// Boş, yalnızca boşluk veya hiç olmayan ad eskisi gibi fallback'e gider; kontrol
// karakterleri boşluğa çevrilir. Kullanıcıya onaydan SONRA bir ad verilse de mesaj
// onay anındaki duruma (fallback) sadık kalır.
func TestFallbackAndNameNormalizationStayTheSameAndIgnoreLaterNames(t *testing.T) {
	db := testdb.Open(t)
	id := approveRecipients(t, db, []recipientFixture{
		{email: "yok@example.test", phone: str("+905550000001"), sms: "granted", mail: "none"},
		{email: "bos@example.test", name: str(""), phone: str("+905550000002"), sms: "granted", mail: "none"},
		{email: "bosluk@example.test", name: str(" \t "), phone: str("+905550000003"), sms: "granted", mail: "none"},
		{email: "kontrol@example.test", name: str("Ali\tVeli"), phone: str("+905550000004"), sms: "granted", mail: "none"},
		{email: "normal@example.test", name: str("  Zeynep  "), phone: str("+905550000005"), sms: "granted", mail: "none"},
	})
	// Adı olmayanlara onaydan sonra ad verilir: mesajlar yine fallback ile gitmeli.
	if _, err := db.Exec(`UPDATE users SET first_name = 'Sonradan' WHERE email IN ('yok@example.test', 'bos@example.test', 'bosluk@example.test')`); err != nil {
		t.Fatal(err)
	}
	srv, _ := startProvider(t, db, mockprovider.Config{})
	obs := &observingProvider{inner: httpProvider(srv), db: db}
	w := startWorker(t, db, obs, fastConfig())

	awaitCampaign(t, db, id, "completed")
	w.stop()

	fallback := "Merhaba " + content.FallbackName + ", %30 indirim seni bekliyor!"
	for phone, want := range map[string]string{
		"+905550000001": fallback,                                       // ad yok (NULL)
		"+905550000002": fallback,                                       // boş
		"+905550000003": fallback,                                       // yalnızca boşluk/sekme
		"+905550000004": "Merhaba Ali Veli, %30 indirim seni bekliyor!", // kontrol karakteri boşluğa
		"+905550000005": "Merhaba Zeynep, %30 indirim seni bekliyor!",   // kenar boşlukları atılır
	} {
		got := obs.forRecipient(phone)
		if len(got) != 1 || got[0].smsText != want {
			t.Errorf("%s: gönderilen %+v, beklenen %q", phone, got, want)
		}
	}
}
