package jsonstrict

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	for _, body := range []string{
		`{"id":1,"id":2}`,
		`{"outer":{"id":1,"id":2}}`,
		`{"items":[{"id":1,"id":2}]}`,
	} {
		if err := Validate([]byte(body), Options{RequireObject: true}); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("duplicate fixture passed strict validation: body=%s err=%v", body, err)
		}
	}
	if err := Validate([]byte(`{"\u0069d":1,"id":2}`), Options{RequireObject: true}); err == nil {
		t.Fatal("escaped duplicate object key passed strict validation")
	}
}

func TestDecodeObjectPreservesNumbersAndRejectsTrailingJSON(t *testing.T) {
	value, err := DecodeObject([]byte(`{"amount":9007199254740993}`), Options{})
	if err != nil || value["amount"] != json.Number("9007199254740993") {
		t.Fatalf("strict decode lost number: value=%#v err=%v", value, err)
	}
	if _, err := DecodeObject([]byte(`{"ok":true}{"other":true}`), Options{}); err == nil {
		t.Fatal("trailing JSON passed strict validation")
	}
}

func TestDecodeObjectConstructingPathEnforcesEveryStrictLimit(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		options Options
	}{
		{name: "duplicate", body: []byte(`{"id":1,"id":2}`)},
		{name: "invalid_utf8", body: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}},
		{name: "invalid_surrogate", body: []byte(`{"x":"\uD800"}`)},
		{name: "depth", body: []byte(`{"a":{"b":1}}`), options: Options{MaxDepth: 1}},
		{name: "tokens", body: []byte(`{"one":1,"two":2}`), options: Options{MaxTokens: 3}},
		{name: "string", body: []byte(`{"text":"long"}`), options: Options{MaxStringBytes: 3}},
		{name: "number", body: []byte(`{"amount":1e10001}`), options: Options{MaxAbsExponent: 10000}},
		{name: "trailing", body: []byte(`{"ok":true}false`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeObject(test.body, test.options); err == nil {
				t.Fatalf("constructing decoder accepted %s fixture", test.name)
			}
		})
	}
}

func TestValidateEnforcesObjectSizeAndDepth(t *testing.T) {
	if err := Validate([]byte(`[]`), Options{RequireObject: true}); err == nil {
		t.Fatal("non-object passed object boundary")
	}
	if err := Validate([]byte(`{"ok":true}`), Options{MaxBytes: 4}); err == nil {
		t.Fatal("oversized JSON passed size boundary")
	}
	if err := Validate([]byte(`{"a":{"b":1}}`), Options{MaxDepth: 1}); err == nil {
		t.Fatal("over-depth JSON passed depth boundary")
	}
}

func TestValidateRejectsInvalidUnicodeAndComplexity(t *testing.T) {
	for _, body := range [][]byte{
		{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'},
		[]byte(`{"x":"\uD800"}`),
		[]byte(`{"x":"\uDC00"}`),
		[]byte(`{"x":"\uD800\u0041"}`),
	} {
		if err := Validate(body, Options{RequireObject: true}); err == nil {
			t.Fatalf("invalid Unicode fixture passed: %q", body)
		}
	}
	if err := Validate([]byte(`{"x":"\uD83D\uDE00"}`), Options{RequireObject: true}); err != nil {
		t.Fatalf("valid surrogate pair was rejected: %v", err)
	}
	if err := Validate([]byte(`{"one":1,"two":2}`), Options{MaxTokens: 3}); err == nil {
		t.Fatal("token limit was not enforced")
	}
	if err := Validate([]byte(`{"text":"long"}`), Options{MaxStringBytes: 3}); err == nil {
		t.Fatal("string limit was not enforced")
	}
}

func TestValidateBoundsExactNumbersBeforeBigParsing(t *testing.T) {
	for _, body := range []string{
		`{"amount":1e10001}`,
		`{"amount":1e-10001}`,
		`{"amount":123456789}`,
	} {
		options := Options{RequireObject: true, MaxNumberBytes: 8, MaxAbsExponent: 10000}
		if err := Validate([]byte(body), options); err == nil {
			t.Fatalf("oversized exact number passed: %s", body)
		}
	}
	if err := Validate([]byte(`{"amount":9007199254740993.0001e+10}`), Options{RequireObject: true}); err != nil {
		t.Fatalf("bounded exact number was rejected: %v", err)
	}
}

func TestDecodeRawObjectKeepsExactFieldCasing(t *testing.T) {
	object, err := DecodeRawObject([]byte(`{"code":1,"Code":2}`), Options{})
	if err != nil || len(object) != 2 || string(object["code"]) != "1" || string(object["Code"]) != "2" {
		t.Fatalf("raw object did not preserve exact keys: object=%#v err=%v", object, err)
	}
}
