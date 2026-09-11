package evidence

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type evidenceWitnessBindingFixture struct {
	bundle          EvidenceAuthorityBundleV1
	request         domainsecurity.MonotonicHeadObserveRequestV1
	observation     domainsecurity.MonotonicHeadObservationV1
	installationID  string
	enrollmentID    string
	authorityKeyID  string
	authorityPublic ed25519.PublicKey
	witnessKeyID    string
	witnessPublic   ed25519.PublicKey
	authorityPriv   ed25519.PrivateKey
	witnessPriv     ed25519.PrivateKey
}

func TestEvidenceAuthorityWitnessBindingRequiresExactFreshObservation(t *testing.T) {
	fixture := newEvidenceWitnessBindingFixture(t, "primary")
	binding, err := NewEvidenceAuthorityWitnessBindingV1(
		fixture.bundle, fixture.request, fixture.observation,
		fixture.installationID, fixture.enrollmentID, fixture.authorityKeyID, fixture.authorityPublic,
		fixture.witnessKeyID, fixture.witnessPublic,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceAuthorityWitnessBindingExactV1(
		binding, fixture.bundle, fixture.request, fixture.observation,
		fixture.installationID, fixture.enrollmentID, fixture.authorityKeyID, fixture.authorityPublic,
		fixture.witnessKeyID, fixture.witnessPublic,
	); err != nil {
		t.Fatalf("exact witnessed binding rejected: %v", err)
	}

	body, err := EvidenceAuthorityWitnessBindingV1Bytes(binding)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEvidenceAuthorityWitnessBindingV1(body)
	if err != nil || parsed != binding {
		t.Fatalf("binding round trip mismatch: parsed=%#v err=%v", parsed, err)
	}
	var withUnknown map[string]any
	if err := json.Unmarshal(body, &withUnknown); err != nil {
		t.Fatal(err)
	}
	withUnknown["unknown"] = true
	unknownBody, _ := json.Marshal(withUnknown)
	if _, err := ParseEvidenceAuthorityWitnessBindingV1(unknownBody); err == nil {
		t.Fatal("unknown witness binding field was accepted")
	}
}

func TestEvidenceAuthorityWitnessBindingRejectsStaleOrMismatchedAuthority(t *testing.T) {
	fixture := newEvidenceWitnessBindingFixture(t, "current")
	binding, err := NewEvidenceAuthorityWitnessBindingV1(
		fixture.bundle, fixture.request, fixture.observation,
		fixture.installationID, fixture.enrollmentID, fixture.authorityKeyID, fixture.authorityPublic,
		fixture.witnessKeyID, fixture.witnessPublic,
	)
	if err != nil {
		t.Fatal(err)
	}

	wrongRequest := evidenceObserveRequest(t, fixture, "other-challenge")
	if err := ValidateEvidenceAuthorityWitnessBindingExactV1(
		binding, fixture.bundle, wrongRequest, fixture.observation,
		fixture.installationID, fixture.enrollmentID, fixture.authorityKeyID, fixture.authorityPublic,
		fixture.witnessKeyID, fixture.witnessPublic,
	); err == nil {
		t.Fatal("observation from another challenge was accepted")
	}

	wrongWitnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x72}, ed25519.SeedSize))
	wrongWitnessPublic := wrongWitnessPrivate.Public().(ed25519.PublicKey)
	if err := ValidateEvidenceAuthorityWitnessBindingExactV1(
		binding, fixture.bundle, fixture.request, fixture.observation,
		fixture.installationID, fixture.enrollmentID, fixture.authorityKeyID, fixture.authorityPublic,
		domainsecurity.SHA256Hex(wrongWitnessPublic), wrongWitnessPublic,
	); err == nil {
		t.Fatal("wrong enrolled witness key was accepted")
	}

	tampered := binding
	tampered.BundleRecordDigest = domainsecurity.SHA256Hex([]byte("other-bundle"))
	tampered.BindingDigest = evidenceAuthorityWitnessBindingDigestV1(tampered)
	if err := ValidateEvidenceAuthorityWitnessBindingExactV1(
		tampered, fixture.bundle, fixture.request, fixture.observation,
		fixture.installationID, fixture.enrollmentID, fixture.authorityKeyID, fixture.authorityPublic,
		fixture.witnessKeyID, fixture.witnessPublic,
	); err == nil {
		t.Fatal("binding for another bundle was accepted")
	}
}

func newEvidenceWitnessBindingFixture(t *testing.T, label string) evidenceWitnessBindingFixture {
	t.Helper()
	authorityPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	authorityPublic := authorityPrivate.Public().(ed25519.PublicKey)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x57}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	fixture := evidenceWitnessBindingFixture{
		installationID: domainsecurity.SHA256Hex([]byte("installation-" + label)),
		enrollmentID:   domainsecurity.SHA256Hex([]byte("enrollment-" + label)),
		authorityKeyID: domainsecurity.SHA256Hex(authorityPublic), authorityPublic: authorityPublic,
		witnessKeyID: domainsecurity.SHA256Hex(witnessPublic), witnessPublic: witnessPublic,
		authorityPriv: authorityPrivate, witnessPriv: witnessPrivate,
	}
	bundle, err := NewEvidenceAuthorityBundleV1(EvidenceAuthorityBundleInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID, Generation: 1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("mutation-" + label)),
		DatasetSnapshotIndexDigest:  domainsecurity.SHA256Hex([]byte("dataset-" + label)),
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("registry-" + label)),
		PublicationIndexDigest:      domainsecurity.SHA256Hex([]byte("publication-" + label)),
		AuthorityKeyID:              fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authorityPriv, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	fixture.bundle = bundle
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace: EvidenceAuthorityBundleWitnessNamespaceV1, Generation: bundle.Generation,
		CurrentStateDigest: bundle.RecordDigest, PreviousStateDigest: domainsecurity.SHA256Hex([]byte("previous-state-" + label)),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("previous-checkpoint-" + label)),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("fence-" + label)), MutationID: bundle.MutationID,
		WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPriv, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	fixture.request = evidenceObserveRequest(t, fixture, "challenge-"+label)
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		fixture.request, checkpoint,
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPriv, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.observation = observation
	return fixture
}

func evidenceObserveRequest(t *testing.T, fixture evidenceWitnessBindingFixture, label string) domainsecurity.MonotonicHeadObserveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace:      EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte(label)), AuthorityKeyID: fixture.authorityKeyID,
		AuthorityPublicKey: fixture.authorityPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authorityPriv, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return request
}
