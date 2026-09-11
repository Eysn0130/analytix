//go:build darwin || linux || windows

package finalauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPrivateCASRecoveryV4SignedCleanup(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	journal := newMemoryPrivateCASRecoveryJournalV1()

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err != nil {
		t.Fatalf("signed V4 cleanup: %v", err)
	}
	if residue := fixture.residueBodies(t); len(residue) != 0 {
		t.Fatalf("signed V4 cleanup retained residue: %q", residue)
	}
	fixture.requireCommittedRecord(t)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.retireCount != 1 || journal.lastManifest == nil || journal.lastWitness == nil ||
		journal.lastCompletion == nil || journal.beginCount != 1 {
		t.Fatalf(
			"signed lifecycle was incomplete: begin=%d retire=%d manifest=%v witness=%v completion=%v",
			journal.beginCount,
			journal.retireCount,
			journal.lastManifest != nil,
			journal.lastWitness != nil,
			journal.lastCompletion != nil,
		)
	}
}

func TestPrivateCASRecoveryV4NoTargetPreflightKeepsLiveGeneration(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	live, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	journal := newMemoryPrivateCASRecoveryJournalV1()

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err != nil {
		t.Fatalf("no-target V4 preflight: %v", err)
	}
	files, err := live.List(context.Background())
	if err != nil || len(files) != 1 {
		t.Fatalf("no-target preflight revoked or changed a live generation: files=%#v err=%v", files, err)
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.beginCount != 0 || journal.retireCount != 0 {
		t.Fatalf("no-target preflight created a signed recovery session: begin=%d retire=%d", journal.beginCount, journal.retireCount)
	}
}

func TestPrivateCASRecoveryV4FrozenOptionalParticipantRetainsPlainResidue(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	before, err := os.Stat(fixture.ordinaryResidue)
	if err != nil {
		t.Fatal(err)
	}
	participants := fixture.participants(t)
	participants[0].Authority = frozenPrivateCASRecoveryV4Authority{participants[0].Authority}
	journal := newMemoryPrivateCASRecoveryJournalV1()

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), participants, journal,
	); err != nil {
		t.Fatalf("frozen optional V4 preflight: %v", err)
	}
	after, err := os.Stat(fixture.ordinaryResidue)
	body, readErr := os.ReadFile(fixture.ordinaryResidue)
	if err != nil || readErr != nil || !os.SameFile(before, after) ||
		!bytes.Equal(body, []byte(privateCASRecoveryV4ResidueBody)) {
		t.Fatalf("frozen optional V4 participant changed plain residue: stat=%v read=%v", err, readErr)
	}
	fixture.requireCommittedRecord(t)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.beginCount != 0 || journal.retireCount != 0 {
		t.Fatalf("frozen optional V4 participant created a cleanup session: begin=%d retire=%d", journal.beginCount, journal.retireCount)
	}
}

func TestPrivateCASRecoveryV4FrozenParticipantSkipsEveryMutationPhaseWhileSiblingConverges(t *testing.T) {
	frozen := newPrivateCASRecoveryV4FixtureForOwner(t, "evidence-registry", []string{"capsules", "indexes"})
	sibling := newPrivateCASRecoveryV4Fixture(t)
	beforeDigest := privateCASRecoveryV4WholeTreeDigest(t, frozen.ownerRoot)
	beforeIdentity := privateCASRecoveryV4TreeIdentity(t, frozen.ownerRoot)
	participants := frozen.participants(t)
	participants[0].Authority = frozenPrivateCASRecoveryV4Authority{participants[0].Authority}
	participants = append(participants, sibling.participants(t)...)
	journal := newMemoryPrivateCASRecoveryJournalV1()

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), participants, journal,
	); err != nil {
		t.Fatalf("cross-participant frozen V4 cleanup: %v", err)
	}
	privateCASRecoveryV4RequireFrozenTree(t, frozen, beforeDigest, beforeIdentity)
	if _, err := os.Lstat(sibling.ordinaryResidue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-frozen sibling residue did not converge: %v", err)
	}
	sibling.requireCommittedRecord(t)
	privateCASRecoveryV4RequireRetiredJournal(t, journal)
}

func TestPrivateCASRecoveryV4FrozenParticipantKeepsGenerationWhileSiblingConverges(t *testing.T) {
	frozen := newPrivateCASRecoveryV4FixtureForOwner(t, "evidence-registry", []string{"capsules", "indexes"})
	if err := os.Remove(frozen.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	sibling := newPrivateCASRecoveryV4Fixture(t)
	frozenLive, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(frozen.ownerRoot, "indexes"), 4096, frozen.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer frozenLive.Close()
	participants := frozen.participants(t)
	participants[0].Authority = frozenPrivateCASRecoveryV4Authority{participants[0].Authority}
	participants = append(participants, sibling.participants(t)...)

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), participants, newMemoryPrivateCASRecoveryJournalV1(),
	); err != nil {
		t.Fatalf("cross-participant generation retirement: %v", err)
	}
	if files, err := frozenLive.List(context.Background()); err != nil || len(files) != 1 {
		t.Fatalf("frozen registry generation was retired: files=%#v err=%v", files, err)
	}
	if _, err := os.Lstat(sibling.ordinaryResidue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-frozen sibling residue did not converge: %v", err)
	}
}

func TestPrivateCASRecoveryV4FrozenParticipantSkipsRollbackAfterSiblingStageFailure(t *testing.T) {
	frozen := newPrivateCASRecoveryV4FixtureForOwner(t, "evidence-registry", []string{"capsules", "indexes"})
	sibling := newPrivateCASRecoveryV4Fixture(t)
	beforeDigest := privateCASRecoveryV4WholeTreeDigest(t, frozen.ownerRoot)
	beforeIdentity := privateCASRecoveryV4TreeIdentity(t, frozen.ownerRoot)
	participants := frozen.participants(t)
	participants[0].Authority = frozenPrivateCASRecoveryV4Authority{participants[0].Authority}
	participants = append(participants, sibling.participants(t)...)
	journal := newMemoryPrivateCASRecoveryJournalV1()
	setPrivateCASRecoveryV4TestHook(t, func(phase string, planIndex int) error {
		if phase == "after_v4_plan_stage" && planIndex == 3 {
			return errors.New("stop after non-frozen sibling stage")
		}
		return nil
	})

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), participants, journal,
	); err == nil {
		t.Fatal("cross-participant stage failure did not stop recovery")
	}
	privateCASRecoveryV4RequireFrozenTree(t, frozen, beforeDigest, beforeIdentity)
	if staged, committed := sibling.recoveryPhaseCounts(t); staged != 1 || committed != 0 {
		t.Fatalf("non-frozen sibling did not retain the expected rollback input: staged=%d committed=%d", staged, committed)
	}
	setPrivateCASRecoveryV4TestHook(t, nil)
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), participants, journal,
	); err != nil {
		t.Fatalf("cross-participant rollback and retry: %v", err)
	}
	privateCASRecoveryV4RequireFrozenTree(t, frozen, beforeDigest, beforeIdentity)
	if _, err := os.Lstat(sibling.ordinaryResidue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-frozen sibling residue did not converge after rollback: %v", err)
	}
	if staged, committed := sibling.recoveryPhaseCounts(t); staged != 0 || committed != 0 {
		t.Fatalf("non-frozen sibling retained transaction phases after retry: staged=%d committed=%d", staged, committed)
	}
	privateCASRecoveryV4RequireRetiredJournal(t, journal)
}

func TestPrivateCASRecoveryV4FrozenOptionalParticipantRejectsTransactionPhases(t *testing.T) {
	for _, test := range []struct {
		name  string
		phase privateCASRecoveryQuarantinePhase
	}{
		{name: "staged", phase: privateCASRecoveryQuarantineStaged},
		{name: "committed", phase: privateCASRecoveryQuarantineCommitted},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryV4Fixture(t)
			transactionID := strings.Repeat("d", 64)
			phaseName, ok := privateCASRecoveryQuarantineName(
				test.phase, transactionID, filepath.Base(fixture.ordinaryResidue), privateCASRecoveryV4Digest[:2],
			)
			if !ok {
				t.Fatal("could not construct frozen transaction phase")
			}
			phasePath := filepath.Join(filepath.Dir(fixture.ordinaryResidue), phaseName)
			if err := os.Rename(fixture.ordinaryResidue, phasePath); err != nil {
				t.Fatal(err)
			}
			participants := fixture.participants(t)
			participants[0].Authority = frozenPrivateCASRecoveryV4Authority{participants[0].Authority}
			journal := newMemoryPrivateCASRecoveryJournalV1()

			if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
				context.Background(), participants, journal,
			); err == nil {
				t.Fatal("frozen optional participant accepted a staged or committed target")
			}
			body, err := os.ReadFile(phasePath)
			if err != nil || !bytes.Equal(body, []byte(privateCASRecoveryV4ResidueBody)) {
				t.Fatalf("frozen phase rejection changed residue: body=%q err=%v", body, err)
			}
			journal.mu.Lock()
			defer journal.mu.Unlock()
			if journal.beginCount != 0 || journal.retireCount != 0 {
				t.Fatalf("frozen phase rejection touched signed journal: begin=%d retire=%d", journal.beginCount, journal.retireCount)
			}
		})
	}
}

func TestPrivateCASRecoveryV4FrozenOptionalParticipantRejectsExistingSignedSession(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	journal := newMemoryPrivateCASRecoveryJournalV1()
	setPrivateCASRecoveryV4TestHook(t, func(phase string, _ int) error {
		if phase == "after_v4_internal_marker" {
			return errors.New("stop after signed session preparation")
		}
		return nil
	})
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err == nil {
		t.Fatal("signed session preparation crash cut did not stop recovery")
	}
	setPrivateCASRecoveryV4TestHook(t, nil)
	participants := fixture.participants(t)
	participants[0].Authority = frozenPrivateCASRecoveryV4Authority{participants[0].Authority}
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), participants, journal,
	); err == nil {
		t.Fatal("frozen optional participant absorbed an existing signed cleanup session")
	}
	if bodies := fixture.residueBodies(t); len(bodies) != 1 ||
		!bytes.Equal(bodies[0], []byte(privateCASRecoveryV4ResidueBody)) {
		t.Fatalf("signed-session conflict changed frozen residue: %q", bodies)
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.retireCount != 0 || journal.lastWitness != nil || journal.lastCompletion != nil {
		t.Fatalf("signed-session conflict advanced cleanup: retire=%d witness=%v completion=%v",
			journal.retireCount, journal.lastWitness != nil, journal.lastCompletion != nil)
	}
}

func TestPrivateCASSemanticApplyRetirementRevokesLiveGenerationAfterValidatedPreflight(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	live, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	called := false
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(context.Context) error {
			called = true
			return nil
		},
	); err != nil {
		t.Fatalf("semantic apply generation retirement: %v", err)
	}
	if !called {
		t.Fatal("semantic apply callback was not invoked")
	}
	if _, err := live.List(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "revoked") {
		t.Fatalf("semantic apply retained a stale live generation: %v", err)
	}
}

func TestPrivateCASSemanticApplyWithNoPrivateRootsRunsUnderRecoveryExclusion(t *testing.T) {
	called := false
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), nil, nil, func(context.Context) error {
			probeContext, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			err := privateCASRecoveryExclusion.acquireRecovery(probeContext)
			if err == nil {
				privateCASRecoveryExclusion.releaseRecovery()
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("semantic apply callback did not retain recovery exclusion: %w", err)
			}
			called = true
			return nil
		},
	); err != nil {
		t.Fatalf("empty semantic apply authority: %v", err)
	}
	if !called {
		t.Fatal("empty semantic apply callback was not invoked")
	}
}

func TestPrivateCASSemanticApplyWithNoParticipantsRejectsAffectedRoot(t *testing.T) {
	called := false
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), nil, []string{"accepted-finals/records"}, func(context.Context) error {
			called = true
			return nil
		},
	); err == nil || !strings.Contains(err.Error(), "unknown root id") {
		t.Fatalf("empty semantic apply authority accepted an affected root: %v", err)
	}
	if called {
		t.Fatal("empty semantic apply authority invoked callback for an affected root")
	}
}

func TestPrivateCASRecoveryV4RejectsEmptyParticipantManifest(t *testing.T) {
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), nil, newMemoryPrivateCASRecoveryJournalV1(),
	); err == nil || !strings.Contains(err.Error(), "participant manifest is required") {
		t.Fatalf("recovery accepted an empty participant manifest: %v", err)
	}
}

func TestPrivateCASSemanticApplyRetirementRejectsRecoveryTargets(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	live, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := os.WriteFile(
		fixture.ordinaryResidue, []byte(privateCASRecoveryV4ResidueBody), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(context.Context) error {
			called = true
			return nil
		},
	); err == nil || !strings.Contains(err.Error(), "unresolved recovery targets") {
		t.Fatalf("semantic apply accepted a recovery target: %v", err)
	}
	if called {
		t.Fatal("semantic apply callback ran with an unresolved recovery target")
	}
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	files, err := live.List(context.Background())
	if err != nil || len(files) != 1 {
		t.Fatalf("rejected semantic apply retirement changed live generation: files=%#v err=%v", files, err)
	}
}

func TestPrivateCASSemanticApplyRetirementHoldsRecoveryExclusionThroughCallback(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(context.Context) error {
			probeContext, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			_, err := OpenSecurePrivateCASWithAccessAuthorityContext(
				probeContext, filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
			)
			if !errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("live generation attached during semantic apply callback: %w", err)
			}
			return nil
		},
	); err != nil {
		t.Fatalf("semantic apply recovery exclusion: %v", err)
	}
	reopened, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatalf("live generation did not reopen after semantic apply callback: %v", err)
	}
	defer reopened.Close()
}

func TestPrivateCASSemanticApplyRetirementPreservesUnaffectedGeneration(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	records, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer records.Close()
	dispositions, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "dispositions"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dispositions.Close()
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(context.Context) error {
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := records.List(context.Background()); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("affected generation remained live: %v", err)
	}
	if files, err := dispositions.List(context.Background()); err != nil || len(files) != 0 {
		t.Fatalf("unaffected generation was changed: files=%#v err=%v", files, err)
	}
}

func TestPrivateCASSemanticApplyRetirementRejectsUnknownRootID(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	live, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	called := false
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), fixture.participants(t),
		[]string{"accepted-finals/records", "accepted-finals/unknown"},
		func(context.Context) error {
			called = true
			return nil
		},
	); err == nil || !strings.Contains(err.Error(), "unknown root id") {
		t.Fatalf("semantic apply accepted an unknown root id: %v", err)
	}
	if called {
		t.Fatal("semantic apply callback ran with an unknown root id")
	}
	if files, err := live.List(context.Background()); err != nil || len(files) != 1 {
		t.Fatalf("unknown root id partially retired a valid generation: files=%#v err=%v", files, err)
	}
}

func TestPrivateCASSemanticApplyCallbackFailureReleasesBarrierWithAffectedGenerationRevoked(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	live, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	injected := errors.New("semantic apply failed")
	if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(context.Context) error {
			return injected
		},
	); !errors.Is(err, injected) {
		t.Fatalf("semantic apply callback failure was not propagated: %v", err)
	}
	if _, err := live.List(context.Background()); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("failed semantic apply retained affected generation: %v", err)
	}
	reopened, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(fixture.ownerRoot, "records"), 4096, fixture.access,
	)
	if err != nil {
		t.Fatalf("semantic apply callback failure retained recovery barrier: %v", err)
	}
	defer reopened.Close()
}

func TestPrivateCASRecoveryV4InternalMarkerCrashDoesNotDeleteAndFreshPrepareResumes(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	journal := newMemoryPrivateCASRecoveryJournalV1()
	setPrivateCASRecoveryV4TestHook(t, func(phase string, _ int) error {
		if phase == "after_v4_internal_marker" {
			return errors.New("stop after internal marker")
		}
		return nil
	})

	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err == nil {
		t.Fatal("internal-marker crash cut did not stop V4 recovery")
	}
	if bodies := fixture.residueBodies(t); len(bodies) != 1 || string(bodies[0]) != privateCASRecoveryV4ResidueBody {
		t.Fatalf("internal marker deleted or changed residue: %q", bodies)
	}
	fixture.requireCommittedRecord(t)
	journal.mu.Lock()
	if journal.witness != nil || journal.completion != nil || journal.manifest == nil {
		journal.mu.Unlock()
		t.Fatal("internal marker was incorrectly promoted to an external commit witness")
	}
	journal.mu.Unlock()

	setPrivateCASRecoveryV4TestHook(t, nil)
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err != nil {
		t.Fatalf("resume after internal-marker crash: %v", err)
	}
	if residue := fixture.residueBodies(t); len(residue) != 0 {
		t.Fatalf("resumed signed recovery retained residue: %q", residue)
	}
	fixture.requireCommittedRecord(t)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.retireCount != 1 || journal.lastWitness == nil || journal.lastCompletion == nil {
		t.Fatalf("resumed signed recovery did not complete exactly once: retire=%d", journal.retireCount)
	}
}

func TestPrivateCASRecoveryV4UsesOneInternalMarkerAcrossMultiplePlans(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	secondDigest := "bb22222222222222222222222222222222222222222222222222222222222222"
	secondShard := filepath.Join(fixture.ownerRoot, "dispositions", secondDigest[:2])
	if err := os.Mkdir(secondShard, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	secondResidue := filepath.Join(
		secondShard,
		"."+secondDigest+".json-abcdefabcdefabcdefabcdef.tmp",
	)
	if err := os.WriteFile(secondResidue, []byte("second-plan-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := newMemoryPrivateCASRecoveryJournalV1()
	setPrivateCASRecoveryV4TestHook(t, func(phase string, _ int) error {
		if phase == "after_v4_internal_marker" {
			return errors.New("stop after the unique internal marker")
		}
		return nil
	})
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err == nil {
		t.Fatal("multi-plan internal-marker crash cut did not stop V4 recovery")
	}
	staged, committed := fixture.recoveryPhaseCounts(t)
	if staged != 1 || committed != 1 {
		t.Fatalf("multi-plan transaction phases = staged:%d committed:%d, want 1:1", staged, committed)
	}
	if bodies := fixture.residueBodies(t); len(bodies) != 2 {
		t.Fatalf("multi-plan marker cut changed target count: %q", bodies)
	}
	fixture.requireCommittedRecord(t)

	setPrivateCASRecoveryV4TestHook(t, nil)
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err != nil {
		t.Fatalf("resume multi-plan signed recovery: %v", err)
	}
	if residue := fixture.residueBodies(t); len(residue) != 0 {
		t.Fatalf("multi-plan signed recovery retained residue: %q", residue)
	}
	fixture.requireCommittedRecord(t)
}

func TestPrivateCASRecoveryV4CommitWitnessRejectsReplacementParentAcrossRestart(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	journal := newMemoryPrivateCASRecoveryJournalV1()
	originalTopologyDigest := fixture.owner(t).PrivateCASRecoveryTopologyDigestV4()
	setPrivateCASRecoveryV4TestHook(t, func(phase string, _ int) error {
		if phase == "after_v4_commit_witness" {
			return errors.New("stop after external commit witness")
		}
		return nil
	})
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err == nil {
		t.Fatal("commit-witness crash cut did not stop V4 recovery")
	}
	setPrivateCASRecoveryV4TestHook(t, nil)
	if bodies := fixture.residueBodies(t); len(bodies) != 1 || string(bodies[0]) != privateCASRecoveryV4ResidueBody {
		t.Fatalf("commit-witness cut deleted or changed residue: %q", bodies)
	}

	backup := fixture.ownerRoot + ".original"
	if err := os.Rename(fixture.ownerRoot, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.ownerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, leaf := range privateCASRecoveryV4Leaves {
		if err := os.Rename(filepath.Join(backup, leaf), filepath.Join(fixture.ownerRoot, leaf)); err != nil {
			t.Fatal(err)
		}
	}
	replacement := fixture.owner(t)
	if replacement.PrivateCASRecoveryTopologyDigestV4() == originalTopologyDigest {
		t.Fatal("replacement parent retained the signed original topology digest")
	}
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participantsWithOwner(replacement), journal,
	); err == nil {
		t.Fatal("fresh restart accepted a signed journal for a replacement parent")
	}
	if bodies := fixture.residueBodies(t); len(bodies) != 1 || string(bodies[0]) != privateCASRecoveryV4ResidueBody {
		t.Fatalf("replacement-parent rejection deleted or changed residue: %q", bodies)
	}
	journal.mu.Lock()
	if journal.retireCount != 0 || journal.witness == nil || journal.completion != nil {
		journal.mu.Unlock()
		t.Fatal("replacement-parent rejection changed the witnessed journal frontier")
	}
	journal.mu.Unlock()

	for _, leaf := range privateCASRecoveryV4Leaves {
		if err := os.Rename(filepath.Join(fixture.ownerRoot, leaf), filepath.Join(backup, leaf)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(fixture.ownerRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, fixture.ownerRoot); err != nil {
		t.Fatal(err)
	}
	restored := fixture.owner(t)
	if restored.PrivateCASRecoveryTopologyDigestV4() != originalTopologyDigest {
		t.Fatalf(
			"restored original parent changed its durable topology digest: before=%s after=%s",
			originalTopologyDigest,
			restored.PrivateCASRecoveryTopologyDigestV4(),
		)
	}
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participantsWithOwner(restored), journal,
	); err != nil {
		t.Fatalf("resume after restoring original parent: %v", err)
	}
	if residue := fixture.residueBodies(t); len(residue) != 0 {
		t.Fatalf("restored-parent resume retained residue: %q", residue)
	}
	fixture.requireCommittedRecord(t)
}

func TestPrivateCASRecoveryV4WitnessedCrashFrontiersResumeWithoutRollback(t *testing.T) {
	for _, test := range []struct {
		name                 string
		phase                string
		expectResidue        bool
		expectCompletionSeen bool
	}{
		{name: "before-marker-finalize", phase: "before_commit_finalize", expectResidue: true},
		{name: "after-cleanup", phase: "after_v4_cleanup"},
		{name: "after-completion-receipt", phase: "after_v4_completion_receipt", expectCompletionSeen: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryV4Fixture(t)
			journal := newMemoryPrivateCASRecoveryJournalV1()
			setPrivateCASRecoveryV4TestHook(t, func(phase string, _ int) error {
				if phase == test.phase {
					return errors.New("stop at witnessed crash frontier")
				}
				return nil
			})
			if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
				context.Background(), fixture.participants(t), journal,
			); err == nil {
				t.Fatal("witnessed crash cut did not stop V4 recovery")
			}
			bodies := fixture.residueBodies(t)
			if test.expectResidue {
				if len(bodies) != 1 || string(bodies[0]) != privateCASRecoveryV4ResidueBody {
					t.Fatalf("witnessed pre-finalize crash changed residue: %q", bodies)
				}
			} else if len(bodies) != 0 {
				t.Fatalf("post-cleanup crash regained residue: %q", bodies)
			}
			fixture.requireCommittedRecord(t)
			journal.mu.Lock()
			if journal.witness == nil || (journal.completion != nil) != test.expectCompletionSeen ||
				journal.retireCount != 0 || journal.beginCount != 1 {
				journal.mu.Unlock()
				t.Fatal("witnessed crash changed the signed journal frontier")
			}
			journal.mu.Unlock()

			setPrivateCASRecoveryV4TestHook(t, nil)
			if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
				context.Background(), fixture.participants(t), journal,
			); err != nil {
				t.Fatalf("resume witnessed V4 recovery: %v", err)
			}
			if residue := fixture.residueBodies(t); len(residue) != 0 {
				t.Fatalf("resumed witnessed recovery retained residue: %q", residue)
			}
			fixture.requireCommittedRecord(t)
			journal.mu.Lock()
			defer journal.mu.Unlock()
			if journal.retireCount != 1 || journal.beginCount != 1 || journal.lastCompletion == nil {
				t.Fatalf(
					"witnessed recovery did not retire exactly once: begin=%d retire=%d",
					journal.beginCount,
					journal.retireCount,
				)
			}
		})
	}
}

func TestPrivateCASRecoveryV4DeletionRequiresFreshWitnessVerification(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	journal := newMemoryPrivateCASRecoveryJournalV1()
	setPrivateCASRecoveryV4TestHook(t, func(phase string, _ int) error {
		if phase == "before_commit_finalize" {
			return errors.New("stop after signed witness")
		}
		return nil
	})
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(), fixture.participants(t), journal,
	); err == nil {
		t.Fatal("signed-witness crash cut did not stop recovery")
	}
	setPrivateCASRecoveryV4TestHook(t, nil)
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		context.Background(),
		fixture.participants(t),
		rejectingPrivateCASRecoveryWitnessVerifier{RecoveryJournalV1: journal},
	); err == nil {
		t.Fatal("deletion proceeded without a fresh witness verification")
	}
	if residue := fixture.residueBodies(t); len(residue) != 1 || string(residue[0]) != privateCASRecoveryV4ResidueBody {
		t.Fatalf("failed witness verification changed signed residue: %q", residue)
	}
	fixture.requireCommittedRecord(t)
}

func TestPrivateCASRecoveryV4RejectsUnsignedLegacyTransactionResidueWithoutJournal(t *testing.T) {
	for _, test := range []struct {
		name  string
		phase privateCASRecoveryQuarantinePhase
	}{
		{name: "staged", phase: privateCASRecoveryQuarantineStaged},
		{name: "committed", phase: privateCASRecoveryQuarantineCommitted},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryV4Fixture(t)
			journal := newMemoryPrivateCASRecoveryJournalV1()
			transactionID := strings.Repeat("c", 64)
			legacyName, ok := privateCASRecoveryQuarantineName(
				test.phase,
				transactionID,
				filepath.Base(fixture.ordinaryResidue),
				privateCASRecoveryV4Digest[:2],
			)
			if !ok {
				t.Fatal("could not construct legacy V2 residue name")
			}
			legacyPath := filepath.Join(filepath.Dir(fixture.ordinaryResidue), legacyName)
			if err := os.Rename(fixture.ordinaryResidue, legacyPath); err != nil {
				t.Fatal(err)
			}
			if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
				context.Background(), fixture.participants(t), journal,
			); err == nil {
				t.Fatal("unsigned legacy transaction residue was accepted without a signed journal")
			}
			body, err := os.ReadFile(legacyPath)
			if err != nil || string(body) != privateCASRecoveryV4ResidueBody {
				t.Fatalf("unsigned legacy rejection changed residue: body=%q err=%v", body, err)
			}
			fixture.requireCommittedRecord(t)
			journal.mu.Lock()
			defer journal.mu.Unlock()
			if journal.preparation != nil || journal.beginCount != 0 || journal.retireCount != 0 {
				t.Fatal("unsigned legacy residue created or retired signed journal authority")
			}
		})
	}
}

const (
	privateCASRecoveryV4Digest      = "aa11111111111111111111111111111111111111111111111111111111111111"
	privateCASRecoveryV4ResidueBody = "signed-v4-recovery-residue"
)

var privateCASRecoveryV4Leaves = []string{"dispositions", "records"}

type privateCASRecoveryV4Fixture struct {
	authorityRoot   string
	ownerRoot       string
	participantID   string
	leaves          []string
	access          *privatecastest.AccessAuthority
	ordinaryResidue string
	committedBody   []byte
}

type frozenPrivateCASRecoveryV4Authority struct {
	PreparedSecurePrivateCASRecoveryAuthorityV3
}

func (frozenPrivateCASRecoveryV4Authority) FreezeSecurePrivateCASRecoveryTargetsV4() bool {
	return true
}

func newPrivateCASRecoveryV4Fixture(t *testing.T) *privateCASRecoveryV4Fixture {
	return newPrivateCASRecoveryV4FixtureForOwner(t, "accepted-finals", privateCASRecoveryV4Leaves)
}

func newPrivateCASRecoveryV4FixtureForOwner(
	t *testing.T,
	participantID string,
	leaves []string,
) *privateCASRecoveryV4Fixture {
	t.Helper()
	authorityRoot := t.TempDir()
	ownerRoot := filepath.Join(authorityRoot, participantID)
	access, err := privatecastest.NewAccessAuthority(authorityRoot)
	if err != nil {
		t.Fatal(err)
	}
	for index, leaf := range leaves {
		store, err := OpenSecurePrivateCASWithAccessAuthority(filepath.Join(ownerRoot, leaf), 4096, access)
		if err != nil {
			t.Fatal(err)
		}
		if index == len(leaves)-1 {
			if err := store.PutIfAbsent(
				context.Background(), privateCASRecoveryV4Digest, []byte(`{"record":"signed-v4"}`),
			); err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	ordinary := filepath.Join(
		ownerRoot,
		leaves[len(leaves)-1],
		privateCASRecoveryV4Digest[:2],
		"."+privateCASRecoveryV4Digest+".json-0123456789abcdef01234567.tmp",
	)
	if err := os.WriteFile(ordinary, []byte(privateCASRecoveryV4ResidueBody), 0o600); err != nil {
		t.Fatal(err)
	}
	return &privateCASRecoveryV4Fixture{
		authorityRoot:   authorityRoot,
		ownerRoot:       ownerRoot,
		participantID:   participantID,
		leaves:          append([]string(nil), leaves...),
		access:          access,
		ordinaryResidue: ordinary,
		committedBody:   []byte(`{"record":"signed-v4"}`),
	}
}

func (fixture *privateCASRecoveryV4Fixture) owner(t *testing.T) *PreparedSecurePrivateCASOwnerRecoveryV1 {
	t.Helper()
	leaves := make([]SecurePrivateCASOwnerLeafV1, 0, len(fixture.leaves))
	for _, leaf := range fixture.leaves {
		leaves = append(leaves, SecurePrivateCASOwnerLeafV1{Name: leaf, MaxBytes: 4096})
	}
	owner, err := PrepareSecurePrivateCASOwnerRecoveryV1(
		context.Background(),
		fixture.ownerRoot,
		leaves,
		fixture.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func (fixture *privateCASRecoveryV4Fixture) participants(t *testing.T) []PreparedSecurePrivateCASRecoveryParticipantV4 {
	t.Helper()
	return fixture.participantsWithOwner(fixture.owner(t))
}

func (fixture *privateCASRecoveryV4Fixture) participantsWithOwner(
	owner *PreparedSecurePrivateCASOwnerRecoveryV1,
) []PreparedSecurePrivateCASRecoveryParticipantV4 {
	roots := make([]SecurePrivateCASRecoveryRootBindingV4, 0, len(fixture.leaves))
	for _, leaf := range fixture.leaves {
		roots = append(roots, SecurePrivateCASRecoveryRootBindingV4{
			RootID: fixture.participantID + "/" + leaf, RootPath: filepath.Join(fixture.ownerRoot, leaf),
		})
	}
	return []PreparedSecurePrivateCASRecoveryParticipantV4{{
		ParticipantID:           fixture.participantID,
		SemanticAuthorityDigest: privateCASRecoveryV4DigestOf(fixture.participantID + "-semantic-authority-v1"),
		Authority:               owner,
		Roots:                   roots,
	}}
}

func (fixture *privateCASRecoveryV4Fixture) residueBodies(t *testing.T) [][]byte {
	t.Helper()
	bodies := make([][]byte, 0, 1)
	for _, leaf := range fixture.leaves {
		leafEntries, err := os.ReadDir(filepath.Join(fixture.ownerRoot, leaf))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, leafEntry := range leafEntries {
			if !leafEntry.IsDir() {
				continue
			}
			shard := filepath.Join(fixture.ownerRoot, leaf, leafEntry.Name())
			entries, err := os.ReadDir(shard)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				body, err := os.ReadFile(filepath.Join(shard, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				bodies = append(bodies, body)
			}
		}
	}
	return bodies
}

func (fixture *privateCASRecoveryV4Fixture) requireCommittedRecord(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(
		fixture.ownerRoot,
		fixture.leaves[len(fixture.leaves)-1],
		privateCASRecoveryV4Digest[:2],
		privateCASRecoveryV4Digest+".json",
	))
	if err != nil || !bytes.Equal(body, fixture.committedBody) {
		t.Fatalf("committed record changed: body=%q err=%v", body, err)
	}
}

func (fixture *privateCASRecoveryV4Fixture) recoveryPhaseCounts(t *testing.T) (staged int, committed int) {
	t.Helper()
	for _, leaf := range fixture.leaves {
		shards, err := os.ReadDir(filepath.Join(fixture.ownerRoot, leaf))
		if err != nil {
			t.Fatal(err)
		}
		for _, shard := range shards {
			if !shard.IsDir() {
				continue
			}
			entries, err := os.ReadDir(filepath.Join(fixture.ownerRoot, leaf, shard.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				switch {
				case strings.HasPrefix(entry.Name(), domainprivatecas.RecoveryStagePrefixV1):
					staged++
				case strings.HasPrefix(entry.Name(), domainprivatecas.RecoveryCommitPrefixV1):
					committed++
				}
			}
		}
	}
	return staged, committed
}

func privateCASRecoveryV4WholeTreeDigest(t *testing.T, root string) string {
	t.Helper()
	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		privateCASWriteFingerprintField(hasher, []byte(filepath.ToSlash(relative)))
		privateCASWriteFingerprintField(hasher, []byte(info.Mode().String()))
		privateCASWriteUint64V4(hasher, uint64(info.Size()))
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			privateCASWriteFingerprintField(hasher, body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func privateCASRecoveryV4TreeIdentity(t *testing.T, root string) map[string]os.FileInfo {
	t.Helper()
	identities := make(map[string]os.FileInfo)
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		identities[filepath.ToSlash(relative)] = info
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return identities
}

func privateCASRecoveryV4RequireFrozenTree(
	t *testing.T,
	fixture *privateCASRecoveryV4Fixture,
	wantDigest string,
	wantIdentity map[string]os.FileInfo,
) {
	t.Helper()
	if got := privateCASRecoveryV4WholeTreeDigest(t, fixture.ownerRoot); got != wantDigest {
		t.Fatalf("frozen participant whole tree changed: got=%s want=%s", got, wantDigest)
	}
	gotIdentity := privateCASRecoveryV4TreeIdentity(t, fixture.ownerRoot)
	if len(gotIdentity) != len(wantIdentity) {
		t.Fatalf("frozen participant identity count changed: got=%d want=%d", len(gotIdentity), len(wantIdentity))
	}
	for relative, before := range wantIdentity {
		after, found := gotIdentity[relative]
		if !found || !os.SameFile(before, after) {
			t.Fatalf("frozen participant identity changed at %q", relative)
		}
	}
	if staged, committed := fixture.recoveryPhaseCounts(t); staged != 0 || committed != 0 {
		t.Fatalf("frozen participant entered a transaction phase: staged=%d committed=%d", staged, committed)
	}
}

func privateCASRecoveryV4RequireRetiredJournal(t *testing.T, journal *memoryPrivateCASRecoveryJournalV1) {
	t.Helper()
	if _, err := journal.Load(context.Background()); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("completed recovery retained a signed journal: %v", err)
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.beginCount != 1 || journal.retireCount != 1 || journal.lastCompletion == nil {
		t.Fatalf(
			"signed recovery lifecycle was not retired: begin=%d retire=%d completion=%v",
			journal.beginCount, journal.retireCount, journal.lastCompletion != nil,
		)
	}
}

func setPrivateCASRecoveryV4TestHook(t *testing.T, hook func(string, int) error) {
	t.Helper()
	privateCASRecoveryTransactionTestHooks.Lock()
	privateCASRecoveryTransactionTestHooks.hook = hook
	privateCASRecoveryTransactionTestHooks.Unlock()
	if hook != nil {
		t.Cleanup(func() {
			privateCASRecoveryTransactionTestHooks.Lock()
			privateCASRecoveryTransactionTestHooks.hook = nil
			privateCASRecoveryTransactionTestHooks.Unlock()
		})
	}
}

type memoryPrivateCASRecoveryJournalV1 struct {
	mu sync.Mutex

	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string

	preparation *domainprivatecas.RecoveryJournalPreparationV1
	chunks      []domainprivatecas.RecoveryTargetChunkV1
	manifest    *domainprivatecas.RecoveryJournalManifestV1
	witness     *domainprivatecas.RecoveryJournalCommitWitnessV1
	completion  *domainprivatecas.RecoveryJournalCompletionReceiptV1

	beginCount  int
	retireCount int

	lastManifest   *domainprivatecas.RecoveryJournalManifestV1
	lastWitness    *domainprivatecas.RecoveryJournalCommitWitnessV1
	lastCompletion *domainprivatecas.RecoveryJournalCompletionReceiptV1
}

type rejectingPrivateCASRecoveryWitnessVerifier struct {
	privatecasport.RecoveryJournalV1
}

func (rejectingPrivateCASRecoveryWitnessVerifier) Verify(context.Context, string, []byte, string) error {
	return errors.New("test witness verification rejected")
}

func newMemoryPrivateCASRecoveryJournalV1() *memoryPrivateCASRecoveryJournalV1 {
	seed := sha256.Sum256([]byte("private-cas-recovery-v4-memory-journal-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &memoryPrivateCASRecoveryJournalV1{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      privateCASRecoveryV4DigestBytes(publicKey),
	}
}

func (journal *memoryPrivateCASRecoveryJournalV1) KeyID(ctx context.Context) (string, error) {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return "", err
	}
	return journal.keyID, nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) Sign(ctx context.Context, body []byte) (string, error) {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(journal.privateKey, append([]byte(nil), body...))), nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) Verify(
	ctx context.Context,
	keyID string,
	body []byte,
	encoded string,
) error {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return err
	}
	signature, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || keyID != journal.keyID || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(journal.publicKey, body, signature) {
		return errors.New("memory recovery journal signature is invalid")
	}
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) BeginPreparationAfterValidatedPreflight(
	ctx context.Context,
	request privatecasport.RecoveryPreparationRequestV1,
) (domainprivatecas.RecoveryJournalPreparationV1, error) {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	draft, err := domainprivatecas.NewRecoveryJournalPreparationDraftV1(
		domainprivatecas.RecoveryJournalPreparationInputV1{
			RootBindingDigest:        privateCASRecoveryV4DigestOf("memory-journal-root-binding"),
			AuthoritySetDigest:       request.AuthoritySetDigest,
			ParticipantCount:         request.ParticipantCount,
			PlanCount:                request.PlanCount,
			TopologyCount:            request.TopologyCount,
			JournalDirectoryIdentity: privateCASRecoveryV4DigestOf("memory-journal-directory"),
			TargetsDirectoryIdentity: privateCASRecoveryV4DigestOf("memory-targets-directory"),
			PreparedAt:               request.PreparedAt,
			AuthorityKeyID:           journal.keyID,
		},
	)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	body, err := domainprivatecas.RecoveryJournalPreparationSigningBytesV1(draft)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	signature, err := journal.Sign(ctx, body)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	preparation, err := domainprivatecas.SealRecoveryJournalPreparationV1(draft, signature)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	if err := journal.PutPreparationIfAbsent(ctx, preparation); err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	journal.mu.Lock()
	journal.beginCount++
	journal.mu.Unlock()
	return preparation, nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) Load(
	ctx context.Context,
) (domainprivatecas.RecoveryJournalSessionV1, error) {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.sessionLocked(ctx)
}

func (journal *memoryPrivateCASRecoveryJournalV1) PutPreparationIfAbsent(
	ctx context.Context,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
) error {
	if err := journal.verifyPreparation(ctx, preparation); err != nil {
		return err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.preparation != nil {
		return privateCASRecoveryV4RequireExactPreparation(*journal.preparation, preparation)
	}
	journal.preparation = privateCASRecoveryV4PreparationPointer(preparation)
	journal.chunks = nil
	journal.manifest = nil
	journal.witness = nil
	journal.completion = nil
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) PutTargetChunkIfAbsent(
	ctx context.Context,
	chunk domainprivatecas.RecoveryTargetChunkV1,
) error {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return err
	}
	if err := domainprivatecas.ValidateRecoveryTargetChunkV1(chunk); err != nil {
		return err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.preparation == nil {
		return errors.New("memory recovery journal has no preparation")
	}
	index := int(chunk.ChunkIndex)
	if index < len(journal.chunks) {
		return privateCASRecoveryV4RequireExactChunk(journal.chunks[index], chunk)
	}
	if index != len(journal.chunks) || journal.manifest != nil {
		return errors.New("memory recovery journal chunk sequence is not append-only")
	}
	journal.chunks = append(journal.chunks, privateCASRecoveryV4CloneChunk(chunk))
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) PutManifestIfAbsent(
	ctx context.Context,
	manifest domainprivatecas.RecoveryJournalManifestV1,
) error {
	if err := journal.verifyManifest(ctx, manifest); err != nil {
		return err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.manifest != nil {
		return privateCASRecoveryV4RequireExactManifest(*journal.manifest, manifest)
	}
	if journal.preparation == nil || journal.witness != nil || journal.completion != nil ||
		domainprivatecas.ValidateRecoveryJournalManifestForPreparationV1(manifest, *journal.preparation) != nil ||
		domainprivatecas.ValidateRecoveryJournalManifestForChunksV1(manifest, journal.chunks) != nil {
		return errors.New("memory recovery journal manifest is outside its prepared target set")
	}
	journal.manifest = privateCASRecoveryV4ManifestPointer(manifest)
	journal.lastManifest = privateCASRecoveryV4ManifestPointer(manifest)
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) PutCommitWitnessIfAbsent(
	ctx context.Context,
	witness domainprivatecas.RecoveryJournalCommitWitnessV1,
) error {
	if err := journal.verifyWitness(ctx, witness); err != nil {
		return err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.witness != nil {
		return privateCASRecoveryV4RequireExactWitness(*journal.witness, witness)
	}
	if journal.manifest == nil || journal.completion != nil ||
		domainprivatecas.ValidateRecoveryJournalCommitWitnessForManifestV1(witness, *journal.manifest) != nil ||
		!privateCASRecoveryV4ChunkContainsTarget(journal.chunks, witness.CommitTargetID) {
		return errors.New("memory recovery journal witness is outside its manifest")
	}
	journal.witness = privateCASRecoveryV4WitnessPointer(witness)
	journal.lastWitness = privateCASRecoveryV4WitnessPointer(witness)
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) PutCompletionReceiptIfAbsent(
	ctx context.Context,
	completion domainprivatecas.RecoveryJournalCompletionReceiptV1,
) error {
	if err := journal.verifyCompletion(ctx, completion); err != nil {
		return err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.completion != nil {
		return privateCASRecoveryV4RequireExactCompletion(*journal.completion, completion)
	}
	if journal.witness == nil ||
		domainprivatecas.ValidateRecoveryJournalCompletionForWitnessV1(completion, *journal.witness) != nil {
		return errors.New("memory recovery journal completion is outside its witness")
	}
	journal.completion = privateCASRecoveryV4CompletionPointer(completion)
	journal.lastCompletion = privateCASRecoveryV4CompletionPointer(completion)
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) Retire(
	ctx context.Context,
	request privatecasport.RecoveryRetirementRequestV1,
) error {
	if err := privateCASRecoveryV4ContextError(ctx); err != nil {
		return err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	session, err := journal.sessionLocked(ctx)
	if err != nil || session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 || session.Manifest == nil ||
		session.CompletionReceipt == nil || session.Manifest.TransactionID != request.TransactionID ||
		session.CompletionReceipt.ReceiptDigest != request.CompletionReceiptDigest {
		return errors.New("memory recovery journal retirement frontier is invalid")
	}
	journal.retireCount++
	journal.preparation = nil
	journal.chunks = nil
	journal.manifest = nil
	journal.witness = nil
	journal.completion = nil
	return nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) sessionLocked(
	ctx context.Context,
) (domainprivatecas.RecoveryJournalSessionV1, error) {
	if journal.preparation == nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, privatecasport.ErrJournalAbsent
	}
	session := domainprivatecas.RecoveryJournalSessionV1{
		State:       domainprivatecas.RecoveryJournalSessionPreparedV1,
		Preparation: *journal.preparation,
		Chunks:      privateCASRecoveryV4CloneChunks(journal.chunks),
	}
	if journal.manifest != nil {
		session.State = domainprivatecas.RecoveryJournalSessionManifestedV1
		session.Manifest = privateCASRecoveryV4ManifestPointer(*journal.manifest)
	}
	if journal.witness != nil {
		session.State = domainprivatecas.RecoveryJournalSessionCommitWitnessedV1
		session.CommitWitness = privateCASRecoveryV4WitnessPointer(*journal.witness)
	}
	if journal.completion != nil {
		session.State = domainprivatecas.RecoveryJournalSessionCompletedV1
		session.CompletionReceipt = privateCASRecoveryV4CompletionPointer(*journal.completion)
	}
	if err := domainprivatecas.ValidateRecoveryJournalSessionV1(session); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if err := journal.verifyPreparation(ctx, session.Preparation); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if session.Manifest != nil {
		if err := journal.verifyManifest(ctx, *session.Manifest); err != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, err
		}
	}
	if session.CommitWitness != nil {
		if err := journal.verifyWitness(ctx, *session.CommitWitness); err != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, err
		}
	}
	if session.CompletionReceipt != nil {
		if err := journal.verifyCompletion(ctx, *session.CompletionReceipt); err != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, err
		}
	}
	return session, nil
}

func (journal *memoryPrivateCASRecoveryJournalV1) verifyPreparation(
	ctx context.Context,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
) error {
	body, err := domainprivatecas.RecoveryJournalPreparationSigningBytesV1(preparation)
	if err != nil {
		return err
	}
	return journal.Verify(ctx, preparation.AuthorityKeyID, body, preparation.AuthoritySignature)
}

func (journal *memoryPrivateCASRecoveryJournalV1) verifyManifest(
	ctx context.Context,
	manifest domainprivatecas.RecoveryJournalManifestV1,
) error {
	body, err := domainprivatecas.RecoveryJournalManifestSigningBytesV1(manifest)
	if err != nil {
		return err
	}
	return journal.Verify(ctx, manifest.AuthorityKeyID, body, manifest.AuthoritySignature)
}

func (journal *memoryPrivateCASRecoveryJournalV1) verifyWitness(
	ctx context.Context,
	witness domainprivatecas.RecoveryJournalCommitWitnessV1,
) error {
	body, err := domainprivatecas.RecoveryJournalCommitWitnessSigningBytesV1(witness)
	if err != nil {
		return err
	}
	return journal.Verify(ctx, witness.AuthorityKeyID, body, witness.AuthoritySignature)
}

func (journal *memoryPrivateCASRecoveryJournalV1) verifyCompletion(
	ctx context.Context,
	completion domainprivatecas.RecoveryJournalCompletionReceiptV1,
) error {
	body, err := domainprivatecas.RecoveryJournalCompletionSigningBytesV1(completion)
	if err != nil {
		return err
	}
	return journal.Verify(ctx, completion.AuthorityKeyID, body, completion.AuthoritySignature)
}

func privateCASRecoveryV4RequireExactPreparation(
	left domainprivatecas.RecoveryJournalPreparationV1,
	right domainprivatecas.RecoveryJournalPreparationV1,
) error {
	leftBody, leftErr := domainprivatecas.RecoveryJournalPreparationV1Bytes(left)
	rightBody, rightErr := domainprivatecas.RecoveryJournalPreparationV1Bytes(right)
	if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
		return errors.New("memory recovery journal preparation conflicts with its immutable record")
	}
	return nil
}

func privateCASRecoveryV4RequireExactChunk(
	left domainprivatecas.RecoveryTargetChunkV1,
	right domainprivatecas.RecoveryTargetChunkV1,
) error {
	leftBody, leftErr := domainprivatecas.RecoveryTargetChunkV1Bytes(left)
	rightBody, rightErr := domainprivatecas.RecoveryTargetChunkV1Bytes(right)
	if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
		return errors.New("memory recovery journal chunk conflicts with its immutable record")
	}
	return nil
}

func privateCASRecoveryV4RequireExactManifest(
	left domainprivatecas.RecoveryJournalManifestV1,
	right domainprivatecas.RecoveryJournalManifestV1,
) error {
	leftBody, leftErr := domainprivatecas.RecoveryJournalManifestV1Bytes(left)
	rightBody, rightErr := domainprivatecas.RecoveryJournalManifestV1Bytes(right)
	if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
		return errors.New("memory recovery journal manifest conflicts with its immutable record")
	}
	return nil
}

func privateCASRecoveryV4RequireExactWitness(
	left domainprivatecas.RecoveryJournalCommitWitnessV1,
	right domainprivatecas.RecoveryJournalCommitWitnessV1,
) error {
	leftBody, leftErr := domainprivatecas.RecoveryJournalCommitWitnessV1Bytes(left)
	rightBody, rightErr := domainprivatecas.RecoveryJournalCommitWitnessV1Bytes(right)
	if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
		return errors.New("memory recovery journal witness conflicts with its immutable record")
	}
	return nil
}

func privateCASRecoveryV4RequireExactCompletion(
	left domainprivatecas.RecoveryJournalCompletionReceiptV1,
	right domainprivatecas.RecoveryJournalCompletionReceiptV1,
) error {
	leftBody, leftErr := domainprivatecas.RecoveryJournalCompletionReceiptV1Bytes(left)
	rightBody, rightErr := domainprivatecas.RecoveryJournalCompletionReceiptV1Bytes(right)
	if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
		return errors.New("memory recovery journal completion conflicts with its immutable record")
	}
	return nil
}

func privateCASRecoveryV4CloneChunk(
	chunk domainprivatecas.RecoveryTargetChunkV1,
) domainprivatecas.RecoveryTargetChunkV1 {
	chunk.Entries = append([]domainprivatecas.RecoveryTargetEntryV1(nil), chunk.Entries...)
	return chunk
}

func privateCASRecoveryV4CloneChunks(
	chunks []domainprivatecas.RecoveryTargetChunkV1,
) []domainprivatecas.RecoveryTargetChunkV1 {
	cloned := make([]domainprivatecas.RecoveryTargetChunkV1, len(chunks))
	for index := range chunks {
		cloned[index] = privateCASRecoveryV4CloneChunk(chunks[index])
	}
	return cloned
}

func privateCASRecoveryV4PreparationPointer(
	value domainprivatecas.RecoveryJournalPreparationV1,
) *domainprivatecas.RecoveryJournalPreparationV1 {
	return &value
}

func privateCASRecoveryV4ManifestPointer(
	value domainprivatecas.RecoveryJournalManifestV1,
) *domainprivatecas.RecoveryJournalManifestV1 {
	value.ChunkDigests = append([]string(nil), value.ChunkDigests...)
	return &value
}

func privateCASRecoveryV4WitnessPointer(
	value domainprivatecas.RecoveryJournalCommitWitnessV1,
) *domainprivatecas.RecoveryJournalCommitWitnessV1 {
	return &value
}

func privateCASRecoveryV4CompletionPointer(
	value domainprivatecas.RecoveryJournalCompletionReceiptV1,
) *domainprivatecas.RecoveryJournalCompletionReceiptV1 {
	return &value
}

func privateCASRecoveryV4ChunkContainsTarget(
	chunks []domainprivatecas.RecoveryTargetChunkV1,
	targetID string,
) bool {
	for _, chunk := range chunks {
		for _, entry := range chunk.Entries {
			if entry.TargetID == targetID {
				return true
			}
		}
	}
	return false
}

func privateCASRecoveryV4ContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func privateCASRecoveryV4DigestOf(value string) string {
	return privateCASRecoveryV4DigestBytes([]byte(value))
}

func privateCASRecoveryV4DigestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

var _ privatecasport.RecoveryJournalV1 = (*memoryPrivateCASRecoveryJournalV1)(nil)
