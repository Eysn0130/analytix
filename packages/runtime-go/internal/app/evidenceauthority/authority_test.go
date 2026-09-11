package evidenceauthority

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

func TestGenerationZeroAndExplicitCanonicalInitialize(t *testing.T) {
	fixture := newAuthorityFixture(t)
	empty, err := fixture.authority.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if empty.HasBundle || empty.Observation.Checkpoint.Generation != 0 {
		t.Fatalf("generation zero invented a bundle: %#v", empty)
	}

	first, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasBundle || first.Bundle.Generation != 1 || first.Bundle.PreviousBundleDigest != "" ||
		first.Bundle.DatasetSnapshotIndexDigest != fixture.genesis.DatasetSnapshotIndexDigest || first.Bundle.DatasetSnapshotCount != 0 ||
		first.Bundle.EvidenceRegistryIndexDigest != fixture.genesis.EvidenceRegistryIndexDigest || first.Bundle.EvidenceRegistryCount != 0 ||
		first.Bundle.PublicationIndexDigest != fixture.genesis.PublicationIndexDigest || first.Bundle.PublicationCount != 0 ||
		first.Bundle.RecordDigest != first.Observation.Checkpoint.CurrentStateDigest {
		t.Fatalf("canonical generation one is incomplete: %#v", first)
	}
	if fixture.witness.advanceCalls.Load() != 1 {
		t.Fatalf("initialize dispatched %d advances", fixture.witness.advanceCalls.Load())
	}
	if _, err := fixture.authority.Initialize(context.Background()); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second initialize was not rejected: %v", err)
	}
	if fixture.witness.advanceCalls.Load() != 1 {
		t.Fatal("second initialize dispatched another Advance")
	}

	current, err := fixture.authority.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Request.ChallengeNonce == first.Request.ChallengeNonce || !equalBundleRecord(current.Bundle, first.Bundle) {
		t.Fatal("Current did not use a fresh challenge for the exact witnessed bundle")
	}
}

func TestBoundFreshHeadChallengeSkipsDurableResolutionPersistenceAndProjection(t *testing.T) {
	fixture := newAuthorityFixture(t)
	if _, err := fixture.authority.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var retained func(context.Context) error
	err := fixture.authority.WithFreshHeadChallenge(
		context.Background(),
		func(head storeport.FreshHead, challenge func(context.Context) error) error {
			if !head.HasBundle || head.Bundle.RecordDigest == "" || challenge == nil {
				t.Fatal("bound challenge did not receive the fully admitted exact head")
			}
			if fixture.bundles.resolveCalls.Load() == 0 || fixture.observations.putCalls.Load() == 0 ||
				fixture.observations.resolveCalls.Load() == 0 || fixture.projection.projectCalls.Load() == 0 {
				t.Fatal("outer full observation did not traverse, persist, and project authority state")
			}
			fixture.bundles.resolveCalls.Store(0)
			fixture.observations.putCalls.Store(0)
			fixture.observations.resolveCalls.Store(0)
			fixture.projection.projectCalls.Store(0)
			fixture.witness.observeCalls.Store(0)
			fixture.checkpointFloor.projectCalls.Store(0)
			retained = challenge
			if err := challenge(context.Background()); err != nil {
				return err
			}
			if err := challenge(context.Background()); err != nil {
				return err
			}
			if fixture.witness.observeCalls.Load() != 2 || fixture.checkpointFloor.projectCalls.Load() != 2 {
				t.Fatalf(
					"inner challenge skipped fresh witness/floor validation: witness=%d floor=%d",
					fixture.witness.observeCalls.Load(), fixture.checkpointFloor.projectCalls.Load(),
				)
			}
			if fixture.bundles.resolveCalls.Load() != 0 || fixture.observations.putCalls.Load() != 0 ||
				fixture.observations.resolveCalls.Load() != 0 || fixture.projection.projectCalls.Load() != 0 {
				t.Fatalf(
					"inner challenge touched durable inventory: bundle_resolve=%d observation_put=%d observation_resolve=%d projection=%d",
					fixture.bundles.resolveCalls.Load(), fixture.observations.putCalls.Load(),
					fixture.observations.resolveCalls.Load(), fixture.projection.projectCalls.Load(),
				)
			}
			nonces := fixture.witness.observedNonces()
			if len(nonces) < 2 || nonces[len(nonces)-1] == nonces[len(nonces)-2] {
				t.Fatal("inner challenge did not issue distinct fresh nonces")
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			fixture.witness.blockNextObservation(entered, release)
			firstDone := make(chan error, 1)
			go func() { firstDone <- challenge(context.Background()) }()
			<-entered
			if concurrentErr := challenge(context.Background()); !errors.Is(concurrentErr, ErrUnavailable) {
				close(release)
				return errors.Join(errors.New("concurrent challenge alias was accepted"), concurrentErr)
			}
			close(release)
			if firstErr := <-firstDone; firstErr != nil {
				return firstErr
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if retained == nil || !errors.Is(retained(context.Background()), ErrUnavailable) {
		t.Fatal("fresh-head challenge remained usable after its issuing callback")
	}
}

func TestBoundFreshHeadChallengeRejectsDriftFloorReplaySignatureAndCancellation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*authorityFixture, context.CancelFunc)
		want   error
	}{
		{
			name: "checkpoint floor conflict",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.checkpointFloor.failure = monotonicheadport.ErrCheckpointFloorConflict
			},
			want: ErrIntegrity,
		},
		{
			name: "replayed witness observation",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				replayed := fixture.witness.lastObservation
				fixture.witness.replayObservation = &replayed
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name: "forged witness signature",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.forgeObservationSignature = true
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name: "wrong installation",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.mutateObservation = func(observation *domainsecurity.MonotonicHeadObservationV1) {
					observation.Checkpoint.InstallationID = digest("wrong-installation")
				}
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name: "wrong enrollment",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.mutateObservation = func(observation *domainsecurity.MonotonicHeadObservationV1) {
					observation.Checkpoint.EnrollmentID = digest("wrong-enrollment")
				}
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name: "wrong authority key",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.mutateRequest = func(request *domainsecurity.MonotonicHeadObserveRequestV1) {
					request.AuthorityKeyID = digest("wrong-authority-key")
				}
				fixture.witness.mu.Unlock()
			},
			want: ErrUnavailable,
		},
		{
			name: "wrong witness key",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.mutateObservation = func(observation *domainsecurity.MonotonicHeadObservationV1) {
					observation.Checkpoint.WitnessKeyID = digest("wrong-witness-key")
				}
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name: "wrong namespace",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.mutateObservation = func(observation *domainsecurity.MonotonicHeadObservationV1) {
					observation.Checkpoint.Namespace = "wrong-namespace"
				}
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name: "wrong state digest",
			mutate: func(fixture *authorityFixture, _ context.CancelFunc) {
				fixture.witness.mu.Lock()
				fixture.witness.mutateObservation = func(observation *domainsecurity.MonotonicHeadObservationV1) {
					observation.Checkpoint.CurrentStateDigest = digest("wrong-state")
				}
				fixture.witness.mu.Unlock()
			},
			want: ErrIntegrity,
		},
		{
			name:   "cancelled challenge",
			mutate: func(_ *authorityFixture, cancel context.CancelFunc) { cancel() },
			want:   context.Canceled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityFixture(t)
			if _, err := fixture.authority.Initialize(context.Background()); err != nil {
				t.Fatal(err)
			}
			err := fixture.authority.WithFreshHeadChallenge(
				context.Background(),
				func(_ storeport.FreshHead, challenge func(context.Context) error) error {
					challengeContext, cancel := context.WithCancel(context.Background())
					defer cancel()
					test.mutate(fixture, cancel)
					return challenge(challengeContext)
				},
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("bound fresh-head failure was misclassified: %v", err)
			}
			if test.name == "checkpoint floor conflict" && !errors.Is(err, monotonicheadport.ErrEquivocation) {
				t.Fatalf("checkpoint floor conflict lost equivocation classification: %v", err)
			}
		})
	}

	t.Run("exact bundle drift", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		if _, err := fixture.authority.Initialize(context.Background()); err != nil {
			t.Fatal(err)
		}
		err := fixture.authority.WithFreshHeadChallenge(
			context.Background(),
			func(head storeport.FreshHead, challenge func(context.Context) error) error {
				if _, err := fixture.authority.AdvanceEvidenceRegistry(context.Background(), storeport.RegistryAdvanceInput{
					ExpectedBundleDigest: head.Bundle.RecordDigest,
					NextIndexDigest:      digest("bound-challenge-drift"),
				}); err != nil {
					return err
				}
				return challenge(context.Background())
			},
		)
		if !errors.Is(err, ErrCurrentChanged) {
			t.Fatalf("cross-bundle challenge remained current: %v", err)
		}
	})
}

func TestGenerationZeroAfterRestartRejectsExistingProjection(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.witness.mu.Lock()
	emptyCheckpoint := fixture.witness.checkpoint
	fixture.witness.mu.Unlock()

	committed, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.witness.mu.Lock()
	fixture.witness.checkpoint = emptyCheckpoint
	fixture.witness.mu.Unlock()

	restarted, err := New(fixture.config(91))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Current(context.Background()); !errors.Is(err, ErrIntegrity) ||
		!errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("fresh generation zero replay accepted a non-empty durable projection: %v", err)
	}
	if fixture.projection.current().RecordDigest != committed.Bundle.RecordDigest {
		t.Fatal("generation zero replay mutated the existing projection")
	}
}

func TestSameGenerationCheckpointEquivocationAfterRestartIsRejected(t *testing.T) {
	fixture := newAuthorityFixture(t)
	committed, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := committed.Observation.Checkpoint
	fork, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: checkpoint.InstallationID, EnrollmentID: checkpoint.EnrollmentID, Namespace: checkpoint.Namespace,
		Generation: checkpoint.Generation, CurrentStateDigest: checkpoint.CurrentStateDigest,
		PreviousStateDigest: checkpoint.PreviousStateDigest, PreviousCheckpointDigest: checkpoint.PreviousCheckpointDigest,
		FenceNonce: digest("restart-fork-fence"), MutationID: checkpoint.MutationID,
		WitnessKeyID: checkpoint.WitnessKeyID, WitnessPublicKey: fixture.witnessPrivate.Public().(ed25519.PublicKey),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	fixture.witness.mu.Lock()
	fixture.witness.checkpoint = fork
	fixture.witness.mu.Unlock()
	restarted, err := New(fixture.config(92))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Current(context.Background()); !errors.Is(err, ErrIntegrity) ||
		!errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) || !errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("same-generation checkpoint fork survived restart: %v", err)
	}
}

func TestCheckpointFloorFailureClassification(t *testing.T) {
	tests := []struct {
		name             string
		failure          error
		wantUnavailable  bool
		wantIntegrity    bool
		wantEquivocation bool
	}{
		{name: "unavailable", failure: monotonicheadport.ErrCheckpointFloorUnavailable, wantUnavailable: true},
		{name: "indeterminate", failure: monotonicheadport.ErrCheckpointFloorIndeterminate, wantUnavailable: true},
		{name: "bootstrap", failure: monotonicheadport.ErrCheckpointFloorBootstrap, wantIntegrity: true},
		{name: "conflict", failure: monotonicheadport.ErrCheckpointFloorConflict, wantIntegrity: true, wantEquivocation: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityFixture(t)
			fixture.checkpointFloor.failure = test.failure
			_, err := fixture.authority.Current(context.Background())
			if !errors.Is(err, test.failure) || errors.Is(err, ErrUnavailable) != test.wantUnavailable ||
				errors.Is(err, ErrIntegrity) != test.wantIntegrity || errors.Is(err, monotonicheadport.ErrEquivocation) != test.wantEquivocation {
				t.Fatalf("checkpoint floor failure was misclassified: %v", err)
			}
		})
	}
}

func TestAdvanceChildChangesOnlySelectedPair(t *testing.T) {
	fixture := newAuthorityFixture(t)
	head, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	children := []Child{ChildDatasetSnapshot, ChildEvidenceRegistry, ChildPublication}
	for position, child := range children {
		previous := head.Bundle
		head, err = fixture.authority.AdvanceChild(context.Background(), AdvanceChildInput{
			ExpectedBundleDigest: previous.RecordDigest,
			Child:                child,
			NextIndexDigest:      digest("next-" + string(child)),
		})
		if err != nil {
			t.Fatalf("advance %s: %v", child, err)
		}
		if head.Bundle.Generation != uint64(position+2) || head.Bundle.PreviousBundleDigest != previous.RecordDigest ||
			domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, head.Bundle) != nil {
			t.Fatalf("invalid %s transition: %#v", child, head.Bundle)
		}
		switch child {
		case ChildDatasetSnapshot:
			if head.Bundle.DatasetSnapshotCount != previous.DatasetSnapshotCount+1 ||
				head.Bundle.EvidenceRegistryIndexDigest != previous.EvidenceRegistryIndexDigest || head.Bundle.PublicationIndexDigest != previous.PublicationIndexDigest {
				t.Fatal("dataset advance changed another child")
			}
		case ChildEvidenceRegistry:
			if head.Bundle.EvidenceRegistryCount != previous.EvidenceRegistryCount+1 ||
				head.Bundle.DatasetSnapshotIndexDigest != previous.DatasetSnapshotIndexDigest || head.Bundle.PublicationIndexDigest != previous.PublicationIndexDigest {
				t.Fatal("registry advance changed another child")
			}
		case ChildPublication:
			if head.Bundle.PublicationCount != previous.PublicationCount+1 ||
				head.Bundle.DatasetSnapshotIndexDigest != previous.DatasetSnapshotIndexDigest || head.Bundle.EvidenceRegistryIndexDigest != previous.EvidenceRegistryIndexDigest {
				t.Fatal("publication advance changed another child")
			}
		}
	}
}

func TestNarrowRegistryAndPublicationCoordinatorsCannotChangeOtherChildren(t *testing.T) {
	fixture := newAuthorityFixture(t)
	head, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	registryHead, err := fixture.authority.AdvanceEvidenceRegistry(context.Background(), storeport.RegistryAdvanceInput{
		ExpectedBundleDigest: head.Bundle.RecordDigest, NextIndexDigest: digest("narrow-registry-index"),
	})
	if err != nil || registryHead.Bundle.EvidenceRegistryCount != head.Bundle.EvidenceRegistryCount+1 ||
		registryHead.Bundle.DatasetSnapshotIndexDigest != head.Bundle.DatasetSnapshotIndexDigest ||
		registryHead.Bundle.PublicationIndexDigest != head.Bundle.PublicationIndexDigest {
		t.Fatalf("narrow registry coordinator changed another child: head=%#v err=%v", registryHead, err)
	}
	publicationHead, err := fixture.authority.AdvancePublication(context.Background(), storeport.PublicationAdvanceInput{
		ExpectedBundleDigest: registryHead.Bundle.RecordDigest, NextIndexDigest: digest("narrow-publication-index"),
	})
	if err != nil || publicationHead.Bundle.PublicationCount != registryHead.Bundle.PublicationCount+1 ||
		publicationHead.Bundle.DatasetSnapshotIndexDigest != registryHead.Bundle.DatasetSnapshotIndexDigest ||
		publicationHead.Bundle.EvidenceRegistryIndexDigest != registryHead.Bundle.EvidenceRegistryIndexDigest {
		t.Fatalf("narrow publication coordinator changed another child: head=%#v err=%v", publicationHead, err)
	}
}

func TestWitnessBindingResolvesOnlyFromFreshCurrentAncestry(t *testing.T) {
	fixture := newAuthorityFixture(t)
	admitted, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainevidence.NewEvidenceAuthorityWitnessBindingV1(
		admitted.Bundle, admitted.Request, admitted.Observation,
		fixture.installationID, fixture.enrollmentID, fixture.installation.KeyID(), fixture.installation.PublicKey(),
		domainsecurity.SHA256Hex(fixture.witnessPrivate.Public().(ed25519.PublicKey)),
		fixture.witnessPrivate.Public().(ed25519.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := fixture.authority.AdvanceChild(context.Background(), AdvanceChildInput{
		ExpectedBundleDigest: admitted.Bundle.RecordDigest,
		Child:                ChildEvidenceRegistry,
		NextIndexDigest:      digest("binding-registry-index"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.authority.AdvanceChild(context.Background(), AdvanceChildInput{
		ExpectedBundleDigest: advanced.Bundle.RecordDigest,
		Child:                ChildPublication,
		NextIndexDigest:      digest("binding-publication-index"),
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := fixture.authority.ResolveWitnessBindingOnFreshChain(context.Background(), binding)
	if err != nil || !equalBundleRecord(resolved.Historical.Bundle, admitted.Bundle) ||
		resolved.Historical.Observation.ObservationDigest != admitted.Observation.ObservationDigest ||
		!resolved.Current.HasBundle || resolved.Current.Bundle.Generation != 3 {
		t.Fatalf("fresh descendant did not prove the admitted binding: resolved=%#v err=%v", resolved, err)
	}

	forged := binding
	forged.ObservationDigest = digest("forged-observation")
	if _, err := fixture.authority.ResolveWitnessBindingOnFreshChain(context.Background(), forged); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("malformed witness binding was not rejected before lookup: %v", err)
	}
}

func TestWitnessBindingRejectsPersistedBundleOutsideFreshChain(t *testing.T) {
	fixture := newAuthorityFixture(t)
	admitted, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainevidence.NewEvidenceAuthorityWitnessBindingV1(
		admitted.Bundle, admitted.Request, admitted.Observation,
		fixture.installationID, fixture.enrollmentID, fixture.installation.KeyID(), fixture.installation.PublicKey(),
		domainsecurity.SHA256Hex(fixture.witnessPrivate.Public().(ed25519.PublicKey)),
		fixture.witnessPrivate.Public().(ed25519.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}

	other := newAuthorityFixture(t)
	other.bundles = fixture.bundles
	other.observations = fixture.observations
	other.projection = &memoryProjection{}
	other.checkpointFloor = &memoryCheckpointFloor{}
	other.authority, err = New(other.config(121))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.authority.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := other.authority.ResolveWitnessBindingOnFreshChain(context.Background(), binding); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("persisted binding outside the fresh authority chain was accepted: %v", err)
	}
}

func TestAdvanceRequiresExactExpectedHeadAndConcurrentOneWinner(t *testing.T) {
	fixture := newAuthorityFixture(t)
	first, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := AdvanceChildInput{
		ExpectedBundleDigest: first.Bundle.RecordDigest,
		Child:                ChildDatasetSnapshot,
		NextIndexDigest:      digest("concurrent-dataset"),
	}
	start := make(chan struct{})
	errorsOut := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, advanceErr := fixture.authority.AdvanceChild(context.Background(), input)
			errorsOut <- advanceErr
		}()
	}
	close(start)
	wins, conflicts := 0, 0
	for range 2 {
		advanceErr := <-errorsOut
		switch {
		case advanceErr == nil:
			wins++
		case errors.Is(advanceErr, ErrCurrentChanged) && errors.Is(advanceErr, monotonicheadport.ErrCASConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", advanceErr)
		}
	}
	if wins != 1 || conflicts != 1 || fixture.witness.advanceCalls.Load() != 2 {
		t.Fatalf("expected initialize plus one winning child dispatch, wins=%d conflicts=%d calls=%d", wins, conflicts, fixture.witness.advanceCalls.Load())
	}
	if _, err := fixture.authority.AdvanceChild(context.Background(), input); !errors.Is(err, ErrCurrentChanged) {
		t.Fatalf("stale expected digest was accepted: %v", err)
	}
}

func TestIndependentAuthoritiesShareWitnessCASOneWinner(t *testing.T) {
	fixture := newAuthorityFixture(t)
	first, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	other, err := New(fixture.config(88))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Current(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixture.witness.barrierNextObserves(2)
	input := AdvanceChildInput{
		ExpectedBundleDigest: first.Bundle.RecordDigest,
		Child:                ChildPublication,
		NextIndexDigest:      digest("cross-process-publication"),
	}
	start := make(chan struct{})
	errorsOut := make(chan error, 2)
	for _, candidate := range []*Authority{fixture.authority, other} {
		go func(candidate *Authority) {
			<-start
			_, advanceErr := candidate.AdvanceChild(context.Background(), input)
			errorsOut <- advanceErr
		}(candidate)
	}
	close(start)
	wins, conflicts := 0, 0
	for range 2 {
		advanceErr := <-errorsOut
		switch {
		case advanceErr == nil:
			wins++
		case errors.Is(advanceErr, monotonicheadport.ErrCASConflict):
			conflicts++
		default:
			t.Fatalf("unexpected independent-authority result: %v", advanceErr)
		}
	}
	if wins != 1 || conflicts != 1 || fixture.witness.generation() != 2 || fixture.witness.advanceCalls.Load() != 3 {
		t.Fatalf("cross-process CAS was not one-winner: wins=%d conflicts=%d generation=%d calls=%d",
			wins, conflicts, fixture.witness.generation(), fixture.witness.advanceCalls.Load())
	}
}

func TestCandidatePersistsBeforeSingleDispatchAndStoreFailuresFailClosed(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.bundles.beforePut = func(bundle domainevidence.EvidenceAuthorityBundleV1) {
		if fixture.witness.advanceCalls.Load() != 0 {
			t.Error("witness was called before immutable bundle persistence")
		}
	}
	if _, err := fixture.authority.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixture.bundles.beforePut = nil
	fixture.bundles.failPut.Store(true)
	current, err := fixture.authority.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before := fixture.witness.advanceCalls.Load()
	_, err = fixture.authority.AdvanceChild(context.Background(), AdvanceChildInput{
		ExpectedBundleDigest: current.Bundle.RecordDigest,
		Child:                ChildPublication,
		NextIndexDigest:      digest("store-failure"),
	})
	if !errors.Is(err, ErrUnavailable) || fixture.witness.advanceCalls.Load() != before {
		t.Fatalf("store failure was not fail-closed before dispatch: %v", err)
	}
}

func TestIndeterminateReconcilesOnlyExactCandidateWithoutRetry(t *testing.T) {
	t.Run("committed", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		fixture.witness.setFault(witnessFaultIndeterminateCommitted)
		head, err := fixture.authority.Initialize(context.Background())
		if err != nil || !head.HasBundle || head.Bundle.Generation != 1 {
			t.Fatalf("committed indeterminate did not reconcile: %v", err)
		}
		if fixture.witness.advanceCalls.Load() != 1 {
			t.Fatal("indeterminate path retried Advance")
		}
	})
	t.Run("not_committed", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		fixture.witness.setFault(witnessFaultIndeterminateUncommitted)
		_, err := fixture.authority.Initialize(context.Background())
		if !errors.Is(err, monotonicheadport.ErrIndeterminate) || !errors.Is(err, ErrAdvanceNotCommitted) {
			t.Fatalf("uncommitted indeterminate was not typed: %v", err)
		}
		if fixture.witness.advanceCalls.Load() != 1 {
			t.Fatal("uncommitted path retried Advance")
		}
	})
	t.Run("third_head", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		fixture.witness.setFault(witnessFaultIndeterminateThirdHead)
		_, err := fixture.authority.Initialize(context.Background())
		if !errors.Is(err, monotonicheadport.ErrEquivocation) || !errors.Is(err, ErrIntegrity) {
			t.Fatalf("third head was accepted: %v", err)
		}
		if fixture.witness.advanceCalls.Load() != 1 {
			t.Fatal("third-head path retried Advance")
		}
	})
}

func TestLocalRollbackMissingAncestryProjectionAndEquivocationFailClosed(t *testing.T) {
	fixture := newAuthorityFixture(t)
	first, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	latest, err := fixture.authority.AdvanceChild(context.Background(), AdvanceChildInput{
		ExpectedBundleDigest: first.Bundle.RecordDigest, Child: ChildEvidenceRegistry, NextIndexDigest: digest("latest-registry"),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.projection.set(first.Bundle)
	current, err := fixture.authority.Current(context.Background())
	if err != nil || current.Bundle.RecordDigest != latest.Bundle.RecordDigest || fixture.projection.current().RecordDigest != latest.Bundle.RecordDigest {
		t.Fatalf("projection influenced current: %v", err)
	}
	fixture.bundles.delete(latest.Bundle.RecordDigest)
	if _, err := fixture.authority.Current(context.Background()); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("missing witness-selected tail was accepted: %v", err)
	}
	fixture.bundles.putRaw(latest.Bundle)
	fixture.bundles.delete(first.Bundle.RecordDigest)
	if _, err := fixture.authority.Current(context.Background()); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("missing bundle ancestry was accepted: %v", err)
	}
	fixture.bundles.putRaw(first.Bundle)
	fixture.projection.fail.Store(true)
	if _, err := fixture.authority.Current(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("projection failure did not fail closed: %v", err)
	}
	fixture.projection.fail.Store(false)
	fixture.witness.equivocateSameGeneration()
	if _, err := fixture.authority.Current(context.Background()); !errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("same-generation witness equivocation was accepted: %v", err)
	}
}

func TestFreshObservationRejectsHigherGenerationFork(t *testing.T) {
	fixture := newAuthorityFixture(t)
	first, err := fixture.authority.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	left, err := fixture.authority.AdvanceChild(context.Background(), AdvanceChildInput{
		ExpectedBundleDigest: first.Bundle.RecordDigest, Child: ChildDatasetSnapshot,
		NextIndexDigest: digest("higher-generation-left"),
	})
	if err != nil {
		t.Fatal(err)
	}
	rightSecond := newForkedEvidenceAuthorityBundle(t, fixture, first.Bundle, ChildPublication, "right-2")
	rightThird := newForkedEvidenceAuthorityBundle(t, fixture, rightSecond, ChildEvidenceRegistry, "right-3")
	rightFourth := newForkedEvidenceAuthorityBundle(t, fixture, rightThird, ChildDatasetSnapshot, "right-4")
	for _, bundle := range []domainevidence.EvidenceAuthorityBundleV1{rightSecond, rightThird, rightFourth} {
		fixture.bundles.putRaw(bundle)
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace:  domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation: rightFourth.Generation, CurrentStateDigest: rightFourth.RecordDigest,
		PreviousStateDigest: rightThird.RecordDigest, PreviousCheckpointDigest: digest("higher-generation-fork-checkpoint"),
		FenceNonce: digest("higher-generation-fork-fence"), MutationID: rightFourth.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(fixture.witness.public), WitnessPublicKey: fixture.witness.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	fixture.witness.mu.Lock()
	fixture.witness.checkpoint = checkpoint
	fixture.witness.mu.Unlock()
	if _, err := fixture.authority.Current(context.Background()); !errors.Is(err, ErrIntegrity) ||
		!errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("higher-generation fork was accepted: %v", err)
	}
	if fixture.projection.current().RecordDigest != left.Bundle.RecordDigest {
		t.Fatal("higher-generation fork mutated the local projection")
	}
}

func newForkedEvidenceAuthorityBundle(
	t *testing.T,
	fixture *authorityFixture,
	previous domainevidence.EvidenceAuthorityBundleV1,
	child Child,
	label string,
) domainevidence.EvidenceAuthorityBundleV1 {
	t.Helper()
	input := domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                 digest("higher-generation-fork-mutation-" + label),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount,
		PublicationIndexDigest: previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: fixture.installation.KeyID(), AuthorityPublicKey: fixture.installation.PublicKey(),
	}
	switch child {
	case ChildDatasetSnapshot:
		input.DatasetSnapshotIndexDigest = digest("higher-generation-fork-dataset-" + label)
		input.DatasetSnapshotCount++
	case ChildEvidenceRegistry:
		input.EvidenceRegistryIndexDigest = digest("higher-generation-fork-registry-" + label)
		input.EvidenceRegistryCount++
	case ChildPublication:
		input.PublicationIndexDigest = digest("higher-generation-fork-publication-" + label)
		input.PublicationCount++
	default:
		t.Fatalf("unknown fork child %q", child)
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(input, func(message []byte) ([]byte, error) {
		return fixture.installation.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestCASUnavailableCancellationAndInputFailuresRemainClosed(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.witness.setFault(witnessFaultCASConflict)
	if _, err := fixture.authority.Initialize(context.Background()); !errors.Is(err, monotonicheadport.ErrCASConflict) {
		t.Fatalf("CAS conflict was hidden: %v", err)
	}
	if fixture.witness.generation() != 0 {
		t.Fatal("CAS conflict changed witness generation")
	}
	unavailable := newAuthorityFixture(t)
	unavailable.witness.setFault(witnessFaultUnavailable)
	if _, err := unavailable.authority.Initialize(context.Background()); !errors.Is(err, monotonicheadport.ErrUnavailable) {
		t.Fatalf("witness unavailability was hidden or accepted: %v", err)
	}
	if unavailable.witness.generation() != 0 {
		t.Fatal("unavailable witness changed generation")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	other := newAuthorityFixture(t)
	if _, err := other.authority.Current(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was hidden: %v", err)
	}
	if _, err := other.authority.AdvanceChild(context.Background(), AdvanceChildInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid child input was accepted: %v", err)
	}
	if _, err := other.authority.Current(nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil context was accepted: %v", err)
	}
	bad := other.config(99)
	bad.Genesis.PublicationIndexDigest = "not-a-digest"
	if _, err := New(bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid genesis was accepted: %v", err)
	}
	bad = other.config(100)
	bad.Genesis.DatasetSnapshotIndexDigest = digest("caller-selected-dataset-genesis")
	if _, err := New(bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("caller-selected dataset genesis was accepted: %v", err)
	}
	bad = other.config(101)
	bad.Genesis.EvidenceRegistryIndexDigest = digest("caller-selected-registry-genesis")
	if _, err := New(bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("caller-selected evidence registry genesis was accepted: %v", err)
	}
	bad = other.config(102)
	bad.Genesis.PublicationIndexDigest = digest("caller-selected-publication-genesis")
	if _, err := New(bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("caller-selected publication genesis was accepted: %v", err)
	}
}

func TestAuthorityNonceReplayMemoryIsBounded(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.authority.mu.Lock()
	defer fixture.authority.mu.Unlock()
	for range evidenceAuthorityNonceReplayWindow + 17 {
		if _, err := fixture.authority.freshNonceLocked(context.Background(), "bounded-test"); err != nil {
			t.Fatal(err)
		}
	}
	if len(fixture.authority.usedNonces) != evidenceAuthorityNonceReplayWindow ||
		len(fixture.authority.nonceOrder) != evidenceAuthorityNonceReplayWindow {
		t.Fatalf("nonce replay window grew without bound: set=%d order=%d",
			len(fixture.authority.usedNonces), len(fixture.authority.nonceOrder))
	}
}

type authorityFixture struct {
	installation    *testInstallationAuthority
	witnessPrivate  ed25519.PrivateKey
	witness         *testWitness
	bundles         *memoryBundleStore
	observations    *memoryObservationStore
	projection      *memoryProjection
	checkpointFloor *memoryCheckpointFloor
	authority       *Authority
	installationID  string
	enrollmentID    string
	genesis         Genesis
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()
	installation := newTestInstallationAuthority(11)
	_, witnessPrivate, err := ed25519.GenerateKey(&repeatReader{value: 22})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &authorityFixture{
		installation:    installation,
		witnessPrivate:  witnessPrivate,
		bundles:         newMemoryBundleStore(),
		observations:    newMemoryObservationStore(),
		projection:      &memoryProjection{},
		checkpointFloor: &memoryCheckpointFloor{},
		installationID:  digest("test-installation"),
		enrollmentID:    digest("test-enrollment"),
		genesis: Genesis{
			DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		},
	}
	fixture.witness = newTestWitness(t, fixture.installationID, fixture.enrollmentID, witnessPrivate)
	fixture.authority, err = New(fixture.config(44))
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *authorityFixture) config(randomStart byte) Config {
	witnessPublic := fixture.witnessPrivate.Public().(ed25519.PublicKey)
	return Config{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Authority: fixture.installation, WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
		Random: &counterReader{next: uint64(randomStart)}, Witness: fixture.witness,
		CheckpointFloor: fixture.checkpointFloor,
		Bundles:         fixture.bundles, Observations: fixture.observations, Projection: fixture.projection, Genesis: fixture.genesis,
	}
}

type testInstallationAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newTestInstallationAuthority(seed byte) *testInstallationAuthority {
	public, private, _ := ed25519.GenerateKey(&repeatReader{value: seed})
	return &testInstallationAuthority{private: private, public: public}
}

func (authority *testInstallationAuthority) KeyID() string {
	return domainsecurity.SHA256Hex(authority.public)
}
func (authority *testInstallationAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}
func (authority *testInstallationAuthority) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return ed25519.Sign(authority.private, body), nil
}
func (authority *testInstallationAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if keyID != authority.KeyID() || !ed25519.PublicKey(publicKey).Equal(authority.public) || !ed25519.Verify(authority.public, body, signature) {
		return errors.New("untrusted")
	}
	return nil
}

type repeatReader struct{ value byte }

func (reader *repeatReader) Read(body []byte) (int, error) {
	for position := range body {
		body[position] = reader.value
	}
	return len(body), nil
}

type counterReader struct {
	mu   sync.Mutex
	next uint64
}

func (reader *counterReader) Read(body []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.next++
	for position := range body {
		body[position] = byte(position*37 + 11)
	}
	if len(body) >= 8 {
		binary.LittleEndian.PutUint64(body[:8], reader.next)
	}
	return len(body), nil
}

func digest(value string) string { return domainsecurity.SHA256Hex([]byte(value)) }

type witnessFault string

const (
	witnessFaultNone                     witnessFault = ""
	witnessFaultCASConflict              witnessFault = "cas_conflict"
	witnessFaultUnavailable              witnessFault = "unavailable"
	witnessFaultIndeterminateCommitted   witnessFault = "indeterminate_committed"
	witnessFaultIndeterminateUncommitted witnessFault = "indeterminate_uncommitted"
	witnessFaultIndeterminateThirdHead   witnessFault = "indeterminate_third_head"
)

type testWitness struct {
	t                         *testing.T
	mu                        sync.Mutex
	private                   ed25519.PrivateKey
	public                    ed25519.PublicKey
	checkpoint                domainsecurity.MonotonicHeadCheckpointV1
	fault                     witnessFault
	advanceCalls              atomic.Int32
	observeCalls              atomic.Int32
	observedNonce             []string
	lastObservation           domainsecurity.MonotonicHeadObservationV1
	replayObservation         *domainsecurity.MonotonicHeadObservationV1
	forgeObservationSignature bool
	mutateRequest             func(*domainsecurity.MonotonicHeadObserveRequestV1)
	mutateObservation         func(*domainsecurity.MonotonicHeadObservationV1)
	observeEntered            chan struct{}
	observeRelease            chan struct{}
	barrierLeft               int
	barrierDone               chan struct{}
}

func newTestWitness(t *testing.T, installationID, enrollmentID string, private ed25519.PrivateKey) *testWitness {
	t.Helper()
	public := private.Public().(ed25519.PublicKey)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1, Generation: 0,
		CurrentStateDigest: digest("enrolled-empty-evidence-authority"), FenceNonce: digest("enrollment-fence"),
		WitnessKeyID: domainsecurity.SHA256Hex(public), WitnessPublicKey: public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return &testWitness{t: t, private: private, public: public, checkpoint: checkpoint}
}

func (witness *testWitness) Observe(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error) {
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	witness.observeCalls.Add(1)
	witness.mu.Lock()
	if witness.mutateRequest != nil {
		witness.mutateRequest(&request)
		witness.mutateRequest = nil
	}
	checkpoint := witness.checkpoint
	barrier := witness.barrierDone
	entered := witness.observeEntered
	release := witness.observeRelease
	witness.observeEntered = nil
	witness.observeRelease = nil
	witness.observedNonce = append(witness.observedNonce, request.ChallengeNonce)
	if witness.barrierLeft > 0 {
		witness.barrierLeft--
		if witness.barrierLeft == 0 {
			close(witness.barrierDone)
		}
	}
	witness.mu.Unlock()
	if barrier != nil {
		select {
		case <-barrier:
		case <-ctx.Done():
			return domainsecurity.MonotonicHeadObservationV1{}, ctx.Err()
		}
	}
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return domainsecurity.MonotonicHeadObservationV1{}, ctx.Err()
		}
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witness.private, message), nil
	})
	if err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	witness.mu.Lock()
	if witness.replayObservation != nil {
		observation = *witness.replayObservation
		witness.replayObservation = nil
	}
	if witness.forgeObservationSignature {
		observation.WitnessSignature = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
		witness.forgeObservationSignature = false
	}
	if witness.mutateObservation != nil {
		witness.mutateObservation(&observation)
		witness.mutateObservation = nil
	}
	witness.lastObservation = observation
	witness.mu.Unlock()
	return observation, nil
}

func (witness *testWitness) observedNonces() []string {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	return append([]string(nil), witness.observedNonce...)
}

func (witness *testWitness) blockNextObservation(entered, release chan struct{}) {
	witness.mu.Lock()
	witness.observeEntered = entered
	witness.observeRelease = release
	witness.mu.Unlock()
}

func (witness *testWitness) barrierNextObserves(count int) {
	witness.mu.Lock()
	witness.barrierLeft = count
	witness.barrierDone = make(chan struct{})
	witness.mu.Unlock()
}

func (witness *testWitness) Advance(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	witness.advanceCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	fault := witness.fault
	witness.fault = witnessFaultNone
	if fault == witnessFaultCASConflict {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrCASConflict
	}
	if fault == witnessFaultUnavailable {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrUnavailable
	}
	previous := witness.checkpoint
	if request.ExpectedGeneration != previous.Generation || request.ExpectedCheckpointDigest != previous.CheckpointDigest ||
		request.ExpectedStateDigest != previous.CurrentStateDigest || request.ExpectedFenceNonce != previous.FenceNonce {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrCASConflict
	}
	if fault == witnessFaultIndeterminateUncommitted {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrIndeterminate
	}
	nextState := request.NextStateDigest
	if fault == witnessFaultIndeterminateThirdHead {
		nextState = digest("incompatible-third-head")
	}
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Namespace: previous.Namespace,
		Generation: request.NextGeneration, CurrentStateDigest: nextState,
		PreviousStateDigest: previous.CurrentStateDigest, PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce: digest("fence:" + request.MutationID + ":" + nextState), MutationID: request.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(witness.public), WitnessPublicKey: witness.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witness.private, message), nil })
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	witness.checkpoint = next
	if fault == witnessFaultIndeterminateCommitted || fault == witnessFaultIndeterminateThirdHead {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrIndeterminate
	}
	return domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witness.private, message), nil
	})
}

func (witness *testWitness) setFault(fault witnessFault) {
	witness.mu.Lock()
	witness.fault = fault
	witness.mu.Unlock()
}

func (witness *testWitness) generation() uint64 {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	return witness.checkpoint.Generation
}

func (witness *testWitness) equivocateSameGeneration() {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	previous := witness.checkpoint
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Namespace: previous.Namespace,
		Generation: previous.Generation, CurrentStateDigest: previous.CurrentStateDigest,
		PreviousStateDigest: previous.PreviousStateDigest, PreviousCheckpointDigest: previous.PreviousCheckpointDigest,
		FenceNonce: digest("equivocated-fence"), MutationID: previous.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(witness.public), WitnessPublicKey: witness.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witness.private, message), nil })
	if err != nil {
		witness.t.Fatal(err)
	}
	witness.checkpoint = checkpoint
}

type memoryBundleStore struct {
	mu           sync.Mutex
	records      map[string]domainevidence.EvidenceAuthorityBundleV1
	failPut      atomic.Bool
	resolveCalls atomic.Int32
	beforePut    func(domainevidence.EvidenceAuthorityBundleV1)
}

func newMemoryBundleStore() *memoryBundleStore {
	return &memoryBundleStore{records: make(map[string]domainevidence.EvidenceAuthorityBundleV1)}
}

func (store *memoryBundleStore) PutIfAbsent(ctx context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store.beforePut != nil {
		store.beforePut(bundle)
	}
	if store.failPut.Load() {
		return errors.New("put unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, found := store.records[bundle.RecordDigest]; found && !equalBundleRecord(current, bundle) {
		return errors.New("content-address conflict")
	}
	store.records[bundle.RecordDigest] = bundle
	return nil
}

func (store *memoryBundleStore) Resolve(ctx context.Context, recordDigest string) (domainevidence.EvidenceAuthorityBundleV1, error) {
	store.resolveCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return domainevidence.EvidenceAuthorityBundleV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	bundle, found := store.records[recordDigest]
	if !found {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("absent")
	}
	return bundle, nil
}

func (store *memoryBundleStore) delete(recordDigest string) {
	store.mu.Lock()
	delete(store.records, recordDigest)
	store.mu.Unlock()
}

func (store *memoryBundleStore) putRaw(bundle domainevidence.EvidenceAuthorityBundleV1) {
	store.mu.Lock()
	store.records[bundle.RecordDigest] = bundle
	store.mu.Unlock()
}

type memoryObservationStore struct {
	mu           sync.Mutex
	records      map[string]storeport.ObservationBundle
	putCalls     atomic.Int32
	resolveCalls atomic.Int32
}

func newMemoryObservationStore() *memoryObservationStore {
	return &memoryObservationStore{records: make(map[string]storeport.ObservationBundle)}
}

func (store *memoryObservationStore) PutIfAbsent(ctx context.Context, bundle storeport.ObservationBundle) error {
	store.putCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	digest := bundle.Observation.ObservationDigest
	if current, found := store.records[digest]; found && !equalObservationBundle(current, bundle) {
		return errors.New("content-address conflict")
	}
	store.records[digest] = bundle
	return nil
}

func (store *memoryObservationStore) Resolve(ctx context.Context, observationDigest string) (storeport.ObservationBundle, error) {
	store.resolveCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return storeport.ObservationBundle{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	bundle, found := store.records[observationDigest]
	if !found {
		return storeport.ObservationBundle{}, errors.New("absent")
	}
	return bundle, nil
}

type memoryProjection struct {
	mu           sync.Mutex
	bundle       domainevidence.EvidenceAuthorityBundleV1
	fail         atomic.Bool
	projectCalls atomic.Int32
}

type memoryCheckpointFloor struct {
	mu           sync.Mutex
	checkpoint   *domainsecurity.MonotonicHeadCheckpointV1
	failure      error
	projectCalls atomic.Int32
}

func (floor *memoryCheckpointFloor) ProjectWitnessSelected(ctx context.Context, selected domainsecurity.MonotonicHeadCheckpointV1) error {
	floor.projectCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	floor.mu.Lock()
	defer floor.mu.Unlock()
	if floor.failure != nil {
		return floor.failure
	}
	if floor.checkpoint == nil {
		if selected.Generation != 0 {
			return monotonicheadport.ErrCheckpointFloorBootstrap
		}
		copy := selected
		floor.checkpoint = &copy
		return nil
	}
	if *floor.checkpoint == selected {
		return nil
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(*floor.checkpoint, selected) != nil {
		return monotonicheadport.ErrCheckpointFloorConflict
	}
	copy := selected
	floor.checkpoint = &copy
	return nil
}

func (projection *memoryProjection) ValidateWitnessEmpty(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if projection.fail.Load() {
		return errors.New("projection unavailable")
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	if projection.bundle.Generation != 0 {
		return errors.New("non-empty projection")
	}
	return nil
}

func (projection *memoryProjection) ProjectWitnessSelected(ctx context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	projection.projectCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	if projection.fail.Load() {
		return errors.New("projection unavailable")
	}
	projection.set(bundle)
	return nil
}

func (projection *memoryProjection) set(bundle domainevidence.EvidenceAuthorityBundleV1) {
	projection.mu.Lock()
	projection.bundle = bundle
	projection.mu.Unlock()
}

func (projection *memoryProjection) current() domainevidence.EvidenceAuthorityBundleV1 {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	return projection.bundle
}

var _ io.Reader = (*counterReader)(nil)
