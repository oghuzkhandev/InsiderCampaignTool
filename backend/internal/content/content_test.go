package content

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderReplacesFirstName(t *testing.T) {
	c := Content{
		SMSText:      "Merhaba {{first_name}}!",
		EmailSubject: "{{first_name}}, fırsat",
		EmailBody:    "Selam {{first_name}},\n{{first_name}} için özel.",
	}
	got := Render(c, "Ayşe")
	want := Content{
		SMSText:      "Merhaba Ayşe!",
		EmailSubject: "Ayşe, fırsat",
		EmailBody:    "Selam Ayşe,\nAyşe için özel.",
	}
	if got != want {
		t.Fatalf("Render = %#v", got)
	}
}

func TestRenderUsesFallbackForMissingName(t *testing.T) {
	c := Content{SMSText: "Merhaba {{first_name}}!", EmailSubject: "{{first_name}}", EmailBody: "{{first_name}}"}
	for _, name := range []string{"", "   ", "\n\t"} {
		got := Render(c, name)
		if got.SMSText != "Merhaba Değerli müşterimiz!" || got.EmailSubject != "Değerli müşterimiz" {
			t.Fatalf("%q için fallback bekleniyordu: %#v", name, got)
		}
	}
	if FallbackName != "Değerli müşterimiz" {
		t.Fatalf("fallback metni ADR-022 ile uyumlu olmalı: %q", FallbackName)
	}
}

func TestRenderDoesNotReinterpretNameContent(t *testing.T) {
	c := Content{SMSText: "Merhaba {{first_name}}", EmailSubject: "s", EmailBody: "b"}
	got := Render(c, "{{first_name}}{{first_name}}")
	if got.SMSText != "Merhaba {{first_name}}{{first_name}}" {
		t.Fatalf("ad yeniden genişletilmemeli: %q", got.SMSText)
	}
}

func TestRenderStripsControlCharactersFromName(t *testing.T) {
	c := Content{SMSText: "s", EmailSubject: "Selam {{first_name}}", EmailBody: "b"}
	got := Render(c, "Ali\r\nBcc: x@y.z")
	if strings.ContainsAny(got.EmailSubject, "\r\n") {
		t.Fatalf("konuda satır sonu olmamalı: %q", got.EmailSubject)
	}
}

func TestCheckRenderedSMSBoundaryWithRealNames(t *testing.T) {
	// 150 sabit karakter + ad.
	c := Content{SMSText: strings.Repeat("a", 150) + "{{first_name}}", EmailSubject: "konu", EmailBody: "gövde"}

	if issues := CheckRendered(c, "Zeynepşşşş"); len(issues) != 0 { // 10 kod noktası: 160
		t.Fatalf("160 kabul edilmeli: %v", issues)
	}
	issues := CheckRendered(c, "Zeynepşşşşş") // 11 kod noktası: 161
	if !reflect.DeepEqual(codes(issues), []string{"TOO_LONG:sms_text"}) {
		t.Fatalf("161 reddedilmeli: %v", issues)
	}
	// Adı olmayan alıcıya fallback (18) yazılır: 150 + 18 = 168.
	if issues := CheckRendered(c, ""); !reflect.DeepEqual(codes(issues), []string{"TOO_LONG:sms_text"}) {
		t.Fatalf("fallback ile aşım yakalanmalı: %v", issues)
	}
}

func TestCheckRenderedSubjectBoundary(t *testing.T) {
	c := Content{SMSText: "sms", EmailSubject: strings.Repeat("a", 75) + "{{first_name}}", EmailBody: "gövde"}
	if issues := CheckRendered(c, "Ahmet"); len(issues) != 0 { // 75 + 5 = 80
		t.Fatalf("80 kabul edilmeli: %v", issues)
	}
	if issues := CheckRendered(c, "Ahmetx"); !reflect.DeepEqual(codes(issues), []string{"TOO_LONG:email_subject"}) { // 81
		t.Fatalf("81 reddedilmeli: %v", issues)
	}
}

func TestCheckRenderedMultiByteName(t *testing.T) {
	// Bayt değil kod noktası sayılır: 5 harfli, 10 baytlık ad.
	c := Content{SMSText: strings.Repeat("a", 155) + "{{first_name}}", EmailSubject: "k", EmailBody: "b"}
	if issues := CheckRendered(c, "Çağşü"); len(issues) != 0 {
		t.Fatalf("5 kod noktası: 160 kabul edilmeli: %v", issues)
	}
}

// Onayda alıcı yalnızca kendi kanalının metniyle ilgilidir: yalnızca SMS
// alıcısının uzun adı konuyu, yalnızca e-posta alıcısının uzun adı SMS'i
// ilgilendirmez.
func TestCheckSMSAndCheckSubjectAreIndependentPerChannel(t *testing.T) {
	c := Content{
		SMSText:      strings.Repeat("a", 150) + "{{first_name}}", // 150 + ad
		EmailSubject: strings.Repeat("b", 75) + "{{first_name}}",  // 75 + ad
		EmailBody:    "g",
	}
	name := "Zeynepşşşşş" // 11 kod noktası: SMS 161, konu 86

	if got := codes(CheckSMS(c, name)); !reflect.DeepEqual(got, []string{"TOO_LONG:sms_text"}) {
		t.Fatalf("CheckSMS = %v", got)
	}
	if got := codes(CheckSubject(c, name)); !reflect.DeepEqual(got, []string{"TOO_LONG:email_subject"}) {
		t.Fatalf("CheckSubject = %v", got)
	}
	// Konuyu etkilemeyen, yalnızca SMS'i aşan metin.
	c.EmailSubject = "kısa"
	if len(CheckSubject(c, name)) != 0 {
		t.Fatal("kısa konu için SMS aşımı konuyu etkilememeli")
	}
	c.SMSText, c.EmailSubject = "kısa", strings.Repeat("b", 75)+"{{first_name}}"
	if len(CheckSMS(c, name)) != 0 {
		t.Fatal("kısa SMS için konu aşımı SMS'i etkilememeli")
	}
	// CheckRendered ikisini birlikte kontrol etmeye devam eder.
	c.SMSText = strings.Repeat("a", 150) + "{{first_name}}"
	if got := codes(CheckRendered(c, name)); !reflect.DeepEqual(got, []string{"TOO_LONG:sms_text", "TOO_LONG:email_subject"}) {
		t.Fatalf("CheckRendered = %v", got)
	}
}

func TestCheckSMSSubjectBoundariesAndFallback(t *testing.T) {
	sms := Content{SMSText: strings.Repeat("a", 143) + "{{first_name}}", EmailSubject: "k", EmailBody: "g"}
	// Adı olmayan alıcıya fallback (18 kod noktası) yazılır: 143 + 18 = 161.
	for _, name := range []string{"", "   "} {
		if len(CheckSMS(sms, name)) == 0 {
			t.Fatalf("%q: fallback ile 161 aşım olmalı", name)
		}
	}
	if len(CheckSMS(sms, "Ali")) != 0 { // 146
		t.Fatal("kısa adla sınırın altında kalmalı")
	}
	if DisplayName("") != FallbackName || DisplayName(" Ali ") != "Ali" {
		t.Fatalf("DisplayName: %q %q", DisplayName(""), DisplayName(" Ali "))
	}
}
