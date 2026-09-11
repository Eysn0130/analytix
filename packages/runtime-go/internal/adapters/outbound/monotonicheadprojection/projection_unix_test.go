//go:build darwin || linux

package monotonicheadprojection

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

func TestCheckpointFloorColdStartAndDirectSuccessorOnly(t *testing.T) {
	fixture := newCheckpointFloorFixture(t, filepath.Join(t.TempDir(), "floor"))
	projection, err := New(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.successor(t, fixture.initial, "first")
	alternateGenesis := fixture.fork(t, fixture.initial, "alternate-genesis")
	if err := projection.ProjectWitnessSelected(context.Background(), alternateGenesis); !errors.Is(err, monotonicheadport.ErrCheckpointFloorBootstrap) {
		t.Fatalf("cold floor accepted a signed but unpinned genesis: %v", err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), first); !errors.Is(err, monotonicheadport.ErrCheckpointFloorBootstrap) {
		t.Fatalf("cold floor accepted a non-genesis checkpoint: %v", err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), fixture.initial); err != nil {
		t.Fatalf("cold floor rejected the manifest-pinned genesis: %v", err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), fixture.initial); err != nil {
		t.Fatalf("exact floor replay was not idempotent: %v", err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatalf("direct successor was rejected: %v", err)
	}

	reopened, err := New(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatalf("restarted floor did not preserve exact checkpoint: %v", err)
	}
	if err := reopened.ProjectWitnessSelected(context.Background(), fixture.initial); !errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) {
		t.Fatalf("restarted floor accepted rollback to genesis: %v", err)
	}

	fork := fixture.fork(t, first, "fork")
	if err := reopened.ProjectWitnessSelected(context.Background(), fork); !errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) {
		t.Fatalf("same-generation checkpoint fork was accepted: %v", err)
	}
	gap := fixture.successor(t, first, "gap-intermediate")
	gap = fixture.successor(t, gap, "gap-selected")
	if err := reopened.ProjectWitnessSelected(context.Background(), gap); !errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) {
		t.Fatalf("checkpoint generation gap was accepted: %v", err)
	}
}

func TestCheckpointFloorDeletionAfterInitializationCannotBootstrap(t *testing.T) {
	fixture := newCheckpointFloorFixture(t, filepath.Join(t.TempDir(), "floor"))
	projection, err := New(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), fixture.initial); err != nil {
		t.Fatal(err)
	}
	first := fixture.successor(t, fixture.initial, "first")
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fixture.config.Root, checkpointFloorFileName)); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ProjectWitnessSelected(context.Background(), first); !errors.Is(err, monotonicheadport.ErrCheckpointFloorBootstrap) {
		t.Fatalf("deleted initialized floor was silently rebuilt: %v", err)
	}
}

func TestCheckpointFloorConcurrentCASWinnerMustMatchExactSelectedBytes(t *testing.T) {
	fixture := newCheckpointFloorFixture(t, filepath.Join(t.TempDir(), "floor"))
	selected := fixture.successor(t, fixture.initial, "selected")
	other := fixture.successor(t, fixture.initial, "other")
	initialBody := checkpointFloorBody(t, fixture.initial)

	t.Run("exact-winner", func(t *testing.T) {
		projection, err := New(fixture.config)
		if err != nil {
			t.Fatal(err)
		}
		store := &checkpointFloorCASWinnerStore{
			observation: checkpointFloorObservation(initialBody),
			winner:      checkpointFloorObservation(checkpointFloorBody(t, selected)),
		}
		projection.store = store
		if err := projection.ProjectWitnessSelected(context.Background(), selected); err != nil {
			t.Fatalf("exact concurrent winner was not accepted idempotently: %v", err)
		}
		if store.reconcileCalls != 0 {
			t.Fatal("compare loser attempted residue reconciliation without a fresh residue observation")
		}
	})

	t.Run("different-winner", func(t *testing.T) {
		projection, err := New(Config{
			Root: filepath.Join(t.TempDir(), "floor"), InstallationID: fixture.config.InstallationID,
			EnrollmentID: fixture.config.EnrollmentID, Namespace: fixture.config.Namespace,
			WitnessKeyID: fixture.config.WitnessKeyID, WitnessPublicKey: fixture.config.WitnessPublicKey,
			InitialCheckpoint: fixture.initial,
		})
		if err != nil {
			t.Fatal(err)
		}
		store := &checkpointFloorCASWinnerStore{
			observation: checkpointFloorObservation(initialBody),
			winner:      checkpointFloorObservation(checkpointFloorBody(t, other)),
		}
		projection.store = store
		if err := projection.ProjectWitnessSelected(context.Background(), selected); !errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) {
			t.Fatalf("different concurrent winner was not rejected: %v", err)
		}
	})

	t.Run("compare-loser-read-failure", func(t *testing.T) {
		projection, err := New(Config{
			Root: filepath.Join(t.TempDir(), "floor"), InstallationID: fixture.config.InstallationID,
			EnrollmentID: fixture.config.EnrollmentID, Namespace: fixture.config.Namespace,
			WitnessKeyID: fixture.config.WitnessKeyID, WitnessPublicKey: fixture.config.WitnessPublicKey,
			InitialCheckpoint: fixture.initial,
		})
		if err != nil {
			t.Fatal(err)
		}
		store := &checkpointFloorCASWinnerStore{
			observation: checkpointFloorObservation(initialBody),
			winner:      checkpointFloorObservation(checkpointFloorBody(t, selected)),
			winnerErr:   errors.New("winner read failed"),
		}
		projection.store = store
		err = projection.ProjectWitnessSelected(context.Background(), selected)
		if !errors.Is(err, monotonicheadport.ErrCheckpointFloorIndeterminate) || errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) {
			t.Fatalf("compare loser read failure was misclassified: %v", err)
		}
	})

	t.Run("residue-after-clean-observe", func(t *testing.T) {
		projection, err := New(Config{
			Root: filepath.Join(t.TempDir(), "floor"), InstallationID: fixture.config.InstallationID,
			EnrollmentID: fixture.config.EnrollmentID, Namespace: fixture.config.Namespace,
			WitnessKeyID: fixture.config.WitnessKeyID, WitnessPublicKey: fixture.config.WitnessPublicKey,
			InitialCheckpoint: fixture.initial,
		})
		if err != nil {
			t.Fatal(err)
		}
		store := &checkpointFloorCASWinnerStore{
			observation: checkpointFloorObservation(initialBody),
			winner:      checkpointFloorObservation(initialBody),
			replaceErr:  authorityprojectionport.ErrResidue,
		}
		projection.store = store
		err = projection.ProjectWitnessSelected(context.Background(), selected)
		if !errors.Is(err, monotonicheadport.ErrCheckpointFloorIndeterminate) || errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) {
			t.Fatalf("residue appearing after clean observe was misclassified: %v", err)
		}
	})
}

type checkpointFloorCASWinnerStore struct {
	observation    authorityprojectionport.Observation
	winner         authorityprojectionport.Observation
	winnerErr      error
	replaceErr     error
	replaced       bool
	reconcileCalls int
}

func (store *checkpointFloorCASWinnerStore) Observe(context.Context) (authorityprojectionport.Observation, error) {
	if store.replaced && store.winnerErr != nil {
		return authorityprojectionport.Observation{}, store.winnerErr
	}
	result := store.observation
	result.Body = bytes.Clone(result.Body)
	return result, nil
}

func (store *checkpointFloorCASWinnerStore) ReplaceExact(context.Context, string, []byte) (authorityprojectionport.ReplaceResult, error) {
	store.observation = store.winner
	store.replaced = true
	err := store.replaceErr
	if err == nil {
		err = authorityprojectionport.ErrCompareFailed
	}
	return authorityprojectionport.ReplaceResult{State: authorityprojectionport.NotCommitted}, err
}

func (store *checkpointFloorCASWinnerStore) ReconcileExact(context.Context, string, string) (authorityprojectionport.ReplaceResult, error) {
	store.reconcileCalls++
	return authorityprojectionport.ReplaceResult{State: authorityprojectionport.NotCommitted}, authorityprojectionport.ErrCompareFailed
}

func checkpointFloorObservation(body []byte) authorityprojectionport.Observation {
	return authorityprojectionport.Observation{Present: true, Digest: domainsecurity.SHA256Hex(body), Body: bytes.Clone(body)}
}

func checkpointFloorBody(t *testing.T, checkpoint domainsecurity.MonotonicHeadCheckpointV1) []byte {
	t.Helper()
	body, err := domainsecurity.MonotonicHeadCheckpointV1Bytes(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

type checkpointFloorFixture struct {
	config  Config
	private ed25519.PrivateKey
	initial domainsecurity.MonotonicHeadCheckpointV1
}

func newCheckpointFloorFixture(t *testing.T, root string) checkpointFloorFixture {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x64}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("checkpoint-floor-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("checkpoint-floor-enrollment"))
	namespace := domainsecurity.ThreadRiskAuthorityNamespaceV1
	initial, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: namespace, Generation: 0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("checkpoint-floor-empty")),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("checkpoint-floor-fence")),
		WitnessKeyID:       domainsecurity.SHA256Hex(public), WitnessPublicKey: public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return checkpointFloorFixture{
		private: private, initial: initial,
		config: Config{
			Root: root, InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: namespace,
			WitnessKeyID: domainsecurity.SHA256Hex(public), WitnessPublicKey: public, InitialCheckpoint: initial,
		},
	}
}

func (fixture checkpointFloorFixture) successor(t *testing.T, previous domainsecurity.MonotonicHeadCheckpointV1, label string) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Namespace: previous.Namespace,
		Generation: previous.Generation + 1, CurrentStateDigest: domainsecurity.SHA256Hex([]byte("state:" + label)),
		PreviousStateDigest: previous.CurrentStateDigest, PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce: domainsecurity.SHA256Hex([]byte("fence:" + label)), MutationID: domainsecurity.SHA256Hex([]byte("mutation:" + label)),
		WitnessKeyID: previous.WitnessKeyID, WitnessPublicKey: fixture.private.Public().(ed25519.PublicKey),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func (fixture checkpointFloorFixture) fork(t *testing.T, checkpoint domainsecurity.MonotonicHeadCheckpointV1, label string) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	fork, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: checkpoint.InstallationID, EnrollmentID: checkpoint.EnrollmentID, Namespace: checkpoint.Namespace,
		Generation: checkpoint.Generation, CurrentStateDigest: checkpoint.CurrentStateDigest,
		PreviousStateDigest: checkpoint.PreviousStateDigest, PreviousCheckpointDigest: checkpoint.PreviousCheckpointDigest,
		FenceNonce: domainsecurity.SHA256Hex([]byte("fence:" + label)), MutationID: checkpoint.MutationID,
		WitnessKeyID: checkpoint.WitnessKeyID, WitnessPublicKey: fixture.private.Public().(ed25519.PublicKey),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return fork
}
