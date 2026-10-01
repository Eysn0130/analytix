package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestEvidenceRegistryHostLocalV3IndexClosedModeAndExactCapsule(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-host-index", "turn-host-index", "case-host-index", "snapshot-host-index", 9)
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	material := evidenceReceiptTestMaterial(t, "9100000")
	proof := evidenceReceiptTestSettlementProof("registry-host-index")
	registry, receipt, err := RegisterEvidenceReceipt(registry,
		evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID)),
		material, proof, evidenceReceiptTestTime())
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
	installationID := domainsecurity.SHA256Hex([]byte("host-index-installation"))
	mode := domainsecurity.SHA256Hex([]byte("host-index-mode-genesis"))
	first, err := NewEvidenceRegistryAuthorityIndexHostLocalV3(EvidenceRegistryAuthorityIndexHostLocalInputV3{
		InstallationID: installationID, ModeCommitmentDigest: mode, Generation: 1,
		PreviousIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		MutationID:          domainsecurity.SHA256Hex([]byte("host-index-mutation-one")),
	}, capsule, keyID, publicKey, sign)
	if err != nil || !EvidenceRegistryAuthorityIndexEntryMatchesCapsuleHostLocalV3(first, capsule) ||
		ValidateEvidenceRegistryAuthorityIndexHostLocalRootV3(first, first.IndexDigest, 1) != nil ||
		ValidateEvidenceRegistryAuthorityIndexV2(first) == nil {
		t.Fatalf("host-local index or distinct V2 rejection failed: %v", err)
	}
	body, err := VersionedEvidenceRegistryAuthorityIndexBytes(first)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseVersionedEvidenceRegistryAuthorityIndex(body)
	if err != nil || parsed != first || !bytes.Equal(body, mustRegistryHostIndexBytesV3(t, parsed)) {
		t.Fatalf("host-local index canonical parse failed: %v", err)
	}
	if _, err := ParseEvidenceRegistryAuthorityIndexV2(body); err == nil {
		t.Fatal("witnessed V2 parser accepted the host-local index")
	}
	for _, mutate := range []func(*EvidenceRegistryAuthorityIndexV2){
		func(index *EvidenceRegistryAuthorityIndexV2) { index.Mode = "witnessed" },
		func(index *EvidenceRegistryAuthorityIndexV2) {
			index.EnrollmentID = domainsecurity.SHA256Hex([]byte("fake-enrollment"))
		},
		func(index *EvidenceRegistryAuthorityIndexV2) {
			index.ModeCommitmentDigest = domainsecurity.SHA256Hex([]byte("other-mode"))
		},
	} {
		candidate := first
		mutate(&candidate)
		if ValidateEvidenceRegistryAuthorityIndexHostLocalV3(candidate) == nil {
			t.Fatal("host-local index accepted a mixed or tampered mode")
		}
	}
	var unknown map[string]any
	if err := json.Unmarshal(body, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["unknown"] = true
	unknownBody, _ := json.Marshal(unknown)
	if _, err := ParseVersionedEvidenceRegistryAuthorityIndex(unknownBody); err == nil {
		t.Fatal("versioned registry parser accepted an unknown field")
	}
	revoked, err := RevokeEvidenceReceipt(registry, receipt.ReceiptID, "source_retracted", evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	nextCapsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, revoked, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewEvidenceRegistryAuthorityIndexHostLocalV3(EvidenceRegistryAuthorityIndexHostLocalInputV3{
		InstallationID: installationID, ModeCommitmentDigest: mode, Generation: 2,
		PreviousIndexDigest: first.IndexDigest,
		MutationID:          domainsecurity.SHA256Hex([]byte("host-index-mutation-two")),
	}, nextCapsule, keyID, publicKey, sign)
	if err != nil || ValidateEvidenceRegistryAuthorityIndexHostLocalTransitionV3(first, next) != nil ||
		ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(first.Entry, next.Entry, nextCapsule) != nil {
		t.Fatalf("host-local revocation index transition failed: %v", err)
	}
}

func mustRegistryHostIndexBytesV3(t *testing.T, index EvidenceRegistryAuthorityIndexV2) []byte {
	t.Helper()
	body, err := VersionedEvidenceRegistryAuthorityIndexBytes(index)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
