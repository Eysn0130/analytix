package runtimeapp

import (
	"crypto/sha256"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func runtimeCheckpointTestHostToolCallID(t *testing.T, seed string) string {
	t.Helper()
	entropy := sha256.Sum256([]byte("runtime-checkpoint-test-tool-call:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
