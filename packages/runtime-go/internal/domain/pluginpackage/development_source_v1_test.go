package pluginpackage

import (
	"strings"
	"testing"
)

func developmentDeclarationFixtureV1(id string) DeclarationV1 {
	return DeclarationV1{SchemaVersion: 1, PackageID: id, PackageVersion: "1.0.0", Contributions: ContributionsV1{
		Skills: []PathContributionV1{}, MCPServers: []MCPServerContributionV1{}, Hooks: []PathContributionV1{},
		PublicUI: []PathContributionV1{{ID: "workspace-editor", Path: "ui/editor.json"}}, Assets: []PathContributionV1{{ID: "editor-adapter", Path: "assets/adapter.json"}},
	}, RequestedCapabilities: []CapabilityRequestV1{{ID: "office.local-edit", ProtocolVersion: 1, ScopeConstraints: []string{"user-selected-object", "explicit-save"}}}, Lifecycle: LifecycleV1{ProtocolVersion: 1, EntryPolicy: "host-static-first-party"}}
}

func developmentRegistrationFixtureV1(t *testing.T, id string) DevelopmentSourceRegistrationV1 {
	t.Helper()
	declaration := developmentDeclarationFixtureV1(id)
	canonical, err := CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := NewDevelopmentSourceRegistrationV1(DevelopmentSourceRegistrationV1{
		Identity: declaration.IdentityV1(), DeclarationCanonicalJSON: string(canonical), DeclarationRawSHA256: developmentSHA256V1(canonical), DeclarationCanonicalSHA256: developmentSHA256V1(canonical),
		SourceTreeSHA256: strings.Repeat("a", 64), SourceTreeFileCount: 4, ManifestSHA256: strings.Repeat("b", 64), PublicUISHA256: strings.Repeat("c", 64), AdapterSHA256: strings.Repeat("d", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func TestDevelopmentSourceRegistrationFreezesOnlyThreeStaticEditors(t *testing.T) {
	for _, id := range []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"} {
		registration := developmentRegistrationFixtureV1(t, id)
		body, err := DevelopmentSourceRegistrationV1Bytes(registration)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseDevelopmentSourceRegistrationV1(body)
		if err != nil || parsed != registration {
			t.Fatalf("registration round trip: %v", err)
		}
		changed := registration
		changed.AdapterSHA256 = strings.Repeat("e", 64)
		if ValidateDevelopmentSourceRegistrationV1(changed) == nil {
			t.Fatal("changed contribution retained registration")
		}
		changed, err = NewDevelopmentSourceRegistrationV1(changed)
		if err != nil || DevelopmentSourceRegistrationSHA256V1(changed) == DevelopmentSourceRegistrationSHA256V1(registration) {
			t.Fatal("new contribution did not change registration")
		}
		for _, mutate := range []func(*DevelopmentSourceRegistrationV1){
			func(r *DevelopmentSourceRegistrationV1) { r.Publishable = true }, func(r *DevelopmentSourceRegistrationV1) { r.FactToolsEnabled = true },
			func(r *DevelopmentSourceRegistrationV1) { r.Origin = "packaged" }, func(r *DevelopmentSourceRegistrationV1) { r.ExecutionMode = "packaged" },
			func(r *DevelopmentSourceRegistrationV1) { r.Identity.PackageID = "another-plugin" }, func(r *DevelopmentSourceRegistrationV1) { r.DeclarationCanonicalSHA256 = strings.Repeat("f", 64) },
		} {
			invalid := registration
			mutate(&invalid)
			if ValidateDevelopmentSourceRegistrationV1(invalid) == nil {
				t.Fatal("invalid source registration accepted")
			}
		}
	}
	for _, mutate := range []func(*DeclarationV1){
		func(d *DeclarationV1) { d.PackageID = "analytix-fund-analysis" },
		func(d *DeclarationV1) { d.Contributions.Hooks = []PathContributionV1{{ID: "run", Path: "hook.js"}} },
		func(d *DeclarationV1) {
			d.RequestedCapabilities[0].ScopeConstraints = []string{"user-selected-object", "arbitrary-shell"}
		},
		func(d *DeclarationV1) { d.Contributions.PublicUI[0].Path = "remote.js" },
	} {
		d := developmentDeclarationFixtureV1("analytix-documents")
		mutate(&d)
		if ValidateDevelopmentSourceDeclarationV1(d) == nil {
			t.Fatal("widened static contribution admitted")
		}
	}
}
