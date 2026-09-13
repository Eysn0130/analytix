package privacyprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPrivateSourceProseProjectsStrictRawJSON(t *testing.T) {
	for _, fixture := range []struct {
		raw             json.RawMessage
		historicalValid bool
	}{
		{json.RawMessage(`"/Users/private-owner/SYNTHETIC-PII.csv"`), true},
		// Existing admission also rejects non-canonical RawMessage bytes.
		{json.RawMessage(`{"text":"\u002fUsers/private-owner/SYNTHETIC-PII.csv"}`), false},
	} {
		input := map[string]any{"text": fixture.raw}
		projected, changed := ProjectPublicValue(input)
		if !changed || ValidatePublicSourceProse(input) == nil || ValidatePublicSourceProse(projected) != nil || ValidatePublicValue(projected) != nil {
			t.Fatal("RawMessage prose bypassed projection")
		}
		if (ValidatePublicValue(input) == nil) != fixture.historicalValid {
			t.Fatal("output policy changed historical RawMessage admission")
		}
		encoded, err := json.Marshal(projected)
		if err != nil || strings.Contains(string(encoded), "SYNTHETIC-PII.csv") || !strings.Contains(string(encoded), "[PRIVATE_PATH]") {
			t.Fatal("RawMessage retained a locator")
		}
	}
}

func TestPrivateSourceTextPreservesOrdinarySemantics(t *testing.T) {
	for _, locator := range []string{
		"/Users/private-owner/SYNTHETIC-PII.csv", "/home/person/input.csv", "/Volumes/evidence/input.csv",
		"/private/tmp/input.csv", "/var/data/input.csv", "/tmp/input.csv", "/cases/input.csv",
		`C:\Users\person\input.csv`, "D:/data/input.csv", "file:///Users/person/input.csv",
		"~/Documents/input.csv", `\\server\private\input.csv`,
	} {
		input := "SYNTHETIC_WRITE_PRIVACY_01\nSource: " + locator + "\nordinary excerpt"
		got := ProjectPrivateSourceText(input)
		if got != "SYNTHETIC_WRITE_PRIVACY_01\nSource: [PRIVATE_PATH]\nordinary excerpt" {
			t.Fatalf("source locator projection failed: %q", got)
		}
		if ProjectPrivateSourceText(got) != got {
			t.Fatal("projection is not idempotent")
		}
		if ContainsRestrictedPII(locator) {
			t.Fatal("a locator changed the PII-only case risk predicate")
		}
	}
	for _, ordinary := range []string{"src/main/index.ts", "../src/main.ts", "/plan", "/usr/bin/env", "https://example.com/home/file"} {
		if ProjectPrivateSourceText(ordinary) != ordinary {
			t.Fatalf("ordinary location changed: %q", ordinary)
		}
	}
	if !ContainsRestrictedPII("/Users/person/13800138000.csv") {
		t.Fatal("path obscured existing PII classification")
	}
	if got := ProjectPrivateSourceText("https://example.com/home/file /Users/person/input.csv"); got != "https://example.com/home/file [PRIVATE_PATH]" {
		t.Fatalf("remote URL hid a separate local locator: %q", got)
	}
}

func TestPrivateSourceTextMasksCompleteQuotedFilenames(t *testing.T) {
	for _, delimiter := range []string{"`", "\"", "'"} {
		input := "Source: " + delimiter + "/Users/private-owner/Case Files/SYNTHETIC-PII (copy)[v1].csv" + delimiter + " ordinary excerpt"
		want := "Source: " + delimiter + "[PRIVATE_PATH]" + delimiter + " ordinary excerpt"
		if got := ProjectPrivateSourceText(input); got != want {
			t.Fatalf("quoted path suffix escaped: %q", got)
		}
	}
	if got := ProjectPrivateSourceText("Source: /Users/private-owner/Case(copy)[v1]/SYNTHETIC-PII.csv ordinary excerpt"); got != "Source: [PRIVATE_PATH] ordinary excerpt" {
		t.Fatalf("unquoted bracketed path suffix escaped: %q", got)
	}
}

func TestPrivateSourceProseDoesNotRebindTypedPaths(t *testing.T) {
	const path = "/Users/private-owner/SYNTHETIC-PII.csv"
	input := map[string]any{
		"workspace": "/Users/developer/project", "workspaceRoot": "/Users/developer/project",
		"path": path, "relativePath": "src/main.ts", "planId": "plan_authority", "contentHash": strings.Repeat("a", 64),
		"text": "Source: " + path, "sourceRequest": "Review " + path,
		"items": []any{map[string]any{"text": "Excerpt: " + path}},
	}
	projected, changed := ProjectPublicValue(input)
	if !changed {
		t.Fatal("ordinary prose retained a private locator")
	}
	out := projected.(map[string]any)
	for _, key := range []string{"workspace", "workspaceRoot", "path", "relativePath", "planId", "contentHash"} {
		if !reflect.DeepEqual(out[key], input[key]) {
			t.Fatalf("typed binding changed: %s", key)
		}
	}
	if out["text"] != "Source: [PRIVATE_PATH]" || out["sourceRequest"] != "Review [PRIVATE_PATH]" ||
		out["items"].([]any)[0].(map[string]any)["text"] != "Excerpt: [PRIVATE_PATH]" {
		t.Fatal("nested prose was not projected")
	}
	if input["text"] != "Source: "+path {
		t.Fatal("private input was mutated")
	}
	if ValidatePublicSourceProse(input) == nil || ValidatePublicSourceProse(out) != nil || ValidatePublicValue(out) != nil {
		t.Fatal("public validation disagrees with projection")
	}
	if ValidatePublicValue(input) != nil {
		t.Fatal("output policy changed historical admission")
	}
	plan, _ := ProjectUntrustedValue(input)
	if plan.(map[string]any)["workspaceRoot"] != input["workspaceRoot"] {
		t.Fatal("GUI plan workspace was rebound")
	}
}
