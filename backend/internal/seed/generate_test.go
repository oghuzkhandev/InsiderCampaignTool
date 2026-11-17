package seed

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

var refNow = time.Date(2026, 11, 17, 12, 30, 45, 0, time.UTC)

func TestGenerateIsDeterministic(t *testing.T) {
	a := Generate(refNow)
	b := Generate(refNow)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("aynı referans zaman aynı kullanıcıları üretmeli")
	}
	if want := RandomUsers + len(scenarioUsers(refNow)); len(a) != want {
		t.Fatalf("kullanıcı sayısı = %d, beklenen %d", len(a), want)
	}
}

func TestGenerateDatesFollowReferenceTime(t *testing.T) {
	later := refNow.Add(10 * day)
	a := Generate(refNow)
	b := Generate(later)
	if len(a) != len(b) {
		t.Fatal("kullanıcı sayısı referans zamana bağlı olmamalı")
	}
	for i := range a {
		if a[i].Email != b[i].Email || a[i].City != b[i].City || !reflect.DeepEqual(a[i].Consents, b[i].Consents) {
			t.Fatalf("%s: tarih dışındaki veri referans zamandan etkilenmemeli", a[i].Email)
		}
		if (a[i].LastCartAddAt == nil) != (b[i].LastCartAddAt == nil) {
			t.Fatalf("%s: sepet tarihi varlığı değişmemeli", a[i].Email)
		}
		if a[i].LastCartAddAt != nil && b[i].LastCartAddAt.Sub(*a[i].LastCartAddAt) != 10*day {
			t.Fatalf("%s: tarihler referans zamanla birlikte kaymalı", a[i].Email)
		}
	}
}

func TestGenerateProducesValidRows(t *testing.T) {
	users := Generate(refNow)
	emails := map[string]bool{}
	cityCount := map[string]int{}
	var noName, noPhone, noSMS, noEmail, denied int

	for _, u := range users {
		if emails[u.Email] {
			t.Fatalf("e-posta tekrar ediyor: %s", u.Email)
		}
		emails[u.Email] = true
		if !strings.HasSuffix(u.Email, "@example.test") {
			t.Fatalf("yalnızca example.test adresleri üretilmeli: %s", u.Email)
		}
		if u.City == "" || u.Language == "" || u.OrderCount < 0 {
			t.Fatalf("geçersiz satır: %#v", u)
		}
		for _, d := range []*time.Time{u.LastCartAddAt, u.LastPurchaseAt} {
			if d != nil && d.After(refNow) {
				t.Fatalf("%s: tarih gelecekte", u.Email)
			}
		}
		if u.LastPurchaseAt != nil && u.OrderCount == 0 {
			t.Fatalf("%s: satın alması olan kullanıcının sipariş sayısı sıfır olamaz", u.Email)
		}

		seen := map[string]bool{}
		hasSMS, hasEmail := false, false
		for _, c := range u.Consents {
			if c.Channel != "sms" && c.Channel != "email" {
				t.Fatalf("%s: geçersiz kanal %q", u.Email, c.Channel)
			}
			if seen[c.Channel] {
				t.Fatalf("%s: aynı kanal iki kez", u.Email)
			}
			seen[c.Channel] = true
			hasSMS = hasSMS || c.Channel == "sms"
			hasEmail = hasEmail || c.Channel == "email"
			if !c.Granted {
				denied++
			}
		}
		cityCount[u.City]++
		if u.FirstName == nil {
			noName++
		}
		if u.Phone == nil {
			noPhone++
		}
		if !hasSMS {
			noSMS++
		}
		if !hasEmail {
			noEmail++
		}
	}

	// Dağılım sağlıklı: yeterli çeşitlilik, hiçbir senaryo boş değil.
	if share := float64(cityCount["İstanbul"]) / float64(len(users)); share < 0.25 || share > 0.45 {
		t.Fatalf("İstanbul oranı %.2f beklenen aralıkta değil", share)
	}
	for name, n := range map[string]int{"adsız": noName, "telefonsuz": noPhone, "SMS kaydı yok": noSMS, "e-posta kaydı yok": noEmail, "izni reddedilmiş": denied} {
		if n == 0 {
			t.Fatalf("%s kullanıcı üretilmeli", name)
		}
	}
}

func TestScenarioUsersCoverExampleBriefCases(t *testing.T) {
	byEmail := map[string]User{}
	for _, u := range scenarioUsers(refNow) {
		byEmail[u.Email] = u
	}
	get := func(slug string) User {
		u, ok := byEmail["senaryo-"+slug+"@example.test"]
		if !ok {
			t.Fatalf("senaryo kullanıcısı yok: %s", slug)
		}
		return u
	}

	if u := get("yakin-alisveris"); u.LastPurchaseAt == nil || refNow.Sub(*u.LastPurchaseAt) != 20*day || refNow.Sub(*u.LastCartAddAt) != 5*day {
		t.Fatal("20 gün önce alışveriş + 5 gün önce sepet senaryosu eksik")
	}
	if u := get("eski-sepet"); refNow.Sub(*u.LastCartAddAt) <= 30*day {
		t.Fatal("30 gün dışında sepet senaryosu eksik")
	}
	if u := get("izin-kaydi-yok"); len(u.Consents) != 0 {
		t.Fatal("izin kaydı olmayan senaryo eksik")
	}
	if u := get("izinsiz"); len(u.Consents) != 2 || u.Consents[0].Granted || u.Consents[1].Granted {
		t.Fatal("izni reddedilmiş senaryo eksik")
	}
	if u := get("sadece-sms"); len(u.Consents) != 2 || !u.Consents[0].Granted || u.Consents[1].Granted {
		t.Fatal("yalnızca SMS senaryosu eksik")
	}
	if u := get("baska-sehir"); u.City == "İstanbul" {
		t.Fatal("başka şehir senaryosu eksik")
	}
}
