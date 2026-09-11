package authorityenrollment

import (
	"testing"

	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCredentialProfileRequiresExactAnchoredManifestV2Selection(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	profile := selectedCredentialProfileFixtureV2(t, fixture)
	input := manifestInputV2(t, fixture)
	input.CredentialProfileGeneration = profile.ProfileGeneration
	input.CredentialProfileDigest = profile.ProfileDigest
	manifest, err := NewManifestV2(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	anchored, err := AnchorManifestForInstallationV2(
		manifest, input.InstallationID, input.InstallationAuthorityKeyID,
		fixture.installationPublicKey, manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCredentialProfileForManifestV2(anchored, profile); err != nil {
		t.Fatalf("exact selected credential profile was rejected: %v", err)
	}
	bound, err := BindCredentialProfileForManifestV2(anchored, profile)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectBoundCredentialProfileForNamespaceV1(bound, ThreadRiskNamespaceV1)
	if err != nil || len(projection.Files) != 3 || projection.Manifest.Enrollment.EnrollmentID != fixture.input.ThreadRisk.EnrollmentID {
		t.Fatalf("bound credential projection mismatch: %#v err=%v", projection, err)
	}
	projection.Files[0].FileSHA256 = domainsecurity.SHA256Hex([]byte("mutated"))
	again, err := ProjectBoundCredentialProfileForNamespaceV1(bound, ThreadRiskNamespaceV1)
	if err != nil || again.Files[0].FileSHA256 == projection.Files[0].FileSHA256 {
		t.Fatal("bound credential profile leaked mutable descriptor storage")
	}
	if err := ValidateCredentialProfileForManifestV2(AnchoredManifestV2{}, profile); err == nil {
		t.Fatal("unanchored manifest selected credential material")
	}
	otherProfile, err := domaincredentials.NewCredentialProfileV1(domaincredentials.CredentialProfileInputV1{
		InstallationID: profile.InstallationID, AuthorityKeyID: profile.AuthorityKeyID,
		ProfileGeneration: profile.ProfileGeneration + 1, Files: credentialProfileInputsV2(fixture),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCredentialProfileForManifestV2(anchored, otherProfile); err == nil {
		t.Fatal("anchored manifest selected another valid profile generation")
	}
}

func TestCredentialProfileRejectsCrossNamespaceRootOrMTLSSwap(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	inputs := credentialProfileInputsV2(fixture)
	for name, mutate := range map[string]func([]domaincredentials.FileDescriptorInputV1){
		"root semantic swap": func(files []domaincredentials.FileDescriptorInputV1) {
			files[0].SemanticSHA256, files[3].SemanticSHA256 = files[3].SemanticSHA256, files[0].SemanticSHA256
		},
		"client semantic mismatch": func(files []domaincredentials.FileDescriptorInputV1) {
			files[1].SemanticSHA256 = domainsecurity.SHA256Hex([]byte("other-client"))
		},
		"enrollment lineage mismatch": func(files []domaincredentials.FileDescriptorInputV1) {
			other := domainsecurity.SHA256Hex([]byte("other-enrollment"))
			for index := 0; index < 3; index++ {
				files[index].EnrollmentID = other
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := append([]domaincredentials.FileDescriptorInputV1(nil), inputs...)
			mutate(files)
			profile, err := domaincredentials.NewCredentialProfileV1(domaincredentials.CredentialProfileInputV1{
				InstallationID: fixture.input.InstallationID, AuthorityKeyID: fixture.input.InstallationAuthorityKeyID,
				ProfileGeneration: 7, Files: files,
			})
			if err != nil {
				t.Fatal(err)
			}
			manifestInput := manifestInputV2(t, fixture)
			manifestInput.CredentialProfileGeneration = profile.ProfileGeneration
			manifestInput.CredentialProfileDigest = profile.ProfileDigest
			manifest, err := NewManifestV2(manifestInput, fixture.sign)
			if err != nil {
				t.Fatal(err)
			}
			anchored, err := AnchorManifestForInstallationV2(
				manifest, manifestInput.InstallationID, manifestInput.InstallationAuthorityKeyID,
				fixture.installationPublicKey, manifest.ManifestDigest,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateCredentialProfileForManifestV2(anchored, profile); err == nil {
				t.Fatal("cross-namespace or mismatched credential semantics were accepted")
			}
		})
	}
}

func selectedCredentialProfileFixtureV2(t *testing.T, fixture manifestFixtureV1) domaincredentials.CredentialProfileV1 {
	t.Helper()
	profile, err := domaincredentials.NewCredentialProfileV1(domaincredentials.CredentialProfileInputV1{
		InstallationID: fixture.input.InstallationID, AuthorityKeyID: fixture.input.InstallationAuthorityKeyID,
		ProfileGeneration: 7, Files: credentialProfileInputsV2(fixture),
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func credentialProfileInputsV2(fixture manifestFixtureV1) []domaincredentials.FileDescriptorInputV1 {
	return []domaincredentials.FileDescriptorInputV1{
		{Role: domaincredentials.RoleWitnessRootCA, Namespace: ThreadRiskNamespaceV1, EnrollmentID: fixture.input.ThreadRisk.EnrollmentID, SizeBytes: 100, FileSHA256: domainsecurity.SHA256Hex([]byte("thread-root-file")), SemanticSHA256: fixture.input.ThreadRisk.RootCASHA256},
		{Role: domaincredentials.RoleWitnessMTLSClientChain, Namespace: ThreadRiskNamespaceV1, EnrollmentID: fixture.input.ThreadRisk.EnrollmentID, SizeBytes: 200, FileSHA256: domainsecurity.SHA256Hex([]byte("thread-chain-file")), SemanticSHA256: fixture.input.ThreadRisk.MTLSClientIdentityCertificateSHA256},
		{Role: domaincredentials.RoleWitnessMTLSClientPrivateKey, Namespace: ThreadRiskNamespaceV1, EnrollmentID: fixture.input.ThreadRisk.EnrollmentID, SizeBytes: 120, FileSHA256: domainsecurity.SHA256Hex([]byte("thread-key-file")), SemanticSHA256: domainsecurity.SHA256Hex([]byte("thread-key-spki"))},
		{Role: domaincredentials.RoleWitnessRootCA, Namespace: SharedEvidenceNamespaceV1, EnrollmentID: fixture.input.SharedEvidence.EnrollmentID, SizeBytes: 110, FileSHA256: domainsecurity.SHA256Hex([]byte("shared-root-file")), SemanticSHA256: fixture.input.SharedEvidence.RootCASHA256},
	}
}
