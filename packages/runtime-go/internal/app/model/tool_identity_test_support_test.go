package model

import (
	"crypto/sha256"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func modelTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.model-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
