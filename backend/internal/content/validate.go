package content

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Code, bir doğrulama sorununun kararlı sebep kodudur. Model çıktısı ham olarak
// kullanıcıya iletilmez; Türkçe açıklama sebep koduna göre üretilir.
//
// Segment'ten farklı olarak bütün içerik sorunları otomatik düzeltme
// denemesine uygundur (CNT-001): yanlış içerik, kitleyi sessizce
// genişletmez.
type Code string

const (
	// CodeMalformedOutput: JSON değil, nesne değil, bilinmeyen anahtar, string
	// olmayan değer.
	CodeMalformedOutput Code = "MALFORMED_OUTPUT"
	// CodeMissingField: zorunlu alan yok veya null.
	CodeMissingField Code = "MISSING_FIELD"
	// CodeEmptyField: alan boş veya yalnızca boşluk.
	CodeEmptyField Code = "EMPTY_FIELD"
	// CodeUnknownVariable: {{first_name}} dışında bir değişken.
	CodeUnknownVariable Code = "UNKNOWN_VARIABLE"
	// CodeMalformedVariable: eşleşmeyen {{ veya }}.
	CodeMalformedVariable Code = "MALFORMED_VARIABLE"
	// CodeTooLong: SMS 160, e-posta konusu 80 kod noktasını aşıyor.
	CodeTooLong Code = "TOO_LONG"
	// CodeInvalidCharacter: NUL karakteri veya e-posta konusunda satır sonu.
	CodeInvalidCharacter Code = "INVALID_CHARACTER"
)

// Issue, modelin çıktısındaki tek bir sorunu anlatır. Field bilinen alan
// adıdır veya modelden gelen anahtardır (temizlenir, sınırlanır). Text yalnızca
// UNKNOWN_VARIABLE için modelin yazdığı değişken adıdır.
type Issue struct {
	Code  Code   `json:"code"`
	Field string `json:"field,omitempty"`
	Text  string `json:"text,omitempty"`
}

// String, düzeltme isteğinde modele gönderilen kısa açıklamadır.
func (i Issue) String() string {
	s := string(i.Code)
	if i.Field != "" {
		s += " alan=" + i.Field
	}
	if i.Text != "" {
		s += " metin=" + i.Text
	}
	return s
}

// Options, doğrulamanın dışarıdan bilinmesi gereken girdileridir.
type Options struct {
	// MaxNameRunes, alıcılar arasındaki en uzun gerçek first_name'in kod noktası
	// sayısıdır. Sıfırsa yalnızca FallbackName uzunluğu hesaba katılır.
	MaxNameRunes int
}

// worstCaseNameRunes, {{first_name}} yerine konacak en uzun ad uzunluğudur.
// Adı olmayan alıcıya fallback yazıldığı için ikisinin büyüğü alınır.
func (o Options) worstCaseNameRunes() int {
	n := utf8.RuneCountInString(FallbackName)
	if o.MaxNameRunes > n {
		n = o.MaxNameRunes
	}
	return n
}

// varPattern, {{...}} biçimindeki bütün yer tutucuları yakalar.
var varPattern = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// Validate, modelin ürettiği JSON'u doğrular. Sorun yoksa Content döner.
// Sorun varsa Content boştur ve sorunlar kararlı sırayla döner.
//
// Beklenen biçim: {"sms_text": "...", "email_subject": "...", "email_body": "..."}
// ve başka anahtar yok. Kontroller: JSON şeması, değişken whitelist'i,
// en uzun ad ile kod noktası uzunluğu.
func Validate(raw []byte, opts Options) (Content, []Issue) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return Content{}, []Issue{{Code: CodeMalformedOutput}}
	}

	var issues []Issue
	for _, key := range sortedKeys(top) {
		if !knownField(key) {
			issues = append(issues, Issue{Code: CodeMalformedOutput, Field: clip(key, maxEchoRunes)})
		}
	}

	values := make(map[string]string, len(fieldOrder))
	for _, name := range fieldOrder {
		rawValue, ok := top[name]
		if !ok || isNull(rawValue) {
			issues = append(issues, Issue{Code: CodeMissingField, Field: name})
			continue
		}
		var s string
		if err := json.Unmarshal(rawValue, &s); err != nil {
			issues = append(issues, Issue{Code: CodeMalformedOutput, Field: name})
			continue
		}
		values[name] = s
	}
	// Şema sorunu varsa metin kontrollerine geçilmez: eksik veya yanlış tipli
	// alan üzerinde uzunluk ve değişken kontrolü anlamsızdır.
	if len(issues) > 0 {
		return Content{}, issues
	}

	for _, name := range fieldOrder {
		issues = append(issues, checkText(name, values[name], opts)...)
	}
	if len(issues) > 0 {
		return Content{}, issues
	}
	return Content{
		SMSText:      values[FieldSMSText],
		EmailSubject: values[FieldEmailSubject],
		EmailBody:    values[FieldEmailBody],
	}, nil
}

// checkText, tek bir alanın metin kurallarını uygular.
func checkText(name, text string, opts Options) []Issue {
	if strings.TrimSpace(text) == "" {
		return []Issue{{Code: CodeEmptyField, Field: name}}
	}

	var issues []Issue
	if strings.ContainsRune(text, 0) {
		issues = append(issues, Issue{Code: CodeInvalidCharacter, Field: name})
	}
	// Konu başlık satırına girer; satır sonu başlık enjeksiyonuna açar.
	if name == FieldEmailSubject && strings.ContainsAny(text, "\r\n") {
		issues = append(issues, Issue{Code: CodeInvalidCharacter, Field: name})
	}

	for _, m := range varPattern.FindAllStringSubmatch(text, -1) {
		if m[1] != VarFirstName {
			issues = append(issues, Issue{Code: CodeUnknownVariable, Field: name, Text: clip(m[1], maxEchoRunes)})
		}
	}
	if rest := varPattern.ReplaceAllString(text, ""); strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		issues = append(issues, Issue{Code: CodeMalformedVariable, Field: name})
	}

	limit := 0
	switch name {
	case FieldSMSText:
		limit = MaxSMSRunes
	case FieldEmailSubject:
		limit = MaxSubjectRunes
	}
	if limit > 0 && worstCaseRunes(text, opts) > limit {
		issues = append(issues, Issue{Code: CodeTooLong, Field: name})
	}
	return issues
}

// worstCaseRunes, her {{first_name}} yerine en uzun adı koyarak kod noktası
// sayısını hesaplar.
func worstCaseRunes(text string, opts Options) int {
	count := strings.Count(text, placeholder)
	rest := strings.ReplaceAll(text, placeholder, "")
	return utf8.RuneCountInString(rest) + count*opts.worstCaseNameRunes()
}

func knownField(key string) bool {
	for _, f := range fieldOrder {
		if f == key {
			return true
		}
	}
	return false
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
