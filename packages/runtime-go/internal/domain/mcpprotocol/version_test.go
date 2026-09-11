package mcpprotocol

import "testing"

func TestCurrentMCPProtocolVersionsAreFactAuthorityCapable(t *testing.T) {
	if PreferredVersion != "2025-11-25" || !SupportedVersion(PreferredVersion) || !FactAuthorityCapable("2025-06-18") {
		t.Fatal("current structured-output MCP revisions are not configured consistently")
	}
	for _, legacy := range []string{"", "2024-11-05", "2025-03-26"} {
		if SupportedVersion(legacy) || FactAuthorityCapable(legacy) {
			t.Fatalf("legacy or unknown MCP revision became fact-authority capable: %q", legacy)
		}
	}
}
