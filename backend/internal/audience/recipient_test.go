package audience

import "testing"

func TestRecipientAddresses(t *testing.T) {
	tests := []struct {
		name      string
		r         Recipient
		wantPhone string
		wantSMS   bool
		wantMail  string
		wantEmail bool
	}{
		{"iki kanala uygun", Recipient{Phone: "+905550000001", Email: "a@x.test", SMSEligible: true, EmailEligible: true}, "+905550000001", true, "a@x.test", true},
		{"yalnızca SMS'e uygun", Recipient{Phone: "+905550000001", SMSEligible: true}, "+905550000001", true, "", false},
		{"kanala uygun değil (izin yok)", Recipient{Phone: "+905550000001", Email: "a@x.test"}, "+905550000001", false, "a@x.test", false},
		{"uygun işaretli ama adres boş (savunma)", Recipient{Phone: "", Email: "  ", SMSEligible: true, EmailEligible: true}, "", false, "", false},
		{"ASCII boşluklar atılır", Recipient{Phone: " \t+905550000001\n", Email: " a@x.test ", SMSEligible: true, EmailEligible: true}, "+905550000001", true, "a@x.test", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phone, okSMS := tt.r.SMSAddress()
			mail, okMail := tt.r.EmailAddress()
			if okSMS != tt.wantSMS || (okSMS && phone != tt.wantPhone) {
				t.Fatalf("SMSAddress = %q, %v", phone, okSMS)
			}
			if okMail != tt.wantEmail || (okMail && mail != tt.wantMail) {
				t.Fatalf("EmailAddress = %q, %v", mail, okMail)
			}
		})
	}
}
