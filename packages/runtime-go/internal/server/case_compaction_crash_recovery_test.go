package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	casethreadauthority "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const caseCompactionCrashSourceMarkerV1 = "CASE_COMPACTION_SOURCE_MUST_SURVIVE_UNCOMMITTED_CUT"

func TestCaseCompactionCrashSafeTwoPhaseFilesystemMatrix(t *testing.T) {
	t.Run("logical_prepare_keeps_source", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatal(err)
		}
		fixture.assertSourceRetained(t, restartedStore)
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("prepared_record_with_retained_source_is_inert_after_restart", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		fixture.stageTarget(t, fixture.authority)
		fixture.assertSourceRetained(t, fixture.store)

		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatal(err)
		}
		fixture.assertSourceRetained(t, restartedStore)
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("signing_failure_keeps_source", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		failingSigner := &caseCompactionFailingSignerV1{inner: fixture.signer}
		registry, err := casethreadapp.NewRegistry(
			context.Background(), failingSigner, fixture.authorityStore,
		)
		if err != nil {
			t.Fatal(err)
		}
		fixture.stageTarget(t, registry)
		failingSigner.fail = true
		fixture.store.SetCaseThreadAuthority(registry)
		if committed, err := fixture.store.CommitCompaction(fixture.prepared.CommitRequest()); err == nil || committed.Committed {
			t.Fatalf("injected signing failure authorized destructive prune: result=%#v err=%v", committed, err)
		}
		if _, found := registry.CommittedContext(fixture.threadID, fixture.prepared.SecurityContext.TurnID); found {
			t.Fatal("injected signing failure left a committed target authority")
		}
		fixture.assertSourceRetained(t, fixture.store)

		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatal(err)
		}
		fixture.assertSourceRetained(t, restartedStore)
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("signed_target_recovers_prune_after_restart", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		fixture.stageAndCommitTarget(t, fixture.authority)
		fixture.assertSourceRetained(t, fixture.store)

		restartedStore, restartedAuthority := fixture.reopen(t)
		inventory, err := casethreadapp.PreflightRestartInventory(restartedAuthority, restartedStore)
		if err != nil || inventory.Quarantined[fixture.threadID] == "" {
			t.Fatalf("missing committed target was not fail-closed before recovery: inventory=%#v err=%v", inventory, err)
		}
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatal(err)
		}
		fixture.assertPrunedToSignedTarget(t, restartedStore)
		firstDigest := fixture.threadDigest(t, restartedStore)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatalf("idempotent recovery retry failed: %v", err)
		}
		if retryDigest := fixture.threadDigest(t, restartedStore); retryDigest != firstDigest {
			t.Fatalf("idempotent recovery retry rewrote the target: first=%s retry=%s", firstDigest, retryDigest)
		}
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("signed_target_without_exact_source_turns_fails_closed", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		fixture.stageAndCommitTarget(t, fixture.authority)
		source, err := fixture.store.GetThreadForAuthorityRepair(fixture.threadID)
		if err != nil {
			t.Fatal(err)
		}
		turns := listAny(source["turns"])
		if len(turns) < 2 {
			t.Fatalf("source fixture has too few turns: %d", len(turns))
		}
		source["turns"] = append([]any(nil), turns[1:]...)
		if err := fixture.store.ReplaceThreadForAuthorityRepair(fixture.threadID, source); err != nil {
			t.Fatal(err)
		}

		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatalf("one damaged case thread stopped additive startup recovery: %v", err)
		}
		if restartedAuthority.CanExecute(fixture.threadID) {
			t.Fatal("target-absent recovery left an incomplete signed case source executable")
		}
		reloaded, err := restartedStore.GetThreadForAuthorityRepair(fixture.threadID)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range listAny(reloaded["turns"]) {
			turn, _ := raw.(map[string]any)
			if stringField(turn, "id") == fixture.prepared.SecurityContext.TurnID {
				t.Fatal("failed recovery materialized a target from incomplete source")
			}
		}
		if err := casethreadapp.RepairCommittedContexts(restartedAuthority, restartedStore); err != nil {
			t.Fatalf("quarantined case damage stopped unrelated committed-context recovery: %v", err)
		}
		ordinary, err := restartedStore.CreateThread(
			map[string]any{"title": "ordinary lane survives case recovery damage"}, "",
		)
		if err != nil {
			t.Fatalf("damaged case compaction disabled the ordinary Agent lane: %v", err)
		}
		if _, err := restartedStore.GetThread(stringField(ordinary, "id")); err != nil {
			t.Fatalf("ordinary thread became unreadable after case quarantine: %v", err)
		}
	})

	t.Run("signed_automatic_target_recovers_exact_mode", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		fixture.prepareMode(t, true)
		fixture.stageAndCommitTarget(t, fixture.authority)
		fixture.assertSourceRetained(t, fixture.store)

		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatal(err)
		}
		if !restartedAuthority.CanExecute(fixture.threadID) {
			t.Fatal("signed automatic compaction was quarantined during exact replay")
		}
		thread, err := restartedStore.GetThreadForAuthorityRepair(fixture.threadID)
		if err != nil {
			t.Fatal(err)
		}
		turns := listAny(thread["turns"])
		var mode any
		for _, raw := range turns {
			turn, _ := raw.(map[string]any)
			if stringField(turn, "id") != fixture.prepared.SecurityContext.TurnID {
				continue
			}
			items := listAny(turn["items"])
			if len(items) == 1 {
				item, _ := items[0].(map[string]any)
				mode = item["auto"]
			}
		}
		if mode != true {
			t.Fatalf("automatic crash recovery changed the signed mode: auto=%#v", mode)
		}
		fixture.assertPrunedToSignedTarget(t, restartedStore)
	})

	t.Run("filesystem_prune_failure_retries_after_restart", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		threadDir := fixture.store.threadDir(fixture.threadID)
		backupDir := threadDir + ".fault-backup"
		var faultErr error
		faultInstalled := false
		fixture.store.caseCompactionCommitHook = func(phase, threadID string) {
			if phase != caseCompactionCommitPhaseBeforeWrite || threadID != fixture.threadID || faultInstalled {
				return
			}
			if faultErr = os.Rename(threadDir, backupDir); faultErr != nil {
				return
			}
			faultInstalled = true
			faultErr = os.WriteFile(threadDir, []byte("injected filesystem path blocker"), 0o600)
		}
		restoredPath := false
		defer func() {
			if !restoredPath {
				_ = os.Remove(threadDir)
				_ = os.Rename(backupDir, threadDir)
			}
		}()
		committed, commitErr := fixture.store.CommitCompaction(fixture.prepared.CommitRequest())
		fixture.store.caseCompactionCommitHook = nil
		if faultErr != nil {
			t.Fatalf("filesystem fault injection failed: %v", faultErr)
		}
		if err := os.Remove(threadDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backupDir, threadDir); err != nil {
			t.Fatal(err)
		}
		restoredPath = true
		if commitErr == nil || committed.Committed {
			t.Fatalf("filesystem path blocker did not inject a prune failure: result=%#v err=%v", committed, commitErr)
		}
		signed, found := fixture.authority.CommittedContext(fixture.threadID, fixture.prepared.SecurityContext.TurnID)
		if !found || signed.SecurityContext != fixture.prepared.SecurityContext ||
			signed.EpochState.StateDigest != fixture.prepared.EpochState.StateDigest {
			t.Fatalf("write failure happened before exact signed readback: signed=%#v found=%t", signed, found)
		}
		fixture.assertSourceRetained(t, fixture.store)

		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatalf("restart did not retry committed prune: %v", err)
		}
		fixture.assertPrunedToSignedTarget(t, restartedStore)
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("signed_and_pruned_success_is_restart_fixed_point", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		committed, err := fixture.store.CommitCompaction(fixture.prepared.CommitRequest())
		if err != nil || !committed.AuthorityCommitted || !committed.Committed {
			t.Fatalf("lock-scoped case compaction did not commit: result=%#v err=%v", committed, err)
		}
		fixture.assertPrunedToSignedTarget(t, fixture.store)

		restartedStore, restartedAuthority := fixture.reopen(t)
		before := fixture.threadDigest(t, restartedStore)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatalf("normal signed/pruned restart was not a fixed point: %v", err)
		}
		if after := fixture.threadDigest(t, restartedStore); after != before {
			t.Fatalf("normal restart rewrote a committed compaction: before=%s after=%s", before, after)
		}
		fixture.assertPrunedToSignedTarget(t, restartedStore)
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("historical_compaction_stays_valid_after_later_turn", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		fixture.stageAndCommitTarget(t, fixture.authority)
		committed, err := fixture.store.CommitCompaction(fixture.prepared.CommitRequest())
		if err != nil || !committed.Committed {
			t.Fatalf("initial signed compaction did not commit: result=%#v err=%v", committed, err)
		}
		fixture.appendLaterCommittedTurn(t)

		restartedStore, restartedAuthority := fixture.reopen(t)
		before := fixture.threadDigest(t, restartedStore)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatalf("historical committed compaction was misclassified as a crash cut: %v", err)
		}
		if after := fixture.threadDigest(t, restartedStore); after != before {
			t.Fatalf("historical committed compaction was replayed: before=%s after=%s", before, after)
		}
		fixture.assertRestartReady(t, restartedStore, restartedAuthority)
	})

	t.Run("legacy_prepared_target_already_pruned_stays_quarantined", func(t *testing.T) {
		fixture := newCaseCompactionCrashFixtureV1(t)
		fixture.stageTarget(t, fixture.authority)
		if err := fixture.store.ReplaceThreadForAuthorityRepair(
			fixture.threadID, fixture.prepared.Thread,
		); err != nil {
			t.Fatal(err)
		}
		restartedStore, restartedAuthority := fixture.reopen(t)
		if err := threadapp.RecoverCommittedCaseCompactions(
			context.Background(), restartedAuthority, restartedStore,
		); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.RepairCommittedContexts(restartedAuthority, restartedStore); err != nil {
			t.Fatal(err)
		}
		inventory, err := casethreadapp.PreflightRestartInventory(restartedAuthority, restartedStore)
		if err != nil || inventory.Quarantined[fixture.threadID] == "" {
			t.Fatalf("prepared-only already-pruned target escaped quarantine: inventory=%#v err=%v", inventory, err)
		}
	})
}

func TestCaseCompactionMutexSerializesGoalTodoAndPatchBeforeSigning(t *testing.T) {
	for _, mutation := range caseCompactionConcurrentMutationsV1() {
		mutation := mutation
		t.Run(mutation.name+"/mutation_first", func(t *testing.T) {
			fixture := newCaseCompactionCrashFixtureV1(t)
			beforeLock := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			fixture.store.caseCompactionCommitHook = func(phase, threadID string) {
				if phase != caseCompactionCommitPhaseBeforeLock || threadID != fixture.threadID {
					return
				}
				once.Do(func() { close(beforeLock) })
				<-release
			}
			type commitOutcome struct {
				result threadapp.CompactionCommitResult
				err    error
			}
			commitDone := make(chan commitOutcome, 1)
			go func() {
				result, err := fixture.store.CommitCompaction(fixture.prepared.CommitRequest())
				commitDone <- commitOutcome{result: result, err: err}
			}()
			<-beforeLock
			if err := mutation.apply(fixture, "mutation-first"); err != nil {
				t.Fatal(err)
			}
			close(release)
			outcome := <-commitDone
			fixture.store.caseCompactionCommitHook = nil
			if !errors.Is(outcome.err, threadapp.ErrCompactionBaselineConflict) || outcome.result.Committed ||
				outcome.result.AuthorityCommitted {
				t.Fatalf("mutation-first ordering did not reject before signing: result=%#v err=%v", outcome.result, outcome.err)
			}
			fixture.assertTargetNeverRegisteredOrCommitted(t)
			fixture.assertSourceMarkerRetained(t, fixture.store)
			mutation.assertApplied(t, fixture, "mutation-first")
		})

		t.Run(mutation.name+"/compaction_first", func(t *testing.T) {
			fixture := newCaseCompactionCrashFixtureV1(t)
			afterApply := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			fixture.store.caseCompactionCommitHook = func(phase, threadID string) {
				if phase != caseCompactionCommitPhaseAfterApply || threadID != fixture.threadID {
					return
				}
				once.Do(func() { close(afterApply) })
				<-release
			}
			type commitOutcome struct {
				result threadapp.CompactionCommitResult
				err    error
			}
			commitDone := make(chan commitOutcome, 1)
			go func() {
				result, err := fixture.store.CommitCompaction(fixture.prepared.CommitRequest())
				commitDone <- commitOutcome{result: result, err: err}
			}()
			<-afterApply
			if _, found := fixture.authority.CommittedContext(
				fixture.threadID, fixture.prepared.SecurityContext.TurnID,
			); found {
				t.Fatal("target was signed before the lock-scoped exact-apply barrier released")
			}
			mutationStarted := make(chan struct{})
			mutationDone := make(chan error, 1)
			go func() {
				close(mutationStarted)
				mutationDone <- mutation.apply(fixture, "compaction-first")
			}()
			<-mutationStarted
			select {
			case err := <-mutationDone:
				t.Fatalf("mutation escaped the compaction repository mutex before signed prune: %v", err)
			default:
			}
			close(release)
			outcome := <-commitDone
			if outcome.err != nil || !outcome.result.AuthorityCommitted || !outcome.result.Committed {
				t.Fatalf("compaction-first ordering did not sign and prune atomically: result=%#v err=%v", outcome.result, outcome.err)
			}
			if err := <-mutationDone; err != nil {
				t.Fatal(err)
			}
			fixture.store.caseCompactionCommitHook = nil
			fixture.assertPrunedToSignedTarget(t, fixture.store)
			mutation.assertApplied(t, fixture, "compaction-first")
		})
	}
}

func TestCaseCompactionSignedWriteFailureAdvancesCaseLaneAndRecoversOnRestart(t *testing.T) {
	fixture := newCaseCompactionCrashFixtureV1(t)
	currentThread, err := fixture.store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	current := mustThreadSecurityContextV1(t, currentThread)
	runtimeState := subagentapp.NewRuntimeState()
	if err := runtimeState.ObserveSecurityContextAndCancelInvalidatedJobs(
		context.Background(), current, time.Second,
	); err != nil {
		t.Fatal(err)
	}

	threadDir := fixture.store.threadDir(fixture.threadID)
	backupDir := threadDir + ".continuing-fault-backup"
	var faultErr error
	faultInstalled := false
	fixture.store.caseCompactionCommitHook = func(phase, threadID string) {
		if phase != caseCompactionCommitPhaseBeforeWrite || threadID != fixture.threadID || faultInstalled {
			return
		}
		if faultErr = os.Rename(threadDir, backupDir); faultErr != nil {
			return
		}
		faultInstalled = true
		faultErr = os.WriteFile(threadDir, []byte("injected continuing-process path blocker"), 0o600)
	}
	restoredPath := false
	defer func() {
		if !restoredPath {
			_ = os.Remove(threadDir)
			_ = os.Rename(backupDir, threadDir)
		}
	}()
	service := threadapp.NewService(threadapp.Dependencies{
		Repository: fixture.store, CaseThreads: fixture.authority,
		WorkspaceReader: filestore.CaseBindingReader{}, WorkspaceSecurity: fixture.workspaceSecurity,
		BeginTransition: threadapp.AdaptBeginTransition(runtimeState.BeginSecurityContextTransition),
	})
	response, compactErr := service.Compact(context.Background(), fixture.threadID, "write failure")
	fixture.store.caseCompactionCommitHook = nil
	if faultErr != nil {
		t.Fatalf("filesystem fault injection failed: %v", faultErr)
	}
	if err := os.Remove(threadDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupDir, threadDir); err != nil {
		t.Fatal(err)
	}
	restoredPath = true
	if compactErr == nil || response != nil {
		t.Fatalf("write failure was not returned to the caller: response=%#v err=%v", response, compactErr)
	}
	fixture.assertSourceRetained(t, fixture.store)

	var signedTarget casethreadapp.CommittedContext
	for _, candidate := range fixture.authority.CommittedContexts() {
		if candidate.SecurityContext.ThreadID == fixture.threadID &&
			candidate.SecurityContext.ContextEpoch == current.ContextEpoch+1 &&
			strings.Contains(candidate.SecurityContext.TurnID, "_compaction_") {
			signedTarget = candidate
			break
		}
	}
	if signedTarget.SecurityContext.TurnID == "" {
		t.Fatal("write failure did not leave the exact signed recovery target")
	}
	for name, mutate := range map[string]func() error{
		"goal": func() error {
			_, mutateErr := fixture.store.SetGoal(fixture.threadID, map[string]any{
				"objective": "must wait for signed compaction recovery", "status": "active", "strictCompletion": true,
			})
			return mutateErr
		},
		"todo": func() error {
			_, mutateErr := fixture.store.SetTodos(fixture.threadID, []any{map[string]any{
				"id": "todo_pending_case_compaction", "content": "must wait", "status": "pending",
			}})
			return mutateErr
		},
		"patch": func() error {
			_, mutateErr := fixture.store.PatchThread(fixture.threadID, map[string]any{"title": "must wait"})
			return mutateErr
		},
	} {
		if mutateErr := mutate(); !errors.Is(mutateErr, threadapp.ErrCaseCompactionPendingRecovery) {
			t.Fatalf("%s mutation crossed a signed recovery gap: %v", name, mutateErr)
		}
	}
	fixture.assertSourceRetained(t, fixture.store)
	oldBinding := compactionJobBinding(t, current)
	oldJobCtx, cancelOldJob := context.WithCancel(context.Background())
	if runtimeState.RegisterBoundBackgroundJob(
		"job_case_compaction_signed_write_failure", oldBinding, cancelOldJob,
	) {
		runtimeState.UnregisterBackgroundJob("job_case_compaction_signed_write_failure")
		t.Fatal("old-context case work resumed after the target authority was signed")
	}
	if oldJobCtx.Err() == nil {
		t.Fatal("rejected old-context case work was not cancelled")
	}
	ordinaryContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_case_compaction_unrelated_ordinary", TurnID: "turn_case_compaction_unrelated_ordinary",
		WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	_, releaseOrdinary, ordinaryErr := runtimeState.AcquireOrdinaryContextEffect(context.Background(), ordinaryContext)
	if ordinaryErr != nil {
		t.Fatalf("signed case recovery gap disabled the additive ordinary lane: %v", ordinaryErr)
	}
	releaseOrdinary()

	restartedStore, restartedAuthority := fixture.reopen(t)
	if err := threadapp.RecoverCommittedCaseCompactions(
		context.Background(), restartedAuthority, restartedStore,
	); err != nil {
		t.Fatalf("restart did not roll forward the signed write failure: %v", err)
	}
	recovered, err := restartedStore.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	recoveredCurrent := mustThreadSecurityContextV1(t, recovered)
	if recoveredCurrent != signedTarget.SecurityContext {
		t.Fatalf("restart recovered the wrong signed target: got=%#v want=%#v", recoveredCurrent, signedTarget.SecurityContext)
	}
	body, err := json.Marshal(recovered)
	if err != nil || strings.Contains(string(body), caseCompactionCrashSourceMarkerV1) {
		t.Fatalf("restart did not finish the signed prune: err=%v thread=%s", err, body)
	}
	fixture.assertRestartReady(t, restartedStore, restartedAuthority)
}

func TestCaseCompactionRevalidatesDSV2AfterTransitionPrepareBeforeRepositorySigning(t *testing.T) {
	fixture := newCaseCompactionCrashFixtureV1(t)
	currentThread, err := fixture.store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	current := mustThreadSecurityContextV1(t, currentThread)
	runtimeState := subagentapp.NewRuntimeState()
	if err := runtimeState.ObserveSecurityContextAndCancelInvalidatedJobs(
		context.Background(), current, time.Second,
	); err != nil {
		t.Fatal(err)
	}
	beforeAuthority := map[string]bool{}
	for _, turnID := range fixture.authority.ContextTurnIDs(fixture.threadID) {
		beforeAuthority[turnID] = true
	}
	repositoryCalled := false
	fixture.store.caseCompactionCommitHook = func(_, _ string) { repositoryCalled = true }
	service := threadapp.NewService(threadapp.Dependencies{
		Repository: fixture.store, CaseThreads: fixture.authority,
		WorkspaceReader: filestore.CaseBindingReader{}, WorkspaceSecurity: fixture.workspaceSecurity,
		BeginTransition: func(
			ctx context.Context,
			target domainsecurity.TurnSecurityContext,
		) (threadapp.CompactionTransition, error) {
			inner, beginErr := runtimeState.BeginSecurityContextTransition(ctx, target)
			if beginErr != nil {
				return nil, beginErr
			}
			return &caseCompactionInvalidateDSV2AfterPrepareV1{
				inner: inner, snapshot: fixture.snapshot,
			}, nil
		},
	})
	response, compactErr := service.Compact(context.Background(), fixture.threadID, "invalidate after prepare")
	fixture.store.caseCompactionCommitHook = nil
	if compactErr == nil || response != nil ||
		!strings.Contains(compactErr.Error(), "turn_security_dataset_snapshot_mismatch") {
		t.Fatalf("post-Prepare DSV2 invalidation did not fail closed: response=%#v err=%v", response, compactErr)
	}
	if repositoryCalled {
		t.Fatal("post-Prepare DSV2 invalidation reached repository signing")
	}
	fixture.assertSourceRetained(t, fixture.store)
	afterIDs := fixture.authority.ContextTurnIDs(fixture.threadID)
	if len(afterIDs) != len(beforeAuthority) {
		t.Fatalf("post-Prepare invalidation changed authority inventory: before=%#v after=%#v", beforeAuthority, afterIDs)
	}
	for _, turnID := range afterIDs {
		if !beforeAuthority[turnID] {
			t.Fatalf("post-Prepare invalidation registered an unexpected target: %s", turnID)
		}
	}
	oldBinding := compactionJobBinding(t, current)
	_, cancelOldJob := context.WithCancel(context.Background())
	if !runtimeState.RegisterBoundBackgroundJob(
		"job_case_compaction_revalidation_abort", oldBinding, cancelOldJob,
	) {
		t.Fatal("pre-signing DSV2 rejection did not abort the tentative transition")
	}
	runtimeState.UnregisterBackgroundJob("job_case_compaction_revalidation_abort")
	cancelOldJob()
}

type caseCompactionInvalidateDSV2AfterPrepareV1 struct {
	inner    threadapp.CompactionTransition
	snapshot *admissionFailureSnapshotAuthority
}

func (transition *caseCompactionInvalidateDSV2AfterPrepareV1) Prepare(
	ctx context.Context,
	target domainsecurity.TurnSecurityContext,
	timeout time.Duration,
) error {
	if err := transition.inner.Prepare(ctx, target, timeout); err != nil {
		return err
	}
	transition.snapshot.resolved.Record.DatasetSnapshotID += "_stale_after_prepare"
	return nil
}

func (transition *caseCompactionInvalidateDSV2AfterPrepareV1) Commit() error {
	return transition.inner.Commit()
}

func (transition *caseCompactionInvalidateDSV2AfterPrepareV1) Abort() {
	transition.inner.Abort()
}

type caseCompactionConcurrentMutationV1 struct {
	name          string
	apply         func(*caseCompactionCrashFixtureV1, string) error
	assertApplied func(*testing.T, *caseCompactionCrashFixtureV1, string)
}

func caseCompactionConcurrentMutationsV1() []caseCompactionConcurrentMutationV1 {
	return []caseCompactionConcurrentMutationV1{
		{
			name: "goal",
			apply: func(fixture *caseCompactionCrashFixtureV1, value string) error {
				_, err := fixture.store.SetGoal(fixture.threadID, map[string]any{
					"objective": value, "status": "active", "strictCompletion": true,
				})
				return err
			},
			assertApplied: func(t *testing.T, fixture *caseCompactionCrashFixtureV1, value string) {
				t.Helper()
				goal, err := fixture.store.GetGoal(fixture.threadID)
				if err != nil || stringField(goal, "objective") != value {
					t.Fatalf("serialized goal mutation is missing: goal=%#v err=%v", goal, err)
				}
			},
		},
		{
			name: "todo",
			apply: func(fixture *caseCompactionCrashFixtureV1, value string) error {
				_, err := fixture.store.SetTodos(fixture.threadID, []any{map[string]any{
					"id": "todo_case_compaction_race", "content": value, "status": "pending",
				}})
				return err
			},
			assertApplied: func(t *testing.T, fixture *caseCompactionCrashFixtureV1, value string) {
				t.Helper()
				todos, err := fixture.store.GetTodos(fixture.threadID)
				body, marshalErr := json.Marshal(todos)
				if err != nil || marshalErr != nil || !strings.Contains(string(body), value) {
					t.Fatalf("serialized Todo mutation is missing: todos=%#v err=%v marshal=%v", todos, err, marshalErr)
				}
			},
		},
		{
			name: "patch",
			apply: func(fixture *caseCompactionCrashFixtureV1, value string) error {
				_, err := fixture.store.PatchThread(fixture.threadID, map[string]any{"title": value})
				return err
			},
			assertApplied: func(t *testing.T, fixture *caseCompactionCrashFixtureV1, value string) {
				t.Helper()
				thread, err := fixture.store.GetThreadForAuthorityRepair(fixture.threadID)
				if err != nil || stringField(thread, "title") != value {
					t.Fatalf("serialized patch mutation is missing: thread=%#v err=%v", thread, err)
				}
			},
		},
	}
}

type caseCompactionCrashFixtureV1 struct {
	durableRoot       string
	authorityPath     string
	authorityStoreDir string
	threadID          string
	sourceDigest      string
	compactionAt      time.Time
	prepared          threadapp.PreparedCompaction
	store             *DurableEventSessionStore
	authorityStore    *casethreadauthority.Store
	authority         *casethreadapp.Registry
	signer            *finalauthorityadapter.FileAuthority
	workspaceSecurity turnsecurityapp.WorkspaceSecurityAuthority
	snapshot          *admissionFailureSnapshotAuthority
}

func newCaseCompactionCrashFixtureV1(t *testing.T) *caseCompactionCrashFixtureV1 {
	t.Helper()
	ctx := context.Background()
	signingRoot := t.TempDir()
	if err := os.Chmod(signingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(signingRoot, "authority.json")
	signer, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	authorityStoreDir := filepath.Join(t.TempDir(), "case-thread-records")
	authorityStore, err := newServerTestCaseThreadStore(t, authorityStoreDir)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := casethreadapp.NewRegistry(ctx, signer, authorityStore)
	if err != nil {
		t.Fatal(err)
	}
	durableRoot := t.TempDir()
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	store.SetCaseThreadAuthority(authority)
	workspace := writeThreadMutationCaseBinding(t)
	observer := filestore.CaseBindingReader{}
	observation, err := observer.Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		t.Fatalf("case compaction workspace observation is invalid: observation=%#v err=%v", observation, err)
	}
	snapshot := newAdmissionFailureSnapshotAuthority(t, observation)
	workspaceSecurity := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: newServerTestRiskAuthority(),
		SnapshotAuthority: snapshot, SnapshotAuthorityV2: snapshot,
		RiskIntent: domainsecurity.RiskClassCase, TrustedCaseThread: true,
	}
	thread, err := store.CreateThread(
		map[string]any{"title": "Case compaction crash recovery", "workspace": workspace}, workspace,
	)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	at := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	for index := 1; index <= 4; index++ {
		turnID := "turn_case_compaction_crash_" + string(rune('0'+index))
		currentThread, currentErr := store.GetThreadForAuthorityRepair(threadID)
		if currentErr != nil {
			t.Fatal(currentErr)
		}
		frozen, freezeErr := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
			Context: context.Background(), Authority: workspaceSecurity, Thread: currentThread,
			ThreadID: threadID, TurnID: turnID, Workspace: workspace, Principal: testIdentityPrincipal(),
			IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		if freezeErr != nil {
			t.Fatal(freezeErr)
		}
		state, stateErr := contextepochapp.BootstrapState(
			threadID, frozen.ContextEpoch,
			[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, at.Add(time.Duration(index)*time.Second),
		)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if err := casethreadapp.RegisterRequired(ctx, authority, frozen); err != nil {
			t.Fatal(err)
		}
		prompt := "case compaction source"
		if index == 1 {
			prompt = caseCompactionCrashSourceMarkerV1
		}
		if err := store.AppendTurnToThread(threadID, map[string]any{
			"id": frozen.TurnID, "threadId": threadID, "status": "completed", "prompt": prompt,
			"securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{
				"id": "item_case_compaction_crash_" + string(rune('0'+index)), "turnId": frozen.TurnID,
				"threadId": threadID, "kind": "user_message", "role": "user", "status": "completed",
				"text": "continue the bounded case analysis",
			}},
		}, "deepseek", map[string]any{
			"securityState":     turnsecurityapp.PublicRecord(frozen),
			"contextEpochState": contextepochapp.PublicState(state),
		}); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.CommitRequired(
			ctx, authority, frozen, state, at.Add(time.Duration(index+10)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	source, err := store.GetThreadForAuthorityRepair(threadID)
	if err != nil {
		t.Fatal(err)
	}
	sourceDigest, err := threadapp.CompactionBaselineDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := turnapp.BuildTaskContinuationSnapshotV1(source)
	if err != nil {
		t.Fatal(err)
	}
	compactionAt := at.Add(time.Minute)
	prepared, err := threadapp.PrepareCaseCompaction(
		source, threadID, "manual", compactionAt, false,
		threadapp.CaseCompactionAuthorization{
			Continuation: continuation, SourceContextDigest: mustThreadSecurityContextV1(t, source).ContextDigest,
			AuthorityTurnIDs: casethreadapp.CommittedTurnIDs(authority, threadID),
		},
	)
	if err != nil || prepared.Result.ReplacedTokens <= 0 {
		t.Fatalf("case compaction crash fixture did not prepare: prepared=%#v err=%v", prepared, err)
	}
	return &caseCompactionCrashFixtureV1{
		durableRoot: durableRoot, authorityPath: authorityPath, authorityStoreDir: authorityStoreDir,
		threadID: threadID, sourceDigest: sourceDigest, compactionAt: compactionAt, prepared: prepared,
		store: store, authorityStore: authorityStore, authority: authority, signer: signer,
		workspaceSecurity: workspaceSecurity, snapshot: snapshot,
	}
}

func (fixture *caseCompactionCrashFixtureV1) prepareMode(t *testing.T, auto bool) {
	t.Helper()
	source, err := fixture.store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := turnapp.BuildTaskContinuationSnapshotV1(source)
	if err != nil {
		t.Fatal(err)
	}
	reason := "manual"
	if auto {
		reason = "automatic_context_threshold"
	}
	prepared, err := threadapp.PrepareCaseCompaction(
		source, fixture.threadID, reason, fixture.compactionAt, auto,
		threadapp.CaseCompactionAuthorization{
			Continuation:        continuation,
			SourceContextDigest: mustThreadSecurityContextV1(t, source).ContextDigest,
			AuthorityTurnIDs:    casethreadapp.CommittedTurnIDs(fixture.authority, fixture.threadID),
		},
	)
	if err != nil || prepared.Result.ReplacedTokens <= 0 {
		t.Fatalf("case compaction mode fixture did not prepare: auto=%t prepared=%#v err=%v", auto, prepared, err)
	}
	fixture.prepared = prepared
}

func (fixture *caseCompactionCrashFixtureV1) reopen(
	t *testing.T,
) (*DurableEventSessionStore, *casethreadapp.Registry) {
	t.Helper()
	signer, err := finalauthorityadapter.OpenOrCreateFileAuthority(fixture.authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	authorityStore, err := newServerTestCaseThreadStore(t, fixture.authorityStoreDir)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := casethreadapp.NewRegistry(context.Background(), signer, authorityStore)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewTempDurableEventSessionStore(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	store.SetCaseThreadAuthority(authority)
	return store, authority
}

func (fixture *caseCompactionCrashFixtureV1) stageTarget(
	t *testing.T,
	authority casethreadapp.Authority,
) {
	t.Helper()
	if err := casethreadapp.RegisterRequired(
		context.Background(), authority, fixture.prepared.SecurityContext,
	); err != nil {
		t.Fatal(err)
	}
}

func (fixture *caseCompactionCrashFixtureV1) stageAndCommitTarget(
	t *testing.T,
	authority casethreadapp.Authority,
) {
	t.Helper()
	fixture.stageTarget(t, authority)
	if err := casethreadapp.CommitRequired(
		context.Background(), authority, fixture.prepared.SecurityContext,
		fixture.prepared.EpochState, fixture.compactionAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
}

func (fixture *caseCompactionCrashFixtureV1) appendLaterCommittedTurn(t *testing.T) {
	t.Helper()
	target := fixture.prepared.SecurityContext
	at := fixture.compactionAt.Add(10 * time.Second)
	future := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: target.ThreadID, TurnID: "turn_case_compaction_after_recovery", WorkspaceRealPath: target.WorkspaceRealPath,
		TenantID: target.TenantID, UserID: target.UserID, CaseID: target.CaseID, CaseBindingHash: target.CaseBindingHash,
		DatasetSnapshotID: target.DatasetSnapshotID, SourceManifestHash: target.SourceManifestHash,
		ContextEpoch: target.ContextEpoch + 1, IssuedAt: at,
	})
	state, err := contextepochapp.BootstrapState(
		fixture.threadID, future.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(future)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.RegisterRequired(context.Background(), fixture.authority, future); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.AppendTurnToThread(fixture.threadID, map[string]any{
		"id": future.TurnID, "threadId": fixture.threadID, "status": "completed",
		"securityContext": turnsecurityapp.PublicRecord(future),
		"items": []any{map[string]any{
			"id": "item_case_compaction_after_recovery", "turnId": future.TurnID,
			"threadId": fixture.threadID, "kind": "user_message", "role": "user",
			"status": "completed", "text": "continue after compaction",
		}},
	}, "deepseek", map[string]any{
		"securityState":     turnsecurityapp.PublicRecord(future),
		"contextEpochState": contextepochapp.PublicState(state),
	}); err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.CommitRequired(
		context.Background(), fixture.authority, future, state, at.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
}

func (fixture *caseCompactionCrashFixtureV1) assertSourceRetained(
	t *testing.T,
	store *DurableEventSessionStore,
) {
	t.Helper()
	if digest := fixture.threadDigest(t, store); digest != fixture.sourceDigest {
		t.Fatalf("uncommitted cut changed source thread: source=%s current=%s", fixture.sourceDigest, digest)
	}
	thread, err := store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(thread)
	if err != nil || !strings.Contains(string(body), caseCompactionCrashSourceMarkerV1) {
		t.Fatalf("source marker was pruned before committed authority: err=%v thread=%s", err, body)
	}
}

func (fixture *caseCompactionCrashFixtureV1) assertSourceMarkerRetained(
	t *testing.T,
	store *DurableEventSessionStore,
) {
	t.Helper()
	thread, err := store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(thread)
	if err != nil || !strings.Contains(string(body), caseCompactionCrashSourceMarkerV1) {
		t.Fatalf("rejected compaction pruned its source marker: err=%v thread=%s", err, body)
	}
}

func (fixture *caseCompactionCrashFixtureV1) assertTargetNeverRegisteredOrCommitted(t *testing.T) {
	t.Helper()
	targetTurnID := fixture.prepared.SecurityContext.TurnID
	if _, found := fixture.authority.CommittedContext(fixture.threadID, targetTurnID); found {
		t.Fatal("baseline-conflicted compaction left a signed target")
	}
	for _, turnID := range fixture.authority.ContextTurnIDs(fixture.threadID) {
		if strings.TrimSpace(turnID) == targetTurnID {
			t.Fatal("baseline-conflicted compaction registered a target before exact CAS replay")
		}
	}
}

func (fixture *caseCompactionCrashFixtureV1) assertPrunedToSignedTarget(
	t *testing.T,
	store *DurableEventSessionStore,
) {
	t.Helper()
	thread, err := store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(thread)
	if err != nil || strings.Contains(string(body), caseCompactionCrashSourceMarkerV1) {
		t.Fatalf("committed recovery did not prune source marker: err=%v thread=%s", err, body)
	}
	current := mustThreadSecurityContextV1(t, thread)
	state, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil || current != fixture.prepared.SecurityContext ||
		state.StateDigest != fixture.prepared.EpochState.StateDigest {
		t.Fatalf("recovered target does not match signed authority: current=%#v state=%#v err=%v", current, state, err)
	}
}

func (fixture *caseCompactionCrashFixtureV1) assertRestartReady(
	t *testing.T,
	store *DurableEventSessionStore,
	authority *casethreadapp.Registry,
) {
	t.Helper()
	if err := casethreadapp.RepairCommittedContexts(authority, store); err != nil {
		t.Fatalf("generic committed-context repair rejected compaction state: %v", err)
	}
	inventory, err := casethreadapp.PreflightRestartInventory(authority, store)
	if err != nil || inventory.Quarantined[fixture.threadID] != "" {
		t.Fatalf("restart inventory quarantined safe compaction state: inventory=%#v err=%v", inventory, err)
	}
	if err := casethreadapp.ApplyRestartInventory(authority, inventory); err != nil ||
		!authority.CanExecute(fixture.threadID) {
		t.Fatalf("safe compaction state did not remain executable: %v", err)
	}
}

func (fixture *caseCompactionCrashFixtureV1) threadDigest(
	t *testing.T,
	store *DurableEventSessionStore,
) string {
	t.Helper()
	thread, err := store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := threadapp.CompactionBaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustThreadSecurityContextV1(t *testing.T, thread map[string]any) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

type caseCompactionFailingSignerV1 struct {
	inner finalauthorityport.Authority
	fail  bool
}

func (authority *caseCompactionFailingSignerV1) KeyID() string { return authority.inner.KeyID() }

func (authority *caseCompactionFailingSignerV1) PublicKey() []byte {
	return authority.inner.PublicKey()
}

func (authority *caseCompactionFailingSignerV1) VerifyTrusted(
	ctx context.Context,
	keyID string,
	publicKey,
	message,
	signature []byte,
) error {
	return authority.inner.VerifyTrusted(ctx, keyID, publicKey, message, signature)
}

func (authority *caseCompactionFailingSignerV1) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if authority.fail {
		return nil, errors.New("injected case compaction signing failure")
	}
	return authority.inner.Sign(ctx, message)
}
