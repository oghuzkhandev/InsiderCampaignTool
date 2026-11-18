// Package content, LLM'in ürettiği SMS ve e-posta içeriğini doğrular ve alıcıya
// göre render eder. Model çıktısı güvenilmeyen girdidir; yalnızca bu paketin
// doğruladığı içerik sürüm olarak kaydedilebilir (CNT-001).
package content

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sınırlar CNT-001 spec'indedir. Uzunluk Unicode kod noktası (rune) sayısıdır.
const (
	MaxSMSRunes     = 160
	MaxSubjectRunes = 80

	// FallbackName, adı bulunmayan alıcı için {{first_name}} yerine yazılır
	// (ADR-022).
	FallbackName = "Değerli müşterimiz"

	// VarFirstName, izin verilen tek değişkenin adıdır.
	VarFirstName = "first_name"

	placeholder = "{{" + VarFirstName + "}}"

	maxEchoRunes = 64
)

// Alan adları, modelin JSON çıktısındaki anahtarlardır.
const (
	FieldSMSText      = "sms_text"
	FieldEmailSubject = "email_subject"
	FieldEmailBody    = "email_body"
)

// fieldOrder, sorunların kararlı sırada raporlanmasını sağlar.
var fieldOrder = []string{FieldSMSText, FieldEmailSubject, FieldEmailBody}

// Content, doğrulanmış (veya render edilmiş) bir içerik sürümünün metinleridir.
type Content struct {
	SMSText      string `json:"sms_text"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

func (c Content) field(name string) string {
	switch name {
	case FieldSMSText:
		return c.SMSText
	case FieldEmailSubject:
		return c.EmailSubject
	default:
		return c.EmailBody
	}
}

// Render, {{first_name}} değişkenini alıcının adıyla değiştirir. Ad boşsa veya
// yalnızca boşluktan oluşuyorsa FallbackName kullanılır. Ad tek geçişte
// yerleştirilir; adın içindeki süslü parantezler yeniden yorumlanmaz.
func Render(c Content, firstName string) Content {
	name := displayName(firstName)
	return Content{
		SMSText:      strings.ReplaceAll(c.SMSText, placeholder, name),
		EmailSubject: strings.ReplaceAll(c.EmailSubject, placeholder, name),
		EmailBody:    strings.ReplaceAll(c.EmailBody, placeholder, name),
	}
}

// displayName, alıcı adını render için hazırlar: kontrol karakterlerini
// (satır sonu dahil) boşluğa çevirir, kenar boşluklarını atar, boşsa fallback
// döndürür. E-posta konusuna satır sonu girmesini böylece engeller.
func displayName(firstName string) string {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, firstName))
	if name == "" {
		return FallbackName
	}
	return name
}

// DisplayName, alıcının render'da görünecek adını döndürür: ad boşsa veya
// yalnızca boşluksa FallbackName.
func DisplayName(firstName string) string { return displayName(firstName) }

// CheckSMS, SMS metnini gerçek bir alıcı adıyla render edip 160 kod noktası
// sınırını kontrol eder. Yalnızca SMS alıcıları için anlamlıdır.
func CheckSMS(c Content, firstName string) []Issue {
	if utf8.RuneCountInString(Render(c, firstName).SMSText) > MaxSMSRunes {
		return []Issue{{Code: CodeTooLong, Field: FieldSMSText}}
	}
	return nil
}

// CheckSubject, e-posta konusunu gerçek bir alıcı adıyla render edip 80 kod
// noktası sınırını kontrol eder. Yalnızca e-posta alıcıları için anlamlıdır.
func CheckSubject(c Content, firstName string) []Issue {
	if utf8.RuneCountInString(Render(c, firstName).EmailSubject) > MaxSubjectRunes {
		return []Issue{{Code: CodeTooLong, Field: FieldEmailSubject}}
	}
	return nil
}

// CheckRendered, her iki kanalın sınırını tek alıcı için kontrol eder. Onayda
// alıcı yalnızca kendi kanalının metniyle ilgilidir; orada CheckSMS ve
// CheckSubject kanal bazında kullanılır (ARCHITECTURE §6).
func CheckRendered(c Content, firstName string) []Issue {
	return append(CheckSMS(c, firstName), CheckSubject(c, firstName)...)
}

// clip, modelden gelen metni kullanıcıya veya düzeltme isteğine koymadan önce
// temizler: kontrol karakterlerini boşluğa çevirir ve rune sayısına göre kırpar.
func clip(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}
