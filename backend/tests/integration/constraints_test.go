package integration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

const (
	pgCheckViolation   = "23514"
	pgUniqueViolation  = "23505"
	pgFKViolation      = "23503"
	pgNotNullViolation = "23502"
)

// wantPgError, hatanın beklenen SQLSTATE ve constraint adıyla geldiğini doğrular.
func wantPgError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("PgError bekleniyordu (%s %s), gelen: %v", code, constraint, err)
	}
	if pgErr.Code != code || (constraint != "" && pgErr.ConstraintName != constraint) {
		t.Fatalf("hata = %s %q, beklenen %s %q (%s)", pgErr.Code, pgErr.ConstraintName, code, constraint, pgErr.Message)
	}
}

func insertUser(db *sql.DB, email string, orderCount int) (int64, error) {
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (email, city, order_count) VALUES ($1, 'İstanbul', $2) RETURNING id`,
		email, orderCount,
	).Scan(&id)
	return id, err
}

func insertCampaign(db *sql.DB, brief, segmentJSON, status string, total, sms, email int) error {
	_, err := db.Exec(
		`INSERT INTO campaigns (brief, segment, status, audience_total, audience_sms, audience_email)
		 VALUES ($1, $2::jsonb, $3, $4, $5, $6)`,
		brief, segmentJSON, status, total, sms, email,
	)
	return err
}

const validSegment = `{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}`

func TestUsersConstraints(t *testing.T) {
	db := testdb.Open(t)

	if _, err := insertUser(db, "a@example.test", 0); err != nil {
		t.Fatal(err)
	}
	_, err := insertUser(db, "a@example.test", 0)
	wantPgError(t, err, pgUniqueViolation, "users_email_unique")

	_, err = insertUser(db, "b@example.test", -1)
	wantPgError(t, err, pgCheckViolation, "users_order_count_nonneg")

	_, err = db.Exec(`INSERT INTO users (email) VALUES ('c@example.test')`)
	wantPgError(t, err, pgNotNullViolation, "") // city zorunlu

	// Ad ve telefon boş olabilir (CNT-001 fallback senaryosu).
	if _, err := db.Exec(`INSERT INTO users (email, city) VALUES ('d@example.test', 'Ankara')`); err != nil {
		t.Fatalf("ad ve telefon olmadan kayıt kabul edilmeli: %v", err)
	}
}

func TestConsentsConstraints(t *testing.T) {
	db := testdb.Open(t)
	id, err := insertUser(db, "a@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, 'sms', true)`, id); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, 'sms', false)`, id)
	wantPgError(t, err, pgUniqueViolation, "user_consents_pkey")

	_, err = db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, 'push', true)`, id)
	wantPgError(t, err, pgCheckViolation, "user_consents_channel_valid")

	_, err = db.Exec(`INSERT INTO user_consents (user_id, channel, granted) VALUES (999999, 'email', true)`)
	wantPgError(t, err, pgFKViolation, "user_consents_user_id_fkey")

	_, err = db.Exec(`INSERT INTO user_consents (user_id, channel) VALUES ($1, 'email')`, id)
	wantPgError(t, err, pgNotNullViolation, "") // granted zorunlu

	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM user_consents`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("kullanıcı silinince izinler de silinmeli: n=%d err=%v", n, err)
	}
}

func TestCampaignsConstraints(t *testing.T) {
	db := testdb.Open(t)

	tests := []struct {
		name       string
		brief      string
		status     string
		total, sms int
		email      int
		constraint string
	}{
		{"boş brief", "", "draft", 1, 1, 1, "campaigns_brief_valid"},
		{"yalnızca boşluk brief", "  \t ", "draft", 1, 1, 1, "campaigns_brief_valid"},
		{"yalnızca satır sonu brief", "\n\r\n", "draft", 1, 1, 1, "campaigns_brief_valid"},
		{"1001 karakterli brief", strings.Repeat("ü", 1001), "draft", 1, 1, 1, "campaigns_brief_valid"},
		{"bilinmeyen durum", "brief", "failed", 1, 1, 1, "campaigns_status_valid"},
		{"negatif toplam", "brief", "draft", -1, 0, 0, ""}, // birden fazla constraint ihlal edilir
		{"negatif sms", "brief", "draft", 5, -1, 0, "campaigns_audience_sms_range"},
		{"sms toplamdan büyük", "brief", "draft", 5, 6, 0, "campaigns_audience_sms_range"},
		{"negatif e-posta", "brief", "draft", 5, 0, -1, "campaigns_audience_email_range"},
		{"e-posta toplamdan büyük", "brief", "draft", 5, 0, 6, "campaigns_audience_email_range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := insertCampaign(db, tt.brief, validSegment, tt.status, tt.total, tt.sms, tt.email)
			wantPgError(t, err, pgCheckViolation, tt.constraint)
		})
	}

	// Sınır değerleri kabul edilir.
	if err := insertCampaign(db, strings.Repeat("ü", 1000), validSegment, "draft", 0, 0, 0); err != nil {
		t.Fatalf("1000 karakterli brief ve sıfır kitle kabul edilmeli: %v", err)
	}
	if err := insertCampaign(db, "brief", validSegment, "draft", 5, 5, 5); err != nil {
		t.Fatalf("sayılar toplama eşit olabilir: %v", err)
	}

	// Segment SQL NULL olamaz; durum varsayılanı draft'tır.
	_, err := db.Exec(`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email) VALUES ('b', NULL, 0, 0, 0)`)
	wantPgError(t, err, pgNotNullViolation, "")
	var status string
	if err := db.QueryRow(`SELECT status FROM campaigns LIMIT 1`).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("varsayılan durum draft olmalı: %q %v", status, err)
	}
}

// segmentCase, hem veritabanı constraint'inin hem de Go doğrulayıcısının aynı
// kararı vermesi gereken, saklanan biçimde bir segmenttir.
type segmentCase struct {
	name  string
	json  string
	valid bool
}

func segmentCases() []segmentCase {
	cond := `{"field":"order_count","op":"eq","value":1}`
	tenConds := strings.TrimSuffix(strings.Repeat(cond+",", 10), ",")
	elevenConds := tenConds + "," + cond
	wrap := func(c string) string { return `{"all":[` + c + `]}` }

	return []segmentCase{
		// Geçerli segmentler.
		{"örnek brief", `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30},{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`, true},
		{"şehir listesi", wrap(`{"field":"city","op":"in","value":["İstanbul","Ankara"]}`), true},
		{"sipariş eq/gte/lte", `{"all":[{"field":"order_count","op":"eq","value":0},{"field":"order_count","op":"gte","value":100000},{"field":"order_count","op":"lte","value":5}]}`, true},
		{"gün sınırları 1 ve 365", `{"all":[{"field":"last_cart_add_at","op":"within_last_days","value":1},{"field":"last_purchase_at","op":"within_last_days","value":365}]}`, true},
		{"tam 10 koşul", `{"all":[` + tenConds + `]}`, true},
		{"ondalık gösterimli tam sayı", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":30.0}`), true},
		{"üstel gösterimli tam sayı", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":1e2}`), true},
		{"64 karakterli şehir", wrap(`{"field":"city","op":"eq","value":"` + strings.Repeat("a", 64) + `"}`), true},
		{"SQL benzeri değer yalnızca değerdir", wrap(`{"field":"city","op":"eq","value":"x'; DROP TABLE users; --"}`), true},

		// Üst düzey yapı.
		{"boş nesne", `{}`, false},
		{"dizi", `[]`, false},
		{"json null", `null`, false},
		{"metin", `"x"`, false},
		{"sayı", `5`, false},
		{"bool", `true`, false},
		{"boş all dizisi", `{"all":[]}`, false},
		{"all null", `{"all":null}`, false},
		{"all nesne", `{"all":{}}`, false},
		{"all metin", `{"all":"x"}`, false},
		{"all sayı", `{"all":3}`, false},
		{"yalnızca any", `{"any":[{"field":"city","op":"eq","value":"İstanbul"}]}`, false},
		{"all ve any birlikte", `{"all":[{"field":"city","op":"eq","value":"İstanbul"}],"any":[]}`, false},
		{"11 koşul", `{"all":[` + elevenConds + `]}`, false},

		// Dizi öğeleri.
		{"öğe null", `{"all":[null]}`, false},
		{"öğe sayı", `{"all":[5]}`, false},
		{"öğe metin", `{"all":["x"]}`, false},
		{"öğe dizi", `{"all":[[]]}`, false},
		{"öğe boş nesne", `{"all":[{}]}`, false},
		{"geçerli öğe yanında null öğe", `{"all":[{"field":"city","op":"eq","value":"İstanbul"},null]}`, false},
		{"öğede iç içe grup", wrap(`{"any":[{"field":"city","op":"eq","value":"İstanbul"}]}`), false},
		{"field eksik", wrap(`{"op":"eq","value":"x"}`), false},
		{"op eksik", wrap(`{"field":"city","value":"x"}`), false},
		{"value eksik", wrap(`{"field":"city","op":"eq"}`), false},
		{"öğede fazladan anahtar", wrap(`{"field":"city","op":"eq","value":"x","extra":1}`), false},

		// Yanlış tipler.
		{"field sayı", wrap(`{"field":1,"op":"eq","value":"x"}`), false},
		{"field null", wrap(`{"field":null,"op":"eq","value":"x"}`), false},
		{"op sayı", wrap(`{"field":"city","op":1,"value":"x"}`), false},
		{"op null", wrap(`{"field":"city","op":null,"value":"x"}`), false},
		{"şehir sayı", wrap(`{"field":"city","op":"eq","value":34}`), false},
		{"şehir bool", wrap(`{"field":"city","op":"eq","value":true}`), false},
		{"şehir null", wrap(`{"field":"city","op":"eq","value":null}`), false},
		{"şehir dizi", wrap(`{"field":"city","op":"eq","value":["İstanbul"]}`), false},
		{"şehir nesne", wrap(`{"field":"city","op":"eq","value":{}}`), false},
		{"boş şehir", wrap(`{"field":"city","op":"eq","value":""}`), false},
		{"boşluk şehir", wrap(`{"field":"city","op":"eq","value":"   "}`), false},
		{"tab şehir", wrap(`{"field":"city","op":"eq","value":"\t"}`), false},
		{"satır sonu şehir", wrap(`{"field":"city","op":"eq","value":"\n \r"}`), false},
		{"in listesinde tab öğe", wrap(`{"field":"city","op":"in","value":["İstanbul","\t"]}`), false},
		{"65 karakterli şehir", wrap(`{"field":"city","op":"eq","value":"` + strings.Repeat("a", 65) + `"}`), false},
		{"in metin", wrap(`{"field":"city","op":"in","value":"İstanbul"}`), false},
		{"in null", wrap(`{"field":"city","op":"in","value":null}`), false},
		{"in boş liste", wrap(`{"field":"city","op":"in","value":[]}`), false},
		{"in 11 değer", wrap(`{"field":"city","op":"in","value":["a","b","c","d","e","f","g","h","i","j","k"]}`), false},
		{"in sayı öğe", wrap(`{"field":"city","op":"in","value":["İstanbul",1]}`), false},
		{"in boş öğe", wrap(`{"field":"city","op":"in","value":["İstanbul",""]}`), false},
		{"in null öğe", wrap(`{"field":"city","op":"in","value":["İstanbul",null]}`), false},
		{"gün metin", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":"30"}`), false},
		{"gün ondalık", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":1.5}`), false},
		{"gün null", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":null}`), false},
		{"gün bool", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":true}`), false},
		{"gün dizi", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":[30]}`), false},
		{"gün sıfır", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":0}`), false},
		{"gün negatif", wrap(`{"field":"last_purchase_at","op":"not_within_last_days","value":-1}`), false},
		{"gün 366", wrap(`{"field":"last_cart_add_at","op":"within_last_days","value":366}`), false},
		{"sipariş negatif", wrap(`{"field":"order_count","op":"gte","value":-1}`), false},
		{"sipariş 100001", wrap(`{"field":"order_count","op":"lte","value":100001}`), false},
		{"sipariş metin", wrap(`{"field":"order_count","op":"eq","value":"5"}`), false},
		{"sipariş ondalık", wrap(`{"field":"order_count","op":"eq","value":2.5}`), false},
		{"sipariş null", wrap(`{"field":"order_count","op":"eq","value":null}`), false},

		// Desteklenmeyen alan ve operatör.
		{"bilinmeyen alan", wrap(`{"field":"favorite_color","op":"eq","value":"mavi"}`), false},
		{"SQL benzeri alan adı", wrap(`{"field":"city; DROP TABLE users","op":"eq","value":"x"}`), false},
		{"bilinmeyen operatör", wrap(`{"field":"city","op":"like","value":"İst%"}`), false},
		{"alana uymayan operatör: order_count within", wrap(`{"field":"order_count","op":"within_last_days","value":3}`), false},
		{"alana uymayan operatör: city gte", wrap(`{"field":"city","op":"gte","value":"x"}`), false},
		{"alana uymayan operatör: cart eq", wrap(`{"field":"last_cart_add_at","op":"eq","value":3}`), false},
		{"büyük/küçük harf farkı", wrap(`{"field":"City","op":"eq","value":"x"}`), false},
	}
}

func TestCampaignSegmentConstraint(t *testing.T) {
	db := testdb.Open(t)

	for _, tt := range segmentCases() {
		t.Run(tt.name, func(t *testing.T) {
			err := insertCampaign(db, "brief", tt.json, "draft", 0, 0, 0)
			if tt.valid {
				if err != nil {
					t.Fatalf("segment kabul edilmeliydi: %v\n%s", err, tt.json)
				}
				return
			}
			wantPgError(t, err, pgCheckViolation, "campaigns_segment_valid")
		})
	}

	// Saklanan biçim yalnızca `all` içerir; modelin `unsupported` alanı kaydedilmez.
	_, err := db.Exec(`INSERT INTO campaigns (brief, segment, audience_total, audience_sms, audience_email)
		VALUES ('b', '{"all":[{"field":"city","op":"eq","value":"x"}],"unsupported":[]}'::jsonb, 0, 0, 0)`)
	wantPgError(t, err, pgCheckViolation, "campaigns_segment_valid")

	// Constraint güncellemede de geçerlidir: geçerli kayıt geçersize çevrilemez.
	if err := insertCampaign(db, "brief", validSegment, "draft", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"all":[]}`, `null`, `{"all":[{"field":"x","op":"eq","value":1}]}`} {
		_, err := db.Exec(`UPDATE campaigns SET segment = $1::jsonb`, bad)
		wantPgError(t, err, pgCheckViolation, "campaigns_segment_valid")
	}
}

// withUnsupported, saklanan biçimdeki JSON'u Go doğrulayıcısının beklediği
// modele çıktı biçimine çevirir (zorunlu boş `unsupported` listesi eklenir).
func withUnsupported(raw string) []byte {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil || top == nil {
		return []byte(raw)
	}
	top["unsupported"] = json.RawMessage(`[]`)
	b, _ := json.Marshal(top)
	return b
}

// TestGoValidatorAndDatabaseAgree, Go doğrulayıcısı ile veritabanı
// constraint'inin bu tablodaki segmentler üzerinde aynı kararı verdiğini doğrular.
// Başka girdilerde aynı kararı verecekleri garanti edilmez.
func TestGoValidatorAndDatabaseAgree(t *testing.T) {
	db := testdb.Open(t)

	for _, tt := range segmentCases() {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := segment.Validate(withUnsupported(tt.json))
			if goValid := len(issues) == 0; goValid != tt.valid {
				t.Fatalf("Go doğrulayıcısı valid=%v dedi, beklenen %v (%v)\n%s", goValid, tt.valid, issues, tt.json)
			}
			err := insertCampaign(db, "brief", tt.json, "draft", 0, 0, 0)
			if dbValid := err == nil; dbValid != tt.valid {
				t.Fatalf("veritabanı valid=%v dedi, beklenen %v (%v)\n%s", dbValid, tt.valid, err, tt.json)
			}
		})
	}
}

// TestStoredSegmentRoundTrip, Go'da doğrulanan segmentin veritabanının kabul
// ettiği biçimde saklandığını ve geri okunduğunda aynı segmenti verdiğini doğrular.
func TestStoredSegmentRoundTrip(t *testing.T) {
	db := testdb.Open(t)

	briefs := []string{
		"Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder.",
		"Istanbul veya Ankara'daki kullanıcılar, son 7 gün sepet",
		"Bursa'daki kullanıcılara sepet hatırlatması",
	}
	gen := &segment.Generator{LLM: llm.NewMock()}
	for _, brief := range briefs {
		seg, err := gen.Generate(t.Context(), brief)
		if err != nil {
			t.Fatalf("%q: %v", brief, err)
		}
		stored, err := json.Marshal(seg)
		if err != nil {
			t.Fatal(err)
		}
		if err := insertCampaign(db, brief, string(stored), "draft", 0, 0, 0); err != nil {
			t.Fatalf("doğrulanmış segment veritabanına yazılabilmeli: %v\n%s", err, stored)
		}

		var back string
		if err := db.QueryRow(`SELECT segment::text FROM campaigns WHERE brief = $1`, brief).Scan(&back); err != nil {
			t.Fatal(err)
		}
		got, issues := segment.Validate(withUnsupported(back))
		if len(issues) != 0 {
			t.Fatalf("geri okunan segment geçerli olmalı: %v", issues)
		}
		if !reflect.DeepEqual(got.Conditions, seg.Conditions) {
			t.Fatalf("geri okunan koşullar farklı:\n%#v\n%#v", got.Conditions, seg.Conditions)
		}
	}

	// Koşulsuz segment saklanabilir biçime çevrilemez.
	if _, err := json.Marshal(segment.Segment{}); err == nil {
		t.Fatal("boş segment JSON'a çevrilmemeli")
	}
}
