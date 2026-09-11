package evidenceauthority

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

type evidenceAuthorityStoreFixture struct {
	authorityPrivate ed25519.PrivateKey
	authorityPublic  ed25519.PublicKey
	witnessPrivate   ed25519.PrivateKey
	witnessPublic    ed25519.PublicKey
	installationID   string
	enrollmentID     string
}

func newEvidenceAuthorityStoreFixture(seed byte) evidenceAuthorityStoreFixture {
	authorityPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed + 1}, ed25519.SeedSize))
	return evidenceAuthorityStoreFixture{
		authorityPrivate: authorityPrivate,
		authorityPublic:  authorityPrivate.Public().(ed25519.PublicKey),
		witnessPrivate:   witnessPrivate,
		witnessPublic:    witnessPrivate.Public().(ed25519.PublicKey),
		installationID:   domainsecurity.SHA256Hex([]byte{seed, 'i'}),
		enrollmentID:     domainsecurity.SHA256Hex([]byte{seed, 'e'}),
	}
}

func (fixture evidenceAuthorityStoreFixture) firstBundle(t *testing.T, label string) domainevidence.EvidenceAuthorityBundleV1 {
	t.Helper()
	return fixture.newBundle(t, domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              fixture.installationID,
		EnrollmentID:                fixture.enrollmentID,
		Generation:                  1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("mutation-" + label + "-1")),
		DatasetSnapshotIndexDigest:  domainsecurity.SHA256Hex([]byte("dataset-" + label + "-0")),
		DatasetSnapshotCount:        0,
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("registry-" + label + "-0")),
		EvidenceRegistryCount:       0,
		PublicationIndexDigest:      domainsecurity.SHA256Hex([]byte("publication-" + label + "-0")),
		PublicationCount:            0,
		AuthorityKeyID:              domainsecurity.SHA256Hex(fixture.authorityPublic),
		AuthorityPublicKey:          fixture.authorityPublic,
	})
}

func (fixture evidenceAuthorityStoreFixture) nextBundle(
	t *testing.T,
	previous domainevidence.EvidenceAuthorityBundleV1,
	child string,
	label string,
) domainevidence.EvidenceAuthorityBundleV1 {
	t.Helper()
	input := domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              previous.InstallationID,
		EnrollmentID:                previous.EnrollmentID,
		Generation:                  previous.Generation + 1,
		PreviousBundleDigest:        previous.RecordDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("mutation-" + label)),
		DatasetSnapshotIndexDigest:  previous.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:        previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       previous.EvidenceRegistryCount,
		PublicationIndexDigest:      previous.PublicationIndexDigest,
		PublicationCount:            previous.PublicationCount,
		AuthorityKeyID:              domainsecurity.SHA256Hex(fixture.authorityPublic),
		AuthorityPublicKey:          fixture.authorityPublic,
	}
	switch child {
	case "dataset":
		input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("dataset-" + label))
		input.DatasetSnapshotCount++
	case "registry":
		input.EvidenceRegistryIndexDigest = domainsecurity.SHA256Hex([]byte("registry-" + label))
		input.EvidenceRegistryCount++
	case "publication":
		input.PublicationIndexDigest = domainsecurity.SHA256Hex([]byte("publication-" + label))
		input.PublicationCount++
	default:
		t.Fatalf("unknown evidence authority child %q", child)
	}
	return fixture.newBundle(t, input)
}

func (fixture evidenceAuthorityStoreFixture) newBundle(
	t *testing.T,
	input domainevidence.EvidenceAuthorityBundleInputV1,
) domainevidence.EvidenceAuthorityBundleV1 {
	t.Helper()
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func (fixture evidenceAuthorityStoreFixture) observationBundle(
	t *testing.T,
	bundle domainevidence.EvidenceAuthorityBundleV1,
	label string,
) storeport.ObservationBundle {
	t.Helper()
	request := fixture.observeRequest(t, label)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           fixture.installationID,
		EnrollmentID:             fixture.enrollmentID,
		Namespace:                domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation:               bundle.Generation,
		CurrentStateDigest:       bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("previous-state-" + label)),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("previous-checkpoint-" + label)),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("fence-" + label)),
		MutationID:               bundle.MutationID,
		WitnessKeyID:             domainsecurity.SHA256Hex(fixture.witnessPublic),
		WitnessPublicKey:         fixture.witnessPublic,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.witnessPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.witnessPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return storeport.ObservationBundle{Bundle: bundle, Request: request, Observation: observation}
}

func (fixture evidenceAuthorityStoreFixture) observeRequest(t *testing.T, label string) domainsecurity.MonotonicHeadObserveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID:     fixture.installationID,
		EnrollmentID:       fixture.enrollmentID,
		Namespace:          domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce:     domainsecurity.SHA256Hex([]byte("challenge-" + label)),
		AuthorityKeyID:     domainsecurity.SHA256Hex(fixture.authorityPublic),
		AuthorityPublicKey: fixture.authorityPublic,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func canonicalEvidenceBundleBody(t *testing.T, bundle domainevidence.EvidenceAuthorityBundleV1) []byte {
	t.Helper()
	body, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(body)
}
