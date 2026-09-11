package evidenceauthority

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
)

func TestProjectionUsesCompleteCanonicalBodyDigestNotBundleRecordDigest(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(81)
	first := fixture.firstBundle(t, "projection-expected")
	second := fixture.nextBundle(t, first, "dataset", "projection-expected-2")
	firstBody := canonicalEvidenceBundleBody(t, first)
	secondBody := canonicalEvidenceBundleBody(t, second)
	firstBodyDigest := domainsecurity.SHA256Hex(firstBody)
	if firstBodyDigest == first.RecordDigest {
		t.Fatal("fixture conflates the projection's full canonical body SHA with the bundle RecordDigest")
	}
	store := &recordingEvidenceProjectionStore{observation: exactEvidenceProjectionObservation(firstBody)}
	projection := &Projection{store: store}
	if err := projection.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if store.replaceCalls != 1 || store.lastExpected != firstBodyDigest || !bytes.Equal(store.lastNext, secondBody) {
		t.Fatalf("projection did not use exact Observe bytes: calls=%d expected=%q next=%q", store.replaceCalls, store.lastExpected, store.lastNext)
	}
	if store.lastExpected == first.RecordDigest {
		t.Fatal("projection used the domain-separated RecordDigest as the mutable-store byte digest")
	}
	if store.reconcileCalls != 0 {
		t.Fatal("ordinary exact replacement used reconciliation")
	}

	absent := &recordingEvidenceProjectionStore{}
	projection = &Projection{store: absent}
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if absent.lastExpected != "" || absent.replaceCalls != 1 {
		t.Fatalf("absent projection did not use explicit empty expected value: %#v", absent)
	}
}

func TestProjectionValidatesWitnessEmptyWithoutErasingCommittedState(t *testing.T) {
	empty := &recordingEvidenceProjectionStore{}
	if err := (&Projection{store: empty}).ValidateWitnessEmpty(context.Background()); err != nil {
		t.Fatalf("exact empty projection was rejected: %v", err)
	}
	if empty.replaceCalls != 0 || empty.reconcileCalls != 0 {
		t.Fatal("exact empty validation mutated the projection")
	}

	fixture := newEvidenceAuthorityStoreFixture(91)
	body := canonicalEvidenceBundleBody(t, fixture.firstBundle(t, "empty-conflict"))
	committed := &recordingEvidenceProjectionStore{observation: exactEvidenceProjectionObservation(body)}
	if err := (&Projection{store: committed}).ValidateWitnessEmpty(context.Background()); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("witness empty accepted a committed projection: %v", err)
	}
	if committed.replaceCalls != 0 || committed.reconcileCalls != 0 {
		t.Fatal("witness empty conflict erased committed state")
	}

	residue := &recordingEvidenceProjectionStore{observeErr: &authorityprojectionport.ResidueError{
		Residues: []authorityprojectionport.Residue{{Name: "uncommitted", Digest: domainsecurity.SHA256Hex(body), Body: body}},
	}}
	if err := (&Projection{store: residue}).ValidateWitnessEmpty(context.Background()); err != nil {
		t.Fatalf("witness empty did not reconcile an absent target with uncommitted residue: %v", err)
	}
	if residue.reconcileCalls != 1 || residue.lastReconcile != "" {
		t.Fatalf("witness empty residue reconciliation was not exact: %#v", residue)
	}
}

func TestProjectionAcceptsDirectTransitionAndProvenGenerationSkip(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(82)
	first := fixture.firstBundle(t, "projection-skip")
	second := fixture.nextBundle(t, first, "dataset", "projection-skip-2")
	third := fixture.nextBundle(t, second, "registry", "projection-skip-3")

	direct := &recordingEvidenceProjectionStore{observation: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, first))}
	if err := (&Projection{store: direct}).ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatalf("direct valid transition failed: %v", err)
	}

	// A different process may advance the witness without projecting the
	// intermediate generation locally, but the immutable CAS must prove that
	// the previously projected bundle is an ancestor of the selected bundle.
	skipped := &recordingEvidenceProjectionStore{observation: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, first))}
	bundles := &recordingEvidenceBundleStore{records: map[string]domainevidence.EvidenceAuthorityBundleV1{
		first.RecordDigest: first, second.RecordDigest: second, third.RecordDigest: third,
	}}
	if err := (&Projection{store: skipped, bundles: bundles}).ProjectWitnessSelected(context.Background(), third); err != nil {
		t.Fatalf("exact witness-selected generation skip failed: %v", err)
	}
	if skipped.replaceCalls != 1 || skipped.reconcileCalls != 0 {
		t.Fatalf("generation skip did not use one exact replacement: %#v", skipped)
	}
}

func TestProjectionRejectsUnprovenAndCrossGenerationForks(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(87)
	first := fixture.firstBundle(t, "projection-ancestry")
	leftSecond := fixture.nextBundle(t, first, "dataset", "projection-left-2")
	rightSecond := fixture.nextBundle(t, first, "registry", "projection-right-2")
	rightThird := fixture.nextBundle(t, rightSecond, "publication", "projection-right-3")
	rightFourth := fixture.nextBundle(t, rightThird, "dataset", "projection-right-4")

	for name, projection := range map[string]*Projection{
		"missing ancestry store": {
			store: &recordingEvidenceProjectionStore{observation: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, first))},
		},
		"higher generation fork": {
			store: &recordingEvidenceProjectionStore{observation: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, leftSecond))},
			bundles: &recordingEvidenceBundleStore{records: map[string]domainevidence.EvidenceAuthorityBundleV1{
				first.RecordDigest: first, rightSecond.RecordDigest: rightSecond,
				rightThird.RecordDigest: rightThird, rightFourth.RecordDigest: rightFourth,
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := projection.store.(*recordingEvidenceProjectionStore)
			selected := rightThird
			if name == "higher generation fork" {
				selected = rightFourth
			}
			if err := projection.ProjectWitnessSelected(context.Background(), selected); !errors.Is(err, ErrProjectionConflict) {
				t.Fatalf("cross-generation authority gap did not fail closed: %v", err)
			}
			if store.replaceCalls != 0 || store.reconcileCalls != 0 {
				t.Fatal("unproven generation advance mutated the projection")
			}
		})
	}
}

func TestProjectionReconcilesOnlyExactWitnessSelectedBytes(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(83)
	bundle := fixture.firstBundle(t, "projection-reconcile")
	body := canonicalEvidenceBundleBody(t, bundle)
	digest := domainsecurity.SHA256Hex(body)
	if digest == bundle.RecordDigest {
		t.Fatal("fixture conflates projection bytes digest with RecordDigest")
	}

	t.Run("residue", func(t *testing.T) {
		store := &recordingEvidenceProjectionStore{
			observeErr: &authorityprojectionport.ResidueError{
				Residues: []authorityprojectionport.Residue{{Name: "residue", Digest: digest, Body: body}},
			},
			reconcileBody: body,
		}
		projection := &Projection{store: store}
		if err := projection.ProjectWitnessSelected(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		if store.reconcileCalls != 1 || store.lastReconcile != digest || store.lastReconcile == bundle.RecordDigest || store.replaceCalls != 0 {
			t.Fatalf("residue was not reconciled solely to the selected canonical body SHA: %#v", store)
		}
	})

	t.Run("replace-indeterminate", func(t *testing.T) {
		store := &recordingEvidenceProjectionStore{
			replaceState:  authorityprojectionport.Indeterminate,
			replaceErr:    authorityprojectionport.ErrCommitIndeterminate,
			reconcileBody: body,
		}
		projection := &Projection{store: store}
		if err := projection.ProjectWitnessSelected(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		if store.reconcileCalls != 1 || store.lastReconcile != digest {
			t.Fatalf("indeterminate replacement did not exact-reconcile selected bytes: %#v", store)
		}
	})

	t.Run("unresolved-residue", func(t *testing.T) {
		store := &recordingEvidenceProjectionStore{
			observeErr: &authorityprojectionport.ResidueError{
				Residues: []authorityprojectionport.Residue{{
					Name: "attacker", Digest: domainsecurity.SHA256Hex([]byte("attacker")), Body: []byte("attacker"),
				}},
			},
			reconcileState: authorityprojectionport.Indeterminate,
			reconcileErr:   authorityprojectionport.ErrCommitIndeterminate,
		}
		projection := &Projection{store: store}
		err := projection.ProjectWitnessSelected(context.Background(), bundle)
		if !errors.Is(err, ErrProjectionIndeterminate) || store.lastReconcile != digest {
			t.Fatalf("unresolved residue did not fail closed: err=%v store=%#v", err, store)
		}
	})

	t.Run("committed-with-error", func(t *testing.T) {
		postCommit := errors.New("post-commit failure")
		store := &recordingEvidenceProjectionStore{replaceErr: postCommit}
		projection := &Projection{store: store}
		err := projection.ProjectWitnessSelected(context.Background(), bundle)
		if !errors.Is(err, ErrProjectionIndeterminate) || !errors.Is(err, postCommit) {
			t.Fatalf("committed projection error was hidden: %v", err)
		}
	})

	t.Run("reconciled-with-error", func(t *testing.T) {
		postReconcile := errors.New("post-reconcile failure")
		store := &recordingEvidenceProjectionStore{
			observeErr: &authorityprojectionport.ResidueError{
				Residues: []authorityprojectionport.Residue{{Name: "residue", Digest: digest, Body: body}},
			},
			reconcileBody: body,
			reconcileErr:  postReconcile,
		}
		projection := &Projection{store: store}
		err := projection.ProjectWitnessSelected(context.Background(), bundle)
		if !errors.Is(err, ErrProjectionIndeterminate) || !errors.Is(err, postReconcile) {
			t.Fatalf("reconciliation error was hidden: %v", err)
		}
	})
}

func TestProjectionCompareFailureAcceptsOnlyExactSelectedWinner(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(84)
	first := fixture.firstBundle(t, "projection-race")
	second := fixture.nextBundle(t, first, "dataset", "projection-race-2")
	body := canonicalEvidenceBundleBody(t, second)
	store := &recordingEvidenceProjectionStore{
		observation:  exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, first)),
		replaceState: authorityprojectionport.NotCommitted,
		replaceErr:   authorityprojectionport.ErrCompareFailed,
		tracedBody:   body,
	}
	if err := (&Projection{store: store}).ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatalf("exact concurrently selected winner was not accepted: %v", err)
	}

	fork := fixture.nextBundle(t, first, "registry", "projection-race-fork")
	forkStore := &recordingEvidenceProjectionStore{
		observation:  exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, first)),
		replaceState: authorityprojectionport.NotCommitted,
		replaceErr:   authorityprojectionport.ErrCompareFailed,
		tracedBody:   body,
	}
	if err := (&Projection{store: forkStore}).ProjectWitnessSelected(context.Background(), fork); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("different concurrent winner did not fail closed: %v", err)
	}
}

func TestProjectionRejectsRollbackSameGenerationForkAndForeignAuthorityWithoutMutation(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(85)
	first := fixture.firstBundle(t, "projection-conflict")
	second := fixture.nextBundle(t, first, "dataset", "projection-conflict-2")
	fork := fixture.nextBundle(t, first, "registry", "projection-conflict-fork")
	foreignFixture := newEvidenceAuthorityStoreFixture(86)
	foreignFirst := foreignFixture.firstBundle(t, "projection-foreign")
	foreignSecond := foreignFixture.nextBundle(t, foreignFirst, "dataset", "projection-foreign-2")
	foreignThird := foreignFixture.nextBundle(t, foreignSecond, "registry", "projection-foreign-3")

	tests := []struct {
		name     string
		observed authorityprojectionport.Observation
		next     domainevidence.EvidenceAuthorityBundleV1
	}{
		{name: "rollback", observed: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, second)), next: first},
		{name: "same-generation-fork", observed: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, second)), next: fork},
		{name: "foreign-authority", observed: exactEvidenceProjectionObservation(canonicalEvidenceBundleBody(t, first)), next: foreignThird},
		{
			name: "attacker-bytes",
			observed: authorityprojectionport.Observation{
				Present: true, Digest: domainsecurity.SHA256Hex([]byte("attacker")), Body: []byte("attacker"),
			},
			next: second,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &recordingEvidenceProjectionStore{observation: test.observed}
			err := (&Projection{store: store}).ProjectWitnessSelected(context.Background(), test.next)
			if !errors.Is(err, ErrProjectionConflict) {
				t.Fatalf("projection conflict returned %v", err)
			}
			if store.replaceCalls != 0 || store.reconcileCalls != 0 {
				t.Fatal("conflicting local projection was mutated")
			}
		})
	}
}

type recordingEvidenceProjectionStore struct {
	mu sync.Mutex

	observation authorityprojectionport.Observation
	observeErr  error

	replaceState authorityprojectionport.CommitState
	replaceErr   error
	replaceCalls int
	lastExpected string
	lastNext     []byte
	tracedBody   []byte

	reconcileState        authorityprojectionport.CommitState
	reconcileErr          error
	reconcileCalls        int
	lastReconcileExpected string
	lastReconcile         string
	reconcileBody         []byte
}

type recordingEvidenceBundleStore struct {
	records map[string]domainevidence.EvidenceAuthorityBundleV1
}

func (store *recordingEvidenceBundleStore) PutIfAbsent(_ context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	if store.records == nil {
		store.records = map[string]domainevidence.EvidenceAuthorityBundleV1{}
	}
	if current, ok := store.records[bundle.RecordDigest]; ok && !equalBundle(current, bundle) {
		return errors.New("conflicting bundle")
	}
	store.records[bundle.RecordDigest] = bundle
	return nil
}

func (store *recordingEvidenceBundleStore) Resolve(_ context.Context, digest string) (domainevidence.EvidenceAuthorityBundleV1, error) {
	bundle, ok := store.records[digest]
	if !ok {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("bundle not found")
	}
	return bundle, nil
}

func (store *recordingEvidenceProjectionStore) Observe(context.Context) (authorityprojectionport.Observation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return cloneEvidenceProjectionObservation(store.observation), store.observeErr
}

func (store *recordingEvidenceProjectionStore) ReplaceExact(
	_ context.Context,
	expected string,
	next []byte,
) (authorityprojectionport.ReplaceResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.replaceCalls++
	store.lastExpected = expected
	store.lastNext = bytes.Clone(next)
	state := store.replaceState
	if state == "" {
		state = authorityprojectionport.Committed
	}
	if len(store.tracedBody) > 0 {
		store.observation = exactEvidenceProjectionObservation(store.tracedBody)
	} else if state == authorityprojectionport.Committed {
		store.observation = exactEvidenceProjectionObservation(next)
	} else if state == authorityprojectionport.Indeterminate && len(store.reconcileBody) > 0 {
		store.observation = exactEvidenceProjectionObservation(store.reconcileBody)
		store.observeErr = &authorityprojectionport.ResidueError{
			Target:   store.observation,
			Residues: []authorityprojectionport.Residue{{Name: "predecessor", Digest: expected}},
		}
	}
	return authorityprojectionport.ReplaceResult{State: state, Digest: domainsecurity.SHA256Hex(next)}, store.replaceErr
}

func (store *recordingEvidenceProjectionStore) ReconcileExact(
	_ context.Context,
	expectedTarget string,
	digest string,
) (authorityprojectionport.ReplaceResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.reconcileCalls++
	store.lastReconcileExpected = expectedTarget
	store.lastReconcile = digest
	if store.observation.Digest != expectedTarget || store.observation.Present != (expectedTarget != "") {
		return authorityprojectionport.ReplaceResult{State: authorityprojectionport.NotCommitted}, authorityprojectionport.ErrCompareFailed
	}
	state := store.reconcileState
	if state == "" {
		state = authorityprojectionport.Committed
	}
	if state == authorityprojectionport.Committed {
		if digest == "" {
			store.observation = authorityprojectionport.Observation{}
			store.observeErr = nil
		} else if len(store.reconcileBody) > 0 {
			store.observation = exactEvidenceProjectionObservation(store.reconcileBody)
			store.observeErr = nil
		}
	}
	return authorityprojectionport.ReplaceResult{State: state, Digest: digest}, store.reconcileErr
}

func exactEvidenceProjectionObservation(body []byte) authorityprojectionport.Observation {
	return authorityprojectionport.Observation{
		Present: true,
		Digest:  domainsecurity.SHA256Hex(body),
		Body:    bytes.Clone(body),
	}
}

func cloneEvidenceProjectionObservation(observation authorityprojectionport.Observation) authorityprojectionport.Observation {
	observation.Body = bytes.Clone(observation.Body)
	return observation
}
