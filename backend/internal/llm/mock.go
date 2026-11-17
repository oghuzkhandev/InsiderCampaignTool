package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Mock, gerçek API anahtarı olmadan demo ve testlerde kullanılan kural tabanlı
// bir istemcidir (LLM_PROVIDER=mock). Brief'ten şehir, sepete ekleme ve satın
// almama koşullarını çıkarır; çıkaramadığı kısımları `unsupported` listesine
// yazar. Aynı brief her zaman aynı çıktıyı üretir.
type Mock struct{}

// NewMock, kural tabanlı MockLLM döndürür.
func NewMock() *Mock { return &Mock{} }

var knownCities = []string{"İstanbul", "Ankara", "İzmir", "Bursa", "Antalya"}

var daysPattern = regexp.MustCompile(`son\s+(\d+)\s+gun`)

type mockCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type mockOutput struct {
	All         []mockCondition `json:"all"`
	Unsupported []string        `json:"unsupported"`
}

// GenerateSegment, brief'i kurallarla yorumlayıp JSON metni döndürür.
func (m *Mock) GenerateSegment(ctx context.Context, req SegmentRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", ctxError(ctx)
	}

	text := fold(req.Brief)
	out := mockOutput{All: []mockCondition{}, Unsupported: []string{}}

	var cities []string
	for _, c := range knownCities {
		if strings.Contains(text, fold(c)) {
			cities = append(cities, c)
		}
	}
	switch len(cities) {
	case 0:
	case 1:
		out.All = append(out.All, mockCondition{Field: "city", Op: "eq", Value: cities[0]})
	default:
		out.All = append(out.All, mockCondition{Field: "city", Op: "in", Value: cities})
	}

	days := 30
	if m := daysPattern.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			days = n
		}
	}
	if strings.Contains(text, "sepet") {
		out.All = append(out.All, mockCondition{Field: "last_cart_add_at", Op: "within_last_days", Value: days})
	}
	if containsAny(text, "alisveris", "satin") && containsAny(text, "yapmayan", "yapmamis", "almayan", "almamis") {
		out.All = append(out.All, mockCondition{Field: "last_purchase_at", Op: "not_within_last_days", Value: days})
	}

	// `unsupported` öğeleri brief'in ifade edilemeyen kısmını anlatır; arayüz
	// bunu "Brief'in şu kısmı ifade edilemedi: ..." cümlesinin içinde gösterir.
	if len(out.All) == 0 {
		out.Unsupported = append(out.Unsupported, clipRunes(strings.TrimSpace(req.Brief), 200))
	} else if len(cities) < 2 && containsAny(text, " veya ", " ya da ") {
		out.Unsupported = append(out.Unsupported, "veya ile birleşen koşullar")
	}

	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var asciiFold = strings.NewReplacer(
	"İ", "i", "I", "i", "ı", "i",
	"Ş", "s", "ş", "s", "Ğ", "g", "ğ", "g",
	"Ü", "u", "ü", "u", "Ö", "o", "ö", "o",
	"Ç", "c", "ç", "c",
)

// fold, Türkçe karakterleri ASCII'ye ve küçük harfe çevirir.
func fold(s string) string { return strings.ToLower(asciiFold.Replace(s)) }

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// clipRunes, s'yi en fazla n karakter (rune) olacak şekilde kırpar.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
