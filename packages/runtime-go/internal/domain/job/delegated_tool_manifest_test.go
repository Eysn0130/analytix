package job

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestDelegatedToolManifestV1BindsCanonicalScopeAndHashes(t *testing.T) {
	schemaHash := domainsecurity.SHA256Hex([]byte("schema"))
	mcpHash := domainsecurity.SHA256Hex([]byte("mcp"))
	manifest, err := NewDelegatedToolManifestV1([]string{"mcp__docs__lookup", "read"}, schemaHash, mcpHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDelegatedToolManifestV1(manifest, []string{"read", "mcp__docs__lookup"}, schemaHash); err != nil {
		t.Fatalf("canonical scope order changed authority: %v", err)
	}
	tampered := CloneDelegatedToolManifestV1(manifest)
	tampered.MCPAuthorityHash = domainsecurity.SHA256Hex([]byte("other-mcp"))
	if err := ValidateDelegatedToolManifestV1(tampered, []string{"read", "mcp__docs__lookup"}, schemaHash); err == nil {
		t.Fatal("tampered MCP authority retained a valid delegated manifest")
	}
	if _, err := NewDelegatedToolManifestV1([]string{"read", "read"}, schemaHash, mcpHash); err == nil {
		t.Fatal("duplicate delegated scope was accepted")
	}
}
