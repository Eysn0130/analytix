package evidence

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestEvidenceRegistryAuthorityIndexV2BindsWitnessEnrollmentAndSingleExtension(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-index-v2", "turn-index-v2", "case-index-v2", "snapshot-index-v2", 9)
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	material := evidenceReceiptTestMaterial(t, "9100000")
	proof := evidenceReceiptTestSettlementProof("registry-index-v2")
	registry, receipt, err := RegisterEvidenceReceipt(registry,
		evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID)), material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := domainsecurity.SHA256Hex(publicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	firstCapsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	installationID := domainsecurity.SHA256Hex([]byte("registry-index-v2-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("registry-index-v2-enrollment"))
	first, err := NewEvidenceRegistryAuthorityIndexV2(EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigestV2(), MutationID: domainsecurity.SHA256Hex([]byte("registry-index-v2-mutation-1")),
	}, firstCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceRegistryAuthorityIndexForInstallationV2(first, installationID, enrollmentID, keyID, publicKey); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(first, first.IndexDigest, 1); err != nil {
		t.Fatal(err)
	}
	if !EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(first, firstCapsule) {
		t.Fatal("V2 index entry did not bind the exact capsule")
	}

	revoked, err := RevokeEvidenceReceipt(registry, receipt.ReceiptID, "source_retracted", evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	secondCapsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, revoked, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEvidenceRegistryAuthorityIndexV2(EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 2, PreviousIndexDigest: first.IndexDigest,
		MutationID: domainsecurity.SHA256Hex([]byte("registry-index-v2-mutation-2")),
	}, secondCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceRegistryAuthorityIndexTransitionV2(first, second); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(first.Entry, second.Entry, secondCapsule); err != nil {
		t.Fatal(err)
	}
	body, err := EvidenceRegistryAuthorityIndexV2Bytes(second)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEvidenceRegistryAuthorityIndexV2(body)
	if err != nil || parsed.IndexDigest != second.IndexDigest {
		t.Fatalf("V2 index did not round trip canonically: parsed=%#v err=%v", parsed, err)
	}
	var unknown map[string]any
	_ = json.Unmarshal(body, &unknown)
	unknown["unknown"] = true
	unknownBody, _ := json.Marshal(unknown)
	if _, err := ParseEvidenceRegistryAuthorityIndexV2(unknownBody); err == nil {
		t.Fatal("V2 index accepted an unknown property")
	}
}

func TestEvidenceRegistryAuthorityIndexV2RejectsV1GenesisAndWrongWitnessCount(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-index-v2-audit", "turn-index-v2-audit", "case-index-v2-audit", "snapshot-index-v2-audit", 10)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "1200000")
	proof := evidenceReceiptTestSettlementProof("registry-index-v2-audit")
	registry, _, err := RegisterEvidenceReceipt(registry,
		evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID)), material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := domainsecurity.SHA256Hex(publicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	capsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	input := EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: domainsecurity.SHA256Hex([]byte("installation")), EnrollmentID: domainsecurity.SHA256Hex([]byte("enrollment")),
		Generation: 1, PreviousIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigest(), MutationID: domainsecurity.SHA256Hex([]byte("mutation")),
	}
	if _, err := NewEvidenceRegistryAuthorityIndexV2(input, capsule, keyID, publicKey, sign); err == nil {
		t.Fatal("V2 registry index accepted the legacy local-index genesis")
	}
	input.PreviousIndexDigest = EvidenceRegistryAuthorityIndexGenesisDigestV2()
	index, err := NewEvidenceRegistryAuthorityIndexV2(input, capsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(index, index.IndexDigest, 2); err == nil {
		t.Fatal("V2 registry index accepted a mismatched witnessed count")
	}
}
