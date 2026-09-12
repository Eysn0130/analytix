package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCompactionCancelsOrRejectsOldEpochJobs(t *testing.T) {
	handler, threadID, current := newUnboundCompactionFixture(t)
	state := handler.runtimeSubagentState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), current, time.Second); err != nil {
		t.Fatal(err)
	}
	binding := compactionJobBinding(t, current)
	jobCtx, cancelJob := context.WithCancel(context.Background())
	cancelObserved := make(chan struct{})
	if !state.RegisterBoundBackgroundJob("job_compaction_old_epoch", binding, cancelJob) {
		t.Fatal("old-epoch mutator registration was rejected before compaction")
	}
	go func() {
		<-jobCtx.Done()
		close(cancelObserved)
		state.UnregisterBackgroundJob("job_compaction_old_epoch")
	}()

	response, err := handler.runtimeThreadService().Compact(context.Background(), threadID, "manual")
	if err != nil {
		t.Fatalf("compact through real ThreadService: %v", err)
	}
	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("compaction did not cancel the old exact-turn mutator")
	}
	if response["replacedTokens"].(float64) <= 0 {
		t.Fatalf("compaction did not replace history: %#v", response)
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	stateRecord, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || stateRecord.AcceptedSnapshot.Epoch != current.ContextEpoch+1 ||
		stateRecord.AcceptedSnapshot.RecoveryDigest != response["sourceDigest"] {
		t.Fatalf("compaction epoch authority mismatch: state=%#v ok=%t err=%v response=%#v", stateRecord, ok, err, response)
	}
	next, err := domainsecurity.ParseTurnSecurityContext(reloaded["securityState"])
	if err != nil || next.ContextEpoch != stateRecord.AcceptedSnapshot.Epoch || next.TurnID != response["turnId"] || next.ThreadID != threadID {
		t.Fatalf("compaction security authority mismatch: context=%#v err=%v", next, err)
	}
	turns := listAny(reloaded["turns"])
	compactTurn, _ := turns[0].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(compactTurn["securityContext"])
	if err != nil || frozen != next || compactTurn["contextEpochSnapshot"] == nil {
		t.Fatalf("compaction turn did not freeze the new authority: turn=%#v context=%#v err=%v", compactTurn, frozen, err)
	}

	lateCtx, lateCancel := context.WithCancel(context.Background())
	defer lateCancel()
	if state.RegisterBoundBackgroundJob("job_compaction_stale_restart", binding, lateCancel) {
		state.UnregisterBackgroundJob("job_compaction_stale_restart")
		t.Fatal("old-epoch mutator was admitted after compaction authority acceptance")
	}
	if lateCtx.Err() == nil {
		t.Fatal("rejected old-epoch mutator was not cancelled")
	}
}

func TestCompactionTransitionFailureAbortsWithoutDurableOrMemoryAuthority(t *testing.T) {
	handler, threadID, current := newUnboundCompactionFixture(t)
	state := handler.runtimeSubagentState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), current, time.Second); err != nil {
		t.Fatal(err)
	}
	binding := compactionJobBinding(t, current)
	_, cancelJob := context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_compaction_stuck", binding, cancelJob) {
		t.Fatal("stuck mutator registration failed")
	}
	before, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest, err := threadapp.CompactionBaselineDigest(before)
	if err != nil {
		t.Fatal(err)
	}
	service := threadapp.NewService(threadapp.Dependencies{
		Repository: handler.store, PublicProjector: handler.publicProjector,
		WorkspaceReader: filestore.CaseBindingReader{},
		BeginTransition: threadapp.AdaptBeginTransition(handler.runtimeSubagentState().BeginSecurityContextTransition), TransitionTimeout: 20 * time.Millisecond,
	})
	if response, err := service.Compact(context.Background(), threadID, "timeout"); err == nil || response != nil || !strings.Contains(err.Error(), "did not stop") {
		t.Fatalf("transition timeout did not fail closed: response=%#v err=%v", response, err)
	}
	state.UnregisterBackgroundJob("job_compaction_stuck")
	after, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest, err := threadapp.CompactionBaselineDigest(after)
	if err != nil || afterDigest != beforeDigest {
		t.Fatalf("failed transition mutated durable compaction state: before=%s after=%s err=%v", beforeDigest, afterDigest, err)
	}

	// Abort must preserve the old in-memory authority. Re-admission of the old
	// exact binding proves the tentative compaction context was not published.
	probeCtx, probeCancel := context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_compaction_abort_probe", binding, probeCancel) {
		t.Fatal("failed compaction polluted the accepted in-memory context")
	}
	state.UnregisterBackgroundJob("job_compaction_abort_probe")
	probeCancel()
	if probeCtx.Err() == nil {
		t.Fatal("probe cancellation did not propagate")
	}
}

func TestCompactionPreservesGeneralTerminalArchiveAndAllowsRestartAndFutureCompletion(t *testing.T) {
	handler, threadID, _ := newUnboundCompactionFixture(t)
	before := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	loaded, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(before, loaded.Events)
	if err != nil || len(beforeEntries) != 4 {
		t.Fatalf("pre-compaction terminal inventory is incomplete: entries=%#v err=%v", beforeEntries, err)
	}
	prepared, err := threadapp.PrepareCompaction(
		before,
		threadID,
		"general-terminal-archive",
		time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := handler.store.CommitCompaction(prepared.CommitRequest())
	if err != nil || !committed.Committed {
		t.Fatalf("compaction commit failed: result=%#v err=%v", committed, err)
	}
	after := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	afterLoaded, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	afterEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(after, afterLoaded.Events)
	if err != nil || len(afterEntries) != 4 {
		t.Fatalf("post-compaction terminal inventory is not restart-safe: entries=%#v err=%v", afterEntries, err)
	}
	archivedOnly := 0
	present := 0
	for _, entry := range afterEntries {
		if entry.State != turnapp.GeneralTerminalPublicationCompleteV1 {
			t.Fatalf("compaction left an unsettled terminal bundle: %#v", entry)
		}
		if entry.ArchivedOnly {
			archivedOnly++
		} else {
			present++
		}
	}
	if archivedOnly != 2 || present != 2 {
		t.Fatalf("compaction archive/present split mismatch: archived=%d present=%d", archivedOnly, present)
	}

	reopened, err := NewTempDurableEventSessionStore(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := reopened.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil || len(plans) != 1 || len(plans[0].RepairTurns) != 0 {
		t.Fatalf("restart recovery rejected compacted exact bundles: plans=%#v err=%v", plans, err)
	}
	if err := reopened.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
		t.Fatalf("restart recovery did not settle compacted derived state: %v", err)
	}
	reloaded := rawGeneralTerminalThreadForTest(t, reopened, threadID)
	current, err := domainsecurity.ParseTurnSecurityContext(reloaded["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	future := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_after_general_terminal_compaction", WorkspaceRealPath: current.WorkspaceRealPath,
		SourceManifestHash: current.SourceManifestHash, ContextEpoch: current.ContextEpoch,
		IssuedAt: time.Date(2026, 7, 15, 1, 1, 0, 0, time.UTC),
	})
	futureRecord := turnsecurityapp.PublicRecord(future)
	if err := reopened.AppendTurnToThread(threadID, map[string]any{
		"id": future.TurnID, "threadId": threadID, "status": "running", "securityContext": futureRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": futureRecord}); err != nil {
		t.Fatal(err)
	}
	finishedAt := "2026-07-15T01:01:01Z"
	reopened.generalTerminalAtomicAppend = func(string, []map[string]any) error {
		return errors.New("simulated future terminal append failure")
	}
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: reopened, SecurityContext: future, ThreadID: threadID, TurnID: future.TurnID,
		Model: "deepseek", CreatedAt: finishedAt, FinishedAt: finishedAt,
	}); err == nil || !result.Changed {
		t.Fatalf("future crash gap after restart was not preserved: result=%#v err=%v", result, err)
	}
	recovered, err := NewTempDurableEventSessionStore(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	recoveryPlans, err := recovered.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil || len(recoveryPlans) != 1 || len(recoveryPlans[0].RepairTurns) != 1 ||
		recoveryPlans[0].RepairTurns[0].TurnID != future.TurnID {
		t.Fatalf("startup preflight did not find the future crash gap: plans=%#v err=%v", recoveryPlans, err)
	}
	if err := recovered.ApplyGeneralTerminalPublicationRecoveryV1(recoveryPlans); err != nil {
		t.Fatalf("startup recovery did not settle the future crash gap: %v", err)
	}
	finalThread := rawGeneralTerminalThreadForTest(t, recovered, threadID)
	finalEvents, err := recovered.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	finalEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(finalThread, finalEvents.Events)
	if err != nil || len(finalEntries) != 5 {
		t.Fatalf("future completion did not extend the canonical archive: entries=%#v err=%v", finalEntries, err)
	}
	futureEntry, found := turnapp.GeneralTerminalPublicationEntryForTurnV1(finalEntries, future.TurnID)
	if !found || futureEntry.State != turnapp.GeneralTerminalPublicationCompleteV1 || futureEntry.ArchivedOnly {
		t.Fatalf("future recovered completion is not a live exact bundle: %#v", futureEntry)
	}
}

func TestCompactionRejectsEarlierMissingBundleWithLaterPublishedBundles(t *testing.T) {
	handler, threadID, _ := newUnboundCompactionFixture(t)
	thread := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	loaded, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
	if err != nil || len(entries) != 4 {
		t.Fatalf("initial terminal inventory mismatch: entries=%#v err=%v", entries, err)
	}
	dropped := map[string]bool{}
	for _, event := range entries[0].Events {
		dropped[stringField(event, "generalTerminalEventId")] = true
	}
	var durable strings.Builder
	for _, event := range loaded.Events {
		if dropped[stringField(event, "generalTerminalEventId")] {
			continue
		}
		body, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		durable.Write(body)
		durable.WriteByte('\n')
	}
	if err := os.WriteFile(handler.store.eventsPath(threadID), []byte(durable.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries, err = turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
	if err == nil || entries != nil {
		t.Fatalf("out-of-order durable bundles did not fail closed: entries=%#v err=%v", entries, err)
	}
	prepared, err := threadapp.PrepareCompaction(
		thread,
		threadID,
		"settle-before-delete",
		time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := handler.store.CommitCompaction(prepared.CommitRequest())
	if err == nil || committed.Committed {
		t.Fatalf("compaction repaired an out-of-order/corrupt event log: result=%#v err=%v", committed, err)
	}
}

func TestCompactionSettlesRealGeneralTerminalCrashGapBeforeHistoryMutation(t *testing.T) {
	handler, threadID := newUnboundCompactionMissingSuffixFixture(t)
	thread := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	loaded, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
	if err != nil || len(entries) != 4 || entries[0].State != turnapp.GeneralTerminalPublicationCompleteV1 ||
		entries[1].State != turnapp.GeneralTerminalPublicationCompleteV1 ||
		entries[2].State != turnapp.GeneralTerminalPublicationCompleteV1 ||
		entries[3].State != turnapp.GeneralTerminalPublicationMissingV1 {
		t.Fatalf("real CAS/append crash gap mismatch: entries=%#v err=%v", entries, err)
	}
	prepared, err := threadapp.PrepareCompaction(
		thread,
		threadID,
		"settle-real-crash-gap",
		time.Date(2026, 7, 15, 2, 30, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := handler.store.CommitCompaction(prepared.CommitRequest())
	if err != nil || !committed.Committed {
		t.Fatalf("compaction did not settle the real crash gap: result=%#v err=%v", committed, err)
	}
	after := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	afterEvents, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	afterEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(after, afterEvents.Events)
	if err != nil || len(afterEntries) != 4 || !afterEntries[0].ArchivedOnly || !afterEntries[1].ArchivedOnly {
		t.Fatalf("compaction removed an unsettled canonical turn: entries=%#v err=%v", afterEntries, err)
	}
	allTerminalEvents := make([]map[string]any, 0)
	for _, entry := range afterEntries {
		if entry.State != turnapp.GeneralTerminalPublicationCompleteV1 {
			t.Fatalf("history barrier left an unsettled suffix: %#v", entry)
		}
		allTerminalEvents = append(allTerminalEvents, entry.Events...)
		assertGeneralTerminalUsageIndexExactlyOnce(t, handler.store, threadID, entry.Events)
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, allTerminalEvents)
	reopened, err := NewTempDurableEventSessionStore(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	if plans, err := reopened.PreflightGeneralTerminalPublicationRecoveryV1(); err != nil ||
		len(plans) != 1 || len(plans[0].RepairTurns) != 0 {
		t.Fatalf("restart found a compaction recovery gap: plans=%#v err=%v", plans, err)
	}
}

func TestCompactionDoesNotCrossActiveUsageIndexRebuild(t *testing.T) {
	handler, threadID, current := newUnboundCompactionFixture(t)
	thread := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	prepared, err := threadapp.PrepareCompaction(
		thread,
		threadID,
		"usage-index-rebuild-barrier",
		time.Date(2026, 7, 15, 2, 45, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest, err := threadapp.MutationBaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := os.ReadFile(handler.store.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	rewind, err := threadapp.PrepareRewindMutation(thread, threadID, "turn_compaction_4", current, time.Date(2026, 7, 15, 2, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rebuildEntered := make(chan struct{})
	rebuildRelease := make(chan struct{})
	rebuildDone := make(chan error, 1)
	var hookOnce sync.Once
	handler.store.beforeUsageIndexThreadHook = func(string) {
		hookOnce.Do(func() { close(rebuildEntered) })
		<-rebuildRelease
	}
	go func() { rebuildDone <- handler.store.EnsureUsageIndex() }()
	<-rebuildEntered
	committed, err := handler.store.CommitCompaction(prepared.CommitRequest())
	rewindResult, rewindErr := handler.store.CommitRewindMutation(threadapp.RewindMutationCommitRequest{
		ThreadID: threadID, RewindTurnID: rewind.RewindTurnID, Stamp: rewind.Stamp,
		ExpectedBaselineDigest: rewind.BaselineDigest, ExpectedContextDigest: current.ContextDigest, CurrentContext: current,
	})
	legacyResult, legacyErr := handler.store.RewindThreadIfBaseline(threadID, "turn_compaction_4", beforeDigest)
	close(rebuildRelease)
	if rebuildErr := <-rebuildDone; rebuildErr != nil {
		t.Fatalf("usage index rebuild failed after barrier test: %v", rebuildErr)
	}
	handler.store.beforeUsageIndexThreadHook = nil
	if !errors.Is(err, errUsageIndexRebuildHistoryMutation) || committed.Committed {
		t.Fatalf("compaction crossed an active usage index rebuild: result=%#v err=%v", committed, err)
	}
	if !errors.Is(rewindErr, errUsageIndexRebuildHistoryMutation) || rewindResult.Committed ||
		!errors.Is(legacyErr, errUsageIndexRebuildHistoryMutation) || legacyResult != nil {
		t.Fatal("a rewind entry point crossed the active usage index rebuild")
	}
	after := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	afterDigest, err := threadapp.MutationBaselineDigest(after)
	if err != nil {
		t.Fatal(err)
	}
	afterEvents, err := os.ReadFile(handler.store.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	if afterDigest != beforeDigest || string(afterEvents) != string(beforeEvents) {
		t.Fatal("rejected history barrier mutated the canonical thread or event log")
	}
	if retry, err := handler.store.CommitCompaction(prepared.CommitRequest()); err != nil || !retry.Committed {
		t.Fatalf("unchanged compaction could not retry after the rebuild drained: committed=%t err=%v", retry.Committed, err)
	}
}

func TestCaseCompactionPreservesSignedAuthorityAndAcceptedHistory(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	authorityRoot := t.TempDir()
	if err := os.Chmod(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	signingAuthority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	authorityStore, err := newServerTestCaseThreadStore(t, filepath.Join(authorityRoot, "records"))
	if err != nil {
		t.Fatal(err)
	}
	signedAuthority, err := casethreadapp.NewRegistry(context.Background(), signingAuthority, authorityStore)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseThreads = signedAuthority
	handler.store.SetCaseThreadAuthority(signedAuthority)
	service := threadapp.NewService(threadapp.Dependencies{
		Repository: handler.store, PublicProjector: handler.publicProjector, CaseThreads: signedAuthority,
		BeginTransition: threadapp.AdaptBeginTransition(handler.runtimeSubagentState().BeginSecurityContextTransition),
	})
	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{"title": "Case compaction", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	at := time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC)
	contexts := make([]domainsecurity.TurnSecurityContext, 0, 4)
	for index := 1; index <= 4; index++ {
		frozen := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: "turn_case_compaction_" + string(rune('0'+index)), WorkspaceRealPath: workspace,
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			CaseID: "case-a", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding-a")),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")),
			ContextEpoch:       7, IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		contexts = append(contexts, frozen)
	}
	current := contexts[len(contexts)-1]
	epochState, err := contextepochapp.BootstrapState(
		threadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, frozen := range contexts {
		if err := domainsecurity.ValidateTurnSecurityContext(frozen); err != nil {
			t.Fatalf("case compaction fixture context is invalid: %#v: %v", frozen, err)
		}
		if err := casethreadapp.RegisterRequired(context.Background(), signedAuthority, frozen); err != nil {
			t.Fatal(err)
		}
		if err := handler.store.AppendTurnToThread(threadID, map[string]any{
			"id": frozen.TurnID, "threadId": threadID, "status": "completed", "securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{
				"id": "case_user_" + string(rune('1'+index)), "turnId": frozen.TurnID, "threadId": threadID,
				"kind": "user_message", "role": "user", "text": "preserve signed case history", "status": "completed",
			}},
		}, "deepseek", map[string]any{
			"securityState": turnsecurityapp.PublicRecord(frozen), "contextEpochState": contextepochapp.PublicState(epochState),
		}); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.CommitRequired(context.Background(), signedAuthority, frozen, epochState, at.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := handler.store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	before, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest, err := threadapp.CompactionBaselineDigest(before)
	if err != nil {
		t.Fatal(err)
	}
	turnIDsBefore := append([]string(nil), signedAuthority.ContextTurnIDs(threadID)...)
	response, compactErr := service.Compact(context.Background(), threadID, "manual")
	if !errors.Is(compactErr, threadapp.ErrCaseCompactionRequiresTrustedArchive) || response != nil {
		t.Fatalf("signed case compaction did not fail closed: response=%#v err=%v", response, compactErr)
	}
	after, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest, err := threadapp.CompactionBaselineDigest(after)
	state, ok, stateErr := contextepochapp.StateFromThread(after)
	turnIDsAfter := signedAuthority.ContextTurnIDs(threadID)
	if err != nil || afterDigest != beforeDigest || stateErr != nil || !ok || state.AcceptedSnapshot.Epoch != current.ContextEpoch ||
		len(listAny(after["turns"])) != len(contexts) || len(turnIDsAfter) != len(turnIDsBefore) || !signedAuthority.ContainsContext(current) {
		t.Fatalf("rejected case compaction changed signed authority/history: before=%s after=%s state=%#v turns=%d authority=%#v err=%v stateErr=%v",
			beforeDigest, afterDigest, state, len(listAny(after["turns"])), turnIDsAfter, err, stateErr)
	}

	// Strip every mutable public case marker and replace the public contexts
	// with internally valid unbound contexts. The signed host inventory remains
	// authoritative and must still prevent deletion from an idle thread.
	downgraded := cloneMap(after)
	downgradedTurns := listAny(downgraded["turns"])
	var downgradedCurrent domainsecurity.TurnSecurityContext
	for index, value := range downgradedTurns {
		turn := value.(map[string]any)
		downgradedContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: stringField(turn, "id"), WorkspaceRealPath: workspace,
			ContextEpoch: current.ContextEpoch, IssuedAt: at.Add(time.Duration(index+1) * time.Second),
		})
		turn["securityContext"] = turnsecurityapp.PublicRecord(downgradedContext)
		downgradedCurrent = downgradedContext
	}
	downgradedState, err := contextepochapp.BootstrapState(
		threadID, downgradedCurrent.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(downgradedCurrent)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"caseId", "caseProjectId", "caseBindingHash", "historyAuthority"} {
		delete(downgraded, key)
	}
	downgraded["status"] = "idle"
	downgraded["securityState"] = turnsecurityapp.PublicRecord(downgradedCurrent)
	downgraded["contextEpochState"] = contextepochapp.PublicState(downgradedState)
	if err := handler.store.ReplaceThreadForAuthorityRepair(threadID, downgraded); err != nil {
		t.Fatal(err)
	}
	attackBefore, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	attackDigest, _ := threadapp.CompactionBaselineDigest(attackBefore)
	if response, err := service.Compact(context.Background(), threadID, "marker stripping"); err == nil || !strings.Contains(err.Error(), "V1 is audit-only") || response != nil {
		t.Fatalf("audit-only V1 marker stripping was not rejected before mutation: response=%#v err=%v", response, err)
	}
	attackAfter, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	attackAfterDigest, _ := threadapp.CompactionBaselineDigest(attackAfter)
	attackState, _, attackStateErr := contextepochapp.StateFromThread(attackAfter)
	if attackAfterDigest != attackDigest || attackStateErr != nil || attackState.AcceptedSnapshot.Epoch != current.ContextEpoch ||
		len(listAny(attackAfter["turns"])) != len(contexts) {
		t.Fatalf("marker-stripping compaction changed durable history/epoch: before=%s after=%s state=%#v err=%v",
			attackDigest, attackAfterDigest, attackState, attackStateErr)
	}
}

func TestCompactionRepositoryRechecksCaseAuthorityAtCAS(t *testing.T) {
	handler, threadID, current := newUnboundCompactionFixture(t)
	state := handler.runtimeSubagentState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), current, time.Second); err != nil {
		t.Fatal(err)
	}
	authority := &flippingCompactionCaseAuthority{threadID: threadID, trueAt: 5}
	handler.store.SetCaseThreadAuthority(authority)
	service := threadapp.NewService(threadapp.Dependencies{
		Repository: handler.store, PublicProjector: handler.publicProjector, CaseThreads: authority,
		WorkspaceReader: filestore.CaseBindingReader{},
		BeginTransition: threadapp.AdaptBeginTransition(handler.runtimeSubagentState().BeginSecurityContextTransition),
	})
	before, err := handler.store.GetThreadForAuthorityRepair(threadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest, _ := threadapp.CompactionBaselineDigest(before)
	response, compactErr := service.Compact(context.Background(), threadID, "authority race")
	if !errors.Is(compactErr, threadapp.ErrCaseCompactionRequiresTrustedArchive) || response != nil || authority.calls.Load() < 5 {
		t.Fatalf("repository CAS did not recheck changed case authority: response=%#v err=%v calls=%d", response, compactErr, authority.calls.Load())
	}
	after, err := handler.store.GetThreadForAuthorityRepair(threadID)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest, _ := threadapp.CompactionBaselineDigest(after)
	if afterDigest != beforeDigest {
		t.Fatalf("authority change between preparation and CAS mutated durable history: before=%s after=%s", beforeDigest, afterDigest)
	}
	// The rejected durable CAS must abort the tentative RuntimeState context.
	binding := compactionJobBinding(t, current)
	_, cancel := context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_compaction_cas_abort_probe", binding, cancel) {
		t.Fatal("repository CAS rejection published tentative compaction authority")
	}
	state.UnregisterBackgroundJob("job_compaction_cas_abort_probe")
	cancel()
}

type flippingCompactionCaseAuthority struct {
	threadID string
	trueAt   int32
	calls    atomic.Int32
}

func (*flippingCompactionCaseAuthority) Register(context.Context, domainsecurity.TurnSecurityContext) error {
	return nil
}
func (*flippingCompactionCaseAuthority) Derive(context.Context, string, string, string) error {
	return nil
}
func (authority *flippingCompactionCaseAuthority) IsCaseThread(threadID string) bool {
	call := authority.calls.Add(1)
	return strings.TrimSpace(threadID) == authority.threadID && call >= authority.trueAt
}
func (*flippingCompactionCaseAuthority) ContainsContext(domainsecurity.TurnSecurityContext) bool {
	return false
}
func (*flippingCompactionCaseAuthority) ContextTurnIDs(string) []string      { return nil }
func (*flippingCompactionCaseAuthority) CanExecute(string) bool              { return true }
func (*flippingCompactionCaseAuthority) ReplaceQuarantine(map[string]string) {}

func newUnboundCompactionMissingSuffixFixture(t *testing.T) (*runtimeServerHandler, string) {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(time.Second) })
	workspace := t.TempDir()
	realWorkspace, err := (filestore.CaseBindingReader{}).WorkspaceRealPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := handler.store.CreateThread(map[string]any{"title": "Compaction crash gap", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	at := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	contexts := make([]domainsecurity.TurnSecurityContext, 0, 4)
	for index := 0; index < 4; index++ {
		contexts = append(contexts, newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: "turn_compaction_gap_" + string(rune('1'+index)), WorkspaceRealPath: realWorkspace,
			ContextEpoch: 1, IssuedAt: at.Add(time.Duration(index) * time.Second),
		}))
	}
	epochState, err := contextepochapp.BootstrapState(
		threadID,
		1,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(contexts[len(contexts)-1])},
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, frozen := range contexts {
		itemID := "item_compaction_gap_" + string(rune('1'+index))
		if err := handler.store.AppendTurnToThread(threadID, map[string]any{
			"id": frozen.TurnID, "threadId": threadID, "status": "running", "prompt": "compact crash gap",
			"securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{
				"id": itemID, "turnId": frozen.TurnID, "threadId": threadID, "kind": "user_message", "role": "user",
				"text": "message for crash-gap compaction", "status": "completed",
			}},
		}, "deepseek", map[string]any{
			"securityState": turnsecurityapp.PublicRecord(frozen), "contextEpochState": contextepochapp.PublicState(epochState),
		}); err != nil {
			t.Fatal(err)
		}
		if index == len(contexts)-1 {
			handler.store.generalTerminalAtomicAppend = func(string, []map[string]any) error {
				return errors.New("simulated terminal append crash")
			}
		}
		finishedAt := at.Add(time.Duration(index+10) * time.Second).Format(time.RFC3339Nano)
		result, commitErr := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
			Store: handler.store, SecurityContext: frozen, ThreadID: threadID, TurnID: frozen.TurnID,
			Model: "deepseek", CreatedAt: finishedAt, FinishedAt: finishedAt,
		})
		handler.store.generalTerminalAtomicAppend = nil
		if index < len(contexts)-1 {
			if commitErr != nil || !result.Changed {
				t.Fatalf("terminal prefix bundle did not commit at index %d: result=%#v err=%v", index, result, commitErr)
			}
		} else if commitErr == nil || !result.Changed {
			t.Fatalf("terminal missing suffix was not preserved at index %d: result=%#v err=%v", index, result, commitErr)
		}
	}
	return handler, threadID
}

func newUnboundCompactionFixture(t *testing.T) (*runtimeServerHandler, string, domainsecurity.TurnSecurityContext) {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(time.Second) })
	workspace := t.TempDir()
	realWorkspace, err := (filestore.CaseBindingReader{}).WorkspaceRealPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := handler.store.CreateThread(map[string]any{"title": "Epoch compaction", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	at := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
	contexts := make([]domainsecurity.TurnSecurityContext, 0, 4)
	for index := 1; index <= 4; index++ {
		contexts = append(contexts, newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: "turn_compaction_" + string(rune('0'+index)), WorkspaceRealPath: realWorkspace,
			ContextEpoch: 1, IssuedAt: at.Add(time.Duration(index) * time.Second),
		}))
	}
	current := contexts[len(contexts)-1]
	epochState, err := contextepochapp.BootstrapState(
		threadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, frozen := range contexts {
		itemID := "item_compaction_" + string(rune('1'+index))
		if err := handler.store.AppendTurnToThread(threadID, map[string]any{
			"id": frozen.TurnID, "threadId": threadID, "status": "running", "prompt": "compact",
			"securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{
				"id": itemID, "turnId": frozen.TurnID, "threadId": threadID, "kind": "user_message", "role": "user",
				"text": "message for compaction", "status": "completed",
			}},
		}, "deepseek", map[string]any{
			"securityState": turnsecurityapp.PublicRecord(frozen), "contextEpochState": contextepochapp.PublicState(epochState),
		}); err != nil {
			t.Fatal(err)
		}
		finishedAt := at.Add(time.Duration(index+10) * time.Second).Format(time.RFC3339Nano)
		if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
			Store: handler.store, SecurityContext: frozen, ThreadID: threadID, TurnID: frozen.TurnID,
			Model: "deepseek", CreatedAt: finishedAt, FinishedAt: finishedAt,
		}); err != nil || !result.Changed {
			t.Fatalf("commit compaction fixture turn %s: result=%#v err=%v", frozen.TurnID, result, err)
		}
	}
	return handler, threadID, current
}

func compactionJobBinding(t *testing.T, current domainsecurity.TurnSecurityContext) *domainjob.SecurityBinding {
	t.Helper()
	arguments, _ := json.Marshal(map[string]any{"prompt": "mutate"})
	callID := serverTestHostToolCallID("compaction_job")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: current, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	binding, err := domainjob.NewSecurityBinding(current, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func (*flippingCompactionCaseAuthority) RestartPreservesThreadV1(string) bool { return false }
