package persistencefs

import (
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	maxStrictJSONDepth          = 256
	maxSemanticControlDepth     = 64
	maxSemanticControlTokens    = 1_000_000
	maxSemanticControlStringLen = 4 << 10
)

func validateStrictJSON(data []byte, requireObject bool) error {
	return domainjsonstrict.Validate(data, domainjsonstrict.Options{
		RequireObject: requireObject,
		MaxDepth:      maxStrictJSONDepth,
	})
}

func validateSemanticControlJSON(data []byte, maxBytes int) error {
	return domainjsonstrict.Validate(data, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxBytes,
		MaxDepth:       maxSemanticControlDepth,
		MaxTokens:      maxSemanticControlTokens,
		MaxStringBytes: maxSemanticControlStringLen,
	})
}

func decodeStrictJSONObject(data []byte) (map[string]any, error) {
	return domainjsonstrict.DecodeObject(data, domainjsonstrict.Options{MaxDepth: maxStrictJSONDepth})
}
