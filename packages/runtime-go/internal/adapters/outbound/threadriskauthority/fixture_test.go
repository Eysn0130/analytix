package threadriskauthority

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/threadriskauthority"
)

type authorityStoreFixture struct {
	authorityPrivate ed25519.PrivateKey
	authorityPublic  ed25519.PublicKey
	witnessPrivate   ed25519.PrivateKey
	witnessPublic    ed25519.PublicKey
	installationID   string
	enrollmentID     string
}

func newAuthorityStoreFixture(seed byte) authorityStoreFixture {
	authorityPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed + 1}, ed25519.SeedSize))
	return authorityStoreFixture{
		authorityPrivate: authorityPrivate,
		authorityPublic:  authorityPrivate.Public().(ed25519.PublicKey),
		witnessPrivate:   witnessPrivate,
		witnessPublic:    witnessPrivate.Public().(ed25519.PublicKey),
		installationID:   domainsecurity.SHA256Hex([]byte{seed, 'i'}),
		enrollmentID:     domainsecurity.SHA256Hex([]byte{seed, 'e'}),
	}
}

func (fixture authorityStoreFixture) firstIndex(t *testing.T, label string) domainsecurity.ThreadRiskAuthorityIndexV1 {
	t.Helper()
	return fixture.newIndex(t, domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID: fixture.installationID,
		EnrollmentID:   fixture.enrollmentID,
		Namespace:      domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:     1,
		Entries: []domainsecurity.ThreadRiskAuthorityEntryV1{{
			ThreadID:            "thread-" + label,
			WorkspaceRealPath:   "/workspace/" + label,
			RiskClass:           domainsecurity.RiskClassGeneral,
			CurrentPolicyDigest: domainsecurity.SHA256Hex([]byte("policy-" + label + "-1")),
		}},
		MutationID:         domainsecurity.SHA256Hex([]byte("mutation-" + label + "-1")),
		AuthorityKeyID:     domainsecurity.SHA256Hex(fixture.authorityPublic),
		AuthorityPublicKey: fixture.authorityPublic,
	})
}

func (fixture authorityStoreFixture) nextIndex(t *testing.T, previous domainsecurity.ThreadRiskAuthorityIndexV1, label string) domainsecurity.ThreadRiskAuthorityIndexV1 {
	t.Helper()
	entries := append([]domainsecurity.ThreadRiskAuthorityEntryV1(nil), previous.Entries...)
	entries[0].PreviousPolicyDigest = entries[0].CurrentPolicyDigest
	entries[0].CurrentPolicyDigest = domainsecurity.SHA256Hex([]byte("policy-" + label))
	return fixture.newIndex(t, domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID:      fixture.installationID,
		EnrollmentID:        fixture.enrollmentID,
		Namespace:           domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:          previous.Generation + 1,
		Entries:             entries,
		PreviousIndexDigest: previous.IndexDigest,
		MutationID:          domainsecurity.SHA256Hex([]byte("mutation-" + label)),
		AuthorityKeyID:      domainsecurity.SHA256Hex(fixture.authorityPublic),
		AuthorityPublicKey:  fixture.authorityPublic,
	})
}

func (fixture authorityStoreFixture) newIndex(t *testing.T, input domainsecurity.ThreadRiskAuthorityIndexInputV1) domainsecurity.ThreadRiskAuthorityIndexV1 {
	t.Helper()
	index, err := domainsecurity.NewThreadRiskAuthorityIndexV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func (fixture authorityStoreFixture) bundle(t *testing.T, index domainsecurity.ThreadRiskAuthorityIndexV1, label string) storeport.ObservationBundle {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID:     fixture.installationID,
		EnrollmentID:       fixture.enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ChallengeNonce:     domainsecurity.SHA256Hex([]byte("challenge-" + label)),
		AuthorityKeyID:     domainsecurity.SHA256Hex(fixture.authorityPublic),
		AuthorityPublicKey: fixture.authorityPublic,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           fixture.installationID,
		EnrollmentID:             fixture.enrollmentID,
		Namespace:                domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:               index.Generation,
		CurrentStateDigest:       index.IndexDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("previous-state-" + label)),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("previous-checkpoint-" + label)),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("fence-" + label)),
		MutationID:               index.MutationID,
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
	return storeport.ObservationBundle{Index: index, Request: request, Observation: observation}
}
