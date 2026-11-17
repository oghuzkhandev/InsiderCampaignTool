package integration

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

func campaignCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM campaigns`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func exampleSegment(t *testing.T) segment.Segment {
	t.Helper()
	seg, issues := segment.Validate([]byte(`{"all":[
		{"field":"city","op":"eq","value":"İstanbul"},
		{"field":"last_cart_add_at","op":"within_last_days","value":30},
		{"field":"last_purchase_at","op":"not_within_last_days","value":30}
	],"unsupported":[]}`))
	if len(issues) != 0 {
		t.Fatalf("geçerli segment bekleniyordu: %v", issues)
	}
	return seg
}

func TestCampaignCreateAndGet(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewStore(db)
	seg := exampleSegment(t)
	counts := audience.Counts{Total: 10, SMS: 6, Email: 7}

	created, err := store.Create(t.Context(), "Son 30 günde sepete ekleyen İstanbul kullanıcıları", seg, counts)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Status != campaign.StatusDraft || created.CreatedAt.IsZero() {
		t.Fatalf("oluşturulan kampanya eksik: %#v", created)
	}

	got, err := store.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || got.Brief != created.Brief || got.Status != campaign.StatusDraft || got.Audience != counts {
		t.Fatalf("okunan kampanya farklı:\n%#v\n%#v", got, created)
	}
	if !reflect.DeepEqual(got.Segment.Conditions, seg.Conditions) {
		t.Fatalf("segment aynen geri okunmalı:\n%#v\n%#v", got.Segment.Conditions, seg.Conditions)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("oluşturulma zamanı farklı: %v != %v", got.CreatedAt, created.CreatedAt)
	}

	other, err := store.Create(t.Context(), "başka brief", seg, counts)
	if err != nil || other.ID == created.ID {
		t.Fatalf("her kampanyanın ayrı kimliği olmalı: %v %v", other.ID, err)
	}
}

func TestCampaignGetNotFound(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewStore(db)

	for _, id := range []string{
		"00000000-0000-0000-0000-000000000000", // geçerli uuid, kayıt yok
		"abc",                                  // geçersiz uuid
		"",
		"1; DROP TABLE campaigns",
	} {
		if _, err := store.Get(t.Context(), id); !errors.Is(err, campaign.ErrNotFound) {
			t.Fatalf("%q için ErrNotFound bekleniyordu: %v", id, err)
		}
	}
	if campaignCount(t, db) != 0 {
		t.Fatal("okuma kayıt oluşturmamalı")
	}
}

func TestCampaignCreateRejectsInvalidInputWithoutRows(t *testing.T) {
	db := testdb.Open(t)
	store := campaign.NewStore(db)
	seg := exampleSegment(t)

	tests := []struct {
		name   string
		brief  string
		seg    segment.Segment
		counts audience.Counts
	}{
		{"koşulsuz segment", "brief", segment.Segment{}, audience.Counts{}},
		{"kataloğa uymayan segment", "brief", segment.Segment{Conditions: []segment.Condition{{Field: "email", Op: "eq", Text: "a@b.c"}}}, audience.Counts{}},
		{"boş brief", "", seg, audience.Counts{}},
		{"yalnızca boşluk brief", " \t\n", seg, audience.Counts{}},
		{"çok uzun brief", strings.Repeat("a", 1001), seg, audience.Counts{}},
		{"SMS toplamdan büyük", "brief", seg, audience.Counts{Total: 1, SMS: 2}},
		{"negatif sayı", "brief", seg, audience.Counts{Total: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Create(t.Context(), tt.brief, tt.seg, tt.counts); err == nil {
				t.Fatal("kampanya kaydedilmemeli")
			}
			if n := campaignCount(t, db); n != 0 {
				t.Fatalf("hatalı girdi kayıt bırakmamalı, satır sayısı %d", n)
			}
		})
	}
}
