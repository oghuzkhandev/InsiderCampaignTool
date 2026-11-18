package seed

import (
	"strings"
	"testing"
	"time"
)

var e2eNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// e2eOracle, E2EBrief'in koşullarını (İstanbul, son 30 günde sepet, son 30 günde
// satın alma yok) ve kanal uygunluğunu (izin ve adres) SQL'den bağımsız hesaplar.
func e2eOracle(users []User, now time.Time) (total, sms, email int) {
	within := func(t *time.Time) bool { return t != nil && !t.Before(now.Add(-30*day)) }
	nonBlank := func(s string) bool { return strings.TrimSpace(s) != "" }
	granted := func(u User, channel string) bool {
		for _, c := range u.Consents {
			if c.Channel == channel && c.Granted {
				return true
			}
		}
		return false
	}
	for _, u := range users {
		if u.City != "İstanbul" || !within(u.LastCartAddAt) || within(u.LastPurchaseAt) {
			continue
		}
		total++
		if granted(u, "sms") && u.Phone != nil && nonBlank(*u.Phone) {
			sms++
		}
		if granted(u, "email") && nonBlank(u.Email) {
			email++
		}
	}
	return total, sms, email
}

func TestGenerateE2EProducesTheDocumentedCounts(t *testing.T) {
	total, sms, email := e2eOracle(GenerateE2E(e2eNow), e2eNow)
	want := E2EExpected
	if total != want.Total || sms != want.SMS || email != want.Email {
		t.Fatalf("toplam/sms/e-posta = %d/%d/%d, beklenen %d/%d/%d", total, sms, email, want.Total, want.SMS, want.Email)
	}
	// Üç sayı birbirinden farklıdır: alanların karışması testlerce görülür.
	if want.Total == want.SMS || want.Total == want.Email || want.SMS == want.Email {
		t.Fatalf("sayılar ayırt edici olmalı: %+v", want)
	}
}

// Sayılar zamana bağlı değildir: referans zaman kayınca (ortam yaşlanınca)
// yalnızca kullanıcının oluşturulduğu andaki tarihler önemlidir; her tarih 30
// günlük sınırdan yeterince uzaktır.
func TestGenerateE2EDatesAreFarFromTheThirtyDayBoundary(t *testing.T) {
	boundary := e2eNow.Add(-30 * day)
	for _, u := range GenerateE2E(e2eNow) {
		for name, ts := range map[string]*time.Time{"sepet": u.LastCartAddAt, "satın alma": u.LastPurchaseAt} {
			if ts == nil {
				continue
			}
			dist := ts.Sub(boundary)
			if dist < 0 {
				dist = -dist
			}
			if dist < E2EMinBoundaryDistance {
				t.Fatalf("%s %s: %s tarihi sınıra %v yakın", u.Email, name, ts, dist)
			}
		}
	}
}

// Birkaç dakika ya da saat sonra oluşturulan veri de aynı sayıları verir.
func TestGenerateE2ECountsAreStableAcrossReferenceTimes(t *testing.T) {
	for _, shift := range []time.Duration{0, 37 * time.Minute, 5 * time.Hour, 24 * time.Hour} {
		now := e2eNow.Add(shift)
		total, sms, email := e2eOracle(GenerateE2E(now), now.Add(2*time.Hour)) // testler seed'den saatler sonra koşar
		if total != E2EExpected.Total || sms != E2EExpected.SMS || email != E2EExpected.Email {
			t.Fatalf("kayma %v: %d/%d/%d", shift, total, sms, email)
		}
	}
}

func TestGenerateE2EIsSmallDeterministicAndUnique(t *testing.T) {
	a, b := GenerateE2E(e2eNow), GenerateE2E(e2eNow)
	if len(a) != 10 || len(a) != len(b) {
		t.Fatalf("10 kullanıcı bekleniyordu: %d", len(a))
	}
	seen := map[string]bool{}
	for i, u := range a {
		if seen[u.Email] || !strings.HasSuffix(u.Email, "@example.test") {
			t.Fatalf("e-posta benzersiz ve example.test olmalı: %q", u.Email)
		}
		seen[u.Email] = true
		if u.Email != b[i].Email || u.City != b[i].City {
			t.Fatal("üretim deterministik olmalı")
		}
	}
}
