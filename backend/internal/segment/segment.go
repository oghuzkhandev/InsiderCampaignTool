// Package segment, LLM'in ürettiği segment kuralını doğrular ve parametreli
// SQL'e çevirir. Model çıktısı hiçbir zaman SQL metnine girmez.
package segment

// Code, bir doğrulama sorununun kararlı sebep kodudur. API bu kodları
// kullanıcıya iletir; model çıktısı ham olarak iletilmez.
type Code string

const (
	// Düzeltilebilir (söz dizimi) sorunlar.
	CodeMalformedOutput  Code = "MALFORMED_OUTPUT"
	CodeInvalidValueType Code = "INVALID_VALUE_TYPE"

	// Düzeltilmeden reddedilen sorunlar.
	CodeInvalidRange         Code = "INVALID_RANGE"
	CodeEmptyFilter          Code = "EMPTY_FILTER"
	CodeUnsupportedField     Code = "UNSUPPORTED_FIELD"
	CodeUnsupportedOperator  Code = "UNSUPPORTED_OPERATOR"
	CodeUnsupportedStructure Code = "UNSUPPORTED_STRUCTURE"
	CodeUnsupportedPart      Code = "UNSUPPORTED_PART"
)

// Sınırlar SEG-001 spec'indedir.
const (
	MaxBriefRunes = 1000

	maxConditions       = 10
	maxListValues       = 10
	maxTextRunes        = 64
	maxDays             = 365
	maxCount            = 100000
	maxUnsupportedItems = 5
	maxUnsupportedRunes = 200
	maxEchoRunes        = 64
)

// Desteklenen alanlar ve operatörler.
const (
	FieldCity         = "city"
	FieldLastCartAdd  = "last_cart_add_at"
	FieldLastPurchase = "last_purchase_at"
	FieldOrderCount   = "order_count"

	OpEq                = "eq"
	OpIn                = "in"
	OpGte               = "gte"
	OpLte               = "lte"
	OpWithinLastDays    = "within_last_days"
	OpNotWithinLastDays = "not_within_last_days"
)

// Issue, modelin çıktısındaki tek bir sorunu anlatır. Field ve Text modelden
// gelir; kontrol karakterleri temizlenir ve uzunluğu sınırlanır.
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

// Repairable, bütün sorunların söz dizimi hatası olup olmadığını söyler.
// Desteklenmeyen alan, operatör ve yapı için düzeltme yapılmaz: model koşulu
// sessizce silip kitleyi genişletebilir.
func Repairable(issues []Issue) bool {
	if len(issues) == 0 {
		return false
	}
	for _, is := range issues {
		if is.Code != CodeMalformedOutput && is.Code != CodeInvalidValueType {
			return false
		}
	}
	return true
}

// Condition, doğrulanmış tek bir koşuldur. Op'a göre Text, Texts veya Number
// dolu olur.
type Condition struct {
	Field  string
	Op     string
	Text   string   // city eq
	Texts  []string // city in
	Number int      // gün sayısı veya order_count değeri
}

// Segment, doğrulanmış koşulların AND ile birleşimidir.
type Segment struct {
	Conditions []Condition
}

type valueKind int

const (
	kindText valueKind = iota
	kindTextList
	kindDays
	kindCount
)

// catalog, desteklenen alan ve operatörlerin tek kaynağıdır. Doğrulayıcı ve
// SQL üretimi bunu kullanır.
var catalog = map[string]map[string]valueKind{
	FieldCity: {
		OpEq: kindText,
		OpIn: kindTextList,
	},
	FieldLastCartAdd: {
		OpWithinLastDays:    kindDays,
		OpNotWithinLastDays: kindDays,
	},
	FieldLastPurchase: {
		OpWithinLastDays:    kindDays,
		OpNotWithinLastDays: kindDays,
	},
	FieldOrderCount: {
		OpEq:  kindCount,
		OpGte: kindCount,
		OpLte: kindCount,
	},
}

// columns, mantıksal alanları sabit SQL sütunlarına eşler.
var columns = map[string]string{
	FieldCity:         "u.city",
	FieldLastCartAdd:  "u.last_cart_add_at",
	FieldLastPurchase: "u.last_purchase_at",
	FieldOrderCount:   "u.order_count",
}
