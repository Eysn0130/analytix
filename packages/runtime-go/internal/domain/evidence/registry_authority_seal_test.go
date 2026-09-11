package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestEvidenceRegistryAuthoritySealBindsExactCurrentHead(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-seal", "turn-seal", "case-seal", "snapshot-seal", 4)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "4200000")
	proof := evidenceReceiptTestSettlementProof("registry-seal")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	registry, receipt, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := NewEvidenceRegistryAuthoritySeal(registry, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil || !EvidenceRegistryAuthoritySealMatchesRegistry(seal, registry) {
		t.Fatalf("signed registry head did not match: seal=%#v err=%v", seal, err)
	}
	revoked, err := RevokeEvidenceReceipt(registry, receipt.ReceiptID, "source_retracted", evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	if EvidenceRegistryAuthoritySealMatchesRegistry(seal, revoked) {
		t.Fatal("old signed head authorized a later registry state")
	}
	seal.Sequence++
	if ValidateEvidenceRegistryAuthoritySeal(seal) == nil {
		t.Fatal("tampered signed registry head remained valid")
	}
}

func TestEvidenceRegistryAuthorityCapsuleBindsContextRegistryAndCanonicalLedger(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-capsule", "turn-capsule", "case-capsule", "snapshot-capsule", 5)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "7300000")
	proof := evidenceReceiptTestSettlementProof("registry-capsule")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	registry, _, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	capsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := CanonicalEvidenceRegistryLedger(registry)
	if err != nil || capsule.CanonicalLedgerSHA256 != domainsecurity.SHA256Hex(ledger) || capsule.CanonicalLedgerByteLength != uint64(len(ledger)) {
		t.Fatalf("capsule did not bind canonical ledger bytes: capsule=%#v err=%v", capsule, err)
	}
	body, err := json.Marshal(capsule)
	parsed, parseErr := ParseEvidenceRegistryAuthorityCapsule(body)
	if err != nil || parseErr != nil || parsed.RecordDigest != capsule.RecordDigest {
		t.Fatalf("capsule strict round trip failed: marshal=%v parse=%v", err, parseErr)
	}
	parsed.SecurityContext.DatasetSnapshotID = "snapshot-other"
	if ValidateEvidenceRegistryAuthorityCapsule(parsed) == nil {
		t.Fatal("capsule accepted a security-context mutation")
	}
}

func TestEvidenceRegistryAuthorityIndexBindsExactSingleStepLineage(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-index", "turn-index", "case-index", "snapshot-index", 6)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "9100000")
	proof := evidenceReceiptTestSettlementProof("registry-index")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	registry, receipt, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
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
	first, err := NewEvidenceRegistryAuthorityIndex(nil, firstCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation != 1 || first.PreviousIndexDigest != EvidenceRegistryAuthorityIndexGenesisDigest() {
		t.Fatalf("genesis lineage mismatch: %#v", first)
	}
	revoked, err := RevokeEvidenceReceipt(registry, receipt.ReceiptID, "source_retracted", evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	secondCapsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, revoked, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEvidenceRegistryAuthorityIndex(&first, secondCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != 2 || second.PreviousIndexDigest != first.RecordDigest || second.Entries[0].RegistrySequence != 2 {
		t.Fatalf("second generation did not bind the exact predecessor: %#v", second)
	}
	entry, ok := EvidenceRegistryAuthorityIndexEntryForContext(second, securityContext)
	if !ok || !EvidenceRegistryAuthorityIndexEntryMatchesCapsule(second, entry, secondCapsule) {
		t.Fatal("index entry did not resolve its exact content-addressed capsule")
	}
}

func TestEvidenceRegistryAuthorityIndexRejectsForeignCapsuleKey(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-index-foreign", "turn-index-foreign", "case-index-foreign", "snapshot-index-foreign", 7)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "1200000")
	proof := evidenceReceiptTestSettlementProof("registry-index-foreign")
	registry, _, err := RegisterEvidenceReceipt(registry,
		evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID)), material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	foreignPublic, foreignPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	foreignCapsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, domainsecurity.SHA256Hex(foreignPublic), foreignPublic,
		func(message []byte) ([]byte, error) { return ed25519.Sign(foreignPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	localPublic, localPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvidenceRegistryAuthorityIndex(nil, foreignCapsule, domainsecurity.SHA256Hex(localPublic), localPublic,
		func(message []byte) ([]byte, error) { return ed25519.Sign(localPrivate, message), nil }); err == nil {
		t.Fatal("foreign-key capsule was laundered through a local authority index")
	}
}

func TestEvidenceRegistryAuthorityIndexRejectsSequenceJumpAndForeignPrefix(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-index-jump", "turn-index-jump", "case-index-jump", "snapshot-index-jump", 8)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := domainsecurity.SHA256Hex(publicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }

	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	firstMaterial := evidenceReceiptTestMaterial(t, "3100000")
	firstProof := evidenceReceiptTestSettlementProof("registry-index-jump-first")
	registry, firstReceipt, err := RegisterEvidenceReceipt(registry,
		evidenceReceiptTestDraft(t, securityContext, firstMaterial, EvidenceSettlementReceiptID(firstProof.SettlementID)), firstMaterial, firstProof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	firstCapsule, _ := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, keyID, publicKey, sign)
	firstIndex, err := NewEvidenceRegistryAuthorityIndex(nil, firstCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RevokeEvidenceReceipt(registry, firstReceipt.ReceiptID, "source_retracted", evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	thirdMaterial := evidenceReceiptTestMaterial(t, "3200000")
	thirdProof := evidenceReceiptTestSettlementProof("registry-index-jump-third")
	third, _, err := RegisterEvidenceReceipt(second,
		evidenceReceiptTestDraft(t, securityContext, thirdMaterial, EvidenceSettlementReceiptID(thirdProof.SettlementID)), thirdMaterial, thirdProof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	jumpCapsule, _ := NewEvidenceRegistryAuthorityCapsule(securityContext, third, keyID, publicKey, sign)
	if _, err := NewEvidenceRegistryAuthorityIndex(&firstIndex, jumpCapsule, keyID, publicKey, sign); err == nil {
		t.Fatal("authority index accepted a sequence jump")
	}

	foreignPrefix, _ := NewEvidenceReceiptRegistry(securityContext)
	foreignMaterial := evidenceReceiptTestMaterial(t, "4100000")
	foreignProof := evidenceReceiptTestSettlementProof("registry-index-foreign-prefix")
	foreignPrefix, foreignReceipt, err := RegisterEvidenceReceipt(foreignPrefix,
		evidenceReceiptTestDraft(t, securityContext, foreignMaterial, EvidenceSettlementReceiptID(foreignProof.SettlementID)), foreignMaterial, foreignProof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	foreignNext, err := RevokeEvidenceReceipt(foreignPrefix, foreignReceipt.ReceiptID, "source_retracted", evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	foreignPrefixCapsule, _ := NewEvidenceRegistryAuthorityCapsule(securityContext, foreignNext, keyID, publicKey, sign)
	if _, err := NewEvidenceRegistryAuthorityIndex(&firstIndex, foreignPrefixCapsule, keyID, publicKey, sign); err == nil {
		t.Fatal("authority index accepted a different complete-record prefix")
	}
}

func TestEvidenceRegistryAuthorityIndexRejectsProjectionKeyCollision(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := domainsecurity.SHA256Hex(publicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	buildCapsule := func(threadID, turnID, label string) EvidenceRegistryAuthorityCapsule {
		securityContext := evidenceReceiptTestContext(t, threadID, turnID, "case-"+label, "snapshot-"+label, 9)
		registry, _ := NewEvidenceReceiptRegistry(securityContext)
		material := evidenceReceiptTestMaterial(t, "5100000")
		proof := evidenceReceiptTestSettlementProof(label)
		registry, _, registerErr := RegisterEvidenceReceipt(registry,
			evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID)), material, proof, evidenceReceiptTestTime())
		if registerErr != nil {
			t.Fatal(registerErr)
		}
		capsule, capsuleErr := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, keyID, publicKey, sign)
		if capsuleErr != nil {
			t.Fatal(capsuleErr)
		}
		return capsule
	}
	firstCapsule := buildCapsule("thread-index-left", "turn-index-left", "index-left")
	index, err := NewEvidenceRegistryAuthorityIndex(nil, firstCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	secondCapsule := buildCapsule("thread-index-right", "turn-index-right", "index-right")
	index, err = NewEvidenceRegistryAuthorityIndex(&index, secondCapsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	index.Entries[1].ProjectionKey = index.Entries[0].ProjectionKey
	index.AuthoritySignature = ""
	index.RecordDigest = ""
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, EvidenceRegistryAuthorityIndexSigningBytes(index)))
	index.RecordDigest = evidenceRegistryAuthorityIndexDigest(index)
	if ValidateEvidenceRegistryAuthorityIndex(index) == nil {
		t.Fatal("authority index accepted colliding projection keys")
	}
}

func TestEvidenceRegistryAuthorityRecordsRejectDuplicateKeysAndNonCanonicalBytes(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-canonical", "turn-canonical", "case-canonical", "snapshot-canonical", 10)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "6100000")
	proof := evidenceReceiptTestSettlementProof("registry-canonical")
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
	index, err := NewEvidenceRegistryAuthorityIndex(nil, capsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, _ := json.Marshal(index)
	duplicateIndex := append([]byte(`{"generation":999,`), indexBody[1:]...)
	if _, err := ParseEvidenceRegistryAuthorityIndex(duplicateIndex); err == nil {
		t.Fatal("authority index accepted a duplicate key with signed last-value semantics")
	}
	capsuleBody, _ := json.Marshal(capsule)
	duplicateCapsule := append([]byte(`{"schemaVersion":999,`), capsuleBody[1:]...)
	if _, err := ParseEvidenceRegistryAuthorityCapsule(duplicateCapsule); err == nil {
		t.Fatal("authority capsule accepted a duplicate key with signed last-value semantics")
	}
	nonCanonical := append([]byte(" "), indexBody...)
	if _, err := ParseEvidenceRegistryAuthorityIndex(nonCanonical); err == nil {
		t.Fatal("authority index accepted semantically equivalent non-canonical bytes")
	}
	if bytes.Equal(indexBody, nonCanonical) {
		t.Fatal("non-canonical fixture did not change raw bytes")
	}
}
