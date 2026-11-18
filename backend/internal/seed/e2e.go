package seed

import (
	"fmt"
	"time"
)

// Veri kümeleri (cmd/seed, SEED_DATASET).
const (
	// DatasetDemo, birkaç bin rastgele kullanıcılık demo verisidir (varsayılan).
	DatasetDemo = "demo"
	// DatasetE2E, uçtan uca testlerin kullandığı küçük ve kontrollü veridir.
	DatasetE2E = "e2e"
)

// E2EBrief, E2E veri kümesinin sayılarının geçerli olduğu brief'tir (CASE.md'deki
// örnek brief). MockLLM bunu şu koşullara çevirir: şehir İstanbul, son 30 günde
// sepete eklemiş ve son 30 günde satın almamış.
const E2EBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

// E2EExpected, E2EBrief için E2E veri kümesinin kesin kitle sayılarıdır. Frontend
// e2e testleri (frontend/e2e) aynı sayıları kullanır; birini değiştirirseniz
// ikisini birlikte güncelleyin.
var E2EExpected = struct{ Total, SMS, Email int }{Total: 7, SMS: 3, Email: 4}

// E2EMinBoundaryDistance, E2E verisindeki her tarihin 30 günlük sınırdan en az bu
// kadar uzak olduğunu söyler. Önizleme ile onay arasında geçen süre veya ortamın
// yaşı kitle sayısını değiştirmez; bu yüzden sayılar kararlıdır.
const E2EMinBoundaryDistance = 15 * day

// GenerateE2E, uçtan uca testler için kontrollü kullanıcıları üretir. Hiçbir
// rastgelelik yoktur; E2EBrief için sayılar E2EExpected'tır:
//
//	eşleşenler (İstanbul, sepet son 30 günde, son 30 günde satın alma yok): 7 kişi
//	  1, 2: iki kanala izinli ve adresli
//	  3:    yalnızca SMS izinli, 4 ve 5: yalnızca e-posta izinli
//	  6:    SMS izinli ama telefonu yok (toplamda sayılır, SMS'e uygun değil)
//	  7:    hiç izin kaydı yok
//	eşleşmeyenler: yakın satın alma, 30 günden eski sepet, başka şehir
func GenerateE2E(now time.Time) []User {
	now = now.UTC().Truncate(time.Minute)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	both := []Consent{{"sms", true}, {"email", true}}
	phone := func(n int) *string { return ptr(fmt.Sprintf("+90555000%04d", n)) }

	u := func(n int, name string, city string, cart, purchase *time.Time, phoneNo *string, consents []Consent) User {
		return User{
			Email: fmt.Sprintf("e2e-%02d@example.test", n), FirstName: ptr(name), Phone: phoneNo,
			City: city, Language: "tr", LastCartAddAt: cart, LastPurchaseAt: purchase, Consents: consents,
		}
	}
	users := []User{
		u(1, "Ada", "İstanbul", ago(5*day), nil, phone(1), both),
		u(2, "Berk", "İstanbul", ago(12*day), ago(90*day), phone(2), both),
		u(3, "Ceren", "İstanbul", ago(8*day), nil, phone(3), []Consent{{"sms", true}, {"email", false}}),
		u(4, "Deniz", "İstanbul", ago(6*day), nil, phone(4), []Consent{{"email", true}}),
		u(5, "Ece", "İstanbul", ago(7*day), nil, nil, []Consent{{"email", true}}),
		u(6, "Fatih", "İstanbul", ago(9*day), nil, nil, []Consent{{"sms", true}}),
		u(7, "Gizem", "İstanbul", ago(4*day), nil, phone(7), nil),

		// Eşleşmeyenler.
		u(8, "Hande", "İstanbul", ago(5*day), ago(5*day), phone(8), both), // yakın satın alma
		u(9, "Ilgın", "İstanbul", ago(60*day), nil, phone(9), both),       // sepet pencerenin dışında
		u(10, "Jale", "Ankara", ago(5*day), nil, phone(10), both),         // başka şehir
	}
	for i := range users {
		if users[i].LastPurchaseAt != nil {
			users[i].OrderCount = 1
		}
	}
	return users
}
