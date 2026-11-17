package segment

import (
	"reflect"
	"strings"
	"testing"
)

const exampleBriefJSON = `{
  "all": [
    {"field": "city", "op": "eq", "value": "İstanbul"},
    {"field": "last_cart_add_at", "op": "within_last_days", "value": 30},
    {"field": "last_purchase_at", "op": "not_within_last_days", "value": 30}
  ],
  "unsupported": []
}`

func codes(issues []Issue) []Code {
	var out []Code
	for _, is := range issues {
		out = append(out, is.Code)
	}
	return out
}

func TestValidateAcceptsSupportedConditions(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []Condition
	}{
		{
			name: "örnek brief",
			json: exampleBriefJSON,
			want: []Condition{
				{Field: "city", Op: "eq", Text: "İstanbul"},
				{Field: "last_cart_add_at", Op: "within_last_days", Number: 30},
				{Field: "last_purchase_at", Op: "not_within_last_days", Number: 30},
			},
		},
		{
			name: "şehir listesi",
			json: `{"all":[{"field":"city","op":"in","value":["İstanbul"," Ankara "]}],"unsupported":[]}`,
			want: []Condition{{Field: "city", Op: "in", Texts: []string{"İstanbul", "Ankara"}}},
		},
		{
			name: "sipariş sayısı aralığı",
			json: `{"all":[{"field":"order_count","op":"gte","value":2},{"field":"order_count","op":"lte","value":10}],"unsupported":[]}`,
			want: []Condition{
				{Field: "order_count", Op: "gte", Number: 2},
				{Field: "order_count", Op: "lte", Number: 10},
			},
		},
		{
			name: "tam sayı olarak yazılmış ondalık gösterim",
			json: `{"all":[{"field":"order_count","op":"eq","value":3.0}],"unsupported":[]}`,
			want: []Condition{{Field: "order_count", Op: "eq", Number: 3}},
		},
		{
			name: "SQL benzeri değer yalnızca değerdir",
			json: `{"all":[{"field":"city","op":"eq","value":"x'; DROP TABLE users; --"}],"unsupported":[]}`,
			want: []Condition{{Field: "city", Op: "eq", Text: "x'; DROP TABLE users; --"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg, issues := Validate([]byte(tt.json))
			if len(issues) != 0 {
				t.Fatalf("sorun beklenmiyordu: %v", issues)
			}
			if !reflect.DeepEqual(seg.Conditions, tt.want) {
				t.Fatalf("koşullar = %#v, beklenen %#v", seg.Conditions, tt.want)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
	manyConditions := `{"field":"order_count","op":"eq","value":1}`
	tooMany := `{"all":[` + strings.Repeat(manyConditions+",", 10) + manyConditions + `],"unsupported":[]}`

	tests := []struct {
		name       string
		json       string
		want       []Code
		repairable bool
	}{
		{"JSON değil", `bu json değil`, []Code{CodeMalformedOutput}, true},
		{"nesne değil", `[1,2]`, []Code{CodeMalformedOutput}, true},
		{"null", `null`, []Code{CodeMalformedOutput}, true},
		{"unsupported alanı eksik", `{"all":[{"field":"city","op":"eq","value":"İstanbul"}]}`, []Code{CodeMalformedOutput}, true},
		{"all alanı eksik", `{"unsupported":[]}`, []Code{CodeMalformedOutput}, true},
		{"bilinmeyen üst anahtar", `{"all":[{"field":"city","op":"eq","value":"İstanbul"}],"unsupported":[],"extra":1}`, []Code{CodeMalformedOutput}, true},
		{"değer eksik", `{"all":[{"field":"city","op":"eq"}],"unsupported":[]}`, []Code{CodeInvalidValueType}, true},
		{"gün sayısı metin", `{"all":[{"field":"last_cart_add_at","op":"within_last_days","value":"30"}],"unsupported":[]}`, []Code{CodeInvalidValueType}, true},
		{"gün sayısı ondalık", `{"all":[{"field":"last_cart_add_at","op":"within_last_days","value":1.5}],"unsupported":[]}`, []Code{CodeInvalidValueType}, true},
		{"şehir sayı", `{"all":[{"field":"city","op":"eq","value":34}],"unsupported":[]}`, []Code{CodeInvalidValueType}, true},
		{"şehir null", `{"all":[{"field":"city","op":"eq","value":null}],"unsupported":[]}`, []Code{CodeInvalidValueType}, true},
		{"in listesi değil", `{"all":[{"field":"city","op":"in","value":"İstanbul"}],"unsupported":[]}`, []Code{CodeInvalidValueType}, true},

		{"bilinmeyen alan", `{"all":[{"field":"favorite_color","op":"eq","value":"mavi"}],"unsupported":[]}`, []Code{CodeUnsupportedField}, false},
		{"SQL benzeri alan adı", `{"all":[{"field":"city; DROP TABLE users","op":"eq","value":"x"}],"unsupported":[]}`, []Code{CodeUnsupportedField}, false},
		{"desteklenmeyen operatör", `{"all":[{"field":"city","op":"like","value":"İst%"}],"unsupported":[]}`, []Code{CodeUnsupportedOperator}, false},
		{"alana uymayan operatör", `{"all":[{"field":"order_count","op":"within_last_days","value":3}],"unsupported":[]}`, []Code{CodeUnsupportedOperator}, false},
		{"gün sayısı sıfır", `{"all":[{"field":"last_cart_add_at","op":"within_last_days","value":0}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"gün sayısı çok büyük", `{"all":[{"field":"last_cart_add_at","op":"within_last_days","value":366}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"negatif sipariş sayısı", `{"all":[{"field":"order_count","op":"gte","value":-1}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"boş şehir", `{"all":[{"field":"city","op":"eq","value":"  "}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"çok uzun şehir", `{"all":[{"field":"city","op":"eq","value":"` + strings.Repeat("a", 65) + `"}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"boş in listesi", `{"all":[{"field":"city","op":"in","value":[]}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"çok değerli in listesi", `{"all":[{"field":"city","op":"in","value":["a","b","c","d","e","f","g","h","i","j","k"]}],"unsupported":[]}`, []Code{CodeInvalidRange}, false},
		{"boş filtre", `{"all":[],"unsupported":[]}`, []Code{CodeEmptyFilter}, false},
		{"üst düzey OR", `{"any":[{"field":"city","op":"eq","value":"İstanbul"}],"all":[],"unsupported":[]}`, []Code{CodeUnsupportedStructure}, false},
		{"koşul içinde iç içe grup", `{"all":[{"any":[{"field":"city","op":"eq","value":"İstanbul"}]}],"unsupported":[]}`, []Code{CodeUnsupportedStructure}, false},
		{"çok fazla koşul", tooMany, []Code{CodeUnsupportedStructure}, false},
		{"unsupported dolu", `{"all":[{"field":"city","op":"eq","value":"İstanbul"}],"unsupported":["ürün kategorisi"]}`, []Code{CodeUnsupportedPart}, false},
		{"unsupported dolu ve boş filtre", `{"all":[],"unsupported":["her şey"]}`, []Code{CodeUnsupportedPart}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg, issues := Validate([]byte(tt.json))
			if len(seg.Conditions) != 0 {
				t.Fatalf("reddedilen çıktıda koşul olmamalı: %#v", seg)
			}
			if got := codes(issues); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("kodlar = %v, beklenen %v (%v)", got, tt.want, issues)
			}
			if got := Repairable(issues); got != tt.repairable {
				t.Fatalf("Repairable = %v, beklenen %v", got, tt.repairable)
			}
		})
	}
}

func TestRepairableRequiresOnlySyntaxIssues(t *testing.T) {
	if Repairable(nil) {
		t.Fatal("sorunsuz çıktı düzeltilmez")
	}
	mixed := []Issue{{Code: CodeMalformedOutput}, {Code: CodeUnsupportedField}}
	if Repairable(mixed) {
		t.Fatal("desteklenmeyen alan içeren karışık sorun düzeltilmemeli")
	}
}

func TestValidateSanitizesEchoedModelText(t *testing.T) {
	long := strings.Repeat("ü", 500)
	_, issues := Validate([]byte(`{"all":[{"field":"a\nb\u0000c","op":"eq","value":"x"}],"unsupported":["` + long + `", "  ", "b"]}`))

	var sawField, sawPart bool
	for _, is := range issues {
		if strings.ContainsAny(is.Field+is.Text, "\n\x00") {
			t.Fatalf("kontrol karakteri temizlenmeli: %q", is)
		}
		if is.Code == CodeUnsupportedField {
			sawField = true
			if is.Field != "a b c" {
				t.Fatalf("alan adı = %q", is.Field)
			}
		}
		if is.Code == CodeUnsupportedPart && len([]rune(is.Text)) > maxUnsupportedRunes {
			t.Fatalf("unsupported metni kırpılmalı, uzunluk %d", len([]rune(is.Text)))
		}
		if is.Code == CodeUnsupportedPart {
			sawPart = true
		}
	}
	if !sawField || !sawPart {
		t.Fatalf("beklenen sorunlar eksik: %v", issues)
	}
}

func TestValidateLimitsUnsupportedItems(t *testing.T) {
	_, issues := Validate([]byte(`{"all":[],"unsupported":["1","2","3","4","5","6","7"]}`))
	if len(issues) != maxUnsupportedItems {
		t.Fatalf("en fazla %d unsupported öğesi gösterilmeli, gelen %d", maxUnsupportedItems, len(issues))
	}
}
