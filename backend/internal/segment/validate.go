package segment

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// groupKeys, desteklenmeyen mantıksal gruplamayı (OR, iç içe grup) gösterir.
var groupKeys = map[string]bool{
	"all": true, "any": true, "and": true, "or": true, "not": true,
}

// Validate, modelin ürettiği JSON'u doğrular. Sorun yoksa Segment döner.
// Sorun varsa Segment boştur ve sorunlar kararlı sırayla döner.
//
// Beklenen biçim: {"all": [{"field","op","value"}...], "unsupported": [...]}.
func Validate(raw []byte) (Segment, []Issue) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return Segment{}, []Issue{{Code: CodeMalformedOutput}}
	}

	var issues []Issue
	for _, key := range sortedKeys(top) {
		switch {
		case key == "all" || key == "unsupported":
		case groupKeys[key]:
			issues = append(issues, Issue{Code: CodeUnsupportedStructure, Field: clip(key, maxEchoRunes)})
		default:
			issues = append(issues, Issue{Code: CodeMalformedOutput, Field: clip(key, maxEchoRunes)})
		}
	}

	issues = append(issues, parseUnsupported(top["unsupported"])...)

	var conds []Condition
	var items []json.RawMessage
	rawAll, ok := top["all"]
	switch {
	case !ok || isNull(rawAll):
		issues = append(issues, Issue{Code: CodeMalformedOutput, Field: "all"})
	case json.Unmarshal(rawAll, &items) != nil:
		issues = append(issues, Issue{Code: CodeMalformedOutput, Field: "all"})
	case len(items) > maxConditions:
		issues = append(issues, Issue{Code: CodeUnsupportedStructure, Field: "all"})
	default:
		for _, item := range items {
			c, is := parseCondition(item)
			issues = append(issues, is...)
			if len(is) == 0 {
				conds = append(conds, c)
			}
		}
		if len(items) == 0 && len(issues) == 0 {
			issues = append(issues, Issue{Code: CodeEmptyFilter, Field: "all"})
		}
	}

	if len(issues) > 0 {
		return Segment{}, issues
	}
	return Segment{Conditions: conds}, nil
}

// parseUnsupported, modelin ifade edemediği kısımları okur. `unsupported`
// zorunludur; dolu liste her öğe için bir UNSUPPORTED_PART sorunu üretir.
func parseUnsupported(raw json.RawMessage) []Issue {
	if len(raw) == 0 || isNull(raw) {
		return []Issue{{Code: CodeMalformedOutput, Field: "unsupported"}}
	}
	var parts []string
	if err := json.Unmarshal(raw, &parts); err != nil {
		return []Issue{{Code: CodeMalformedOutput, Field: "unsupported"}}
	}
	var issues []Issue
	for _, p := range parts {
		text := clip(p, maxUnsupportedRunes)
		if text == "" {
			continue
		}
		if len(issues) == maxUnsupportedItems {
			break
		}
		issues = append(issues, Issue{Code: CodeUnsupportedPart, Text: text})
	}
	return issues
}

func parseCondition(raw json.RawMessage) (Condition, []Issue) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return Condition{}, []Issue{{Code: CodeMalformedOutput, Field: "all"}}
	}

	var issues []Issue
	for _, key := range sortedKeys(obj) {
		switch {
		case key == "field" || key == "op" || key == "value":
		case groupKeys[key]:
			issues = append(issues, Issue{Code: CodeUnsupportedStructure, Field: clip(key, maxEchoRunes)})
		default:
			issues = append(issues, Issue{Code: CodeMalformedOutput, Field: clip(key, maxEchoRunes)})
		}
	}
	if len(issues) > 0 {
		return Condition{}, issues
	}

	field, okField := stringOf(obj["field"])
	op, okOp := stringOf(obj["op"])
	if !okField || !okOp {
		return Condition{}, []Issue{{Code: CodeMalformedOutput, Field: "all"}}
	}

	ops, known := catalog[field]
	if !known {
		return Condition{}, []Issue{{Code: CodeUnsupportedField, Field: clip(field, maxEchoRunes)}}
	}
	kind, known := ops[op]
	if !known {
		return Condition{}, []Issue{{Code: CodeUnsupportedOperator, Field: field, Text: clip(op, maxEchoRunes)}}
	}

	cond := Condition{Field: field, Op: op}
	if code := parseValue(kind, obj["value"], &cond); code != "" {
		return Condition{}, []Issue{{Code: code, Field: field}}
	}
	return cond, nil
}

// parseValue, değeri operatörün beklediği tipe göre okur. Hata durumunda
// sebep kodunu döner.
func parseValue(kind valueKind, raw json.RawMessage, c *Condition) Code {
	if len(raw) == 0 || isNull(raw) {
		return CodeInvalidValueType
	}
	switch kind {
	case kindText:
		s, code := parseText(raw)
		c.Text = s
		return code
	case kindTextList:
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil {
			return CodeInvalidValueType
		}
		if len(list) == 0 || len(list) > maxListValues {
			return CodeInvalidRange
		}
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item == "" || utf8.RuneCountInString(item) > maxTextRunes {
				return CodeInvalidRange
			}
			c.Texts = append(c.Texts, item)
		}
		return ""
	case kindDays:
		n, code := parseInt(raw, 1, maxDays)
		c.Number = n
		return code
	case kindCount:
		n, code := parseInt(raw, 0, maxCount)
		c.Number = n
		return code
	}
	return CodeMalformedOutput
}

func parseText(raw json.RawMessage) (string, Code) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", CodeInvalidValueType
	}
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxTextRunes {
		return "", CodeInvalidRange
	}
	return s, ""
}

// parseInt yalnızca JSON sayısını kabul eder; "30" gibi metin, ondalıklı
// sayı, bool ve null tip hatasıdır.
func parseInt(raw json.RawMessage, min, max int) (int, Code) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || t[0] == '"' {
		return 0, CodeInvalidValueType
	}
	var f float64
	if err := json.Unmarshal(t, &f); err != nil || f != math.Trunc(f) {
		return 0, CodeInvalidValueType
	}
	if f < float64(min) || f > float64(max) {
		return 0, CodeInvalidRange
	}
	return int(f), ""
}

func stringOf(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || isNull(raw) {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
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

// clip, modelden gelen metni kullanıcıya göstermeden önce temizler: kontrol
// karakterlerini boşluğa çevirir ve rune sayısına göre kırpar.
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
