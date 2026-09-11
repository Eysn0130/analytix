package authorityenrollment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestManifestV2CanonicalAnchorAndCredentialSelection(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	input := manifestInputV2(t, fixture)
	manifest, err := NewManifestV2(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManifestV2(body)
	if err != nil || parsed != manifest {
		t.Fatalf("manifest V2 round trip mismatch: %#v err=%v", parsed, err)
	}
	anchored, err := ParseAnchoredManifestV2(
		body, input.InstallationID, input.InstallationAuthorityKeyID,
		fixture.installationPublicKey, manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectAnchoredManifestForNamespaceV2(anchored, ThreadRiskNamespaceV1)
	if err != nil || projection.CredentialProfileGeneration != input.CredentialProfileGeneration ||
		projection.CredentialProfileDigest != input.CredentialProfileDigest ||
		projection.Enrollment.Namespace != ThreadRiskNamespaceV1 {
		t.Fatalf("manifest V2 projection mismatch: %#v err=%v", projection, err)
	}
	projection.InstallationAuthorityPublicKey[0] ^= 0xff
	again, err := ProjectAnchoredManifestForNamespaceV2(anchored, ThreadRiskNamespaceV1)
	if err != nil || !bytes.Equal(again.InstallationAuthorityPublicKey, fixture.installationPublicKey) {
		t.Fatal("manifest V2 projection leaked mutable key bytes")
	}
	if _, err := ProjectAnchoredManifestForNamespaceV2(AnchoredManifestV2{}, ThreadRiskNamespaceV1); err == nil {
		t.Fatal("unanchored manifest V2 projected credential authority")
	}
}

func TestManifestV2CredentialProfileTamperInvalidatesSignatureAndDigest(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	manifest, err := NewManifestV2(manifestInputV2(t, fixture), fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	changedDigest := manifest
	changedDigest.CredentialProfileDigest = domainsecurity.SHA256Hex([]byte("other-profile"))
	if err := ValidateManifestV2(changedDigest); err == nil {
		t.Fatal("manifest V2 accepted a changed credential profile digest")
	}
	changedGeneration := manifest
	changedGeneration.CredentialProfileGeneration++
	if err := ValidateManifestV2(changedGeneration); err == nil {
		t.Fatal("manifest V2 accepted a changed credential profile generation")
	}
	if bytes.Equal(ManifestSigningBytesV2(manifest), ManifestSigningBytesV2(changedDigest)) {
		t.Fatal("credential profile selection was absent from manifest V2 signing bytes")
	}
}

func TestManifestV2RejectsInstallationWitnessRoleCollapseAndEnrollmentAlias(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	roleCollapse := manifestInputV2(t, fixture)
	roleCollapse.ThreadRisk.WitnessKeyID = fixture.input.InstallationAuthorityKeyID
	roleCollapse.ThreadRisk.WitnessPublicKey = fixture.installationPublicKey
	roleCollapse.ThreadRisk.InitialCheckpoint = initialManifestCheckpointV1(
		t, roleCollapse.InstallationID, roleCollapse.ThreadRisk.EnrollmentID,
		ThreadRiskNamespaceV1, fixture.installationPrivateKey,
	)
	if _, err := NewManifestV2(roleCollapse, fixture.sign); err == nil {
		t.Fatal("manifest V2 accepted installation authority as witness")
	}

	alias := manifestInputV2(t, fixture)
	sharedPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x61}, ed25519.SeedSize))
	sharedPublic := sharedPrivate.Public().(ed25519.PublicKey)
	alias.SharedEvidence.EnrollmentID = alias.ThreadRisk.EnrollmentID
	alias.SharedEvidence.WitnessKeyID = domainsecurity.SHA256Hex(sharedPublic)
	alias.SharedEvidence.WitnessPublicKey = sharedPublic
	alias.SharedEvidence.InitialCheckpoint = initialManifestCheckpointV1(
		t, alias.InstallationID, alias.SharedEvidence.EnrollmentID,
		SharedEvidenceNamespaceV1, sharedPrivate,
	)
	if _, err := NewManifestV2(alias, fixture.sign); err == nil {
		t.Fatal("manifest V2 accepted one enrollment ID for both authority namespaces")
	}
}

func TestManifestV2IsDomainSeparatedAndNotV1Compatible(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	v1 := fixture.newManifest(t)
	v2, err := NewManifestV2(manifestInputV2(t, fixture), fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ManifestSigningBytesV1(v1), ManifestSigningBytesV2(v2)) {
		t.Fatal("manifest V1 and V2 signing contracts are not domain separated")
	}
	v1Body, _ := ManifestV1Bytes(v1)
	v2Body, _ := ManifestV2Bytes(v2)
	if _, err := ParseManifestV2(v1Body); err == nil {
		t.Fatal("V1 manifest was accepted as production V2 enrollment")
	}
	if _, err := ParseManifestV1(v2Body); err == nil {
		t.Fatal("V2 manifest was accepted through the V1 parser")
	}
}

func TestManifestV2RejectsInvalidProfileSelectionAndNoncanonicalAnchor(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	validInput := manifestInputV2(t, fixture)
	for name, mutate := range map[string]func(*ManifestInputV2){
		"zero generation":        func(input *ManifestInputV2) { input.CredentialProfileGeneration = 0 },
		"unsafe JSON generation": func(input *ManifestInputV2) { input.CredentialProfileGeneration = 1 << 53 },
		"invalid digest":         func(input *ManifestInputV2) { input.CredentialProfileDigest = "not-a-digest" },
	} {
		t.Run(name, func(t *testing.T) {
			input := validInput
			mutate(&input)
			if _, err := NewManifestV2(input, fixture.sign); err == nil {
				t.Fatal("invalid credential profile selection was accepted")
			}
		})
	}
	manifest, err := NewManifestV2(validInput, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, installationID, authorityKeyID, digest string
	}{
		{"installation whitespace", " " + validInput.InstallationID, validInput.InstallationAuthorityKeyID, manifest.ManifestDigest},
		{"authority whitespace", validInput.InstallationID, validInput.InstallationAuthorityKeyID + "\n", manifest.ManifestDigest},
		{"digest whitespace", validInput.InstallationID, validInput.InstallationAuthorityKeyID, manifest.ManifestDigest + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AnchorManifestForInstallationV2(
				manifest, test.installationID, test.authorityKeyID, fixture.installationPublicKey, test.digest,
			); err == nil {
				t.Fatal("manifest V2 accepted a noncanonical external anchor")
			}
		})
	}
}

func TestParseManifestV2RejectsOpenAmbiguousOrNoncanonicalJSON(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	manifest, err := NewManifestV2(manifestInputV2(t, fixture), fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ManifestV2Bytes(manifest)
	candidates := map[string][]byte{
		"unknown":      append([]byte(`{"unknown":true,`), body[1:]...),
		"duplicate":    append([]byte(`{"schemaVersion":2,`), body[1:]...),
		"trailing":     append(append([]byte(nil), body...), []byte(`{}`)...),
		"noncanonical": append(append([]byte(nil), body...), '\n'),
		"null profile": bytes.Replace(body, []byte(`"credentialProfileDigest":"`), []byte(`"credentialProfileDigest":null,"ignored":"`), 1),
	}
	for name, candidate := range candidates {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifestV2(candidate); err == nil {
				t.Fatal("unsafe manifest V2 JSON was accepted")
			}
		})
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil || decoded["purpose"] != ManifestPurposeV2 {
		t.Fatalf("manifest V2 fixture is not valid closed JSON: %v", err)
	}
	if strings.Contains(string(body), "privateKey") {
		t.Fatal("manifest V2 serialized private credential material")
	}
}

func manifestInputV2(t *testing.T, fixture manifestFixtureV1) ManifestInputV2 {
	t.Helper()
	return ManifestInputV2{
		InstallationID:                 fixture.input.InstallationID,
		InstallationAuthorityKeyID:     fixture.input.InstallationAuthorityKeyID,
		InstallationAuthorityPublicKey: append(ed25519.PublicKey(nil), fixture.installationPublicKey...),
		CredentialProfileGeneration:    7,
		CredentialProfileDigest:        domainsecurity.SHA256Hex([]byte("credential-profile-v1")),
		IssuedAt:                       fixture.input.IssuedAt, ThreadRisk: fixture.input.ThreadRisk, SharedEvidence: fixture.input.SharedEvidence,
	}
}
