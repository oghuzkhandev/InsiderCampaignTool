// Package seed, demo kullanıcılarını ve kanal izinlerini üretir ve yazar.
//
// Üretim deterministiktir (sabit tohum). Tarihler yalnızca kullanıcı ilk
// oluşturulurken verilen referans zamana göre hesaplanır; sonraki
// çalıştırmalar var olan kullanıcıların tarihlerine ve izinlerine dokunmaz.
package seed

import (
	"fmt"
	"math/rand/v2"
	"time"
)

// RandomUsers, rastgele üretilen demo kullanıcı sayısıdır. Sabit senaryo
// kullanıcıları buna ek olarak gelir.
const RandomUsers = 3000

// Consent, tek kanal için izin kaydıdır.
type Consent struct {
	Channel string
	Granted bool
}

// User, yazılacak bir demo kullanıcısıdır. Consents'te olmayan kanal için
// kayıt yazılmaz (kayıt yoksa izin yoktur).
type User struct {
	Email          string
	FirstName      *string
	Phone          *string
	City           string
	Language       string
	LastCartAddAt  *time.Time
	LastPurchaseAt *time.Time
	OrderCount     int
	Consents       []Consent
}

const day = 24 * time.Hour

var (
	firstNames = []string{
		"Ahmet", "Mehmet", "Ayşe", "Fatma", "Mustafa", "Zeynep", "Elif", "Emre",
		"Can", "Deniz", "Selin", "Burak", "Ece", "Kerem", "Merve", "Oğuz",
		"Sinem", "Tolga", "Yasemin", "Barış", "Gizem", "Cem", "Pınar", "Onur",
	}
	// Ağırlıklı şehir listesi: İstanbul en kalabalık.
	cities = []struct {
		name   string
		weight int
	}{
		{"İstanbul", 35}, {"Ankara", 15}, {"İzmir", 12}, {"Bursa", 8}, {"Antalya", 6},
		{"Adana", 5}, {"Konya", 5}, {"Gaziantep", 4}, {"Kayseri", 5}, {"Eskişehir", 5},
	}
)

func ptr[T any](v T) *T { return &v }

// Generate, referans zamana göre demo kullanıcılarını üretir. Aynı now için
// her zaman aynı listeyi döndürür.
func Generate(now time.Time) []User {
	now = now.UTC().Truncate(time.Minute)
	users := scenarioUsers(now)

	rng := rand.New(rand.NewPCG(20261117, 1))
	totalWeight := 0
	for _, c := range cities {
		totalWeight += c.weight
	}

	for i := 1; i <= RandomUsers; i++ {
		u := User{
			Email:    fmt.Sprintf("kullanici-%04d@example.test", i),
			Language: "tr",
		}
		if rng.IntN(100) >= 3 {
			u.FirstName = ptr(firstNames[rng.IntN(len(firstNames))])
		}
		if rng.IntN(100) >= 4 {
			u.Phone = ptr(fmt.Sprintf("+90555%07d", rng.IntN(10_000_000)))
		}
		if rng.IntN(100) < 10 {
			u.Language = "en"
		}

		pick := rng.IntN(totalWeight)
		for _, c := range cities {
			if pick < c.weight {
				u.City = c.name
				break
			}
			pick -= c.weight
		}

		if rng.IntN(100) < 60 { // sepete ekleyenler: son 90 gün
			u.LastCartAddAt = ptr(now.Add(-time.Duration(rng.IntN(90*24*60)) * time.Minute))
		}
		if rng.IntN(100) < 40 { // satın alanlar: son 120 gün
			u.LastPurchaseAt = ptr(now.Add(-time.Duration(rng.IntN(120*24*60)) * time.Minute))
			u.OrderCount = 1 + rng.IntN(8)
		}

		u.Consents = randomConsent(rng, "sms", 75, 10)
		u.Consents = append(u.Consents, randomConsent(rng, "email", 80, 10)...)
		users = append(users, u)
	}
	return users
}

// randomConsent, kanal için granted yüzdesi, reddedilen yüzdesi ve kalanı
// "kayıt yok" olacak şekilde bir izin kaydı (veya hiç) döndürür.
func randomConsent(rng *rand.Rand, channel string, grantedPct, deniedPct int) []Consent {
	switch n := rng.IntN(100); {
	case n < grantedPct:
		return []Consent{{Channel: channel, Granted: true}}
	case n < grantedPct+deniedPct:
		return []Consent{{Channel: channel, Granted: false}}
	default:
		return nil
	}
}

// scenarioUsers, örnek brief için tarih sınırı, satın alma ve kanal izni
// senaryolarını kapsayan sabit kullanıcılardır.
func scenarioUsers(now time.Time) []User {
	ago := func(d time.Duration) *time.Time { return ptr(now.Add(-d)) }
	both := []Consent{{"sms", true}, {"email", true}}

	return []User{
		{Email: "senaryo-iki-kanal@example.test", FirstName: ptr("Ada"), Phone: ptr("+905550000001"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(5 * day), Consents: both},
		{Email: "senaryo-sadece-sms@example.test", FirstName: ptr("Berk"), Phone: ptr("+905550000002"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(10 * day), LastPurchaseAt: ago(60 * day), OrderCount: 2,
			Consents: []Consent{{"sms", true}, {"email", false}}},
		{Email: "senaryo-sadece-eposta@example.test", FirstName: ptr("Ceren"), Phone: ptr("+905550000003"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(20 * day), Consents: []Consent{{"email", true}}},
		{Email: "senaryo-izinsiz@example.test", FirstName: ptr("Derya"), Phone: ptr("+905550000004"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(3 * day), Consents: []Consent{{"sms", false}, {"email", false}}},
		{Email: "senaryo-izin-kaydi-yok@example.test", FirstName: ptr("Efe"), Phone: ptr("+905550000005"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(7 * day), LastPurchaseAt: ago(45 * day), OrderCount: 1},
		{Email: "senaryo-yakin-alisveris@example.test", FirstName: ptr("Funda"), Phone: ptr("+905550000006"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(5 * day), LastPurchaseAt: ago(20 * day), OrderCount: 3, Consents: both},
		{Email: "senaryo-eski-sepet@example.test", FirstName: ptr("Gökhan"), Phone: ptr("+905550000007"), City: "İstanbul", Language: "tr",
			LastCartAddAt: ago(31 * day), Consents: both},
		{Email: "senaryo-baska-sehir@example.test", FirstName: ptr("Hande"), Phone: ptr("+905550000008"), City: "Ankara", Language: "tr",
			LastCartAddAt: ago(5 * day), Consents: both},
		{Email: "senaryo-sepet-yok@example.test", FirstName: ptr("Ilgın"), Phone: ptr("+905550000009"), City: "İstanbul", Language: "tr",
			Consents: both},
	}
}
