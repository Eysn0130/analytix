package pluginpackage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func developmentDeclarationFixtureV1(id string) DeclarationV1 {
	return DeclarationV1{SchemaVersion: 1, PackageID: id, PackageVersion: "1.0.0", Contributions: ContributionsV1{
		Skills: []PathContributionV1{}, MCPServers: []MCPServerContributionV1{}, Hooks: []PathContributionV1{},
		PublicUI: []PathContributionV1{{ID: "workspace-editor", Path: "ui/editor.json"}}, Assets: []PathContributionV1{{ID: "editor-adapter", Path: "assets/adapter.json"}},
	}, RequestedCapabilities: []CapabilityRequestV1{{ID: "office.local-preview", ProtocolVersion: 1, ScopeConstraints: []string{"user-selected-object", "read-only"}}}, Lifecycle: LifecycleV1{ProtocolVersion: 1, EntryPolicy: "host-static-first-party"}}
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

func TestHistoricalDevelopmentRegistrationIsNotNewAdmissionV1(t *testing.T) {
	for _, id := range []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"} {
		registration := developmentRegistrationFixtureV1(t, id)
		previewBytes, _ := DevelopmentSourceRegistrationV1Bytes(registration)
		historicalBytes, historicalDigest, err := ReconstructHistoricalDevelopmentSourceRegistrationV1(registration)
		if err != nil || !bytes.Equal(previewBytes, historicalBytes) || historicalDigest != DevelopmentSourceRegistrationSHA256V1(registration) {
			t.Fatal("preview canonical identity changed", err)
		}
		declaration := developmentDeclarationFixtureV1(id)
		declaration.RequestedCapabilities[0].ID = "office.local-edit"
		declaration.RequestedCapabilities[0].ScopeConstraints = []string{"user-selected-object", "explicit-save"}
		setDeclaration := func() {
			canonical, err := CanonicalDeclarationV1Bytes(declaration)
			if err != nil {
				t.Fatal(err)
			}
			registration.DeclarationCanonicalJSON = string(canonical)
			registration.DeclarationRawSHA256 = developmentSHA256V1(canonical)
			registration.DeclarationCanonicalSHA256 = developmentSHA256V1(canonical)
		}
		setDeclaration()
		// The old v1 encoder used these exact struct JSON bytes and domain.
		oldBytes, _ := json.Marshal(registration)
		oldDigest := developmentSHA256V1(append([]byte("analytix.development-source-registration/v1\x00"), oldBytes...))
		body, digest, err := ReconstructHistoricalDevelopmentSourceRegistrationV1(registration)
		if err != nil || !bytes.Equal(body, oldBytes) || digest != oldDigest {
			t.Fatal("legacy canonical bytes changed", err)
		}
		if ValidateDevelopmentSourceDeclarationV1(declaration) == nil || ValidateDevelopmentSourceRegistrationV1(registration) == nil {
			t.Fatal("legacy source newly admitted")
		}
		if _, err := NewDevelopmentSourceRegistrationV1(registration); err == nil {
			t.Fatal("legacy registration created")
		}
		if _, err := ParseDevelopmentSourceRegistrationV1(body); err == nil {
			t.Fatal("legacy registration parsed for new binding")
		}
		if DevelopmentSourceRegistrationSHA256V1(registration) != "" {
			t.Fatal("legacy acquired a current registration digest")
		}
		for _, scope := range []string{"read-only", "arbitrary-shell", "external-network"} {
			declaration.RequestedCapabilities[0].ScopeConstraints = []string{"user-selected-object", scope}
			setDeclaration()
			if _, _, err := ReconstructHistoricalDevelopmentSourceRegistrationV1(registration); err == nil {
				t.Fatal("widened/mixed historical scope accepted")
			}
		}
	}
}
