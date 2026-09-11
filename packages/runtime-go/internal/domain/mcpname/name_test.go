package mcpname

import (
	"strings"
	"testing"
)

func TestCanonicalToolIdentityRejectsReservedSeparator(t *testing.T) {
	for _, input := range []struct{ server, tool string }{
		{server: "foo__bar", tool: "lookup"},
		{server: "analytix_funds__shadow", tool: "query"},
		{server: "foo", tool: "a_ _b"},
		{server: "Foo", tool: "lookup"},
		{server: "foo ", tool: "lookup"},
	} {
		if name, err := Canonical(input.server, input.tool); err == nil || name != "" {
			t.Fatalf("ambiguous namespace was accepted: server=%q tool=%q name=%q", input.server, input.tool, name)
		}
	}
	if name, err := Canonical("foo", "bar_lookup"); err != nil || name != "mcp__foo__bar_lookup" {
		t.Fatalf("valid canonical identity changed: name=%q err=%v", name, err)
	}
	for _, tool := range []string{"Lookup.V2", "bar__lookup", ".leading", strings.Repeat("A", 128)} {
		name, err := Canonical("foo", tool)
		server, parsedTool, ok := Parse(name)
		if err != nil || !ok || server != "foo" || parsedTool != tool {
			t.Fatalf("valid SEP-986 tool identity did not round-trip: tool=%q name=%q server=%q parsed=%q ok=%v err=%v", tool, name, server, parsedTool, ok, err)
		}
	}
	for _, tool := range []string{"", "contains space", "contains/slash", strings.Repeat("a", 129)} {
		if name, err := Canonical("foo", tool); err == nil || name != "" {
			t.Fatalf("invalid SEP-986 tool identity was accepted: tool=%q name=%q", tool, name)
		}
	}
}

func TestParseCanonicalToolIdentityRequiresExactlyThreeSegments(t *testing.T) {
	server, tool, ok := Parse("mcp__docs__lookup")
	if !ok || server != "docs" || tool != "lookup" {
		t.Fatalf("valid canonical identity was rejected: %q %q %v", server, tool, ok)
	}
	server, tool, ok = Parse("mcp__foo__bar__lookup")
	if !ok || server != "foo" || tool != "bar__lookup" {
		t.Fatalf("tool name containing the delimiter did not round-trip: %q %q %v", server, tool, ok)
	}
	for _, value := range []string{"lookup", " mcp__docs__lookup ", "mcp__Docs__lookup", "mcp__docs", "mcp____lookup", "mcp__docs__", "mcp__docs__bad name"} {
		if _, _, ok := Parse(value); ok {
			t.Fatalf("ambiguous identity was parsed: %q", value)
		}
	}
}
