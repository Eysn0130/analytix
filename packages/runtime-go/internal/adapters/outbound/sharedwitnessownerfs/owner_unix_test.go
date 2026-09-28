//go:build darwin || linux

package sharedwitnessownerfs

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	monotonichead "analytix.local/runtime-go/internal/ports/monotonichead"
)

type ownerFixture struct {
	userData            string
	root                string
	anchor              Anchor
	installationPrivate ed25519.PrivateKey
	witnessPrivate      ed25519.PrivateKey
}

func newOwnerFixture(t *testing.T) ownerFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	userData := filepath.Join(base, "electron-user-data")
	root := filepath.Join(userData, OwnerRootName)
	if err := os.Mkdir(userData, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	installationPublic, installationPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	witnessPublic, witnessPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	installationID := domainsecurity.SHA256Hex([]byte("isolated-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("isolated-enrollment"))
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainenrollment.SharedEvidenceNamespaceV1, Generation: 0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("gen0 state")),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("gen0 fence")),
		WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return ownerFixture{userData, root, Anchor{
		InstallationID: installationID, AuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		AuthorityPublicKey: installationPublic, EnrollmentID: enrollmentID, GenesisCheckpoint: checkpoint,
	}, installationPrivate, witnessPrivate}
}

func (fixture ownerFixture) create(t *testing.T) *Owner {
	t.Helper()
	owner, err := createForIsolatedFixture(context.Background(), fixture.userData, fixture.anchor, fixture.witnessPrivate)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func (fixture ownerFixture) open(t *testing.T) *Owner {
	t.Helper()
	owner, err := OpenExisting(context.Background(), fixture.userData, fixture.anchor)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func (fixture ownerFixture) advanceRequest(t *testing.T, checkpoint domainsecurity.MonotonicHeadCheckpointV1, mutation string, nextState string) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.anchor.InstallationID, EnrollmentID: fixture.anchor.EnrollmentID,
		Namespace:          domainenrollment.SharedEvidenceNamespaceV1,
		ExpectedGeneration: checkpoint.Generation, ExpectedCheckpointDigest: checkpoint.CheckpointDigest,
		ExpectedStateDigest: checkpoint.CurrentStateDigest, ExpectedFenceNonce: checkpoint.FenceNonce,
		NextGeneration: checkpoint.Generation + 1, NextStateDigest: domainsecurity.SHA256Hex([]byte(nextState)),
		MutationID:     domainsecurity.SHA256Hex([]byte(mutation)),
		AuthorityKeyID: fixture.anchor.AuthorityKeyID, AuthorityPublicKey: fixture.anchor.AuthorityPublicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func (fixture ownerFixture) resolveRequest(t *testing.T, advance domainsecurity.MonotonicHeadAdvanceRequestV1) domainsecurity.MonotonicHeadMutationResolveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadMutationResolveRequestV1(advance,
		domainsecurity.SHA256Hex([]byte("fresh challenge")),
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func (fixture ownerFixture) observe(t *testing.T, owner *Owner) domainsecurity.MonotonicHeadObservationV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: fixture.anchor.InstallationID, EnrollmentID: fixture.anchor.EnrollmentID,
		Namespace:      domainenrollment.SharedEvidenceNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("fresh observe challenge")),
		AuthorityKeyID: fixture.anchor.AuthorityKeyID, AuthorityPublicKey: fixture.anchor.AuthorityPublicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	observation, err := owner.Observe(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.ValidateMonotonicHeadObservationForRequestV1(observation, request,
		fixture.anchor.InstallationID, fixture.anchor.AuthorityKeyID, fixture.anchor.AuthorityPublicKey,
		fixture.anchor.EnrollmentID, fixture.anchor.GenesisCheckpoint.WitnessKeyID,
		mustWitnessPublic(fixture.anchor.GenesisCheckpoint)); err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestOwnerRestartRebuildsHeadAndExactMutationIndex(t *testing.T) {
	fixture := newOwnerFixture(t)
	owner := fixture.create(t)
	first := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "mutation one", "state one")
	firstReceipt, err := owner.Advance(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := fixture.advanceRequest(t, firstReceipt.Checkpoint, "mutation two", "state two")
	secondReceipt, err := owner.Advance(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	owner = fixture.open(t)
	defer owner.Close()
	if got := fixture.observe(t, owner).Checkpoint; got != secondReceipt.Checkpoint {
		t.Fatal("restart lost the latest head")
	}
	resolution, err := owner.ResolveMutation(context.Background(), fixture.resolveRequest(t, first))
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != domainsecurity.MonotonicHeadMutationResolutionCommittedV1 ||
		resolution.Committed == nil || resolution.Committed.AdvanceReceipt != firstReceipt {
		t.Fatal("restart did not reconstruct the original committed receipt")
	}
	replay, err := owner.Advance(context.Background(), first)
	if err != nil || replay != firstReceipt {
		t.Fatalf("exact retry changed receipt: %v", err)
	}
	conflicting := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "mutation one", "different state")
	if _, err := owner.Advance(context.Background(), conflicting); !errors.Is(err, monotonichead.ErrMutationConflict) {
		t.Fatalf("mutation conflict: %v", err)
	}
	if _, err := owner.ResolveMutation(context.Background(), fixture.resolveRequest(t, conflicting)); !errors.Is(err, monotonichead.ErrMutationConflict) {
		t.Fatalf("resolve conflict: %v", err)
	}
	stale := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "mutation three", "state three")
	if _, err := owner.Advance(context.Background(), stale); !errors.Is(err, monotonichead.ErrCASConflict) {
		t.Fatalf("stale CAS: %v", err)
	}
	if _, err := createForIsolatedFixture(context.Background(), fixture.userData, fixture.anchor, fixture.witnessPrivate); err == nil {
		t.Fatal("existing owner was reinitialized")
	}
}

func TestOwnerCrashCutsRecoverOnlyPublishedGeneration(t *testing.T) {
	for _, cut := range []struct {
		stage     string
		committed bool
	}{
		{"after_temp_sync", false}, {"after_publish", true}, {"after_directory_sync", true},
	} {
		t.Run(cut.stage, func(t *testing.T) {
			fixture := newOwnerFixture(t)
			owner := fixture.create(t)
			request := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "cut mutation", "next state")
			owner.writeFault = func(stage string) error {
				if stage == cut.stage {
					return errors.New("injected crash cut")
				}
				return nil
			}
			if _, err := owner.Advance(context.Background(), request); !errors.Is(err, monotonichead.ErrIndeterminate) {
				t.Fatalf("advance cut: %v", err)
			}
			if _, err := owner.Advance(context.Background(), request); !errors.Is(err, monotonichead.ErrUnavailable) {
				t.Fatalf("poisoned owner served a retry: %v", err)
			}
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
			owner = fixture.open(t)
			defer owner.Close()
			resolution, err := owner.ResolveMutation(context.Background(), fixture.resolveRequest(t, request))
			if err != nil {
				t.Fatal(err)
			}
			if cut.committed {
				if resolution.Status != domainsecurity.MonotonicHeadMutationResolutionCommittedV1 || resolution.Committed == nil ||
					resolution.CurrentCheckpoint.Generation != 1 {
					t.Fatal("published record was not recovered")
				}
				replay, err := owner.Advance(context.Background(), request)
				if err != nil || replay != resolution.Committed.AdvanceReceipt {
					t.Fatalf("committed response lost on restart: %v", err)
				}
			} else if resolution.Status != domainsecurity.MonotonicHeadMutationResolutionAbsentV1 ||
				resolution.CurrentCheckpoint.Generation != 0 {
				t.Fatal("unpublished temp advanced the head")
			}
		})
	}
}

func TestOwnerReplayFailsWhenPinnedRootChanges(t *testing.T) {
	fixture := newOwnerFixture(t)
	owner := fixture.create(t)
	defer owner.Close()
	request := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "mutation before root change", "next state")
	if _, err := owner.Advance(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(fixture.root, fixture.root+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Advance(context.Background(), request); !errors.Is(err, monotonichead.ErrUnavailable) {
		t.Fatalf("replay succeeded after owner root identity changed: %v", err)
	}
}

func TestOwnerRestartRejectsGapAndTamper(t *testing.T) {
	for _, damage := range []struct {
		name  string
		apply func(t *testing.T, stateDir string)
	}{
		{"gap", func(t *testing.T, stateDir string) {
			if err := os.Remove(filepath.Join(stateDir, journalName(1))); err != nil {
				t.Fatal(err)
			}
		}},
		{"broken-chain", func(t *testing.T, stateDir string) {
			path := filepath.Join(stateDir, journalName(2))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record journalRecord
			if err := json.Unmarshal(body, &record); err != nil {
				t.Fatal(err)
			}
			record.PreviousRecordDigest = domainsecurity.SHA256Hex([]byte("wrong predecessor"))
			body, err = json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"record-tamper", func(t *testing.T, stateDir string) {
			path := filepath.Join(stateDir, journalName(1))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body[len(body)-2] ^= 1
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(damage.name, func(t *testing.T) {
			fixture := newOwnerFixture(t)
			owner := fixture.create(t)
			first := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "first", "first state")
			firstReceipt, err := owner.Advance(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			second := fixture.advanceRequest(t, firstReceipt.Checkpoint, "second", "second state")
			if _, err := owner.Advance(context.Background(), second); err != nil {
				t.Fatal(err)
			}
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
			damage.apply(t, filepath.Join(fixture.root, stateDirectoryName))
			if reopened, err := OpenExisting(context.Background(), fixture.userData, fixture.anchor); err == nil {
				reopened.Close()
				t.Fatal("damaged owner reopened")
			}
		})
	}
}

func TestOwnerIndependentLockAndNoReenrollment(t *testing.T) {
	if root := os.Getenv("ANALYTIX_TEST_OWNER_LOCK_ROOT"); root != "" {
		lease, err := persistencefs.AcquireSeparateOwnerLifetimeLease(root)
		if lease != nil {
			lease.Close()
		}
		if !errors.Is(err, persistencefs.ErrPersistenceInUse) {
			t.Fatalf("competing process acquired owner root: %v", err)
		}
		return
	}
	fixture := newOwnerFixture(t)
	if _, err := OpenExisting(context.Background(), fixture.userData, fixture.anchor); err == nil {
		t.Fatal("missing genesis was accepted")
	}
	owner := fixture.create(t)
	dataDir := filepath.Join(filepath.Dir(fixture.userData), "go-data")
	durableDir := filepath.Join(filepath.Dir(fixture.userData), "go-durable")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(durableDir, 0o700); err != nil {
		t.Fatal(err)
	}
	goLease, err := persistencefs.AcquireCompositeLeaseWithStartupUserData(
		persistencefs.RootSet{DataDir: dataDir, DurableDir: durableDir}, fixture.userData,
	)
	if err != nil {
		t.Fatalf("owner lease unexpectedly held the Go persistence roots: %v", err)
	}
	if err := goLease.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestOwnerIndependentLockAndNoReenrollment$")
	command.Env = append(os.Environ(), "ANALYTIX_TEST_OWNER_LOCK_ROOT="+fixture.root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("competing process lock check failed: %v: %s", err, output)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	otherPublic, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = otherPrivate
	otherAnchor := fixture.anchor
	otherAnchor.AuthorityPublicKey = otherPublic
	otherAnchor.AuthorityKeyID = domainsecurity.SHA256Hex(otherPublic)
	if opened, err := OpenExisting(context.Background(), fixture.userData, otherAnchor); err == nil {
		opened.Close()
		t.Fatal("different installation authority accepted the persisted owner")
	}
	owner = fixture.open(t)
	owner.Close()
}
