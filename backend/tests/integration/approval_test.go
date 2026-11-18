package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

func approvePath(id string) string { return "/api/campaigns/" + id + "/approve" }

func approvePost(h http.Handler, id string, version int) *httptest.ResponseRecorder {
	return apiDo(h, http.MethodPost, approvePath(id), `{"content_version":`+strconv.Itoa(version)+`}`)
}

type countsJSON struct {
	SMS   int `json:"sms"`
	Email int `json:"email"`
	Total int `json:"total"`
}

type approveJSON struct {
	ID                     string     `json:"id"`
	Status                 string     `json:"status"`
	ApprovedContentVersion int        `json:"approved_content_version"`
	ApprovedAt             string     `json:"approved_at"`
	Queued                 countsJSON `json:"queued"`
}

func mustApproveOK(t *testing.T, h http.Handler, id string, version int) approveJSON {
	t.Helper()
	rec := approvePost(h, id, version)
	if rec.Code != http.StatusOK {
		t.Fatalf("onay = %d\n%s", rec.Code, rec.Body.String())
	}
	var r approveJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func wantApproveError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	got, _ := decodeAPIError(t, rec)
	if rec.Code != status || got != code {
		t.Fatalf("beklenen %d %s, gelen %d %s\n%s", status, code, rec.Code, got, rec.Body.String())
	}
}

// recipientFixture, onay testleri için bir kullanıcı ve izin durumlarıdır.
type recipientFixture struct {
	email string
	city  string // boşsa İstanbul
	name  *string
	phone *string
	sms   string // "granted", "denied" veya "none" (kayıt yok)
	mail  string
}

func str(s string) *string { return &s }

func addRecipients(t *testing.T, db *sql.DB, rs []recipientFixture) map[string]int64 {
	t.Helper()
	ids := map[string]int64{}
	for _, r := range rs {
		city := r.city
		if city == "" {
			city = "İstanbul"
		}
		var id int64
		if err := db.QueryRow(`INSERT INTO users (email, city, first_name, phone) VALUES ($1, $2, $3, $4) RETURNING id`,
			r.email, city, r.name, r.phone).Scan(&id); err != nil {
			t.Fatalf("%s: %v", r.email, err)
		}
		ids[r.email] = id
		for channel, state := range map[string]string{"sms": r.sms, "email": r.mail} {
			switch state {
			case "granted", "denied":
				if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, $2, $3)`, id, channel, state == "granted"); err != nil {
					t.Fatal(err)
				}
			case "none", "":
			default:
				t.Fatalf("bilinmeyen izin durumu %q", state)
			}
		}
	}
	return ids
}

type messageRow struct {
	UserEmail, Channel, Recipient, Status, Key string
	Attempts                                   int
	Claimed                                    bool
}

func messageRows(t *testing.T, db *sql.DB, campaignID string) []messageRow {
	t.Helper()
	rows, err := db.Query(`SELECT u.email, m.channel, m.recipient, m.status, m.idempotency_key, m.attempts, m.claim_token IS NOT NULL
		FROM messages m JOIN users u ON u.id = m.user_id WHERE m.campaign_id = $1 ORDER BY u.email, m.channel`, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []messageRow
	for rows.Next() {
		var m messageRow
		if err := rows.Scan(&m.UserEmail, &m.Channel, &m.Recipient, &m.Status, &m.Key, &m.Attempts, &m.Claimed); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func messageCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type campaignState2 struct {
	Status  string
	Version sql.NullInt32
	At      sql.NullTime
}

func approvalState(t *testing.T, db *sql.DB, id string) campaignState2 {
	t.Helper()
	var s campaignState2
	if err := db.QueryRow(`SELECT status, approved_content_version, approved_at FROM campaigns WHERE id = $1`, id).Scan(&s.Status, &s.Version, &s.At); err != nil {
		t.Fatal(err)
	}
	return s
}

func wantDraftUntouched(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if s := approvalState(t, db, id); s.Status != "draft" || s.Version.Valid || s.At.Valid {
		t.Fatalf("kampanya draft ve onaysız kalmalı: %+v", s)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM messages WHERE campaign_id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("hiçbir mesaj oluşmamalı: %d %v", n, err)
	}
}

// Kanal bazında yalnızca ilgili kanala açıkça izin vermiş ve adresi olan
// alıcılar için `queued` mesaj oluşur; her şey onay ile birlikte yazılır.
func TestApproveQueuesMessagesPerEligibleRecipientAndChannel(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)

	addRecipients(t, db, []recipientFixture{
		{email: "iki-kanal@example.test", name: str("Ayşe"), phone: str("+905550000001"), sms: "granted", mail: "granted"},
		{email: "yalniz-sms@example.test", phone: str("+905550000002"), sms: "granted", mail: "denied"},
		{email: "yalniz-eposta@example.test", phone: str("+905550000003"), sms: "none", mail: "granted"},
		{email: "reddedildi@example.test", phone: str("+905550000004"), sms: "denied", mail: "denied"},
		{email: "kayit-yok@example.test", phone: str("+905550000005"), sms: "none", mail: "none"},
		{email: "telefonsuz@example.test", sms: "granted", mail: "granted"},
		{email: "telefon-bos@example.test", phone: str("  "), sms: "granted", mail: "none"},
		{email: "baska-sehir@example.test", city: "Ankara", phone: str("+905550000006"), sms: "granted", mail: "granted"},
	})

	res := mustApproveOK(t, h, id, 1)

	if res.ID != id || res.Status != "approved" || res.ApprovedContentVersion != 1 || res.ApprovedAt == "" {
		t.Fatalf("yanıt = %+v", res)
	}
	if res.Queued != (countsJSON{SMS: 2, Email: 3, Total: 5}) {
		t.Fatalf("kuyruğa alınanlar = %+v", res.Queued)
	}
	// İzinli ama telefonu olmayan (yok/boşluk) iki kişi SMS'e uygun değildir
	// (ADR-023 B): ne sayıya ne kuyruğa girerler.

	want := []messageRow{
		{UserEmail: "iki-kanal@example.test", Channel: "email", Recipient: "iki-kanal@example.test"},
		{UserEmail: "iki-kanal@example.test", Channel: "sms", Recipient: "+905550000001"},
		{UserEmail: "telefonsuz@example.test", Channel: "email", Recipient: "telefonsuz@example.test"},
		{UserEmail: "yalniz-eposta@example.test", Channel: "email", Recipient: "yalniz-eposta@example.test"},
		{UserEmail: "yalniz-sms@example.test", Channel: "sms", Recipient: "+905550000002"},
	}
	got := messageRows(t, db, id)
	if len(got) != len(want) {
		t.Fatalf("mesajlar = %+v", got)
	}
	for i := range want {
		g := got[i]
		if g.UserEmail != want[i].UserEmail || g.Channel != want[i].Channel || g.Recipient != want[i].Recipient {
			t.Fatalf("mesaj %d = %+v, beklenen %+v", i, g, want[i])
		}
		if g.Status != "queued" || g.Attempts != 0 || g.Claimed {
			t.Fatalf("mesaj %d queued, 0 deneme, sahipsiz olmalı: %+v", i, g)
		}
		if !strings.HasPrefix(g.Key, id+":") || !strings.HasSuffix(g.Key, ":"+g.Channel) {
			t.Fatalf("idempotency anahtarı campaign:user:channel olmalı: %q", g.Key)
		}
	}

	st := approvalState(t, db, id)
	if st.Status != "approved" || !st.Version.Valid || st.Version.Int32 != 1 || !st.At.Valid {
		t.Fatalf("kampanya onaylanmalı: %+v", st)
	}
	// GET onay bilgisini verir.
	m := getCampaignJSON(t, h, id)
	if string(m["approved_content_version"]) != "1" || string(m["status"]) != `"approved"` {
		t.Fatalf("GET onay bilgisi: %s %s", m["status"], m["approved_content_version"])
	}
}

// Örnek brief: uçtan uca (kampanya, MockLLM içeriği, onay). Kuyruktaki sayılar,
// önizlemedeki kanala uygun kişi sayılarıyla birebir aynıdır (ADR-023 B) ve
// Go'da bağımsız hesaplanan oracle ile uyuşur.
func TestApproveExampleBriefMatchesPreviewCountsOnSeedData(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())

	created := decodeCampaign(t, apiPost(h, exampleBriefText))
	evs := parseEvents(t, postGenerate(h, created.ID).Body.String())
	if lastEvent(t, evs).Name != "validated" {
		t.Fatalf("içerik üretilmeli: %v", eventNames(evs))
	}

	res := mustApproveOK(t, h, created.ID, 1)

	seg, issues := segment.Validate([]byte(`{"all":[
		{"field":"city","op":"eq","value":"İstanbul"},
		{"field":"last_cart_add_at","op":"within_last_days","value":30},
		{"field":"last_purchase_at","op":"not_within_last_days","value":30}],"unsupported":[]}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var wantSMS, wantEmail int
	for _, u := range seed.Generate(now) {
		if !matches(u, seg) {
			continue
		}
		if eligible(u, "sms") {
			wantSMS++
		}
		if eligible(u, "email") {
			wantEmail++
		}
	}
	if res.Queued.SMS != wantSMS || res.Queued.Email != wantEmail {
		t.Fatalf("kuyruk = %+v, oracle SMS=%d e-posta=%d", res.Queued, wantSMS, wantEmail)
	}
	// ADR-023 (B): önizleme sayısı ile kuyruk aynı kuralı kullanır, birebir eşittir.
	if created.Audience.SMS != res.Queued.SMS || created.Audience.Email != res.Queued.Email {
		t.Fatalf("önizleme %+v ile kuyruk %+v eşit olmalı", created.Audience, res.Queued)
	}
	t.Logf("önizleme SMS=%d e-posta=%d = kuyruk SMS=%d e-posta=%d", created.Audience.SMS, created.Audience.Email, res.Queued.SMS, res.Queued.Email)
	if n := messageCount(t, db); n != res.Queued.Total {
		t.Fatalf("mesaj satırı sayısı %d, yanıt %d", n, res.Queued.Total)
	}
}

// Çift tıklama ve tekrarlar: aynı sürümle onay yeniden hesaplamaz ve yeni iş
// oluşturmaz.
func TestApproveSameVersionAgainIsIdempotent(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	ids := addRecipients(t, db, []recipientFixture{
		{email: "a@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"},
		{email: "b@example.test", sms: "granted", mail: "none"}, // telefonsuz
	})

	first := mustApproveOK(t, h, id, 1)
	rowsBefore := messageRows(t, db, id)

	// İzin onaydan sonra değişir ve yeni bir eşleşen kullanıcı gelir: tekrar
	// onay bunları görmez, mevcut sonucu döndürür.
	if _, err := db.Exec(`UPDATE user_consents SET granted = false WHERE user_id = $1`, ids["a@example.test"]); err != nil {
		t.Fatal(err)
	}
	addRecipients(t, db, []recipientFixture{{email: "yeni@example.test", phone: str("+905550000009"), sms: "granted", mail: "granted"}})

	second := mustApproveOK(t, h, id, 1)

	if second.ApprovedAt != first.ApprovedAt || second.Queued != first.Queued || second.Status != "approved" || second.ApprovedContentVersion != 1 {
		t.Fatalf("tekrar aynı sonucu vermeli:\n %+v\n %+v", first, second)
	}
	if got := messageRows(t, db, id); !reflect.DeepEqual(got, rowsBefore) {
		t.Fatalf("tekrar onay mesaj satırlarını değiştirmemeli:\n%+v\n%+v", rowsBefore, got)
	}

	// Gönderim başladıktan sonra da (sending) aynı sürümle 200.
	if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	third := mustApproveOK(t, h, id, 1)
	if third.Status != "sending" || third.Queued != first.Queued || third.ApprovedAt != first.ApprovedAt {
		t.Fatalf("sending iken tekrar: %+v", third)
	}

	// Farklı sürümle 409; hiçbir şey değişmez.
	wantApproveError(t, approvePost(h, id, 2), http.StatusConflict, "CAMPAIGN_ALREADY_APPROVED")
	wantApproveError(t, approvePost(h, id, 9), http.StatusConflict, "CAMPAIGN_ALREADY_APPROVED")
	if got := messageRows(t, db, id); !reflect.DeepEqual(got, rowsBefore) {
		t.Fatal("reddedilen onaylar mesajları değiştirmemeli")
	}
}

// Eski sürüm, hiç sürüm olmaması ve olmayan kampanya: hiçbir iş oluşmaz.
func TestApproveRejectsStaleMissingAndUnknownWithoutQueueing(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	addRecipients(t, db, []recipientFixture{{email: "a@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"}})

	// Doğrulanmış sürüm yok.
	wantApproveError(t, approvePost(h, id, 1), http.StatusConflict, "NO_VALIDATED_CONTENT")
	wantDraftUntouched(t, db, id)

	// Kullanıcı v1'i görürken v2 kaydedildi: v1 onayı 409.
	mustAddContent(t, db, id, 1)
	mustAddContent(t, db, id, 2)
	wantApproveError(t, approvePost(h, id, 1), http.StatusConflict, "STALE_CONTENT_VERSION")
	wantDraftUntouched(t, db, id)
	// Var olmayan (daha yüksek) sürüm de son sürüm değildir.
	wantApproveError(t, approvePost(h, id, 3), http.StatusConflict, "STALE_CONTENT_VERSION")
	wantDraftUntouched(t, db, id)

	// Olmayan ve geçersiz kampanya kimlikleri.
	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "abc", "1%3BDROP%20TABLE%20campaigns"} {
		wantApproveError(t, approvePost(h, bad, 1), http.StatusNotFound, "CAMPAIGN_NOT_FOUND")
	}
	if n := messageCount(t, db); n != 0 {
		t.Fatalf("hiçbir mesaj oluşmamalı: %d", n)
	}

	// Doğru (son) sürümle onay çalışır.
	if res := mustApproveOK(t, h, id, 2); res.Queued.Total != 2 {
		t.Fatalf("son sürümle onay: %+v", res)
	}
}

// Render sınırı: yalnızca kendi kanalının metni, her alıcı için kontrol edilir.
func TestApproveRejectsWhenRenderedTextExceedsLimitForAnyRecipient(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)

	long := strings.Repeat("Ü", 30) // 30 kod noktası
	addRecipients(t, db, []recipientFixture{
		{email: "uzun-sms@example.test", name: str(long), phone: str("+905550000001"), sms: "granted", mail: "none"},
		{email: "uzun-sms2@example.test", name: str(long + "x"), phone: str("+905550000002"), sms: "granted", mail: "none"},
		{email: "uzun-eposta@example.test", name: str(long), phone: str("+905550000003"), sms: "none", mail: "granted"},
		{email: "kisa@example.test", name: str("Ayşe"), phone: str("+905550000004"), sms: "granted", mail: "granted"},
	})
	// SMS: 140 + ad → 30 karakterlik adla 170 (aşar). Konu kısa: e-posta alıcıları
	// etkilenmez.
	if err := addContent(db, id, 1, "human", strings.Repeat("a", 140)+"{{first_name}}", "Kısa konu", "Gövde"); err != nil {
		t.Fatal(err)
	}

	rec := approvePost(h, id, 1)

	wantApproveError(t, rec, http.StatusUnprocessableEntity, "RENDER_LIMIT_EXCEEDED")
	var e struct {
		Error struct {
			Reasons []struct {
				Code, Field, Text string
				Count             int
			}
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Error.Reasons) != 1 || e.Error.Reasons[0].Field != "sms_text" || e.Error.Reasons[0].Count != 2 || e.Error.Reasons[0].Text != long {
		t.Fatalf("yalnızca SMS alanı, 2 alıcı ve örnek ad beklenir: %+v", e.Error.Reasons)
	}
	wantDraftUntouched(t, db, id)

	// Metin kısaltılıp yeni sürüm kaydedilince (aynı alıcılar) onay geçer.
	rec = putEdit(h, id, mustJSON(map[string]string{
		"sms_text": strings.Repeat("a", 100) + "{{first_name}}", "email_subject": "Kısa konu", "email_body": "Gövde"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	if res := mustApproveOK(t, h, id, 2); res.Queued != (countsJSON{SMS: 3, Email: 2, Total: 5}) {
		t.Fatalf("kısaltılmış metinle onay: %+v", res)
	}
}

func TestApproveRenderLimitForSubjectOnlyAffectsEmailRecipients(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	long := strings.Repeat("Ş", 20)
	addRecipients(t, db, []recipientFixture{
		{email: "uzun-sms@example.test", name: str(long), phone: str("+905550000001"), sms: "granted", mail: "none"},
		{email: "uzun-eposta@example.test", name: str(long), sms: "none", mail: "granted"},
	})
	// Konu: 70 + 20 = 90 > 80. SMS kısa.
	if err := addContent(db, id, 1, "human", "Merhaba {{first_name}}", strings.Repeat("k", 70)+"{{first_name}}", "Gövde"); err != nil {
		t.Fatal(err)
	}

	rec := approvePost(h, id, 1)

	wantApproveError(t, rec, http.StatusUnprocessableEntity, "RENDER_LIMIT_EXCEEDED")
	body := rec.Body.String()
	if !strings.Contains(body, `"field":"email_subject"`) || !strings.Contains(body, `"count":1`) || strings.Contains(body, "sms_text") {
		t.Fatalf("yalnızca e-posta konusu ve 1 alıcı beklenir: %s", rec.Body.String())
	}
	wantDraftUntouched(t, db, id)
}

// Adı olmayan alıcıya fallback yazılır ve uzunluk onda da kontrol edilir.
func TestApproveChecksFallbackNameLength(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	addRecipients(t, db, []recipientFixture{
		{email: "adsiz@example.test", phone: str("+905550000001"), sms: "granted", mail: "none"}, // ad yok
		{email: "bosluk@example.test", name: str("   "), phone: str("+905550000002"), sms: "granted", mail: "none"},
	})
	// 143 + fallback(18) = 161
	if err := addContent(db, id, 1, "human", strings.Repeat("a", 143)+"{{first_name}}", "Konu", "Gövde"); err != nil {
		t.Fatal(err)
	}

	rec := approvePost(h, id, 1)

	wantApproveError(t, rec, http.StatusUnprocessableEntity, "RENDER_LIMIT_EXCEEDED")
	if !strings.Contains(rec.Body.String(), `"count":2`) || !strings.Contains(rec.Body.String(), content.FallbackName) {
		t.Fatalf("2 alıcı ve fallback örneği beklenir: %s", rec.Body.String())
	}
	wantDraftUntouched(t, db, id)
}

func TestApproveRefusesWhenNoEligibleRecipients(t *testing.T) {
	cases := map[string][]recipientFixture{
		"eşleşen yok": {{email: "baska@example.test", city: "Ankara", phone: str("+905550000001"), sms: "granted", mail: "granted"}},
		"hepsi reddetmiş": {
			{email: "a@example.test", phone: str("+905550000001"), sms: "denied", mail: "denied"},
			{email: "b@example.test", phone: str("+905550000002"), sms: "denied", mail: "none"},
		},
		"izin kaydı yok":               {{email: "a@example.test", phone: str("+905550000001"), sms: "none", mail: "none"}},
		"yalnızca izinli ama adressiz": {{email: "a@example.test", sms: "granted", mail: "none"}},
	}
	for name, users := range cases {
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t)
			h := newAPI(db, llm.NewMock())
			id := newCampaign(t, db)
			mustAddContent(t, db, id, 1)
			addRecipients(t, db, users)

			wantApproveError(t, approvePost(h, id, 1), http.StatusUnprocessableEntity, "NO_ELIGIBLE_RECIPIENTS")
			wantDraftUntouched(t, db, id)
		})
	}
}

// Onay atomiktir: mesajlar yazılamazsa onay da geri alınır; kampanya draft kalır.
func TestApproveIsAtomicWhenMessageInsertFails(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	addRecipients(t, db, []recipientFixture{
		{email: "a@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"},
		{email: "b@example.test", phone: str("+905550000002"), sms: "granted", mail: "granted"},
	})

	// Yalnızca campaigns_test'te, e-posta kanalının mesajları yazılırken hata veren
	// geçici bir tetikleyici: SMS satırları yazıldıktan sonra patlar.
	if err := testdb.RequireTestDatabase(db); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE FUNCTION zz_fail_email_messages() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test: e-posta mesajı yazılamadı'; END $$`,
		`CREATE TRIGGER zz_fail_email_messages BEFORE INSERT ON messages FOR EACH ROW WHEN (NEW.channel = 'email') EXECUTE FUNCTION zz_fail_email_messages()`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS zz_fail_email_messages ON messages`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS zz_fail_email_messages()`)
	})

	rec := approvePost(h, id, 1)

	wantApproveError(t, rec, http.StatusInternalServerError, "INTERNAL")
	wantDraftUntouched(t, db, id) // SMS satırları da geri alındı
	if n := countContents(t, db, id); n != 1 {
		t.Fatalf("içerik sürümü korunmalı: %d", n)
	}

	// Tetikleyici kalkınca aynı onay başarılı olur.
	if _, err := db.Exec(`DROP TRIGGER zz_fail_email_messages ON messages`); err != nil {
		t.Fatal(err)
	}
	if res := mustApproveOK(t, h, id, 1); res.Queued != (countsJSON{SMS: 2, Email: 2, Total: 4}) {
		t.Fatalf("yeniden onay: %+v", res)
	}
}

// İnsan onayından önce hiçbir yolla kuyruğa iş girmez (CLAUDE.md kuralı).
func TestNothingIsQueuedBeforeApproval(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())

	created := decodeCampaign(t, apiPost(h, exampleBriefText))
	postGenerate(h, created.ID)
	postGenerate(h, created.ID)
	putEdit(h, created.ID, `{"sms_text":"Selam {{first_name}}","email_subject":"Konu","email_body":"Gövde"}`)
	getCampaignJSON(t, h, created.ID)
	// Reddedilen onaylar da kuyruğa iş girmez.
	approvePost(h, created.ID, 1)  // eski sürüm
	approvePost(h, created.ID, 99) // olmayan sürüm

	if n := messageCount(t, db); n != 0 {
		t.Fatalf("onaydan önce kuyrukta mesaj olmamalı: %d", n)
	}
	wantDraftUntouched(t, db, created.ID)
}

// Alıcılar onay anında seçilir: önizlemeden sonra izni geri çekilen kişi
// kuyruğa girmez, sonradan eşleşen izinli kişi girer. Önizleme sayısı ile kuyruk
// sayısı bu durumda farklı olabilir ve yanıt gerçek kuyruğu bildirir.
func TestApproveSelectsRecipientsAtApprovalTime(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	ids := addRecipients(t, db, []recipientFixture{
		{email: "kalir@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"},
		{email: "cekilir@example.test", phone: str("+905550000002"), sms: "granted", mail: "granted"},
	})
	before, sms, mail := count(t, db, `{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}`)
	if before != 2 || sms != 2 || mail != 2 {
		t.Fatalf("önizleme sayıları: %d %d %d", before, sms, mail)
	}

	if _, err := db.Exec(`UPDATE user_consents SET granted = false WHERE user_id = $1 AND channel = 'sms'`, ids["cekilir@example.test"]); err != nil {
		t.Fatal(err)
	}
	addRecipients(t, db, []recipientFixture{{email: "yeni@example.test", phone: str("+905550000003"), sms: "granted", mail: "none"}})

	res := mustApproveOK(t, h, id, 1)

	// SMS: kalir + yeni (çekilen yok); e-posta: kalir + çekilen (e-posta izni durdu).
	if res.Queued != (countsJSON{SMS: 2, Email: 2, Total: 4}) {
		t.Fatalf("onay anındaki izinlere göre kuyruk: %+v", res.Queued)
	}
	for _, m := range messageRows(t, db, id) {
		if m.UserEmail == "cekilir@example.test" && m.Channel == "sms" {
			t.Fatal("izni geri çekilen kullanıcıya SMS işi oluşmamalı")
		}
	}
}

// Aynı kullanıcı için ayrı kampanyalar bağımsız iş oluşturur.
func TestApproveTwoCampaignsAreIndependentForTheSameUser(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	a, b := newCampaign(t, db), newCampaign(t, db)
	mustAddContent(t, db, a, 1)
	mustAddContent(t, db, b, 1)
	addRecipients(t, db, []recipientFixture{{email: "ortak@example.test", phone: str("+905550000001"), sms: "granted", mail: "granted"}})

	ra, rb := mustApproveOK(t, h, a, 1), mustApproveOK(t, h, b, 1)

	if ra.Queued.Total != 2 || rb.Queued.Total != 2 || messageCount(t, db) != 4 {
		t.Fatalf("her kampanya kendi işlerini oluşturmalı: %+v %+v toplam %d", ra, rb, messageCount(t, db))
	}
	keysA, keysB := []string{}, []string{}
	for _, m := range messageRows(t, db, a) {
		keysA = append(keysA, m.Key)
	}
	for _, m := range messageRows(t, db, b) {
		keysB = append(keysB, m.Key)
	}
	for _, k := range keysA {
		if slices.Contains(keysB, k) {
			t.Fatalf("idempotency anahtarları kampanyalar arasında ayrı olmalı: %q", k)
		}
	}
}

// Onaydan sonra içerik üretimi ve düzenleme 409'dur ve sürüm eklenmez.
func TestContentEndpointsAreClosedAfterApproval(t *testing.T) {
	db := testdb.Open(t)
	h := newAPI(db, llm.NewMock())
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	addRecipients(t, db, []recipientFixture{{email: "a@example.test", phone: str("+905550000001"), sms: "granted", mail: "none"}})
	mustApproveOK(t, h, id, 1)

	wantApproveError(t, putEdit(h, id, `{"sms_text":"a","email_subject":"b","email_body":"c"}`), http.StatusConflict, "CAMPAIGN_NOT_DRAFT")
	wantApproveError(t, postGenerate(h, id), http.StatusConflict, "CAMPAIGN_NOT_DRAFT")
	if n := countContents(t, db, id); n != 1 {
		t.Fatalf("onaydan sonra sürüm eklenmemeli: %d", n)
	}
}

// Onay, alıcı listesini tek sorguyla bir kez seçer: aynı kişi için birden çok
// kanal satırı kimlik sırasına göre tutarlıdır ve alıcı sorgusu kitle
// sayılarıyla aynı kümeyi döndürür.
func TestRecipientsQueryAgreesWithCountQuery(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	seg, issues := segment.Validate([]byte(`{"all":[
		{"field":"city","op":"in","value":["İstanbul","Ankara"]},
		{"field":"last_cart_add_at","op":"within_last_days","value":30}],"unsupported":[]}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	counts, err := audience.Count(t.Context(), db, seg, now)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := audience.Recipients(t.Context(), db, seg, now)
	if err != nil {
		t.Fatal(err)
	}
	var sms, mail int
	for i, r := range rs {
		if r.SMSEligible {
			sms++
		}
		if r.EmailEligible {
			mail++
		}
		if !r.SMSEligible && !r.EmailEligible {
			t.Fatalf("izinsiz kullanıcı alıcı listesinde olmamalı: %+v", r)
		}
		if i > 0 && rs[i-1].UserID >= r.UserID {
			t.Fatal("alıcılar kimliğe göre artan sırada ve benzersiz olmalı")
		}
	}
	if sms != counts.SMS || mail != counts.Email {
		t.Fatalf("alıcı sorgusu SMS=%d e-posta=%d, sayım %+v", sms, mail, counts)
	}
}
