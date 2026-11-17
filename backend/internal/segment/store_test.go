package segment

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestSegmentMarshalsToStoredShape(t *testing.T) {
	seg := mustValidate(t, exampleBriefJSON)
	got, err := json.Marshal(seg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"all":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"last_cart_add_at","op":"within_last_days","value":30},{"field":"last_purchase_at","op":"not_within_last_days","value":30}]}`
	if string(got) != want {
		t.Fatalf("saklanan biçim:\n%s\nbeklenen:\n%s", got, want)
	}
}

func TestStoredShapeRoundTripsThroughValidate(t *testing.T) {
	seg := mustValidate(t, `{"all":[
		{"field":"city","op":"in","value":["İstanbul","Ankara"]},
		{"field":"order_count","op":"gte","value":2}
	],"unsupported":[]}`)
	stored, err := json.Marshal(seg)
	if err != nil {
		t.Fatal(err)
	}
	// Saklanan biçime `unsupported: []` eklenince aynı segment doğrulanmalı.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(stored, &top); err != nil {
		t.Fatal(err)
	}
	top["unsupported"] = json.RawMessage(`[]`)
	again, _ := json.Marshal(top)
	back, issues := Validate(again)
	if len(issues) != 0 {
		t.Fatalf("geri okunan segment geçerli olmalı: %v", issues)
	}
	if !reflect.DeepEqual(back.Conditions, seg.Conditions) {
		t.Fatalf("koşullar değişti: %#v != %#v", back.Conditions, seg.Conditions)
	}
}

func TestSegmentMarshalRefusesEmptyOrUnknown(t *testing.T) {
	if _, err := json.Marshal(Segment{}); !errors.Is(err, ErrEmptySegment) {
		t.Fatalf("boş segment için ErrEmptySegment bekleniyordu: %v", err)
	}
	bad := Segment{Conditions: []Condition{{Field: "email", Op: "eq", Text: "a@b.c"}}}
	if _, err := json.Marshal(bad); err == nil {
		t.Fatal("kataloğa uymayan koşul saklanmamalı")
	}
}

func TestParseStored(t *testing.T) {
	want := mustValidate(t, exampleBriefJSON)
	stored, _ := json.Marshal(want)
	got, err := ParseStored(stored)
	if err != nil || !reflect.DeepEqual(got.Conditions, want.Conditions) {
		t.Fatalf("ParseStored = %#v, %v", got, err)
	}

	bad := []string{
		``, `null`, `[]`, `{}`, `{"all":[]}`, `{"all":null}`,
		`{"all":[{"field":"city","op":"eq","value":"x"}],"unsupported":[]}`,
		`{"all":[{"field":"city","op":"eq","value":1}]}`,
		`{"all":[{"field":"x","op":"eq","value":"a"}]}`,
		`{"any":[]}`,
	}
	for _, raw := range bad {
		if _, err := ParseStored([]byte(raw)); !errors.Is(err, ErrInvalidStored) {
			t.Fatalf("%q için ErrInvalidStored bekleniyordu: %v", raw, err)
		}
	}
}
