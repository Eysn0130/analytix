package threadriskauthority

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
)

func TestProjectionUsesOnlyObservedExpectedDigest(t *testing.T) {
	fixture := newAuthorityStoreFixture(81)
	first := fixture.firstIndex(t, "projection-expected")
	second := fixture.nextIndex(t, first, "projection-expected-2")
	firstBody := canonicalIndexBody(t, first)
	secondBody := canonicalIndexBody(t, second)
	store := &recordingProjectionStore{
		observation: authorityprojectionport.Observation{
			Present: true,
			Digest:  domainsecurity.SHA256Hex(firstBody),
			Body:    firstBody,
		},
	}
	projection := &Projection{store: store}
	if err := projection.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if store.replaceCalls != 1 || store.lastExpected != domainsecurity.SHA256Hex(firstBody) || !bytes.Equal(store.lastNext, secondBody) {
		t.Fatalf("projection did not use exact Observe result: calls=%d expected=%q next=%q", store.replaceCalls, store.lastExpected, store.lastNext)
	}
	if store.reconcileCalls != 0 {
		t.Fatal("ordinary exact replacement used reconciliation")
	}

	absent := &recordingProjectionStore{}
	projection = &Projection{store: absent}
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if absent.lastExpected != "" || absent.replaceCalls != 1 {
		t.Fatalf("absent projection did not use explicit empty expected value: %#v", absent)
	}
}

func TestProjectionValidatesWitnessEmptyWithoutErasingCommittedState(t *testing.T) {
	empty := &recordingProjectionStore{}
	if err := (&Projection{store: empty}).ValidateWitnessEmpty(context.Background()); err != nil {
		t.Fatalf("exact empty projection was rejected: %v", err)
	}
	if empty.replaceCalls != 0 || empty.reconcileCalls != 0 {
		t.Fatal("exact empty validation mutated the projection")
	}

	fixture := newAuthorityStoreFixture(91)
	body := canonicalIndexBody(t, fixture.firstIndex(t, "empty-conflict"))
	committed := &recordingProjectionStore{observation: exactProjectionObservation(body)}
	if err := (&Projection{store: committed}).ValidateWitnessEmpty(context.Background()); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("witness empty accepted a committed projection: %v", err)
	}
	if committed.replaceCalls != 0 || committed.reconcileCalls != 0 {
		t.Fatal("witness empty conflict erased committed state")
	}

	residue := &recordingProjectionStore{observeErr: &authorityprojectionport.ResidueError{
		Residues: []authorityprojectionport.Residue{{Name: "uncommitted", Digest: domainsecurity.SHA256Hex(body), Body: body}},
	}}
	if err := (&Projection{store: residue}).ValidateWitnessEmpty(context.Background()); err != nil {
		t.Fatalf("witness empty did not reconcile an absent target with uncommitted residue: %v", err)
	}
	if residue.reconcileCalls != 1 || residue.lastReconcile != "" {
		t.Fatalf("witness empty residue reconciliation was not exact: %#v", residue)
	}
}

func TestProjectionReconcilesOnlyExactWitnessSelectedBytes(t *testing.T) {
	fixture := newAuthorityStoreFixture(82)
	index := fixture.firstIndex(t, "projection-reconcile")
	body := canonicalIndexBody(t, index)
	digest := domainsecurity.SHA256Hex(body)

	t.Run("residue", func(t *testing.T) {
		store := &recordingProjectionStore{
			observation: authorityprojectionport.Observation{},
			observeErr: &authorityprojectionport.ResidueError{
				Residues: []authorityprojectionport.Residue{{Name: "residue", Digest: digest, Body: body}},
			},
			reconcileBody: body,
		}
		projection := &Projection{store: store}
		if err := projection.ProjectWitnessSelected(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		if store.reconcileCalls != 1 || store.lastReconcile != digest || store.replaceCalls != 0 {
			t.Fatalf("residue was not reconciled solely to selected bytes: %#v", store)
		}
	})

	t.Run("replace-indeterminate", func(t *testing.T) {
		store := &recordingProjectionStore{
			replaceState:  authorityprojectionport.Indeterminate,
			replaceErr:    authorityprojectionport.ErrCommitIndeterminate,
			reconcileBody: body,
		}
		projection := &Projection{store: store}
		if err := projection.ProjectWitnessSelected(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		if store.reconcileCalls != 1 || store.lastReconcile != digest {
			t.Fatalf("indeterminate replacement did not exact-reconcile selected bytes: %#v", store)
		}
	})

	t.Run("unresolved", func(t *testing.T) {
		store := &recordingProjectionStore{
			observeErr: &authorityprojectionport.ResidueError{
				Residues: []authorityprojectionport.Residue{{Name: "attacker", Digest: domainsecurity.SHA256Hex([]byte("attacker")), Body: []byte("attacker")}},
			},
			reconcileState: authorityprojectionport.Indeterminate,
			reconcileErr:   authorityprojectionport.ErrCommitIndeterminate,
		}
		projection := &Projection{store: store}
		err := projection.ProjectWitnessSelected(context.Background(), index)
		if !errors.Is(err, ErrProjectionIndeterminate) || store.lastReconcile != digest {
			t.Fatalf("unresolved residue did not fail closed: err=%v store=%#v", err, store)
		}
	})

	t.Run("committed-with-error", func(t *testing.T) {
		postCommit := errors.New("post-commit failure")
		store := &recordingProjectionStore{replaceErr: postCommit}
		projection := &Projection{store: store}
		err := projection.ProjectWitnessSelected(context.Background(), index)
		if !errors.Is(err, ErrProjectionIndeterminate) || !errors.Is(err, postCommit) {
			t.Fatalf("committed projection error was hidden: %v", err)
		}
	})

	t.Run("reconciled-with-error", func(t *testing.T) {
		postReconcile := errors.New("post-reconcile failure")
		store := &recordingProjectionStore{
			observeErr: &authorityprojectionport.ResidueError{
				Residues: []authorityprojectionport.Residue{{Name: "residue", Digest: digest, Body: body}},
			},
			reconcileBody: body,
			reconcileErr:  postReconcile,
		}
		projection := &Projection{store: store}
		err := projection.ProjectWitnessSelected(context.Background(), index)
		if !errors.Is(err, ErrProjectionIndeterminate) || !errors.Is(err, postReconcile) {
			t.Fatalf("reconciliation error was hidden: %v", err)
		}
	})
}

func TestProjectionRejectsAttackerRollbackAndForkBytesWithoutMutation(t *testing.T) {
	fixture := newAuthorityStoreFixture(83)
	first := fixture.firstIndex(t, "projection-conflict")
	second := fixture.nextIndex(t, first, "projection-conflict-2")
	third := fixture.nextIndex(t, second, "projection-conflict-3")

	tests := []struct {
		name     string
		observed authorityprojectionport.Observation
		next     domainsecurity.ThreadRiskAuthorityIndexV1
	}{
		{
			name: "attacker-bytes",
			observed: authorityprojectionport.Observation{
				Present: true, Digest: domainsecurity.SHA256Hex([]byte("attacker")), Body: []byte("attacker"),
			},
			next: second,
		},
		{
			name: "skipped-lineage",
			observed: authorityprojectionport.Observation{
				Present: true, Digest: domainsecurity.SHA256Hex(canonicalIndexBody(t, first)), Body: canonicalIndexBody(t, first),
			},
			next: third,
		},
		{
			name: "same-generation-fork",
			observed: authorityprojectionport.Observation{
				Present: true, Digest: domainsecurity.SHA256Hex(canonicalIndexBody(t, second)), Body: canonicalIndexBody(t, second),
			},
			next: fixture.nextIndex(t, first, "projection-conflict-fork"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &recordingProjectionStore{observation: test.observed}
			projection := &Projection{store: store}
			if err := projection.ProjectWitnessSelected(context.Background(), test.next); !errors.Is(err, ErrProjectionConflict) {
				t.Fatalf("projection conflict returned %v", err)
			}
			if store.replaceCalls != 0 || store.reconcileCalls != 0 {
				t.Fatal("conflicting local projection was mutated")
			}
		})
	}
}

func TestProjectionCompareRaceAcceptsOnlyExactSelectedWinner(t *testing.T) {
	fixture := newAuthorityStoreFixture(84)
	first := fixture.firstIndex(t, "projection-race")
	second := fixture.nextIndex(t, first, "projection-race-2")
	body := canonicalIndexBody(t, second)
	store := &recordingProjectionStore{
		observation: authorityprojectionport.Observation{
			Present: true,
			Digest:  domainsecurity.SHA256Hex(canonicalIndexBody(t, first)),
			Body:    canonicalIndexBody(t, first),
		},
		replaceState: authorityprojectionport.NotCommitted,
		replaceErr:   authorityprojectionport.ErrCompareFailed,
		tracedBody:   body,
	}
	projection := &Projection{store: store}
	if err := projection.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatalf("exact concurrently selected winner was not accepted: %v", err)
	}

	fork := fixture.nextIndex(t, first, "projection-race-fork")
	forkStore := &recordingProjectionStore{
		observation: authorityprojectionport.Observation{
			Present: true,
			Digest:  domainsecurity.SHA256Hex(canonicalIndexBody(t, first)),
			Body:    canonicalIndexBody(t, first),
		},
		replaceState: authorityprojectionport.NotCommitted,
		replaceErr:   authorityprojectionport.ErrCompareFailed,
		tracedBody:   body,
	}
	if err := (&Projection{store: forkStore}).ProjectWitnessSelected(context.Background(), fork); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("different concurrent winner did not fail closed: %v", err)
	}
}

type recordingProjectionStore struct {
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

func (store *recordingProjectionStore) Observe(context.Context) (authorityprojectionport.Observation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return cloneProjectionObservation(store.observation), store.observeErr
}

func (store *recordingProjectionStore) ReplaceExact(_ context.Context, expected string, next []byte) (authorityprojectionport.ReplaceResult, error) {
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
		store.observation = exactProjectionObservation(store.tracedBody)
	} else if state == authorityprojectionport.Committed {
		store.observation = exactProjectionObservation(next)
	} else if state == authorityprojectionport.Indeterminate && len(store.reconcileBody) > 0 {
		store.observation = exactProjectionObservation(store.reconcileBody)
		store.observeErr = &authorityprojectionport.ResidueError{
			Target:   store.observation,
			Residues: []authorityprojectionport.Residue{{Name: "predecessor", Digest: expected}},
		}
	}
	return authorityprojectionport.ReplaceResult{State: state, Digest: domainsecurity.SHA256Hex(next)}, store.replaceErr
}

func (store *recordingProjectionStore) ReconcileExact(_ context.Context, expectedTarget, digest string) (authorityprojectionport.ReplaceResult, error) {
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
			store.observation = exactProjectionObservation(store.reconcileBody)
			store.observeErr = nil
		}
	}
	return authorityprojectionport.ReplaceResult{State: state, Digest: digest}, store.reconcileErr
}

func exactProjectionObservation(body []byte) authorityprojectionport.Observation {
	return authorityprojectionport.Observation{Present: true, Digest: domainsecurity.SHA256Hex(body), Body: bytes.Clone(body)}
}

func cloneProjectionObservation(observation authorityprojectionport.Observation) authorityprojectionport.Observation {
	observation.Body = bytes.Clone(observation.Body)
	return observation
}
