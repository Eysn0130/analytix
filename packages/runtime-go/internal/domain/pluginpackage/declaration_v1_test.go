package pluginpackage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDeclarationV1ParsesClosedTypedSchemaAndOwnsCanonicalIdentity(t *testing.T) {
	declaration := validDeclarationV1()
	pretty, err := json.MarshalIndent(declaration, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDeclarationV1(pretty)
	if err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}
	if parsed.IdentityV1() != (PackageIdentityV1{PackageID: "analytix-fund-analysis", PackageVersion: "0.16.16"}) {
		t.Fatalf("canonical package identity drifted: %#v", parsed.IdentityV1())
	}
	if DeclarationRelativePathV1 != ".analytix-plugin/package.json" || parsed.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("canonical declaration owner drifted: path=%q schema=%d", DeclarationRelativePathV1, parsed.SchemaVersion)
	}
	canonical, err := CanonicalDeclarationV1Bytes(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(canonical, []byte("\n")) || bytes.HasPrefix(canonical, []byte(" ")) {
		t.Fatalf("canonical declaration retained presentation whitespace: %q", canonical)
	}
	if reparsed, err := ParseDeclarationV1(canonical); err != nil || reparsed.IdentityV1() != parsed.IdentityV1() {
		t.Fatalf("canonical declaration did not round trip: parsed=%#v err=%v", reparsed, err)
	}
}

func TestDeclarationV1CanonicalBytesNormalizeSetOrdering(t *testing.T) {
	left := validDeclarationV1()
	right := validDeclarationV1()
	reversePathContributions(right.Contributions.Skills)
	reverseCapabilityRequests(right.RequestedCapabilities)
	reverseStrings(right.RequestedCapabilities[0].ScopeConstraints)
	rightBefore := mustJSON(t, right)

	leftBytes, err := CanonicalDeclarationV1Bytes(left)
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := CanonicalDeclarationV1Bytes(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("equivalent declarations produced different canonical bytes:\nleft=%s\nright=%s", leftBytes, rightBytes)
	}
	if rightAfter := mustJSON(t, right); !bytes.Equal(rightBefore, rightAfter) {
		t.Fatalf("canonical encoding mutated its caller:\nbefore=%s\nafter=%s", rightBefore, rightAfter)
	}
}

func TestSemanticVersionV1UsesExactSemVerSyntax(t *testing.T) {
	for _, value := range []string{
		"0.0.0",
		"1.2.3-alpha-beta.1+build.5",
		"999999999999999999999999.0.1",
	} {
		if !validSemanticVersionV1(value) {
			t.Fatalf("valid semantic version rejected: %q", value)
		}
	}
	for _, value := range []string{
		"", "1", "1.2", "01.2.3", "1.02.3", "1.2.03", "v1.2.3",
		"1.2.3-", "1.2.3+", "1.2.3-alpha..one", "1.2.3-01",
		"1.2.3+build+second", "1.2.3+build!", "1.2.3 alpha",
	} {
		if validSemanticVersionV1(value) {
			t.Fatalf("invalid semantic version passed: %q", value)
		}
	}
}

func TestDeclarationV1RejectsUnknownDuplicateMissingAndWrongCaseFields(t *testing.T) {
	validBody := mustJSON(t, validDeclarationV1())
	unknownTop := strings.Replace(string(validBody), "{", `{"unknown":true,`, 1)
	wrongCase := strings.Replace(string(validBody), `"schemaVersion"`, `"SchemaVersion"`, 1)
	duplicateNested := strings.Replace(string(validBody), `"id":"funds.case.read"`, `"id":"duplicate","id":"funds.case.read"`, 1)

	missingTopObject := rawObject(t, validBody)
	delete(missingTopObject, "requestedCapabilities")
	missingTop := mustJSON(t, missingTopObject)

	unknownNestedObject := rawObject(t, validBody)
	lifecycle := unknownNestedObject["lifecycle"].(map[string]any)
	lifecycle["ready"] = true
	unknownNested := mustJSON(t, unknownNestedObject)

	for name, body := range map[string][]byte{
		"unknown_top":    []byte(unknownTop),
		"wrong_case":     []byte(wrongCase),
		"duplicate":      []byte(duplicateNested),
		"missing_top":    missingTop,
		"unknown_nested": unknownNested,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDeclarationV1(body); err == nil {
				t.Fatalf("hostile declaration passed: %s", body)
			}
		})
	}
}

func TestDeclarationV1RejectsInvalidIdentityLifecycleAndCollectionSemantics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DeclarationV1)
	}{
		{name: "schema", mutate: func(value *DeclarationV1) { value.SchemaVersion = 2 }},
		{name: "package_id", mutate: func(value *DeclarationV1) { value.PackageID = "Analytix Funds" }},
		{name: "semver_short", mutate: func(value *DeclarationV1) { value.PackageVersion = "0.16" }},
		{name: "semver_prefix", mutate: func(value *DeclarationV1) { value.PackageVersion = "v0.16.16" }},
		{name: "semver_leading_zero", mutate: func(value *DeclarationV1) { value.PackageVersion = "00.16.16" }},
		{name: "semver_prerelease_leading_zero", mutate: func(value *DeclarationV1) { value.PackageVersion = "0.16.16-01" }},
		{name: "missing_contribution_array", mutate: func(value *DeclarationV1) { value.Contributions.Hooks = nil }},
		{name: "no_contributions", mutate: func(value *DeclarationV1) { value.Contributions = emptyContributionsV1() }},
		{name: "duplicate_contribution_id", mutate: func(value *DeclarationV1) {
			value.Contributions.Assets[0].ID = value.Contributions.Skills[0].ID
		}},
		{name: "duplicate_contribution_path", mutate: func(value *DeclarationV1) {
			value.Contributions.Assets[0].Path = value.Contributions.Skills[0].Path
		}},
		{name: "duplicate_mcp_entrypoint", mutate: func(value *DeclarationV1) {
			value.Contributions.MCPServers = append(value.Contributions.MCPServers, MCPServerContributionV1{ID: "analytix_funds_two", Entrypoint: value.Contributions.MCPServers[0].Entrypoint})
		}},
		{name: "missing_capability_array", mutate: func(value *DeclarationV1) { value.RequestedCapabilities = nil }},
		{name: "duplicate_capability", mutate: func(value *DeclarationV1) {
			value.RequestedCapabilities[1].ID = value.RequestedCapabilities[0].ID
		}},
		{name: "invalid_capability_protocol", mutate: func(value *DeclarationV1) { value.RequestedCapabilities[0].ProtocolVersion = 0 }},
		{name: "missing_scope_constraints", mutate: func(value *DeclarationV1) { value.RequestedCapabilities[0].ScopeConstraints = nil }},
		{name: "duplicate_scope_constraint", mutate: func(value *DeclarationV1) {
			value.RequestedCapabilities[0].ScopeConstraints = []string{"case:bound", "case:bound"}
		}},
		{name: "lifecycle_protocol", mutate: func(value *DeclarationV1) { value.Lifecycle.ProtocolVersion = 0 }},
		{name: "entry_policy", mutate: func(value *DeclarationV1) { value.Lifecycle.EntryPolicy = "Host Static" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validDeclarationV1()
			test.mutate(&value)
			if err := ValidateDeclarationV1(value); err == nil {
				t.Fatalf("invalid declaration passed: %#v", value)
			}
			if _, err := ParseDeclarationV1(mustJSON(t, value)); err == nil {
				t.Fatalf("serialized invalid declaration passed: %#v", value)
			}
		})
	}
}

func TestValidPackageRelativePathV1(t *testing.T) {
	for _, value := range []string{
		".analytix-plugin/package.json",
		".codex-plugin/plugin.json",
		"skills/analytix-fund-analysis/SKILL.md",
		"mcp/server.mjs",
	} {
		if !ValidPackageRelativePathV1(value) {
			t.Fatalf("valid package-relative path rejected: %q", value)
		}
	}
	for _, value := range []string{
		"", ".", "..", "../outside", "skills/../outside", "/absolute", "C:/absolute", `C:\\absolute`,
		`mcp\\server.mjs`, "mcp//server.mjs", "mcp/./server.mjs", "mcp/server name.mjs",
		"mcp/服务器.mjs", "mcp/\tserver.mjs",
	} {
		if ValidPackageRelativePathV1(value) {
			t.Fatalf("hostile package-relative path passed: %q", value)
		}
	}
}

func validDeclarationV1() DeclarationV1 {
	return DeclarationV1{
		SchemaVersion:  SchemaVersionV1,
		PackageID:      "analytix-fund-analysis",
		PackageVersion: "0.16.16",
		Contributions: ContributionsV1{
			Skills: []PathContributionV1{
				{ID: "index", Path: "skills/index/SKILL.md"},
				{ID: "analytix-fund-analysis", Path: "skills/analytix-fund-analysis/SKILL.md"},
			},
			MCPServers: []MCPServerContributionV1{{ID: "analytix_funds", Entrypoint: "mcp/server.mjs"}},
			Hooks:      []PathContributionV1{},
			Assets:     []PathContributionV1{{ID: "funds-icon", Path: "assets/icon.png"}},
			PublicUI:   []PathContributionV1{},
		},
		RequestedCapabilities: []CapabilityRequestV1{
			{ID: "funds.source.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
			{ID: "funds.case.read", ProtocolVersion: 1, ScopeConstraints: []string{"source:verified", "case:bound"}},
		},
		Lifecycle: LifecycleV1{ProtocolVersion: 1, EntryPolicy: "host-static-first-party"},
	}
}

func emptyContributionsV1() ContributionsV1 {
	return ContributionsV1{
		Skills: []PathContributionV1{}, MCPServers: []MCPServerContributionV1{}, Hooks: []PathContributionV1{},
		Assets: []PathContributionV1{}, PublicUI: []PathContributionV1{},
	}
}

func rawObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func reversePathContributions(values []PathContributionV1) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseCapabilityRequests(values []CapabilityRequestV1) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
