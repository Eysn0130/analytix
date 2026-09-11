package authoritycredentials

import (
	"bytes"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCredentialProfileV1CanonicalRoundTrip(t *testing.T) {
	profile := profileFixtureV1(t, true)
	body, err := CredentialProfileV1Bytes(profile)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCredentialProfileV1(body)
	if err != nil {
		t.Fatal(err)
	}
	parsedBody, err := CredentialProfileV1Bytes(parsed)
	if err != nil || !bytes.Equal(body, parsedBody) || parsed.ProfileDigest != profile.ProfileDigest {
		t.Fatalf("credential profile round trip mismatch: err=%v", err)
	}
	if len(parsed.Files) != 6 || parsed.Files[0].FixedName != "thread-risk-root-ca.der" ||
		parsed.Files[3].FixedName != "shared-evidence-root-ca.der" {
		t.Fatalf("credential descriptors are not deterministically ordered: %#v", parsed.Files)
	}
}

func TestCredentialProfileV1RejectsCrossGenerationRoleAndInventoryDrift(t *testing.T) {
	valid := profileFixtureV1(t, true)
	tests := map[string]func(*CredentialProfileV1){
		"cross generation":        func(profile *CredentialProfileV1) { profile.Files[0].Generation++ },
		"fixed name substitution": func(profile *CredentialProfileV1) { profile.Files[0].FixedName = "other.der" },
		"cross namespace substitution": func(profile *CredentialProfileV1) {
			profile.Files[0].Namespace = domainsecurity.EvidenceRegistryAuthorityNamespaceV1
		},
		"cross enrollment mix": func(profile *CredentialProfileV1) { profile.Files[1].EnrollmentID = digestV1("other-enrollment") },
		"missing client key":   func(profile *CredentialProfileV1) { profile.Files = profile.Files[:len(profile.Files)-1] },
		"duplicate role":       func(profile *CredentialProfileV1) { profile.Files[1] = profile.Files[0] },
		"noncanonical digest":  func(profile *CredentialProfileV1) { profile.Files[0].FileSHA256 = " " + profile.Files[0].FileSHA256 },
		"oversized root":       func(profile *CredentialProfileV1) { profile.Files[0].SizeBytes = MaxRootCABytesV1 + 1 },
		"reordered descriptors": func(profile *CredentialProfileV1) {
			profile.Files[0], profile.Files[1] = profile.Files[1], profile.Files[0]
		},
		"unsafe JSON generation": func(profile *CredentialProfileV1) {
			profile.ProfileGeneration = MaxSafeJSONIntegerV1 + 1
			for index := range profile.Files {
				profile.Files[index].Generation = profile.ProfileGeneration
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			profile := cloneProfileV1(valid)
			mutate(&profile)
			profile.ProfileDigest = credentialProfileDigestV1(profile)
			if err := ValidateCredentialProfileV1(profile); err == nil {
				t.Fatal("unsafe credential profile was accepted")
			}
		})
	}
}

func TestCredentialProfileV1RejectsTamperEvenWhenShapeRemainsValid(t *testing.T) {
	profile := profileFixtureV1(t, false)
	profile.Files[0].FileSHA256 = digestV1("replacement")
	if err := ValidateCredentialProfileV1(profile); err == nil {
		t.Fatal("profile accepted a file hash change without a new selected profile digest")
	}
	profile.ProfileDigest = credentialProfileDigestV1(profile)
	if err := ValidateCredentialProfileV1(profile); err != nil {
		t.Fatalf("valid newly digested profile was rejected: %v", err)
	}
}

func TestParseCredentialProfileV1RejectsOpenAmbiguousOrNoncanonicalJSON(t *testing.T) {
	body, err := CredentialProfileV1Bytes(profileFixtureV1(t, false))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"unknown":      append([]byte(`{"unknown":true,`), body[1:]...),
		"duplicate":    append([]byte(`{"schemaVersion":1,`), body[1:]...),
		"trailing":     append(append([]byte(nil), body...), []byte(`{}`)...),
		"noncanonical": append(append([]byte(nil), body...), '\n'),
		"null files":   bytes.Replace(body, []byte(`"files":[`), []byte(`"files":null,"ignored":[`), 1),
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCredentialProfileV1(candidate); err == nil {
				t.Fatal("unsafe credential profile JSON was accepted")
			}
		})
	}
}

func profileFixtureV1(t *testing.T, mtls bool) CredentialProfileV1 {
	t.Helper()
	files := []FileDescriptorInputV1{
		{Role: RoleWitnessRootCA, Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, EnrollmentID: digestV1("thread-enrollment"), SizeBytes: 100, FileSHA256: digestV1("thread-root-file"), SemanticSHA256: digestV1("thread-root-der")},
		{Role: RoleWitnessRootCA, Namespace: domainsecurity.EvidenceRegistryAuthorityNamespaceV1, EnrollmentID: digestV1("shared-enrollment"), SizeBytes: 110, FileSHA256: digestV1("shared-root-file"), SemanticSHA256: digestV1("shared-root-der")},
	}
	if mtls {
		files = append(files,
			FileDescriptorInputV1{Role: RoleWitnessMTLSClientChain, Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, EnrollmentID: digestV1("thread-enrollment"), SizeBytes: 200, FileSHA256: digestV1("thread-chain-file"), SemanticSHA256: digestV1("thread-leaf-der")},
			FileDescriptorInputV1{Role: RoleWitnessMTLSClientPrivateKey, Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, EnrollmentID: digestV1("thread-enrollment"), SizeBytes: 120, FileSHA256: digestV1("thread-key-file"), SemanticSHA256: digestV1("thread-key-spki")},
			FileDescriptorInputV1{Role: RoleWitnessMTLSClientChain, Namespace: domainsecurity.EvidenceRegistryAuthorityNamespaceV1, EnrollmentID: digestV1("shared-enrollment"), SizeBytes: 210, FileSHA256: digestV1("shared-chain-file"), SemanticSHA256: digestV1("shared-leaf-der")},
			FileDescriptorInputV1{Role: RoleWitnessMTLSClientPrivateKey, Namespace: domainsecurity.EvidenceRegistryAuthorityNamespaceV1, EnrollmentID: digestV1("shared-enrollment"), SizeBytes: 130, FileSHA256: digestV1("shared-key-file"), SemanticSHA256: digestV1("shared-key-spki")},
		)
	}
	profile, err := NewCredentialProfileV1(CredentialProfileInputV1{
		InstallationID: digestV1("installation"), AuthorityKeyID: digestV1("authority"),
		ProfileGeneration: 7, Files: files,
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func cloneProfileV1(profile CredentialProfileV1) CredentialProfileV1 {
	profile.Files = append([]FileDescriptorV1(nil), profile.Files...)
	return profile
}

func digestV1(value string) string {
	return domainsecurity.SHA256Hex([]byte(value))
}
