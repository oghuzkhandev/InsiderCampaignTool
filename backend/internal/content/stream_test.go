package content

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// assemble, Delta'ları önizleme alanlarına uygular (Replace dahil).
func assemble(deltas []Delta) Content {
	var c Content
	for _, d := range deltas {
		var dst *string
		switch d.Field {
		case FieldSMSText:
			dst = &c.SMSText
		case FieldEmailSubject:
			dst = &c.EmailSubject
		case FieldEmailBody:
			dst = &c.EmailBody
		default:
			panic("bilinmeyen alan: " + d.Field)
		}
		if d.Replace {
			*dst = ""
		}
		*dst += d.Text
	}
	return c
}

func feedAll(chunks ...string) []Delta {
	var p deltaParser
	var all []Delta
	for _, c := range chunks {
		all = append(all, p.Feed(c)...)
	}
	return all
}

// escapedJSON, Go'nun üretmeyeceği kaçışları (\u00fc, vekil çifti, \/, \b)
// içerir. Beklenen sonucu encoding/json hesaplar.
const escapedJSON = `{"sms_text":"Merhaba {{first_name}}, \"hoş\" geldin \u00fc \ud83d\ude00 \/ tab\t!",` +
	` "email_subject" : "Konu \u0041\u00DC",` +
	"\n\t" + `"email_body":"Satır1\nSatır2 \\ \b\f\r Çağşü 😀 \ud83c\udf89"}`

func wantFromEncodingJSON(t *testing.T, s string) Content {
	t.Helper()
	var c Content
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParserDecodesEscapesLikeEncodingJSON(t *testing.T) {
	want := wantFromEncodingJSON(t, escapedJSON)
	if got := assemble(feedAll(escapedJSON)); got != want {
		t.Fatalf("tek parça:\n got  %#v\n want %#v", got, want)
	}
}

// Parça sınırı her baytta olabilir: kaçış dizisinin ortası, \uXXXX'in ortası,
// vekil çiftinin arası ve çok baytlı UTF-8 karakterin ortası dahil.
func TestParserIsIndependentOfChunkBoundaries(t *testing.T) {
	inputs := []string{escapedJSON, string(raw("Merhaba {{first_name}} 😀 ğüşiöç", "Konu <&> \"x\"", "a\nb\n\nc \\ d"))}
	for _, in := range inputs {
		want := wantFromEncodingJSON(t, in)

		for i := 0; i <= len(in); i++ {
			if got := assemble(feedAll(in[:i], in[i:])); got != want {
				t.Fatalf("bölme noktası %d:\n got  %#v\n want %#v", i, got, want)
			}
		}

		var singles []string
		for i := 0; i < len(in); i++ {
			singles = append(singles, in[i:i+1])
		}
		if got := assemble(feedAll(singles...)); got != want {
			t.Fatalf("bayt bayt:\n got  %#v\n want %#v", got, want)
		}
	}
}

func TestParserEmitsFirstDeltaWithReplaceAndOnlyThen(t *testing.T) {
	deltas := feedAll(`{"sms_text":"Mer`, `haba","email_subject":"K`, `onu"}`)
	want := []Delta{
		{Field: "sms_text", Text: "Mer", Replace: true},
		{Field: "sms_text", Text: "haba"},
		{Field: "email_subject", Text: "K", Replace: true},
		{Field: "email_subject", Text: "onu"},
	}
	if !reflect.DeepEqual(deltas, want) {
		t.Fatalf("deltalar = %#v", deltas)
	}
}

func TestParserEmitsDeltaForEmptyValueSoPreviewIsCleared(t *testing.T) {
	deltas := feedAll(`{"sms_text":""}`)
	if len(deltas) != 1 || deltas[0] != (Delta{Field: "sms_text", Replace: true}) {
		t.Fatalf("deltalar = %#v", deltas)
	}
}

func TestParserDuplicateKeyReplacesPreview(t *testing.T) {
	in := `{"sms_text":"ilk","sms_text":"son"}`
	deltas := feedAll(in)
	if got := assemble(deltas); got.SMSText != "son" {
		t.Fatalf("son değer geçerli olmalı: %#v", got)
	}
	if want := wantFromEncodingJSON(t, in); want.SMSText != "son" {
		t.Fatalf("encoding/json ile aynı davranış beklenir: %#v", want)
	}
}

func TestParserSkipsUnknownKeysAndNonStringValues(t *testing.T) {
	in := `{"extra":"gizli","n":12.5,"arr":[1,{"sms_text":"iç"},"]}"],"obj":{"a":{"b":"}"}},` +
		`"t":true,"nul":null,"sms_text":"gerçek","email_subject":"k","email_body":"g"}`
	got := assemble(feedAll(in))
	want := Content{SMSText: "gerçek", EmailSubject: "k", EmailBody: "g"}
	if got != want {
		t.Fatalf("içerik = %#v", got)
	}
}

func TestParserNonStringKnownValueProducesNoDelta(t *testing.T) {
	if deltas := feedAll(`{"sms_text":5,"email_subject":["a"]}`); len(deltas) != 0 {
		t.Fatalf("string olmayan değer için önizleme olmamalı: %#v", deltas)
	}
}

func TestParserStopsQuietlyOnSyntaxErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want Content // hata noktasına kadar çözümlenenler
	}{
		{"markdown çiti", "```json\n{\"sms_text\":\"a\"}\n```", Content{}},
		{"düz metin", `Merhaba dünya`, Content{}},
		{"dizi", `["a"]`, Content{}},
		{"iki nokta eksik", `{"sms_text" "a"}`, Content{}},
		{"tırnaksız anahtar", `{sms_text:"a"}`, Content{}},
		{"geçersiz kaçış", `{"sms_text":"a\qb","email_subject":"k"}`, Content{SMSText: "a"}},
		{"ham satır sonu", "{\"sms_text\":\"a\nb\",\"email_subject\":\"k\"}", Content{SMSText: "a"}},
		{"bozuk \\u", `{"sms_text":"a\u12G4","email_subject":"k"}`, Content{SMSText: "a"}},
		{"değer yok", `{"sms_text":,"email_subject":"k"}`, Content{}},
		{"sondaki virgül", `{"sms_text":"a",}`, Content{SMSText: "a"}},
		{"değerden sonra çöp", `{"sms_text":"a" x,"email_subject":"k"}`, Content{SMSText: "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := assemble(feedAll(tc.in)); got != tc.want {
				t.Fatalf("içerik = %#v, beklenen %#v", got, tc.want)
			}
			// Hatadan sonra gelen parçalar da sessizce yok sayılır, panik yok.
			feedAll(tc.in, `{"sms_text":"x"}`, "\xff\xfe")
		})
	}
}

func TestParserIgnoresTextAfterObjectEnds(t *testing.T) {
	got := assemble(feedAll(`{"sms_text":"a"} {"sms_text":"b"}`, `{"sms_text":"c"}`))
	if got.SMSText != "a" {
		t.Fatalf("nesne bittikten sonrası yok sayılmalı: %#v", got)
	}
}

func TestParserHandlesLoneSurrogatesAndInvalidUTF8LikeEncodingJSON(t *testing.T) {
	in := "{\"sms_text\":\"a\\ud83dz \\ude00 b\xffc\"}"
	want := wantFromEncodingJSON(t, in)
	if got := assemble(feedAll(in)); got != want {
		t.Fatalf("got %q, want %q", got.SMSText, want.SMSText)
	}
	if !strings.Contains(want.SMSText, "\uFFFD") {
		t.Fatal("test girdisi U+FFFD üretmeli")
	}
}

func TestPartialExtractsParsableFieldsOfInvalidOutput(t *testing.T) {
	// SMS çok uzun olduğu için geçersiz; üç alan da çözümlenebilir.
	in := string(raw(strings.Repeat("a", 200), "Konu", "Gövde"))
	if got := Partial(in); got != (Content{SMSText: strings.Repeat("a", 200), EmailSubject: "Konu", EmailBody: "Gövde"}) {
		t.Fatalf("Partial = %#v", got)
	}
	// Eksik alan: yalnızca çözümlenenler.
	if got := Partial(`{"sms_text":"a","email_subject":"k"}`); got != (Content{SMSText: "a", EmailSubject: "k"}) {
		t.Fatalf("Partial = %#v", got)
	}
	// Hiç çözümlenemiyor.
	for _, in := range []string{"", "merhaba", "```json\n{}\n```", "[]"} {
		if got := Partial(in); got != (Content{}) {
			t.Fatalf("%q için boş beklenir: %#v", in, got)
		}
	}
	// Çift anahtarda son değer.
	if got := Partial(`{"sms_text":"ilk","sms_text":"son"}`); got.SMSText != "son" {
		t.Fatalf("Partial = %#v", got)
	}
}

func FuzzParserNeverPanicsAndMatchesEncodingJSONOnValidInput(f *testing.F) {
	for _, s := range []string{escapedJSON, `{}`, `{"sms_text":"a"}`, `{"sms_text":"\ud83d"}`, "\xff", `{"a":[1,2,{"b":"c"}],"sms_text":"x"}`, `{"sms_teXt":"000000"}`, `{"sms_text":null,"email_body":5}`} {
		f.Add(s, 3)
	}
	f.Fuzz(func(t *testing.T, in string, split int) {
		if split < 0 {
			split = -split
		}
		if len(in) > 0 {
			split %= len(in) + 1
		} else {
			split = 0
		}
		got := assemble(feedAll(in[:split], in[split:]))

		// Geçerli JSON ise encoding/json'ın harf duyarlı çözümüyle aynı sonucu
		// vermeli (son değer geçerli, string olmayan değer boş). Struct'a
		// Unmarshal harf duyarsız eşler; Validate ve çözücü birebir eşler, bu
		// yüzden oracle olarak map kullanılır. Çözücü gövdenin geri kalanını
		// doğrulamadığı için geçersiz girdide yalnızca panik olmaması beklenir.
		var generic map[string]any
		if json.Unmarshal([]byte(in), &generic) != nil || generic == nil {
			return
		}
		str := func(k string) string {
			s, _ := generic[k].(string)
			return s
		}
		want := Content{SMSText: str(FieldSMSText), EmailSubject: str(FieldEmailSubject), EmailBody: str(FieldEmailBody)}
		if got != want {
			t.Fatalf("girdi %q:\n got  %#v\n want %#v", in, got, want)
		}
	})
}

// Anahtarlar birebir eşlenir (Validate ile aynı): encoding/json'ın struct
// çözümünün aksine harf duyarsız eşleme yoktur.
func TestParserMatchesKeysCaseSensitively(t *testing.T) {
	if deltas := feedAll(`{"sms_teXt":"a","SMS_TEXT":"b","sms_text ":"c"}`); len(deltas) != 0 {
		t.Fatalf("farklı yazılmış anahtar alan sayılmamalı: %#v", deltas)
	}
	if _, issues := Validate([]byte(`{"sms_teXt":"a","email_subject":"b","email_body":"c"}`), Options{}); len(issues) == 0 {
		t.Fatal("Validate de aynı anahtarı reddetmeli")
	}
}
