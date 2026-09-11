package pluginpackage

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestStaticAdmissionOwnsPolicyButNotPublisherExactVersion(t *testing.T) {
	declaration := validDeclarationV1()
	declaration.PackageVersion = "0.16.17"
	decision, err := AdmitStaticFirstPartyV1(staticAdmissionInputV1(t, declaration))
	if err != nil {
		t.Fatalf("legal publisher version was rejected: %v", err)
	}
	if decision.Identity != (PackageIdentityV1{PackageID: FirstPartyFundsPackageIDV1, PackageVersion: "0.16.17"}) {
		t.Fatalf("admitted identity drifted: %#v", decision.Identity)
	}
	if len(decision.RequestedCapabilities) != len(declaration.RequestedCapabilities) {
		t.Fatalf("admitted capability request projection drifted: %#v", decision.RequestedCapabilities)
	}
}

func TestStaticAdmissionRejectsUnsupportedPolicyAndMissingEvidence(t *testing.T) {
	tests := map[string]func(*StaticAdmissionInputV1){
		"wrong first-party identity": func(input *StaticAdmissionInputV1) {
			declaration := validDeclarationV1()
			declaration.PackageID = "another-first-party"
			*input = staticAdmissionInputV1(t, declaration)
		},
		"unsupported lifecycle": func(input *StaticAdmissionInputV1) {
			declaration := validDeclarationV1()
			declaration.Lifecycle.ProtocolVersion = 2
			*input = staticAdmissionInputV1(t, declaration)
		},
		"unsupported entry policy": func(input *StaticAdmissionInputV1) {
			declaration := validDeclarationV1()
			declaration.Lifecycle.EntryPolicy = "publisher-managed"
			*input = staticAdmissionInputV1(t, declaration)
		},
		"capability above ceiling": func(input *StaticAdmissionInputV1) {
			declaration := validDeclarationV1()
			declaration.RequestedCapabilities = append(declaration.RequestedCapabilities, CapabilityRequestV1{
				ID: "funds.admin", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound"},
			})
			*input = staticAdmissionInputV1(t, declaration)
		},
		"missing artifact integrity": func(input *StaticAdmissionInputV1) {
			input.Evidence.ArtifactIntegrityVerified = false
		},
		"missing provenance": func(input *StaticAdmissionInputV1) {
			input.Evidence.ProvenanceAuthorityDigest = ""
		},
		"missing signing requirement": func(input *StaticAdmissionInputV1) {
			input.Evidence.SigningAlgorithm = ""
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := staticAdmissionInputV1(t, validDeclarationV1())
			mutate(&input)
			if _, err := AdmitStaticFirstPartyV1(input); err == nil {
				t.Fatal("unsupported package was admitted")
			}
		})
	}
}

func staticAdmissionInputV1(t *testing.T, declaration DeclarationV1) StaticAdmissionInputV1 {
	t.Helper()
	canonical, err := CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	sha := hex.EncodeToString(digest[:])
	return StaticAdmissionInputV1{
		CanonicalDeclaration: canonical, DeclarationRawSHA256: sha, DeclarationCanonicalSHA256: sha,
		Evidence: StaticAdmissionEvidenceV1{
			ArtifactIntegrityVerified: true,
			PackageAuthoritySHA256:    repeatHexV1('a'),
			ProvenanceAuthorityDigest: repeatHexV1('b'),
			ProvenanceClassification:  "development_dirty_non_publishable",
			ProvenanceDispositionKind: "development_non_publishable",
			PlatformAnchor:            "macos_nonpublishable_resource_seal",
			SigningAlgorithm:          StaticAdmissionSigningAlgorithmV1,
		},
	}
}

func repeatHexV1(value byte) string {
	body := make([]byte, sha256.Size*2)
	for index := range body {
		body[index] = value
	}
	return string(body)
}
