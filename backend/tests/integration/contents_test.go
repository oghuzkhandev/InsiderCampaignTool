package integration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/content"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

const (
	pgRestrictViolation = "23001" // tetikleyicilerin ERRCODE'u (restrict_violation)

	goodSMS     = "Merhaba {{first_name}}, %30 indirim seni bekliyor!"
	goodSubject = "Sepetin seni bekliyor"
	goodBody    = "Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada."
)

// execer, *sql.DB ve *sql.Tx için ortak yazma arayüzüdür.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func newCampaign(t testing.TB, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(
		`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email)
		 VALUES ('brief', $1::jsonb, 5, 5, 5) RETURNING id`, validSegment,
	).Scan(&id)
	if err != nil {
		t.Fatalf("kampanya eklenemedi: %v", err)
	}
	return id
}

func addContent(db execer, campaignID string, version int, source, sms, subject, body string) error {
	_, err := db.Exec(
		`INSERT INTO campaign_contents (campaign_id, version, source, sms_text, email_subject, email_body)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		campaignID, version, source, sms, subject, body,
	)
	return err
}

func mustAddContent(t testing.TB, db execer, campaignID string, version int) {
	t.Helper()
	if err := addContent(db, campaignID, version, "llm", goodSMS, goodSubject, goodBody); err != nil {
		t.Fatalf("sürüm %d eklenemedi: %v", version, err)
	}
}

const approveSQL = `UPDATE campaigns SET status = 'approved', approved_content_version = $2, approved_at = now() WHERE id = $1`

func mustApprove(t testing.TB, db execer, campaignID string, version int) {
	t.Helper()
	if _, err := db.Exec(approveSQL, campaignID, version); err != nil {
		t.Fatalf("onaylanamadı: %v", err)
	}
}

// wantRestrict, tetikleyicinin restrict_violation hatasını ve mesajını doğrular.
func wantRestrict(t *testing.T, err error, msgPart string) {
	t.Helper()
	wantPgError(t, err, pgRestrictViolation, "")
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && !strings.Contains(pgErr.Message, msgPart) {
		t.Fatalf("hata mesajı %q içermeli: %q", msgPart, pgErr.Message)
	}
}

func TestContentsConstraints(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)

	tests := []struct {
		name       string
		version    int
		source     string
		sms        string
		subject    string
		body       string
		code       string
		constraint string
	}{
		{"sürüm sıfır", 0, "llm", goodSMS, goodSubject, goodBody, pgCheckViolation, "campaign_contents_version_positive"},
		{"negatif sürüm", -1, "llm", goodSMS, goodSubject, goodBody, pgCheckViolation, "campaign_contents_version_positive"},
		{"bilinmeyen kaynak", 1, "bot", goodSMS, goodSubject, goodBody, pgCheckViolation, "campaign_contents_source_valid"},
		{"boş SMS", 1, "llm", "", goodSubject, goodBody, pgCheckViolation, "campaign_contents_sms_valid"},
		{"yalnızca boşluk SMS", 1, "llm", " \t\n ", goodSubject, goodBody, pgCheckViolation, "campaign_contents_sms_valid"},
		{"SMS'te bilinmeyen değişken", 1, "llm", "Merhaba {{last_name}}", goodSubject, goodBody, pgCheckViolation, "campaign_contents_sms_valid"},
		{"konuda bilinmeyen değişken", 1, "llm", goodSMS, "{{city}} fırsatı", goodBody, pgCheckViolation, "campaign_contents_subject_valid"},
		{"gövdede yarım değişken", 1, "llm", goodSMS, goodSubject, "Merhaba {{first_name", pgCheckViolation, "campaign_contents_body_valid"},
		{"boş konu", 1, "llm", goodSMS, "", goodBody, pgCheckViolation, "campaign_contents_subject_valid"},
		{"konuda satır sonu (LF)", 1, "llm", goodSMS, "a\nb", goodBody, pgCheckViolation, "campaign_contents_subject_valid"},
		{"konuda satır sonu (CR)", 1, "llm", goodSMS, "a\rb", goodBody, pgCheckViolation, "campaign_contents_subject_valid"},
		{"boş gövde", 1, "llm", goodSMS, goodSubject, "", pgCheckViolation, "campaign_contents_body_valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addContent(db, id, tt.version, tt.source, tt.sms, tt.subject, tt.body)
			wantPgError(t, err, tt.code, tt.constraint)
		})
	}

	// NULL alanlar.
	for _, col := range []string{"sms_text", "email_subject", "email_body", "source"} {
		vals := map[string]string{"sms_text": "'a'", "email_subject": "'b'", "email_body": "'c'", "source": "'llm'"}
		vals[col] = "NULL"
		_, err := db.Exec(`INSERT INTO campaign_contents (campaign_id, version, source, sms_text, email_subject, email_body)
			VALUES ($1, 1, `+vals["source"]+`, `+vals["sms_text"]+`, `+vals["email_subject"]+`, `+vals["email_body"]+`)`, id)
		wantPgError(t, err, pgNotNullViolation, "")
	}

	// Geçerli sürümler: iki kaynak, SMS ve gövdede satır sonu, değişkensiz metin.
	if err := addContent(db, id, 1, "llm", goodSMS, goodSubject, goodBody); err != nil {
		t.Fatalf("geçerli sürüm kabul edilmeli: %v", err)
	}
	if err := addContent(db, id, 2, "human", "Satır1\nSatır2", "Konu", "Gövde"); err != nil {
		t.Fatalf("insan sürümü ve satır sonları kabul edilmeli: %v", err)
	}

	// Aynı (kampanya, sürüm) ikinci kez yazılamaz.
	wantPgError(t, addContent(db, id, 1, "llm", goodSMS, goodSubject, goodBody), pgUniqueViolation, "campaign_contents_pkey")

	// Olmayan kampanya: tetikleyici geçirir, yabancı anahtar kendi adıyla reddeder.
	err := addContent(db, "00000000-0000-0000-0000-000000000000", 1, "llm", goodSMS, goodSubject, goodBody)
	wantPgError(t, err, pgFKViolation, "campaign_contents_campaign_id_fkey")
}

func TestContentsAreDeletedWithTheirDraftCampaign(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	mustAddContent(t, db, id, 2)

	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = $1`, id); err != nil {
		t.Fatalf("kampanya silinebilmeli: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_contents`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sürümler de silinmeli: n=%d err=%v", n, err)
	}
}

// contentTextCase, bir içerik üçlüsü için Go doğrulayıcısının ve veritabanı
// kısıtının AYRI AYRI beklenen kararını yazar. Veritabanı kısıtı Go
// doğrulayıcısının tam aynası değildir (migration 00002); farklı karar
// verdikleri satırlar bilerek ve açıkça işaretlidir.
type contentTextCase struct {
	name       string
	sms        string
	subject    string
	body       string
	goValid    bool
	dbValid    bool
	constraint string // dbValid=false ise beklenen kısıt
	note       string // Go ve veritabanı farklı karar veriyorsa nedeni
}

func contentTextCases() []contentTextCase {
	return []contentTextCase{
		{name: "geçerli içerik", sms: goodSMS, subject: goodSubject, body: goodBody, goValid: true, dbValid: true},
		{name: "değişkensiz geçerli içerik", sms: "Merhaba", subject: "Konu", body: "Gövde", goValid: true, dbValid: true},
		{name: "çift değişken", sms: "{{first_name}} {{first_name}}", subject: goodSubject, body: goodBody, goValid: true, dbValid: true},
		{name: "üçlü parantez", sms: "{{{first_name}}}", subject: goodSubject, body: goodBody, goValid: true, dbValid: true},
		{name: "tek parantez değişken değildir", sms: "Kod {A1}", subject: goodSubject, body: goodBody, goValid: true, dbValid: true},
		{name: "SMS ve gövdede satır sonu serbest", sms: "a\nb", subject: goodSubject, body: "g1\r\n\r\ng2", goValid: true, dbValid: true},
		{name: "SMS 160 kod noktası", sms: strings.Repeat("ğ", 160), subject: goodSubject, body: goodBody, goValid: true, dbValid: true},
		{name: "konu 80 kod noktası", sms: goodSMS, subject: strings.Repeat("ş", 80), body: goodBody, goValid: true, dbValid: true},

		{name: "boş SMS", sms: "", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "boşluk SMS", sms: "  ", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "bilinmeyen değişken", sms: "Merhaba {{last_name}}", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "boşluklu değişken", sms: goodSMS, subject: "{{ first_name }}", body: goodBody, constraint: "campaign_contents_subject_valid"},
		{name: "büyük harfli değişken", sms: goodSMS, subject: goodSubject, body: "{{First_Name}}", constraint: "campaign_contents_body_valid"},
		{name: "boş değişken", sms: "{{}}", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "yarım açılış", sms: goodSMS, subject: goodSubject, body: "Merhaba {{first_name", constraint: "campaign_contents_body_valid"},
		{name: "yarım kapanış", sms: "first_name}}", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "açık parantez artığı", sms: "{{ {{first_name}}", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "iç içe değişken", sms: "{{first_{{first_name}}name}}", subject: goodSubject, body: goodBody, constraint: "campaign_contents_sms_valid"},
		{name: "konuda LF", sms: goodSMS, subject: "a\nb", body: goodBody, constraint: "campaign_contents_subject_valid"},
		{name: "konuda CR", sms: goodSMS, subject: "a\rb", body: goodBody, constraint: "campaign_contents_subject_valid"},
		{name: "konuda CRLF başlık enjeksiyonu", sms: goodSMS, subject: "a\r\nBcc: x@y.z", body: goodBody, constraint: "campaign_contents_subject_valid"},

		// Bilinen farklar: yalnızca Go reddeder.
		{name: "SMS 161 kod noktası", sms: strings.Repeat("ğ", 161), subject: goodSubject, body: goodBody, dbValid: true,
			note: "uzunluk sınırı yalnızca Go'da: render sonrası uzunluk alıcı adına bağlı"},
		{name: "konu 81 kod noktası", sms: goodSMS, subject: strings.Repeat("ş", 81), body: goodBody, dbValid: true,
			note: "uzunluk sınırı yalnızca Go'da"},
		{name: "yalnızca NBSP gövde", sms: goodSMS, subject: goodSubject, body: "  ", dbValid: true,
			note: "DB yalnızca ASCII boşluğa bakar; Go Unicode boşlukları da boş sayar"},
	}
}

func goContentValid(c contentTextCase) bool {
	raw, _ := json.Marshal(map[string]string{"sms_text": c.sms, "email_subject": c.subject, "email_body": c.body})
	_, issues := content.Validate(raw, content.Options{})
	return len(issues) == 0
}

// TestContentTextConstraintAndGoValidator, bu tablodaki girdilerde iki
// doğrulayıcının beklenen kararlarını sabitler. Başka girdilerde aynı kararı
// verecekleri garanti edilmez; veritabanı yalnızca temel koruma sağlar.
func TestContentTextConstraintAndGoValidator(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)

	for _, tt := range contentTextCases() {
		t.Run(tt.name, func(t *testing.T) {
			if got := goContentValid(tt); got != tt.goValid {
				t.Fatalf("Go doğrulayıcısı valid=%v dedi, beklenen %v", got, tt.goValid)
			}

			if _, err := db.Exec(`DELETE FROM campaign_contents WHERE campaign_id = $1`, id); err != nil {
				t.Fatal(err)
			}
			err := addContent(db, id, 1, "llm", tt.sms, tt.subject, tt.body)
			if tt.dbValid {
				if err != nil {
					t.Fatalf("veritabanı kabul etmeliydi: %v", err)
				}
			} else {
				wantPgError(t, err, pgCheckViolation, tt.constraint)
			}

			// Farklı karar yalnızca açıkça işaretlenmiş satırlarda olabilir.
			if (tt.goValid != tt.dbValid) != (tt.note != "") {
				t.Fatalf("Go (%v) ve DB (%v) kararı farklıysa not zorunlu, aynıysa not olmamalı: %q", tt.goValid, tt.dbValid, tt.note)
			}
		})
	}
}

func TestContentVersionsAreImmutable(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)

	for _, set := range []string{
		`sms_text = 'başka'`, `email_subject = 'başka'`, `email_body = 'başka'`,
		`source = 'human'`, `version = 2`, `created_at = now()`,
	} {
		_, err := db.Exec(`UPDATE campaign_contents SET `+set+` WHERE campaign_id = $1`, id)
		wantRestrict(t, err, "değişmez")
	}

	var sms string
	if err := db.QueryRow(`SELECT sms_text FROM campaign_contents WHERE campaign_id = $1 AND version = 1`, id).Scan(&sms); err != nil || sms != goodSMS {
		t.Fatalf("satır değişmemeli: %q %v", sms, err)
	}

	// Hiçbir satırı etkilemeyen güncelleme tetikleyiciyi çalıştırmaz.
	if _, err := db.Exec(`UPDATE campaign_contents SET sms_text = 'x' WHERE campaign_id = $1 AND version = 99`, id); err != nil {
		t.Fatalf("satır yoksa hata olmamalı: %v", err)
	}
}

func TestNoContentVersionCanBeAddedAfterApproval(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	other := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	mustApprove(t, db, id, 1)

	wantRestrict(t, addContent(db, id, 2, "human", goodSMS, goodSubject, goodBody), "onaylanmış kampanya")
	wantRestrict(t, addContent(db, id, 2, "llm", goodSMS, goodSubject, goodBody), "onaylanmış kampanya")

	// Durum sending ve completed iken de aynı.
	if _, err := db.Exec(`UPDATE campaigns SET status = 'sending' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	wantRestrict(t, addContent(db, id, 2, "llm", goodSMS, goodSubject, goodBody), "onaylanmış kampanya")
	if _, err := db.Exec(`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	wantRestrict(t, addContent(db, id, 2, "llm", goodSMS, goodSubject, goodBody), "onaylanmış kampanya")

	// Başka bir draft kampanya etkilenmez.
	mustAddContent(t, db, other, 1)

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_contents WHERE campaign_id = $1`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("onaylı kampanyada yalnızca onaylanan sürüm kalmalı: n=%d err=%v", n, err)
	}
}

func TestApprovedContentVersionCannotChange(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	mustAddContent(t, db, id, 2)
	mustApprove(t, db, id, 1)

	_, err := db.Exec(`UPDATE campaigns SET approved_content_version = 2 WHERE id = $1`, id)
	wantRestrict(t, err, "değiştirilemez")
	_, err = db.Exec(`UPDATE campaigns SET approved_content_version = NULL WHERE id = $1`, id)
	wantRestrict(t, err, "değiştirilemez")
	_, err = db.Exec(`UPDATE campaigns SET status = 'draft', approved_content_version = NULL, approved_at = NULL WHERE id = $1`, id)
	wantRestrict(t, err, "değiştirilemez")

	// Onaylanan sürümün kendisi aynı değerle yeniden yazılabilir ve durum ilerler.
	if _, err := db.Exec(`UPDATE campaigns SET status = 'sending', approved_content_version = 1 WHERE id = $1`, id); err != nil {
		t.Fatalf("aynı sürümle güncelleme kabul edilmeli: %v", err)
	}

	// Onaylanan sürüm satırı silinemez (kampanya ona bağlı).
	_, err = db.Exec(`DELETE FROM campaign_contents WHERE campaign_id = $1 AND version = 1`, id)
	wantPgError(t, err, pgFKViolation, "campaigns_approved_content_fk")
}

func TestCampaignApprovalConstraints(t *testing.T) {
	db := testdb.Open(t)
	id := newCampaign(t, db)
	other := newCampaign(t, db)
	mustAddContent(t, db, id, 1)
	mustAddContent(t, db, other, 1)
	mustAddContent(t, db, other, 2)

	tests := []struct {
		name       string
		sql        string
		args       []any
		code       string
		constraint string
	}{
		{"draft iken onaylanan sürüm", `UPDATE campaigns SET approved_content_version = 1 WHERE id = $1`, nil, pgCheckViolation, "campaigns_draft_has_no_approval"},
		{"draft iken onay zamanı", `UPDATE campaigns SET approved_at = now() WHERE id = $1`, nil, pgCheckViolation, "campaigns_draft_has_no_approval"},
		// İki kısıt birden ihlal edilir; PostgreSQL ada göre ilkini bildirir.
		{"draft iken tamamlanma zamanı", `UPDATE campaigns SET completed_at = now() WHERE id = $1`, nil, pgCheckViolation, "campaigns_completed_at_matches"},
		{"sürümsüz onay", `UPDATE campaigns SET status = 'approved', approved_at = now() WHERE id = $1`, nil, pgCheckViolation, "campaigns_approved_has_content"},
		{"onay zamanı olmayan onay", `UPDATE campaigns SET status = 'approved', approved_content_version = 1 WHERE id = $1`, nil, pgCheckViolation, "campaigns_approved_has_content"},
		{"sending sürümsüz", `UPDATE campaigns SET status = 'sending' WHERE id = $1`, nil, pgCheckViolation, "campaigns_approved_has_content"},
		{"completed zamansız", `UPDATE campaigns SET status = 'completed', approved_content_version = 1, approved_at = now() WHERE id = $1`, nil, pgCheckViolation, "campaigns_completed_at_matches"},
		{"approved iken tamamlanma zamanı", `UPDATE campaigns SET status = 'approved', approved_content_version = 1, approved_at = now(), completed_at = now() WHERE id = $1`, nil, pgCheckViolation, "campaigns_completed_at_matches"},
		{"olmayan sürüm", `UPDATE campaigns SET status = 'approved', approved_content_version = 7, approved_at = now() WHERE id = $1`, nil, pgFKViolation, "campaigns_approved_content_fk"},
		{"başka kampanyanın sürümü", `UPDATE campaigns SET status = 'approved', approved_content_version = 2, approved_at = now() WHERE id = $1`, nil, pgFKViolation, "campaigns_approved_content_fk"},
		// Bilinmeyen durumda yalnızca durum kısıtı konuşur.
		{"bilinmeyen durum", `UPDATE campaigns SET status = 'failed' WHERE id = $1`, nil, pgCheckViolation, "campaigns_status_valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Exec(tt.sql, id)
			wantPgError(t, err, tt.code, tt.constraint)
		})
	}

	// Geçerli yaşam döngüsü: draft → approved → sending → completed.
	steps := []string{
		`UPDATE campaigns SET status = 'approved', approved_content_version = 1, approved_at = now() WHERE id = $1`,
		`UPDATE campaigns SET status = 'sending' WHERE id = $1`,
		`UPDATE campaigns SET status = 'completed', completed_at = now() WHERE id = $1`,
	}
	for _, s := range steps {
		if _, err := db.Exec(s, id); err != nil {
			t.Fatalf("geçerli adım reddedildi: %v\n%s", err, s)
		}
	}

	// Yeni kampanya onay alanları olmadan draft'tır (mevcut kayıtlar için de geçerli).
	var v sql.NullInt32
	var a, c sql.NullTime
	if err := db.QueryRow(`SELECT approved_content_version, approved_at, completed_at FROM campaigns WHERE id = $1`, other).Scan(&v, &a, &c); err != nil {
		t.Fatal(err)
	}
	if v.Valid || a.Valid || c.Valid {
		t.Fatalf("yeni kampanyada onay alanları boş olmalı: %v %v %v", v, a, c)
	}
}
