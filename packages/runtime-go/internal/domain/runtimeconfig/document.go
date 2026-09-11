package runtimeconfig

import (
	"bytes"
	"errors"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const MaxJSONDocumentBytesV1 = 4 << 20

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// NormalizeAndValidateJSONDocumentV1 defines the shared runtime configuration
// JSON boundary. A single leading UTF-8 BOM is retained only as an import
// compatibility concession and is never part of the normalized snapshot.
func NormalizeAndValidateJSONDocumentV1(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxJSONDocumentBytesV1 {
		return nil, errors.New("runtime configuration JSON size is invalid")
	}
	normalized := raw
	if bytes.HasPrefix(normalized, utf8BOM) {
		normalized = normalized[len(utf8BOM):]
	}
	if len(normalized) == 0 || bytes.HasPrefix(normalized, utf8BOM) {
		return nil, errors.New("runtime configuration JSON BOM is invalid")
	}
	if err := domainjsonstrict.Validate(normalized, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxJSONDocumentBytesV1, MaxDepth: 32,
		MaxTokens: 131_072, MaxStringBytes: 1 << 20, MaxNumberBytes: 128, MaxAbsExponent: 308,
	}); err != nil {
		return nil, err
	}
	return append([]byte(nil), normalized...), nil
}
