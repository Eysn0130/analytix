package reportpublication

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPublicationIndexBindsInstallationWitnessReceiptAndTarget(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x51}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	first := publicationIndexFixture(t, privateKey, publicKey, PublicationIndexInputV1{
		InstallationID: domainsecurity.SHA256Hex([]byte("installation")), EnrollmentID: domainsecurity.SHA256Hex([]byte("enrollment")),
		Generation: 1, PreviousIndexDigest: PublicationIndexGenesisDigestV1(), MutationID: domainsecurity.SHA256Hex([]byte("mutation-1")),
		ReceiptID: domainsecurity.SHA256Hex([]byte("receipt-1")), ReceiptRecordDigest: domainsecurity.SHA256Hex([]byte("receipt-record-1")),
		TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target-1")), AuthorityKeyID: keyID,
	})
	if err := ValidatePublicationIndexForInstallationV1(first, first.InstallationID, first.EnrollmentID, keyID, publicKey); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicationIndexWitnessRootV1(first, first.IndexDigest, 1); err != nil {
		t.Fatal(err)
	}
	body, err := PublicationIndexV1Bytes(first)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePublicationIndexV1(body)
	if err != nil || parsed.IndexDigest != first.IndexDigest {
		t.Fatalf("canonical publication index did not round trip: parsed=%#v err=%v", parsed, err)
	}
	var mutated map[string]any
	_ = json.Unmarshal(body, &mutated)
	mutated["unknown"] = true
	unknown, _ := json.Marshal(mutated)
	if _, err := ParsePublicationIndexV1(unknown); err == nil {
		t.Fatal("publication index accepted an unknown property")
	}

	second := publicationIndexFixture(t, privateKey, publicKey, PublicationIndexInputV1{
		InstallationID: first.InstallationID, EnrollmentID: first.EnrollmentID, Generation: 2,
		PreviousIndexDigest: first.IndexDigest, MutationID: domainsecurity.SHA256Hex([]byte("mutation-2")),
		ReceiptID: domainsecurity.SHA256Hex([]byte("receipt-2")), ReceiptRecordDigest: domainsecurity.SHA256Hex([]byte("receipt-record-2")),
		TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target-2")), AuthorityKeyID: keyID,
	})
	if err := ValidatePublicationIndexTransitionV1(first, second); err != nil {
		t.Fatal(err)
	}

	replayedTargetInput := PublicationIndexInputV1{
		InstallationID: first.InstallationID, EnrollmentID: first.EnrollmentID, Generation: 2,
		PreviousIndexDigest: first.IndexDigest, MutationID: domainsecurity.SHA256Hex([]byte("mutation-replay")),
		ReceiptID: domainsecurity.SHA256Hex([]byte("receipt-replay")), ReceiptRecordDigest: domainsecurity.SHA256Hex([]byte("receipt-record-replay")),
		TargetIdentityDigest: first.TargetIdentityDigest, AuthorityKeyID: keyID,
	}
	replayedTarget := publicationIndexFixture(t, privateKey, publicKey, replayedTargetInput)
	if err := ValidatePublicationIndexTransitionV1(first, replayedTarget); err == nil {
		t.Fatal("publication index replayed an already delivered target")
	}
}

func TestPublicationIndexRejectsWrongGenesisAndWitnessCount(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x52}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	input := PublicationIndexInputV1{
		InstallationID: domainsecurity.SHA256Hex([]byte("installation")), EnrollmentID: domainsecurity.SHA256Hex([]byte("enrollment")),
		Generation: 1, PreviousIndexDigest: domainsecurity.SHA256Hex([]byte("attacker-genesis")), MutationID: domainsecurity.SHA256Hex([]byte("mutation")),
		ReceiptID: domainsecurity.SHA256Hex([]byte("receipt")), ReceiptRecordDigest: domainsecurity.SHA256Hex([]byte("receipt-record")),
		TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("target")), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}
	if _, err := NewPublicationIndexV1(input, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }); err == nil {
		t.Fatal("publication index accepted an attacker-selected genesis")
	}
	input.PreviousIndexDigest = PublicationIndexGenesisDigestV1()
	index := publicationIndexFixture(t, privateKey, publicKey, input)
	if err := ValidatePublicationIndexWitnessRootV1(index, index.IndexDigest, 2); err == nil {
		t.Fatal("publication index accepted a mismatched witnessed count")
	}
}

func publicationIndexFixture(t *testing.T, privateKey ed25519.PrivateKey, publicKey ed25519.PublicKey, input PublicationIndexInputV1) PublicationIndexV1 {
	t.Helper()
	input.AuthorityPublicKey = publicKey
	index, err := NewPublicationIndexV1(input, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return index
}
