package audience

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// ADR-031: kanal uygunluğu = açık izin VE adres. İzin yoksa adres eksikliği ikinci planda
// kalır (no_consent); yalnızca izin varken boş adres no_address olur.
func TestChannelStatus(t *testing.T) {
	tests := []struct {
		name             string
		consent, address bool
		want             ChannelStatus
	}{
		{"izin ve adres", true, true, ChannelEligible},
		{"izin var, adres yok", true, false, ChannelNoAddress},
		{"izin yok, adres var", false, true, ChannelNoConsent},
		{"izin yok, adres de yok", false, false, ChannelNoConsent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelStatus(tt.consent, tt.address); got != tt.want {
				t.Fatalf("channelStatus(%v, %v) = %q, beklenen %q", tt.consent, tt.address, got, tt.want)
			}
		})
	}
}

// Eksik bilgi (NULL, boş veya yalnızca boşluk) nil olur; dolu değer kırpılmış gelir.
func TestPresentMapsMissingValuesToNil(t *testing.T) {
	valid := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	nbsp := string(rune(0x00a0))    // ASCII olmayan boşluk
	emSpace := string(rune(0x2003)) // ASCII olmayan boşluk

	for name, tc := range map[string]struct {
		in   sql.NullString
		trim func(string) string
		want string // "" = nil bekleniyor
	}{
		"NULL ad":                         {sql.NullString{}, strings.TrimSpace, ""},
		"boş ad":                          {valid(""), strings.TrimSpace, ""},
		"boşluktan ibaret ad":             {valid("  \t "), strings.TrimSpace, ""},
		"Unicode boşluklu ad":             {valid(nbsp + emSpace), strings.TrimSpace, ""},
		"dolu ad kırpılır":                {valid("  Ayşe "), strings.TrimSpace, "Ayşe"},
		"NULL telefon":                    {sql.NullString{}, trimAddress, ""},
		"boşluktan ibaret telefon":        {valid(" \t\n"), trimAddress, ""},
		"dolu telefon kırpılır":           {valid(" +905550000001 "), trimAddress, "+905550000001"},
		"ASCII dışı boşluk adres sayılır": {valid(nbsp), trimAddress, nbsp}, // SQL'deki adres kontrolüyle aynı
	} {
		t.Run(name, func(t *testing.T) {
			got := present(tc.in, tc.trim)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("nil bekleniyordu, %q geldi", *got)
				}
				return
			}
			if got == nil || *got != tc.want {
				t.Fatalf("%q bekleniyordu, geldi: %v", tc.want, got)
			}
		})
	}
}

// Geçersiz sayfa veya sınır, veritabanına hiç dokunmadan reddedilir (db nil: dokunulursa panik).
func TestListMembersRejectsInvalidPagingBeforeTouchingTheDatabase(t *testing.T) {
	seg := segment.Segment{Conditions: []segment.Condition{{Field: segment.FieldCity, Op: segment.OpEq, Text: "İstanbul"}}}
	for name, c := range map[string]struct{ page, limit int }{
		"sayfa 0": {0, 10}, "negatif sayfa": {-1, 10}, "limit 0": {1, 0}, "negatif limit": {1, -5},
	} {
		if _, err := ListMembers(t.Context(), nil, seg, time.Now(), c.page, c.limit); !errors.Is(err, segment.ErrInvalidPage) {
			t.Errorf("%s: ErrInvalidPage bekleniyordu: %v", name, err)
		}
	}
}

// Koşulsuz segment için sorgu çalışmaz (db nil: dokunulursa panik).
func TestListMembersRefusesAnUnfilteredSegment(t *testing.T) {
	if _, err := ListMembers(t.Context(), nil, segment.Segment{}, time.Now(), 1, 10); !errors.Is(err, segment.ErrEmptySegment) {
		t.Fatalf("ErrEmptySegment bekleniyordu: %v", err)
	}
}
