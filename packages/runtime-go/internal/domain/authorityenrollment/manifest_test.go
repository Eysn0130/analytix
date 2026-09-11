package authorityenrollment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestManifestV1CanonicalRoundTripAndInstallationAnchor(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	manifest := fixture.newManifest(t)

	if err := ValidateManifestV1(manifest); err != nil {
		t.Fatalf("ValidateManifestV1() error = %v", err)
	}
	if err := ValidateManifestForInstallationV1(
		manifest, fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID, fixture.installationPublicKey,
	); err != nil {
		t.Fatalf("ValidateManifestForInstallationV1() error = %v", err)
	}
	anchored, err := AnchorManifestForInstallationV1(
		manifest, fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID,
		fixture.installationPublicKey, manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatalf("AnchorManifestForInstallationV1() error = %v", err)
	}
	if manifest.ThreadRisk.Namespace != ThreadRiskNamespaceV1 || manifest.SharedEvidence.Namespace != SharedEvidenceNamespaceV1 {
		t.Fatalf("manifest namespaces = %q, %q", manifest.ThreadRisk.Namespace, manifest.SharedEvidence.Namespace)
	}
	threadRisk, err := EnrollmentForNamespaceV1(anchored, ThreadRiskNamespaceV1)
	if err != nil || threadRisk != manifest.ThreadRisk {
		t.Fatalf("thread risk enrollment = %#v, %v", threadRisk, err)
	}
	sharedEvidence, err := EnrollmentForNamespaceV1(anchored, SharedEvidenceNamespaceV1)
	if err != nil || sharedEvidence != manifest.SharedEvidence {
		t.Fatalf("shared evidence enrollment = %#v, %v", sharedEvidence, err)
	}
	if _, err := EnrollmentForNamespaceV1(anchored, "analytix.thread-risk-authority/v2"); err == nil {
		t.Fatal("EnrollmentForNamespaceV1() accepted an unknown namespace")
	}
	if _, err := EnrollmentForNamespaceV1(AnchoredManifestV1{}, ThreadRiskNamespaceV1); err == nil {
		t.Fatal("EnrollmentForNamespaceV1() accepted an unanchored manifest")
	}
	projection, err := ProjectAnchoredManifestForNamespaceV1(anchored, ThreadRiskNamespaceV1)
	if err != nil || projection.ManifestDigest != manifest.ManifestDigest ||
		projection.InstallationID != manifest.InstallationID ||
		projection.InstallationAuthorityAlgorithm != manifest.InstallationAuthorityAlgorithm ||
		projection.InstallationAuthorityKeyID != manifest.InstallationAuthorityKeyID ||
		!bytes.Equal(projection.InstallationAuthorityPublicKey, fixture.installationPublicKey) ||
		projection.Enrollment != manifest.ThreadRisk {
		t.Fatalf("anchored manifest projection = %#v, %v", projection, err)
	}
	projection.InstallationAuthorityPublicKey[0] ^= 0xff
	projectedAgain, err := ProjectAnchoredManifestForNamespaceV1(anchored, ThreadRiskNamespaceV1)
	if err != nil || !bytes.Equal(projectedAgain.InstallationAuthorityPublicKey, fixture.installationPublicKey) {
		t.Fatalf("anchored projection leaked mutable key storage: %#v, %v", projectedAgain, err)
	}
	if _, err := ProjectAnchoredManifestForNamespaceV1(AnchoredManifestV1{}, ThreadRiskNamespaceV1); err == nil {
		t.Fatal("ProjectAnchoredManifestForNamespaceV1() accepted an unanchored manifest")
	}
	if _, err := ProjectAnchoredManifestForNamespaceV1(anchored, "analytix.thread-risk-authority/v2"); err == nil {
		t.Fatal("ProjectAnchoredManifestForNamespaceV1() accepted an unknown namespace")
	}

	body, err := ManifestV1Bytes(manifest)
	if err != nil {
		t.Fatalf("ManifestV1Bytes() error = %v", err)
	}
	parsed, err := ParseManifestV1(body)
	if err != nil || parsed != manifest {
		t.Fatalf("ParseManifestV1() = %#v, %v", parsed, err)
	}
	parsedAnchor, err := ParseAnchoredManifestV1(
		body, fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID,
		fixture.installationPublicKey, manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatalf("ParseAnchoredManifestV1() error = %v", err)
	}
	if parsedEnrollment, err := EnrollmentForNamespaceV1(parsedAnchor, ThreadRiskNamespaceV1); err != nil || parsedEnrollment != manifest.ThreadRisk {
		t.Fatalf("parsed anchored enrollment = %#v, %v", parsedEnrollment, err)
	}
	if _, err := AnchorManifestForInstallationV1(
		manifest, fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID,
		fixture.installationPublicKey, domainsecurity.SHA256Hex([]byte("stale-manifest-anchor")),
	); err == nil {
		t.Fatal("current manifest anchor accepted a different manifest digest")
	}
	text := string(body)
	for _, prohibited := range []string{"privateKey", "bearerToken", "authorization", "clientCertificate\""} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("canonical manifest contains prohibited secret-bearing field %q", prohibited)
		}
	}
	if !strings.Contains(text, `"mtlsClientIdentityCertificateSha256"`) {
		t.Fatal("canonical manifest omitted configured mTLS certificate digest")
	}

	wrongInstallation := domainsecurity.SHA256Hex([]byte("other-installation"))
	if err := ValidateManifestForInstallationV1(
		manifest, wrongInstallation, fixture.input.InstallationAuthorityKeyID, fixture.installationPublicKey,
	); err == nil {
		t.Fatal("installation anchor accepted another installation")
	}
	otherPublic, _, err := ed25519.GenerateKey(bytes.NewReader(bytes.Repeat([]byte{0x7f}, 128)))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifestForInstallationV1(
		manifest, fixture.input.InstallationID, domainsecurity.SHA256Hex(otherPublic), otherPublic,
	); err == nil {
		t.Fatal("installation anchor accepted another authority")
	}
	for name, identifiers := range map[string][2]string{
		"installation leading whitespace": {" " + fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID},
		"installation trailing newline":   {fixture.input.InstallationID + "\n", fixture.input.InstallationAuthorityKeyID},
		"key leading whitespace":          {fixture.input.InstallationID, " " + fixture.input.InstallationAuthorityKeyID},
		"key trailing newline":            {fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateManifestForInstallationV1(
				manifest, identifiers[0], identifiers[1], fixture.installationPublicKey,
			); err == nil {
				t.Fatal("installation anchor accepted a noncanonical identifier")
			}
		})
	}
	if _, err := AnchorManifestForInstallationV1(
		manifest, fixture.input.InstallationID, fixture.input.InstallationAuthorityKeyID,
		fixture.installationPublicKey, manifest.ManifestDigest+"\n",
	); err == nil {
		t.Fatal("current manifest anchor accepted a noncanonical digest")
	}
}

func TestManifestV1DigestAndSignatureAreNonSelfReferential(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	manifest := fixture.newManifest(t)
	originalSigningBytes := ManifestSigningBytesV1(manifest)
	originalDigest := manifestDigestV1(manifest)

	changedEnvelope := manifest
	changedEnvelope.InstallationAuthoritySignature = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x51}, ed25519.SignatureSize))
	changedEnvelope.ManifestDigest = domainsecurity.SHA256Hex([]byte("replacement-digest"))
	if !bytes.Equal(originalSigningBytes, ManifestSigningBytesV1(changedEnvelope)) {
		t.Fatal("signature or digest became self-referential signing input")
	}
	if originalDigest != manifestDigestV1(changedEnvelope) {
		t.Fatal("signature or digest became self-referential digest input")
	}
	if err := ValidateManifestV1(changedEnvelope); err == nil {
		t.Fatal("validation accepted replaced signature and digest")
	}

	tamperedPayload := manifest
	tamperedPayload.ThreadRisk.RootCASHA256 = domainsecurity.SHA256Hex([]byte("different-root"))
	if bytes.Equal(originalSigningBytes, ManifestSigningBytesV1(tamperedPayload)) {
		t.Fatal("payload mutation did not change signing bytes")
	}
	if originalDigest == manifestDigestV1(tamperedPayload) {
		t.Fatal("payload mutation did not change manifest digest")
	}
	if err := ValidateManifestV1(tamperedPayload); err == nil {
		t.Fatal("validation accepted tampered payload")
	}

	swapped := manifest
	swapped.ThreadRisk, swapped.SharedEvidence = swapped.SharedEvidence, swapped.ThreadRisk
	resignManifestV1(t, &swapped, fixture.installationPrivateKey)
	if err := ValidateManifestV1(swapped); err == nil {
		t.Fatal("a freshly signed manifest substituted the two authority namespaces")
	}
}

func TestParseManifestV1RejectsAmbiguousOrOpenJSON(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	body, err := ManifestV1Bytes(fixture.newManifest(t))
	if err != nil {
		t.Fatal(err)
	}

	duplicateTop := bytes.Replace(body, []byte(`"schemaVersion":1,`), []byte(`"schemaVersion":1,"schemaVersion":1,`), 1)
	duplicateNested := bytes.Replace(
		body,
		[]byte(`"namespace":"`+ThreadRiskNamespaceV1+`",`),
		[]byte(`"namespace":"`+ThreadRiskNamespaceV1+`","namespace":"`+ThreadRiskNamespaceV1+`",`),
		1,
	)
	unknownTop := setRawObjectFieldV1(t, body, "", "privateKey", json.RawMessage(`"secret"`))
	unknownNested := setRawObjectFieldV1(t, body, "threadRisk", "bearerToken", json.RawMessage(`"secret"`))
	nullTop := setRawObjectFieldV1(t, body, "", "installationAuthorityPublicKey", json.RawMessage("null"))
	nullNested := setRawObjectFieldV1(t, body, "threadRisk", "endpointOrigin", json.RawMessage("null"))
	nullOptional := setRawObjectFieldV1(t, body, "threadRisk", "mtlsClientIdentityCertificateSha256", json.RawMessage("null"))
	emptyOptional := setRawObjectFieldV1(t, body, "sharedEvidence", "mtlsClientIdentityCertificateSha256", json.RawMessage(`""`))
	reordered := reorderRawObjectV1(t, body)

	tests := map[string][]byte{
		"duplicate top-level key":  duplicateTop,
		"duplicate nested key":     duplicateNested,
		"unknown private key":      unknownTop,
		"unknown bearer token":     unknownNested,
		"null top-level field":     nullTop,
		"null nested field":        nullNested,
		"null optional field":      nullOptional,
		"empty optional field":     emptyOptional,
		"trailing JSON":            append(append([]byte(nil), body...), []byte(`{}`)...),
		"leading whitespace":       append([]byte(" "), body...),
		"trailing newline":         append(append([]byte(nil), body...), '\n'),
		"noncanonical field order": reordered,
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifestV1(candidate); err == nil {
				t.Fatal("ParseManifestV1() accepted invalid JSON")
			}
		})
	}
}

func TestParseManifestV1RequiresEveryClosedSchemaField(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	body, err := ManifestV1Bytes(fixture.newManifest(t))
	if err != nil {
		t.Fatal(err)
	}
	topRequired := []string{
		"schemaVersion", "purpose", "installationId", "installationAuthorityAlgorithm",
		"installationAuthorityKeyId", "installationAuthorityPublicKey", "issuedAt", "threadRisk",
		"sharedEvidence", "installationAuthoritySignature", "manifestDigest",
	}
	for _, field := range topRequired {
		t.Run("top missing "+field, func(t *testing.T) {
			candidate := deleteRawObjectFieldV1(t, body, "", field)
			if _, err := ParseManifestV1(candidate); err == nil {
				t.Fatal("ParseManifestV1() accepted a missing required field")
			}
		})
		t.Run("top null "+field, func(t *testing.T) {
			candidate := setRawObjectFieldV1(t, body, "", field, json.RawMessage("null"))
			if _, err := ParseManifestV1(candidate); err == nil {
				t.Fatal("ParseManifestV1() accepted a null required field")
			}
		})
	}

	nestedRequired := []string{
		"namespace", "enrollmentId", "endpointOrigin", "witnessAlgorithm", "witnessKeyId",
		"witnessPublicKey", "rootCaSha256", "serverName", "timeoutMs", "initialCheckpoint",
	}
	for _, objectName := range []string{"threadRisk", "sharedEvidence"} {
		for _, field := range nestedRequired {
			t.Run(objectName+" missing "+field, func(t *testing.T) {
				candidate := deleteRawObjectFieldV1(t, body, objectName, field)
				if _, err := ParseManifestV1(candidate); err == nil {
					t.Fatal("ParseManifestV1() accepted a missing nested field")
				}
			})
			t.Run(objectName+" null "+field, func(t *testing.T) {
				candidate := setRawObjectFieldV1(t, body, objectName, field, json.RawMessage("null"))
				if _, err := ParseManifestV1(candidate); err == nil {
					t.Fatal("ParseManifestV1() accepted a null nested field")
				}
			})
		}
	}
}

func TestNewManifestV1RejectsUnsafeEndpointAndServerIdentity(t *testing.T) {
	fixture := newManifestFixtureV1(t)
	tests := []struct {
		name       string
		origin     string
		serverName string
	}{
		{"plain HTTP", "http://risk-witness.example.test", "risk-witness.example.test"},
		{"trailing slash", "https://risk-witness.example.test/", "risk-witness.example.test"},
		{"path", "https://risk-witness.example.test/v1", "risk-witness.example.test"},
		{"traversal path", "https://risk-witness.example.test/../admin", "risk-witness.example.test"},
		{"encoded path", "https://risk-witness.example.test/%2e%2e/admin", "risk-witness.example.test"},
		{"query", "https://risk-witness.example.test?token=secret", "risk-witness.example.test"},
		{"fragment", "https://risk-witness.example.test#secret", "risk-witness.example.test"},
		{"userinfo", "https://user:secret@risk-witness.example.test", "risk-witness.example.test"},
		{"uppercase host", "https://Risk-Witness.example.test", "risk-witness.example.test"},
		{"invalid port", "https://risk-witness.example.test:65536", "risk-witness.example.test"},
		{"zero port", "https://risk-witness.example.test:0", "risk-witness.example.test"},
		{"noncanonical port", "https://risk-witness.example.test:0443", "risk-witness.example.test"},
		{"empty port", "https://risk-witness.example.test:", "risk-witness.example.test"},
		{"server mismatch", "https://risk-witness.example.test", "other.example.test"},
		{"server traversal", "https://risk-witness.example.test", "../risk-witness.example.test"},
		{"wildcard server", "https://risk-witness.example.test", "*.example.test"},
		{"server port", "https://risk-witness.example.test", "risk-witness.example.test:443"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := fixture.input
			input.ThreadRisk.EndpointOrigin = test.origin
			input.ThreadRisk.ServerName = test.serverName
			if _, err := NewManifestV1(input, fixture.sign); err == nil {
				t.Fatal("NewManifestV1() accepted unsafe endpoint configuration")
			}
		})
	}

	input := fixture.input
	input.ThreadRisk.EndpointOrigin = "https://[2001:db8::1]:8443"
	input.ThreadRisk.ServerName = "2001:db8::1"
	if _, err := NewManifestV1(input, fixture.sign); err != nil {
		t.Fatalf("NewManifestV1() rejected canonical IPv6 HTTPS origin: %v", err)
	}
}

func TestNewManifestV1RejectsInvalidKeysTimeoutsAndSigner(t *testing.T) {
	fixture := newManifestFixtureV1(t)

	for _, timeout := range []uint32{0, MinWitnessTimeoutMSV1 - 1, MaxWitnessTimeoutMSV1 + 1} {
		input := fixture.input
		input.ThreadRisk.TimeoutMS = timeout
		if _, err := NewManifestV1(input, fixture.sign); err == nil {
			t.Fatalf("NewManifestV1() accepted timeout %d", timeout)
		}
	}
	for _, timeout := range []uint32{MinWitnessTimeoutMSV1, MaxWitnessTimeoutMSV1} {
		input := fixture.input
		input.ThreadRisk.TimeoutMS = timeout
		if _, err := NewManifestV1(input, fixture.sign); err != nil {
			t.Fatalf("NewManifestV1() rejected timeout %d: %v", timeout, err)
		}
	}

	badWitnessKey := fixture.input
	badWitnessKey.ThreadRisk.WitnessKeyID = domainsecurity.SHA256Hex([]byte("wrong-witness-key"))
	if _, err := NewManifestV1(badWitnessKey, fixture.sign); err == nil {
		t.Fatal("NewManifestV1() accepted a witness key id mismatch")
	}

	badInitialCheckpoint := fixture.input
	badInitialCheckpoint.ThreadRisk.InitialCheckpoint = fixture.input.SharedEvidence.InitialCheckpoint
	if _, err := NewManifestV1(badInitialCheckpoint, fixture.sign); err == nil {
		t.Fatal("NewManifestV1() accepted another namespace's initial checkpoint")
	}

	badMTLS := fixture.input
	badMTLS.ThreadRisk.MTLSClientIdentityCertificateSHA256 = "not-a-sha256"
	if _, err := NewManifestV1(badMTLS, fixture.sign); err == nil {
		t.Fatal("NewManifestV1() accepted an invalid mTLS identity digest")
	}

	installationAsWitness := fixture.input
	installationAsWitness.ThreadRisk.WitnessKeyID = fixture.input.InstallationAuthorityKeyID
	installationAsWitness.ThreadRisk.WitnessPublicKey = fixture.installationPublicKey
	if _, err := NewManifestV1(installationAsWitness, fixture.sign); err == nil {
		t.Fatal("NewManifestV1() accepted the installation authority as its own witness")
	}

	otherPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x6a}, ed25519.SeedSize))
	wrongSigner := func(message []byte) ([]byte, error) { return ed25519.Sign(otherPrivateKey, message), nil }
	if _, err := NewManifestV1(fixture.input, wrongSigner); err == nil {
		t.Fatal("NewManifestV1() accepted a signature from another installation key")
	}
	if _, err := NewManifestV1(fixture.input, nil); err == nil {
		t.Fatal("NewManifestV1() accepted a nil signer")
	}
}

type manifestFixtureV1 struct {
	input                  ManifestInputV1
	installationPublicKey  ed25519.PublicKey
	installationPrivateKey ed25519.PrivateKey
}

func newManifestFixtureV1(t *testing.T) manifestFixtureV1 {
	t.Helper()
	installationPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x11}, ed25519.SeedSize))
	installationPublicKey := installationPrivateKey.Public().(ed25519.PublicKey)
	threadPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x21}, ed25519.SeedSize))
	threadPublicKey := threadPrivateKey.Public().(ed25519.PublicKey)
	sharedPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, ed25519.SeedSize))
	sharedPublicKey := sharedPrivateKey.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("installation-v1"))
	threadEnrollmentID := domainsecurity.SHA256Hex([]byte("thread-risk-enrollment"))
	sharedEnrollmentID := domainsecurity.SHA256Hex([]byte("shared-evidence-enrollment"))
	threadInitial := initialManifestCheckpointV1(t, installationID, threadEnrollmentID, ThreadRiskNamespaceV1, threadPrivateKey)
	sharedInitial := initialManifestCheckpointV1(t, installationID, sharedEnrollmentID, SharedEvidenceNamespaceV1, sharedPrivateKey)
	return manifestFixtureV1{
		installationPublicKey:  installationPublicKey,
		installationPrivateKey: installationPrivateKey,
		input: ManifestInputV1{
			InstallationID:                 installationID,
			InstallationAuthorityKeyID:     domainsecurity.SHA256Hex(installationPublicKey),
			InstallationAuthorityPublicKey: installationPublicKey,
			IssuedAt:                       time.Date(2026, 7, 13, 8, 9, 10, 123_456_789, time.UTC),
			ThreadRisk: WitnessEnrollmentInputV1{
				EnrollmentID:                        threadEnrollmentID,
				EndpointOrigin:                      "https://risk-witness.example.test:8443",
				WitnessKeyID:                        domainsecurity.SHA256Hex(threadPublicKey),
				WitnessPublicKey:                    threadPublicKey,
				RootCASHA256:                        domainsecurity.SHA256Hex([]byte("thread-risk-root-ca")),
				MTLSClientIdentityCertificateSHA256: domainsecurity.SHA256Hex([]byte("thread-risk-client-identity")),
				ServerName:                          "risk-witness.example.test",
				TimeoutMS:                           5_000,
				InitialCheckpoint:                   threadInitial,
			},
			SharedEvidence: WitnessEnrollmentInputV1{
				EnrollmentID:      sharedEnrollmentID,
				EndpointOrigin:    "https://evidence-witness.example.test",
				WitnessKeyID:      domainsecurity.SHA256Hex(sharedPublicKey),
				WitnessPublicKey:  sharedPublicKey,
				RootCASHA256:      domainsecurity.SHA256Hex([]byte("shared-evidence-root-ca")),
				ServerName:        "evidence-witness.example.test",
				TimeoutMS:         7_500,
				InitialCheckpoint: sharedInitial,
			},
		},
	}
}

func initialManifestCheckpointV1(
	t *testing.T,
	installationID, enrollmentID, namespace string,
	witnessPrivateKey ed25519.PrivateKey,
) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	witnessPublicKey := witnessPrivateKey.Public().(ed25519.PublicKey)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: namespace, Generation: 0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("initial-state:" + namespace)),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("initial-fence:" + namespace)),
		WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublicKey), WitnessPublicKey: witnessPublicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witnessPrivateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func (fixture manifestFixtureV1) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.installationPrivateKey, message), nil
}

func (fixture manifestFixtureV1) newManifest(t *testing.T) ManifestV1 {
	t.Helper()
	manifest, err := NewManifestV1(fixture.input, fixture.sign)
	if err != nil {
		t.Fatalf("NewManifestV1() error = %v", err)
	}
	return manifest
}

func resignManifestV1(t *testing.T, manifest *ManifestV1, privateKey ed25519.PrivateKey) {
	t.Helper()
	manifest.InstallationAuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, ManifestSigningBytesV1(*manifest)))
	manifest.ManifestDigest = manifestDigestV1(*manifest)
}

func setRawObjectFieldV1(t *testing.T, body []byte, objectName, field string, value json.RawMessage) []byte {
	t.Helper()
	object := decodeRawObjectV1(t, body)
	if objectName == "" {
		object[field] = value
	} else {
		nested := decodeRawObjectV1(t, object[objectName])
		nested[field] = value
		object[objectName] = marshalJSONV1(t, nested)
	}
	return marshalJSONV1(t, object)
}

func deleteRawObjectFieldV1(t *testing.T, body []byte, objectName, field string) []byte {
	t.Helper()
	object := decodeRawObjectV1(t, body)
	if objectName == "" {
		delete(object, field)
	} else {
		nested := decodeRawObjectV1(t, object[objectName])
		delete(nested, field)
		object[objectName] = marshalJSONV1(t, nested)
	}
	return marshalJSONV1(t, object)
}

func reorderRawObjectV1(t *testing.T, body []byte) []byte {
	t.Helper()
	return marshalJSONV1(t, decodeRawObjectV1(t, body))
}

func decodeRawObjectV1(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func marshalJSONV1(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
