package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// GET /api/campaigns/{id}/recipients (gönderim takibindeki "Alıcılar" çekmecesi): kampanyanın
// mesajlarını alıcılarıyla listeler. Gerçek PostgreSQL ile. Yalnızca campaigns_test.

type recipientsPageJSON struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Items []struct {
		ID            int64      `json:"id"`
		UserID        int64      `json:"user_id"`
		FirstName     *string    `json:"first_name"`
		Channel       string     `json:"channel"`
		Address       string     `json:"address"`
		Status        string     `json:"status"`
		Attempts      int        `json:"attempts"`
		SentAt        *time.Time `json:"sent_at"`
		FailureReason *string    `json:"failure_reason"`
	} `json:"items"`
}

func getRecipients(t *testing.T, h http.Handler, id, query string) recipientsPageJSON {
	t.Helper()
	rec := apiDo(h, http.MethodGet, "/api/campaigns/"+id+"/recipients"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: durum %d: %s", query, rec.Code, rec.Body.String())
	}
	var out recipientsPageJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON değil: %v\n%s", err, rec.Body.String())
	}
	return out
}

func approvedCampaign(t *testing.T, db *sql.DB) (http.Handler, string) {
	t.Helper()
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	id := draftWithContent(t, h)
	if r := apiDo(h, http.MethodPost, approvePath(id), `{"content_version":1}`); r.Code != http.StatusOK {
		t.Fatalf("onay: %d %s", r.Code, r.Body)
	}
	return h, id
}

func countWhere(t *testing.T, db *sql.DB, id, where string, args ...any) int {
	t.Helper()
	var n int
	q := `SELECT count(*) FROM messages WHERE campaign_id = $1 AND ` + where
	if err := db.QueryRow(q, append([]any{id}, args...)...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRecipientsListEveryMessageWithItsAddressAndSortedByNameThenChannel(t *testing.T) {
	db := testdb.Open(t)
	h, id := approvedCampaign(t, db)

	got := getRecipients(t, h, id, "?limit=20")

	want := countWhere(t, db, id, "true")
	if want == 0 || got.Total != want {
		t.Fatalf("toplam mesaj sayısı %d olmalı: %d", want, got.Total)
	}
	if len(got.Items) != min(20, want) {
		t.Fatalf("sayfa boyutu: %d", len(got.Items))
	}
	// Adres, onay anında mesaja kopyalanan değerdir.
	for _, it := range got.Items {
		var addr, first string
		if err := db.QueryRow(`SELECT recipient, first_name FROM messages WHERE id = $1`, it.ID).Scan(&addr, &first); err != nil {
			t.Fatal(err)
		}
		if it.Address != addr || it.Channel == "" || it.Status != "queued" {
			t.Fatalf("alıcı veritabanıyla uyuşmuyor: %+v (adres %q)", it, addr)
		}
		if strings.TrimSpace(first) == "" && it.FirstName != nil {
			t.Fatalf("adsız alıcıda first_name null olmalı: %+v", it)
		}
	}
	// Sıra: ad (yoksa adres) küçük harfle, sonra kanal, sonra kimlik.
	key := func(i int) string {
		name := ""
		if got.Items[i].FirstName != nil {
			name = *got.Items[i].FirstName
		} else {
			name = got.Items[i].Address
		}
		return strings.ToLower(name)
	}
	if !sort.SliceIsSorted(got.Items, func(i, j int) bool { return key(i) < key(j) }) {
		t.Fatal("alıcılar ada göre sıralı olmalı")
	}
}

func TestRecipientsFiltersByChannelAndStatus(t *testing.T) {
	db := testdb.Open(t)
	h, id := approvedCampaign(t, db)
	// Bir SMS mesajını başarısız, bir e-postayı gönderildi yap (yalnızca test veritabanı).
	if _, err := db.Exec(`UPDATE messages SET status = 'failed', failure_reason = 'rejected'
		WHERE id = (SELECT min(id) FROM messages WHERE campaign_id = $1 AND channel = 'sms')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE messages SET status = 'sent', sent_at = now(), attempts = 2
		WHERE id = (SELECT min(id) FROM messages WHERE campaign_id = $1 AND channel = 'email')`, id); err != nil {
		t.Fatal(err)
	}

	sms := getRecipients(t, h, id, "?channel=sms&limit=20")
	if sms.Total != countWhere(t, db, id, "channel = 'sms'") {
		t.Fatalf("sms toplamı: %d", sms.Total)
	}
	for _, it := range sms.Items {
		if it.Channel != "sms" {
			t.Fatalf("yalnızca sms beklenirdi: %+v", it)
		}
	}

	failed := getRecipients(t, h, id, "?status=failed")
	if failed.Total != 1 || len(failed.Items) != 1 || failed.Items[0].FailureReason == nil || *failed.Items[0].FailureReason != "rejected" {
		t.Fatalf("tek başarısız mesaj ve nedeni bekleniyordu: %+v", failed)
	}

	sent := getRecipients(t, h, id, "?status=sent")
	if sent.Total != 1 || sent.Items[0].SentAt == nil || sent.Items[0].Attempts != 2 || sent.Items[0].Channel != "email" {
		t.Fatalf("tek gönderilen e-posta bekleniyordu: %+v", sent)
	}

	none := getRecipients(t, h, id, "?channel=email&status=failed")
	if none.Total != 0 || none.Items == nil || len(none.Items) != 0 {
		t.Fatalf("eşleşme yoksa toplam 0 ve boş dizi: %+v", none)
	}
}

func TestRecipientsSearchMatchesNameAndAddressCaseInsensitivelyAndTreatsWildcardsAsText(t *testing.T) {
	db := testdb.Open(t)
	h, id := approvedCampaign(t, db)
	var name, addr string
	if err := db.QueryRow(`SELECT first_name, recipient FROM messages
		WHERE campaign_id = $1 AND btrim(first_name) <> '' ORDER BY id LIMIT 1`, id).Scan(&name, &addr); err != nil {
		t.Fatal(err)
	}

	byName := getRecipients(t, h, id, "?limit=20&q="+url.QueryEscape(strings.ToUpper(name)))
	if byName.Total != countWhere(t, db, id, "strpos(lower(first_name || ' ' || recipient), lower($2)) > 0", name) || byName.Total == 0 {
		t.Fatalf("ada göre arama: %d", byName.Total)
	}
	byAddress := getRecipients(t, h, id, "?q="+url.QueryEscape(strings.ToUpper(addr)))
	if byAddress.Total == 0 {
		t.Fatalf("adrese göre arama sonuç vermeliydi: %q", addr)
	}
	for _, wild := range []string{"%", "_", "%%"} {
		if got := getRecipients(t, h, id, "?q="+url.QueryEscape(wild)); got.Total != countWhere(t, db, id, "strpos(lower(first_name || ' ' || recipient), $2) > 0", wild) {
			t.Fatalf("%q düz metin olarak aranmalı: %d", wild, got.Total)
		}
	}
}

func TestRecipientsPaginatesWithoutGapsOrDuplicatesAndBeyondTheEndIsEmpty(t *testing.T) {
	db := testdb.Open(t)
	h, id := approvedCampaign(t, db)
	total := countWhere(t, db, id, "true")

	seen := map[int64]bool{}
	for page := 1; page <= (total+4)/5; page++ {
		got := getRecipients(t, h, id, "?limit=5&page="+strconv.Itoa(page))
		if got.Total != total {
			t.Fatalf("sayfa %d: toplam %d, beklenen %d", page, got.Total, total)
		}
		for _, it := range got.Items {
			if seen[it.ID] {
				t.Fatalf("mesaj %d iki sayfada göründü", it.ID)
			}
			seen[it.ID] = true
		}
	}
	if len(seen) != total {
		t.Fatalf("her mesaj tam bir kez görünmeli: %d / %d", len(seen), total)
	}
	beyond := getRecipients(t, h, id, "?limit=5&page=1000")
	if beyond.Total != total || len(beyond.Items) != 0 {
		t.Fatalf("sayfa sonrası boş liste ve doğru toplam: %+v", beyond)
	}
}

func TestRecipientsOfCampaignWithoutMessagesIsEmptyAndUnknownCampaignIs404(t *testing.T) {
	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	h := newAPI(db, llm.NewMock())
	draft := draftWithContent(t, h)

	got := getRecipients(t, h, draft, "")
	if got.Total != 0 || got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("onaysız kampanyanın alıcısı yoktur: %+v", got)
	}

	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		rec := apiDo(h, http.MethodGet, "/api/campaigns/"+id+"/recipients", "")
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "CAMPAIGN_NOT_FOUND") {
			t.Fatalf("%s: 404 bekleniyordu: %d %s", id, rec.Code, rec.Body.String())
		}
	}
}

func TestRecipientsOnlyListsThatCampaignsMessages(t *testing.T) {
	db := testdb.Open(t)
	h, first := approvedCampaign(t, db)
	second := draftWithContent(t, h)
	if r := apiDo(h, http.MethodPost, approvePath(second), `{"content_version":1}`); r.Code != http.StatusOK {
		t.Fatalf("onay: %d %s", r.Code, r.Body)
	}

	a, b := getRecipients(t, h, first, "?limit=20"), getRecipients(t, h, second, "?limit=20")

	if a.Total != countWhere(t, db, first, "true") || b.Total != countWhere(t, db, second, "true") {
		t.Fatalf("her kampanya kendi mesajlarını görmeli: %d, %d", a.Total, b.Total)
	}
	ids := map[int64]bool{}
	for _, it := range a.Items {
		ids[it.ID] = true
	}
	for _, it := range b.Items {
		if ids[it.ID] {
			t.Fatalf("mesaj %d iki kampanyada göründü", it.ID)
		}
	}
}
