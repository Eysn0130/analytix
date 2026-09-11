package runtimeconfig

import (
	"bytes"
	"testing"
)

func TestNormalizeAndValidateJSONDocumentV1AcceptsOneLeadingUTF8BOM(t *testing.T) {
	want := []byte(`{"runtime":{"enabled":true}}`)
	got, err := NormalizeAndValidateJSONDocumentV1(append([]byte{0xef, 0xbb, 0xbf}, want...))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("single BOM snapshot mismatch: got=%q err=%v", got, err)
	}
}

func TestNormalizeAndValidateJSONDocumentV1RejectsAmbiguousOrMalformedJSON(t *testing.T) {
	for _, body := range [][]byte{
		{},
		[]byte("\xef\xbb\xbf\xef\xbb\xbf{}"),
		{0xff, 0xfe, '{', '}'},
		[]byte(`null`),
		[]byte(`[]`),
		[]byte(`{"runtime":{},"runtime":{}}`),
		[]byte(`{"\u0072untime":{},"runtime":{}}`),
		[]byte(`{"runtime":{}} {"runtime":{}}`),
		[]byte(`{"value":1e309}`),
	} {
		if normalized, err := NormalizeAndValidateJSONDocumentV1(body); err == nil || normalized != nil {
			t.Fatalf("invalid runtime configuration passed: body=%q normalized=%q err=%v", body, normalized, err)
		}
	}
}

func TestNormalizeAndValidateJSONDocumentV1EnforcesSizeBound(t *testing.T) {
	body := append([]byte(`{"value":"`), bytes.Repeat([]byte{'x'}, MaxJSONDocumentBytesV1)...)
	body = append(body, []byte(`"}`)...)
	if _, err := NormalizeAndValidateJSONDocumentV1(body); err == nil {
		t.Fatal("oversized runtime configuration passed")
	}
}
