package segment

import (
	"encoding/json"
	"errors"
	"fmt"
)

type storedCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// MarshalJSON, doğrulanmış segmenti veritabanında saklanan biçime çevirir:
// {"all":[{"field","op","value"}...]}. Modelin ham metni değil, doğrulanmış
// yapı yazılır. Koşulsuz veya kataloğa uymayan segment için hata döner.
func (s Segment) MarshalJSON() ([]byte, error) {
	if len(s.Conditions) == 0 {
		return nil, ErrEmptySegment
	}
	all := make([]storedCondition, 0, len(s.Conditions))
	for _, c := range s.Conditions {
		kind, ok := catalog[c.Field][c.Op]
		if !ok {
			return nil, fmt.Errorf("segment: desteklenmeyen koşul %q %q", c.Field, c.Op)
		}
		sc := storedCondition{Field: c.Field, Op: c.Op}
		switch kind {
		case kindText:
			sc.Value = c.Text
		case kindTextList:
			sc.Value = c.Texts
		default:
			sc.Value = c.Number
		}
		all = append(all, sc)
	}
	return json.Marshal(struct {
		All []storedCondition `json:"all"`
	}{all})
}

// ErrInvalidStored, saklanan segment beklenen biçimde olmadığında döner.
var ErrInvalidStored = errors.New("segment: saklanan biçim geçersiz")

// ParseStored, MarshalJSON'un yazdığı saklama biçimini okur ve doğrulayıcıdan
// geçirir. Saklanan biçimde `unsupported` anahtarı bulunmaz; varsa reddedilir.
func ParseStored(raw []byte) (Segment, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return Segment{}, ErrInvalidStored
	}
	if _, ok := top["unsupported"]; ok {
		return Segment{}, ErrInvalidStored
	}
	top["unsupported"] = json.RawMessage(`[]`)
	withList, err := json.Marshal(top)
	if err != nil {
		return Segment{}, ErrInvalidStored
	}
	seg, issues := Validate(withList)
	if len(issues) != 0 {
		return Segment{}, ErrInvalidStored
	}
	return seg, nil
}
