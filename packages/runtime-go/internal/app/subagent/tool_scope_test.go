package subagent

import "testing"

func TestSubagentMCPServerBlocklistRejectsAmbiguousIdentity(t *testing.T) {
	if got := MCPServerIDFromToolName("mcp__docs__lookup"); got != "docs" {
		t.Fatalf("valid MCP tool identity failed: %q", got)
	}
	if got := MCPServerIDFromToolName("mcp__foo__bar__lookup"); got != "foo" {
		t.Fatalf("valid repeated-underscore MCP tool identity failed: %q", got)
	}
	for _, name := range []string{"mcp__Docs__lookup", "mcp____lookup", "mcp__docs__"} {
		if got := MCPServerIDFromToolName(name); got != "" {
			t.Fatalf("ambiguous MCP tool identity was parsed: %q => %q", name, got)
		}
	}
}
