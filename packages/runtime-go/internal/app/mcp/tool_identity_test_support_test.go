package mcp

import (
	"crypto/sha256"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func mcpTestHostToolCallID(t *testing.T, seed string) string {
	t.Helper()
	entropy := sha256.Sum256([]byte("app-mcp-test-tool-call:\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
