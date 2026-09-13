//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/jobs"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	"analytix.local/runtime-go/internal/server"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func runtimeDerivedReportHistoryFixtureV1(t *testing.T, derivation string, compacted ...bool) (*runtimeOriginalReservedReportHistoryFixtureV1, *runtimeOriginalReservedReportHistoryFixtureV1, map[string]any) {
	t.Helper()
	return runtimeDerivedReportHistoryWithLegacyFinalDisplayFixtureV1(t, derivation, false, compacted...)
}

func runtimeDerivedReportHistoryWithLegacyFinalDisplayFixtureV1(t *testing.T, derivation string, legacyFinalDisplay bool, compacted ...bool) (*runtimeOriginalReservedReportHistoryFixtureV1, *runtimeOriginalReservedReportHistoryFixtureV1, map[string]any) {
	t.Helper()
	ctx := context.Background()
	witness, config := runtimeWitnessedRegistryConfigV2(t)
	workspace := workspacetest.New(t)
	contextFor := func(threadID, turnID string) domainsecurity.TurnSecurityContext {
		frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
			CaseID: "case-derived-report", CaseBindingHash: domainsecurity.SHA256Hex([]byte("derived-report-binding")),
			ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		return frozen
	}
	sourceContext := contextFor("thread-derived-source", "turn_920")
	reportContext := sourceContext
	if len(compacted) > 0 && compacted[0] {
		// Keep the closed report checkpoint independent of the synthetic
		// completed source turns admitted to the real compaction producer.
		reportContext = contextFor("thread-derived-checkpoint", "turn-derived-checkpoint")
	}
	source := newRuntimeOriginalReportAtInstallationFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{
		postWitnessCut: publicationapp.RestartAttemptStageDispositionV1,
	}, witness, config, reportContext)
	caseStore, err := casestore.NewStore(filepath.Join(source.core.roots.DataDir, "private", "case-thread-authority"), source.core.access)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := casethreadapp.NewRegistry(ctx, witness.Authority, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := server.NewProductionDurableEventSessionStore(source.core.roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	durable.SetCaseThreadAuthority(registry)
	if len(compacted) > 0 && compacted[0] {
		created, err := durable.CreateThread(map[string]any{"title": "compacted case source"}, "")
		if err != nil {
			t.Fatal(err)
		}
		createdID, _ := created["id"].(string)
		if createdID == "" {
			t.Fatal("compaction source has no identity")
		}
		sourceContext = contextFor(createdID, "turn_920")
		var finalizer evidenceapp.CasePublicationFinalizer
		checkFinalCAS := func(string) {}
		if len(compacted) == 2 && compacted[1] {
			// Native startup requires terminal authority for the source itself.
			// Publish real boundary finals before compaction; it retains those
			// protected turns and adds a real compaction authority boundary.
			privateRoot := filepath.Join(source.core.roots.DataDir, "private")
			shared := runtimeWitnessedRegistryDirectCompositionV2(t, witness, config)
			finals, err := finalauthority.NewPrivateStore(filepath.Join(privateRoot, "accepted-finals"), source.core.access)
			if err != nil {
				t.Fatal(err)
			}
			cas, err := finalauthority.NewAcceptedFinalCASReader(source.core.roots.DurableDir)
			if err != nil {
				t.Fatal(err)
			}
			providerStore, err := cachetelemetrystore.NewStore(filepath.Join(privateRoot, "provider-cache-telemetry"), source.core.access)
			if err != nil {
				t.Fatal(err)
			}
			provider, err := cachetelemetryapp.NewDurableService(witness.Authority, providerStore)
			if err != nil {
				t.Fatal(err)
			}
			terminals, err := turnterminalstore.NewStore(filepath.Join(privateRoot, "turn-terminal-authority"), source.core.access)
			if err != nil {
				t.Fatal(err)
			}
			coordinator, err := turnterminalapp.NewCoordinator(witness.Authority, finals, terminals, provider)
			if err != nil {
				t.Fatal(err)
			}
			finalizer = evidenceapp.NewCasePublicationFinalizerWithAuthority(shared.registry, shared.registry, witness.Authority, finals, startupFinalEventIOForTest(durable, cas), coordinator)
			checkFinalCAS = func(stage string) {
				t.Helper()
				if err := finals.VisitDispositions(ctx, func(disposition domainevidence.AcceptedFinalDispositionRecord) error {
					observed, err := cas.ReadAcceptedFinalCASObservation(ctx, disposition.ThreadID, disposition.TurnID)
					if err != nil {
						return err
					}
					if disposition.TurnCASDigest != observed.TurnProjectionSHA256 || !observed.HasWinner || disposition.WinnerDigest != observed.Winner.RecordDigest {
						return fmt.Errorf("source final CAS changed at %s for %s", stage, disposition.TurnID)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
		}
		// Create real committed turns, then persist both compaction producers
		// before deriving history. Never insert projection markers in a fixture.
		for index := 0; index <= 3; index++ {
			frozen := contextFor(sourceContext.ThreadID, fmt.Sprintf("turn_%d", 920+index))
			issuedAt, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
			if err != nil {
				t.Fatal(err)
			}
			at := issuedAt.Add(time.Duration(index) * time.Second)
			state, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch,
				[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, at)
			if err != nil {
				t.Fatal(err)
			}
			if err := casethreadapp.RegisterRequired(ctx, registry, frozen); err != nil {
				t.Fatal(err)
			}
			status := "completed"
			if finalizer != nil {
				status = "running"
			}
			if err := durable.AppendTurnToThread(frozen.ThreadID, map[string]any{
				"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": status,
				"prompt": "retain bounded case history", "finishedAt": at.Format(time.RFC3339Nano),
				"contextEpochSnapshot": contextepochapp.PublicSnapshot(state.AcceptedSnapshot),
				"securityContext":      frozen, "items": []any{map[string]any{
					"id": fmt.Sprintf("user_compaction_%d", index), "threadId": frozen.ThreadID, "turnId": frozen.TurnID,
					"kind": "user_message", "role": "user", "status": "completed", "text": "retained case request",
				}},
			}, "synthetic-provider", map[string]any{"securityState": frozen, "contextEpochState": contextepochapp.PublicState(state)}); err != nil {
				t.Fatal(err)
			}
			if err := casethreadapp.CommitRequired(ctx, registry, frozen, state, at); err != nil {
				t.Fatal(err)
			}
			if finalizer != nil {
				result, err := finalizer.PersistBoundary(ctx, evidenceapp.PersistCaseBoundaryInput{
					Store: durable, Context: frozen, TerminalReason: evidenceapp.TerminalProviderFailure,
					ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, AcceptedAt: at.Add(time.Minute),
				})
				if err != nil || !result.Persistence.Changed {
					t.Fatalf("real source boundary final did not commit: %v", err)
				}
				checkFinalCAS("terminal commit")
			}
		}
		if _, err := durable.PatchThread(sourceContext.ThreadID, map[string]any{"status": "idle"}); err != nil {
			t.Fatal(err)
		}
		checkFinalCAS("thread patch")
		current, err := durable.GetThreadForAuthorityRepair(sourceContext.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range current["turns"].([]any) {
			status := raw.(map[string]any)["status"]
			if status != "completed" && status != "aborted" && status != "failed" {
				t.Fatal("compaction fixture contains an active source turn")
			}
		}
		continuation, err := turnapp.BuildTaskContinuationSnapshotV1(current)
		if err != nil {
			t.Fatal(err)
		}
		frozen, err := domainsecurity.ParseTurnSecurityContext(current["securityState"])
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := threadapp.PrepareCaseCompaction(current, sourceContext.ThreadID, "manual", time.Now().UTC(), false,
			threadapp.CaseCompactionAuthorization{Continuation: continuation, SourceContextDigest: frozen.ContextDigest, AuthorityTurnIDs: casethreadapp.CommittedTurnIDs(registry, sourceContext.ThreadID)})
		if err != nil {
			t.Fatal(err)
		}
		committed, err := durable.CommitCompaction(prepared.CommitRequest())
		if err != nil || !committed.AuthorityCommitted || !committed.Committed {
			t.Fatalf("real case compaction did not commit: result=%#v err=%v", committed, err)
		}
		checkFinalCAS("compaction commit")
		current, err = durable.GetThreadForAuthorityRepair(sourceContext.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		for _, turnID := range casethreadapp.CommittedTurnIDs(registry, sourceContext.ThreadID) {
			record, found := registry.CommittedContext(sourceContext.ThreadID, turnID)
			if !found {
				t.Fatal("compaction source lost committed context")
			}
			repaired, err := turnapp.RepairCommittedTurnContext(turnapp.CommittedContextRepairInput{Thread: current, SecurityContext: record.SecurityContext, EpochState: record.EpochState})
			if err != nil {
				t.Fatalf("compacted source is not restart-consistent for %s: %v", turnID, err)
			}
			beforeJSON, beforeErr := json.Marshal(current)
			afterJSON, afterErr := json.Marshal(repaired)
			if beforeErr != nil || afterErr != nil || string(beforeJSON) != string(afterJSON) {
				t.Fatalf("compacted source needs a startup context repair for %s", turnID)
			}
		}
	}
	// These restart fixtures represent legacy unbound lineage, including report
	// checkpoints that intentionally cannot pass current active admission. Use
	// the real legacy signer and durable writer without creating an active binding.
	sourceThread, err := durable.GetThreadForAuthorityRepair(sourceContext.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	targetID := "thread-legacy-derived-" + derivation
	if _, err := os.Lstat(filepath.Join(source.core.roots.DurableDir, "threads", targetID)); !os.IsNotExist(err) {
		t.Fatalf("legacy derivation target is not absent: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var legacyTarget map[string]any
	switch derivation {
	case "fork":
		legacyTarget, err = threadapp.BuildFork(threadapp.AuthorizeCaseForkV1(threadapp.ForkInput{
			Source: sourceThread, ForkID: targetID, ParentThreadID: sourceContext.ThreadID, Now: now,
		}))
	case "resume":
		legacyTarget, err = threadapp.BuildResume(threadapp.AuthorizeCaseResumeV1(threadapp.ResumeInput{
			Source: sourceThread, ThreadID: targetID, SessionID: sourceContext.ThreadID, Now: now,
		}))
	default:
		t.Fatalf("unsupported legacy derivation %q", derivation)
	}
	if err != nil {
		t.Fatal(err)
	}
	// The old derived format retained source compaction markers. Restore those
	// exact marker values only in this historical fixture; new producers strip them.
	sourceTurns, _ := sourceThread["turns"].([]any)
	legacyTurns, _ := legacyTarget["turns"].([]any)
	if len(sourceTurns) != len(legacyTurns) {
		t.Fatal("legacy derivation changed inherited turn count")
	}
	legacyFinalDisplays := 0
	for index, value := range sourceTurns {
		sourceTurn := value.(map[string]any)
		legacyTurn := legacyTurns[index].(map[string]any)
		if sourceTurn["id"] != legacyTurn["id"] {
			t.Fatal("legacy derivation changed inherited turn identity")
		}
		if marker, present := sourceTurn["caseHistoryProjection"]; present {
			legacyTurn["caseHistoryProjection"] = marker
		} else if view, present := sourceTurn["acceptedFinalView"]; legacyFinalDisplay && present {
			// Historical derivation retained the source's real final display.
			// Current BuildFork/BuildResume intentionally remove it; restore it
			// only for the exact legacy preservation contract under test.
			legacyTurn["acceptedFinalView"] = contracts.CloneValue(view)
			legacyFinalDisplays++
		}
	}
	if legacyFinalDisplay && legacyFinalDisplays == 0 {
		t.Fatal("real source finalizer produced no legacy final display fixture")
	}
	receipt, err := registry.DeriveWithReceipt(ctx, sourceContext.ThreadID, targetID, derivation)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ActiveInheritedHistory != nil {
		t.Fatal("legacy lineage acquired active inherited history authority")
	}
	if err := durable.ReplaceThreadForAuthorityRepair(targetID, legacyTarget); err != nil {
		t.Fatal(err)
	}
	records, err := caseStore.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := casethreadapp.VerifyCommittedContextInventoryV1(ctx, records, witness.Authority); err != nil {
		t.Fatal(err)
	}
	lineages := 0
	for _, record := range records {
		if domainsecurity.CaseThreadAuthorityIsLineage(record) && record.ThreadID == targetID {
			if err := casethreadapp.ValidateDerivedLineageReceipt(record, sourceContext.ThreadID, targetID, derivation); err != nil {
				t.Fatal(err)
			}
			lineages++
		}
		if record.SecurityContext != nil && record.SecurityContext.ThreadID == targetID {
			t.Fatal("derived history acquired execution context")
		}
	}
	if lineages != 1 {
		t.Fatal("real derivation did not persist one exact lineage")
	}
	read := func(path string) map[string]any {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var thread map[string]any
		if err := json.Unmarshal(body, &thread); err != nil {
			t.Fatal(err)
		}
		return thread
	}
	primary := filepath.Join(source.core.roots.DurableDir, "threads", targetID, "thread.json")
	before := read(primary)
	turns := before["turns"].([]any)
	if len(turns) == 0 {
		t.Fatal("real derivation lost inherited turn")
	}
	inherited := turns[0].(map[string]any)
	if inherited["id"] != sourceContext.TurnID {
		t.Fatal("real derivation changed historical turn identity")
	}
	if _, present := inherited["securityContext"]; present {
		t.Fatal("real derivation retained source context")
	}
	for _, raw := range inherited["items"].([]any) {
		if raw.(map[string]any)["kind"] != "user_message" {
			t.Fatal("real derivation retained execution items")
		}
	}
	target := newRuntimeOriginalReportAtInstallationFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{
		preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, existingHead: true, deferRefresh: true,
	}, witness, config, contextFor(targetID, "turn-derived-new-report"))
	after := read(primary)
	if len(after["turns"].([]any)) != len(turns)+1 || !reflect.DeepEqual(turns, after["turns"].([]any)[:len(turns)]) {
		t.Fatal("new report rewrote inherited history")
	}
	return source, target, inherited
}

func TestRuntimeOriginalDerivedReportHistory(t *testing.T) {
	for _, derivation := range []string{"fork", "resume"} {
		t.Run(derivation, func(t *testing.T) {
			_, target, _ := runtimeDerivedReportHistoryFixtureV1(t, derivation)
			before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
			err := target.refreshE()
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
				t.Fatal("read-only refresh changed original derived history")
			}
			if err != nil {
				t.Fatalf("real %s history blocked report refresh: %v", derivation, err)
			}
			contexts := target.scope.Contexts()
			if len(contexts) != 1 || contexts[0].TurnID != "turn-derived-new-report" {
				t.Fatal("inherited history gained execution context")
			}
			ctx := context.Background()
			store, err := pendingstore.NewStoreContext(ctx, filepath.Join(target.core.roots.DataDir, "private", "pending-work"), target.core.access)
			if err != nil {
				t.Fatal(err)
			}
			service := pendingapp.NewService(target.core.verification, store, runtimeOriginalReportThreadsV1{ctx, target.core.primaries})
			if err := service.PreserveReportRestartScopeV1(ctx, *target.scope); err != nil {
				t.Fatal(err)
			}
			ids := service.RestartPreservedTurnIDsV1()
			if len(ids) != 2 || !containsDerivedHistoryIDV1(ids, "turn_920") || !containsDerivedHistoryIDV1(ids, "turn-derived-new-report") {
				t.Fatal("inherited turn allocation identity was released")
			}
		})
	}
}

type runtimeInheritedExecutionDenialV1 string

func TestRuntimeCompactedDerivedPublicListProjection(t *testing.T) {
	for _, derivation := range []string{"fork", "resume"} {
		t.Run(derivation, func(t *testing.T) {
			_, target, _ := runtimeDerivedReportHistoryFixtureV1(t, derivation, true)
			target.refresh(t)
			ctx := context.Background()
			caseStore, err := casestore.NewStore(filepath.Join(target.core.roots.DataDir, "private", "case-thread-authority"), target.core.access)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := casethreadapp.NewRegistry(ctx, target.core.verification, caseStore)
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.PreserveRestartScopeV1(ctx, target.scope.DeniedThreadIDsV1()); err != nil {
				t.Fatal(err)
			}
			durable, err := server.NewProductionDurableEventSessionStore(target.core.roots.DurableDir)
			if err != nil {
				t.Fatal(err)
			}
			durable.SetCaseThreadAuthority(registry)
			id := target.scope.Contexts()[0].ThreadID
			thread, err := durable.GetThread(id)
			if err != nil {
				t.Fatal(err)
			}
			projector := threadapp.NewTrustedPublicProjectorWithCaseThreads(nil, registry)
			if _, err := projector.ProjectThread(thread); err == nil {
				t.Fatal("inherited compaction passed without its sealed proof")
			}
			before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
			projector = threadapp.NewTrustedPublicProjectorWithPreservedHistoryV1(nil, registry, nil, nil, target.scope)
			projected, err := projector.ProjectThread(thread)
			if err != nil {
				t.Fatal(err)
			}
			if projected["historyAuthority"] != threadapp.CaseBoundaryOnlyHistoryAuthority || projected["workspace"] != nil || len(projected["turns"].([]any)) != len(thread["turns"].([]any)) {
				t.Fatal("held listing lost its private metadata-only boundary")
			}
			for _, raw := range projected["turns"].([]any) {
				turn := raw.(map[string]any)
				if len(turn["items"].([]any)) != 0 || turn["caseHistoryProjection"] != nil || turn["securityContext"] != nil || turn["acceptedFinal"] != nil {
					t.Fatal("inherited listing supplied content or execution authority")
				}
			}
			for _, fault := range []string{"null_context", "changed_marker", "changed_metadata"} {
				t.Run(fault, func(t *testing.T) {
					body, err := json.Marshal(thread)
					if err != nil {
						t.Fatal(err)
					}
					var changed map[string]any
					if err := json.Unmarshal(body, &changed); err != nil {
						t.Fatal(err)
					}
					turn := changed["turns"].([]any)[0].(map[string]any)
					switch fault {
					case "null_context":
						turn["securityContext"] = nil
					case "changed_marker":
						turn["caseHistoryProjection"] = "unknown_authority"
					case "changed_metadata":
						turn["model"] = "changed"
					}
					if _, err := projector.ProjectThread(changed); err == nil {
						t.Fatal("unproved inherited listing was admitted")
					}
				})
			}
			unheld, err := casethreadapp.NewRegistry(ctx, target.core.verification, caseStore)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := threadapp.NewTrustedPublicProjectorWithPreservedHistoryV1(nil, unheld, nil, nil, target.scope).ProjectThread(thread); err == nil {
				t.Fatal("unheld marker used a history-only exception")
			}
			if _, err := threadapp.NewTrustedPublicProjectorWithPreservedHistoryV1(nil, registry, nil, nil, pendingapp.ReportRestartScopeV1{}).ProjectThread(thread); err == nil {
				t.Fatal("unsealed observer supplied inherited authority")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
				t.Fatal("public projection modified preserved bytes")
			}
			body, err := os.ReadFile(target.primary)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target.primary, append(body, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := projector.ProjectThread(thread); err == nil {
				t.Fatal("stale public observation survived primary drift")
			}
		})
	}
}

func (held runtimeInheritedExecutionDenialV1) CanExecute(threadID string) bool {
	return threadID != string(held)
}

func TestRuntimeInheritedFinalReadersKeepSealedDenial(t *testing.T) {
	_, target, _ := runtimeDerivedReportHistoryWithLegacyFinalDisplayFixtureV1(t, "fork", true, true, true)
	target.refresh(t)
	ctx := context.Background()
	heldID := target.scope.Contexts()[0].ThreadID
	snapshot, err := target.scope.ReadPrimaryThreadSnapshotV1(ctx, heldID)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	for _, raw := range snapshot.Thread["turns"].([]any) {
		turn := raw.(map[string]any)
		if turn["caseHistoryProjection"] == "compaction_authority_v1" {
			summary = turn
		}
	}
	if summary == nil {
		t.Fatal("real compaction boundary is missing")
	}
	caseStore, err := casestore.NewStore(filepath.Join(target.core.roots.DataDir, "private", "case-thread-authority"), target.core.access)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := casethreadapp.NewRegistry(ctx, target.core.verification, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := server.NewProductionDurableEventSessionStore(target.core.roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	rawReader := threadapp.NewCaseThreadRestartAuthorityReaderV1(ctx, durable, registry)
	wrapped := runtimeRestartFinalPublicReaderV1(ctx, rawReader, target.scope)
	type validator interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	}
	type committedReader interface {
		CommittedContext(string, string) (casethreadapp.CommittedContext, bool)
	}
	before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
	if rawReader.(validator).ValidateCaseCompactionAuthorityTurnV1(heldID, snapshot.Thread, summary) == nil {
		t.Fatal("inherited summary gained signed execution authority")
	}
	if err := wrapped.(validator).ValidateCaseCompactionAuthorityTurnV1(heldID, snapshot.Thread, summary); err != nil {
		t.Fatal(err)
	}
	rawIDs, err := rawReader.AllThreadIDs()
	if err != nil {
		t.Fatal(err)
	}
	wrappedIDs, err := wrapped.AllThreadIDs()
	if err != nil || !reflect.DeepEqual(rawIDs, wrappedIDs) {
		t.Fatal("final reader filtered the inventory", err)
	}
	currentBoundaries := 0
	for _, id := range rawIDs {
		if id == heldID {
			continue
		}
		thread, err := rawReader.GetThread(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range thread["turns"].([]any) {
			turn := raw.(map[string]any)
			if turn["caseHistoryProjection"] != "compaction_authority_v1" {
				continue
			}
			if err := rawReader.(validator).ValidateCaseCompactionAuthorityTurnV1(id, thread, turn); err != nil {
				t.Fatal(err)
			}
			if err := wrapped.(validator).ValidateCaseCompactionAuthorityTurnV1(id, thread, turn); err != nil {
				t.Fatal(err)
			}
			expected, found := rawReader.(committedReader).CommittedContext(id, turn["id"].(string))
			actual, forwarded := wrapped.(committedReader).CommittedContext(id, turn["id"].(string))
			if !found || !forwarded || !reflect.DeepEqual(expected, actual) {
				t.Fatal("current committed context was hidden or changed")
			}
			currentBoundaries++
		}
	}
	if currentBoundaries != 1 {
		t.Fatal("real source compaction authority was not checked")
	}
	for _, fault := range []string{"null_context", "changed_history", "erased_current_context", "wrong_thread", "unsealed_scope", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			turn := make(map[string]any, len(summary))
			for key, value := range summary {
				turn[key] = value
			}
			thread := snapshot.Thread
			scope := *target.scope
			checkCtx := ctx
			switch fault {
			case "null_context":
				turn["securityContext"] = nil
			case "changed_history":
				turn["model"] = "changed"
			case "erased_current_context":
				last := snapshot.Thread["turns"].([]any)
				for key := range turn {
					delete(turn, key)
				}
				for key, value := range last[len(last)-1].(map[string]any) {
					turn[key] = value
				}
				delete(turn, "securityContext")
			case "wrong_thread":
				thread = map[string]any{"id": "wrong-thread"}
			case "unsealed_scope":
				scope = pendingapp.ReportRestartScopeV1{}
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				checkCtx = cancelled
			}
			if scope.ValidateInheritedTurnV1(checkCtx, thread, turn) == nil {
				t.Fatal("unproved history was admitted")
			}
		})
	}
	executable := threadapp.NewExecutableAuthorityReader(wrapped, runtimeInheritedExecutionDenialV1(heldID))
	if _, err := executable.GetThread(heldID); err == nil {
		t.Fatal("held history became executable")
	}
	if executable.(validator).ValidateCaseCompactionAuthorityTurnV1(heldID, snapshot.Thread, summary) == nil {
		t.Fatal("held compaction became executable")
	}
	if _, found := executable.(committedReader).CommittedContext(heldID, summary["id"].(string)); found {
		t.Fatal("held history acquired a committed context")
	}
	root := target.core.roots.DurableDir
	index := filepath.Join(root, "thread_summaries.jsonl")
	contextlessFinalDisplays := 0
	for _, raw := range snapshot.Thread["turns"].([]any) {
		turn := raw.(map[string]any)
		if view, present := turn["acceptedFinalView"]; present {
			if _, hasContext := turn["securityContext"]; !hasContext {
				if view == nil {
					t.Fatal("legacy final display fixture is null")
				}
				contextlessFinalDisplays++
			}
		}
	}
	if contextlessFinalDisplays == 0 {
		t.Fatal("sealed-denial fixture has no contextless accepted-final display")
	}
	if _, err := eventlog.PrepareSemanticRestartPreservationV1(ctx, root, index, []string{heldID}); err == nil {
		t.Fatal("contextless accepted-final display passed without inherited proof")
	}
	preserved, err := eventlog.PrepareSemanticRestartPreservationWithAbsentThreadsV1(ctx, root, index, []string{heldID}, nil, target.scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := preserved.Revalidate(ctx, root); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
		t.Fatal("sealed observation or refusal changed Original bytes")
	}
	body, err := os.ReadFile(target.primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.primary, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	drift := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
	if target.scope.ValidateInheritedTurnV1(ctx, snapshot.Thread, summary) == nil {
		t.Fatal("changed primary reused a sealed proof")
	}
	if preserved.Revalidate(ctx, root) == nil {
		t.Fatal("changed primary passed event preservation")
	}
	if !reflect.DeepEqual(drift, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
		t.Fatal("drift refusal rewrote the observed bytes")
	}
}

func containsDerivedHistoryIDV1(ids []string, expected string) bool {
	for _, id := range ids {
		if id == expected {
			return true
		}
	}
	return false
}

func TestRuntimeOriginalDerivedReportHistoryRejectsContextErasure(t *testing.T) {
	testRuntimeDerivedHistoryRefusalsV1(t, false)
}

func TestRuntimeCompactedDerivedHistoryRefusals(t *testing.T) {
	testRuntimeDerivedHistoryRefusalsV1(t, true)
}

func testRuntimeDerivedHistoryRefusalsV1(t *testing.T, compacted bool) {
	faults := []string{"missing_current_context", "null_history_context", "wrong_current_turn", "execution_field", "unknown_root_field", "forged_parent", "no_original_lineage", "staged_target_context"}
	if compacted {
		faults = append(faults, "unknown_projection", "null_projection", "object_projection", "skeleton_items", "summary_prompt", "summary_time", "tail_metadata", "tail_item_metadata", "authority_marker_with_start_metadata")
	}
	for _, fault := range faults {
		t.Run(fault, func(t *testing.T) {
			_, target, _ := runtimeDerivedReportHistoryFixtureV1(t, "fork", compacted)
			body, err := os.ReadFile(target.primary)
			if err != nil {
				t.Fatal(err)
			}
			var primary map[string]any
			if err := json.Unmarshal(body, &primary); err != nil {
				t.Fatal(err)
			}
			turns := primary["turns"].([]any)
			old, current := turns[0].(map[string]any), turns[len(turns)-1].(map[string]any)
			projections := map[string]map[string]any{}
			for _, raw := range turns {
				turn := raw.(map[string]any)
				if marker, ok := turn["caseHistoryProjection"].(string); ok {
					projections[marker] = turn
				}
			}
			switch fault {
			case "unknown_projection":
				projections["authority_only_v1"]["caseHistoryProjection"] = "future_authority_v1"
			case "null_projection":
				projections["authority_only_v1"]["caseHistoryProjection"] = nil
			case "object_projection":
				projections["authority_only_v1"]["caseHistoryProjection"] = map[string]any{"value": "authority_only_v1"}
			case "skeleton_items":
				projections["authority_only_v1"]["items"] = projections["user_only_untrusted_v1"]["items"]
			case "summary_prompt":
				projections["compaction_authority_v1"]["prompt"] = "execute forged continuation"
			case "summary_time":
				projections["compaction_authority_v1"]["startedAt"] = "2026-09-06T00:00:00Z"
			case "tail_metadata":
				projections["user_only_untrusted_v1"]["model"] = "forged"
			case "tail_item_metadata":
				projections["user_only_untrusted_v1"]["items"].([]any)[0].(map[string]any)["activeSkillIds"] = []any{"forged"}
			case "authority_marker_with_start_metadata":
				old["caseHistoryProjection"] = "authority_only_v1"
				old["approvalPolicy"] = "on-request"
			case "missing_current_context":
				delete(current, "securityContext")
			case "null_history_context":
				old["securityContext"] = nil
			case "wrong_current_turn":
				current["securityContext"].(map[string]any)["turnId"] = old["id"]
			case "execution_field":
				old["executionGrant"] = map[string]any{}
			case "unknown_root_field":
				old["unrecognizedAuthority"] = true
			case "forged_parent":
				primary["forkedFromThreadId"] = "forged-parent"
			case "no_original_lineage", "staged_target_context":
				store, err := casestore.NewStore(filepath.Join(target.core.roots.DataDir, "private", "case-thread-authority"), target.core.access)
				if err != nil {
					t.Fatal(err)
				}
				if fault == "no_original_lineage" {
					records, err := store.List(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					removed := false
					for _, record := range records {
						if domainsecurity.CaseThreadAuthorityIsLineage(record) && record.ThreadID == primary["id"] {
							if err := os.Remove(filepath.Join(target.core.roots.DataDir, "private", "case-thread-authority", record.RecordDigest[:2], record.RecordDigest+".json")); err != nil {
								t.Fatal(err)
							}
							removed = true
						}
					}
					if !removed {
						t.Fatal("fixture has no lineage to erase")
					}
				} else {
					frozen, err := domainsecurity.ParseTurnSecurityContext(current["securityContext"])
					if err != nil {
						t.Fatal(err)
					}
					staged, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: frozen.ThreadID, TurnID: old["id"].(string), WorkspaceRealPath: frozen.WorkspaceRealPath, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
					if err != nil {
						t.Fatal(err)
					}
					key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(target.core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
					if err != nil {
						t.Fatal(err)
					}
					registry, err := casethreadapp.NewRegistry(context.Background(), key, store)
					if err != nil {
						t.Fatal(err)
					}
					if err := registry.Register(context.Background(), staged); err != nil {
						t.Fatal(err)
					}
				}
			}
			body, err = json.Marshal(primary)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target.primary, body, 0600); err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
			if err := target.refreshE(); err == nil {
				t.Fatal("invalid derived history was accepted")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
				t.Fatal("derived history refusal changed Original bytes")
			}
		})
	}
}

func TestRuntimeDerivedReportHistoryOrdinaryHTTPWitnessOutage(t *testing.T) {
	source, target, _ := runtimeDerivedReportHistoryFixtureV1(t, "fork")
	target.refresh(t)
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, publicationapp.RestartAttemptStageDispositionV1, source)
}

func TestRuntimeCompactedDerivedNativeTerminalScope(t *testing.T) {
	_, target, _ := runtimeDerivedReportHistoryFixtureV1(t, "fork", true, true)
	body, err := os.ReadFile(target.primary)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := finalauthority.ParsePrimaryThreadSnapshotV1(context.Background(), target.attempt.ThreadID, body)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range snapshot.Thread["turns"].([]any) {
		turn := raw.(map[string]any)
		if _, present := turn["securityContext"]; present {
			continue
		}
		originalErr := threadapp.ValidateCaseDerivedHistoryTurnV1(target.attempt.ThreadID, turn)
		if originalErr != nil {
			encoded, err := json.Marshal(turn)
			if err != nil {
				t.Fatal(err)
			}
			var ordinary map[string]any
			if err := json.Unmarshal(encoded, &ordinary); err != nil {
				t.Fatal(err)
			}
			fields := map[string]string{}
			for key, value := range turn {
				if !reflect.DeepEqual(value, ordinary[key]) {
					fields[key] = fmt.Sprintf("%T -> %T", value, ordinary[key])
				}
			}
			t.Fatalf("actual terminal history rejected: original=%v ordinaryJSON=%v typeDifferences=%v", originalErr, threadapp.ValidateCaseDerivedHistoryTurnV1(target.attempt.ThreadID, ordinary), fields)
		}
	}
	target.refresh(t)
}

func TestRuntimeCompactedDerivedReportHistoryOrdinaryHTTPWitnessOutage(t *testing.T) {
	source, target, _ := runtimeDerivedReportHistoryFixtureV1(t, "fork", true, true)
	target.refresh(t)
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, publicationapp.RestartAttemptStageDispositionV1, source)
}

func TestRuntimeDerivedReportHistoryEarlySemanticObservation(t *testing.T) {
	for _, futureOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "original_lineage", true: "final_only_lineage"}[futureOnly], func(t *testing.T) {
			_, target, _ := runtimeDerivedReportHistoryFixtureV1(t, "resume", true)
			target.refresh(t)
			const prefix, tail = "private/aa-derived-prefix.txt", "private/z-derived-tail.txt"
			var lineageBody []byte
			var lineagePath string
			if futureOnly {
				store, err := casestore.NewStore(filepath.Join(target.core.roots.DataDir, "private", "case-thread-authority"), target.core.access)
				if err != nil {
					t.Fatal(err)
				}
				records, err := store.List(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				for _, record := range records {
					if domainsecurity.CaseThreadAuthorityIsLineage(record) && record.ThreadID == target.scope.Contexts()[0].ThreadID {
						lineageBody, err = domainsecurity.CaseThreadAuthorityRecordBytes(record)
						if err != nil {
							t.Fatal(err)
						}
						lineagePath = filepath.Join("private", "case-thread-authority", record.RecordDigest[:2], record.RecordDigest+".json")
						if err := os.Remove(filepath.Join(target.core.roots.DataDir, lineagePath)); err != nil {
							t.Fatal(err)
						}
					}
				}
				if lineagePath == "" {
					t.Fatal("no Original lineage for future-only fixture")
				}
			}
			absent := tail
			if futureOnly {
				absent = lineagePath
			}
			runtimePendingSemanticCutForTestV1(t, target.core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if lineagePath != "" {
					if err := os.WriteFile(filepath.Join(stage.DataDir, lineagePath), lineageBody, 0600); err != nil {
						return err
					}
				}
				if err := os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied"), 0600); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(stage.DataDir, tail), []byte("remaining"), 0600)
			}, prefix, absent)
			before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
			observed, err := runtimeObservePendingCoreForTestV1(t, target.core)
			if futureOnly {
				if err == nil || observed != nil {
					t.Fatal("Final-only lineage supplied Original history authority")
				}
			} else if err != nil || observed == nil || observed.pendingSemantic == nil {
				t.Fatalf("early semantic pending observer rejected real inherited history: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
				t.Fatal("early derived observation changed original bytes")
			}
		})
	}
}

func TestRuntimeDerivedChildHistoryUsesOriginalScope(t *testing.T) {
	ctx := context.Background()
	core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
	job := core.jobRecords[0]
	parent, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, job.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, targets[0].ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext, err := domainsecurity.ParseTurnSecurityContext(parent.Thread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	childContext, err := domainsecurity.ParseTurnSecurityContext(child.Thread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	store, err := casestore.NewStore(filepath.Join(core.roots.DataDir, "private", "case-thread-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := casethreadapp.NewRegistry(ctx, key, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, parentContext); err != nil {
		t.Fatal(err)
	}
	derived, err := threadapp.BuildFork(threadapp.AuthorizeCaseForkV1(threadapp.ForkInput{Source: parent.Thread, ForkID: child.ThreadID, ParentThreadID: parent.ThreadID, Now: "2026-09-07T00:01:00Z"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Derive(ctx, parent.ThreadID, child.ThreadID, "fork"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, childContext); err != nil {
		t.Fatal(err)
	}
	derived["turns"] = append(derived["turns"].([]any), child.Thread["turns"].([]any)...)
	derived["securityState"] = child.Thread["securityState"]
	body, err := json.Marshal(derived)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core.roots.DurableDir, "threads", child.ThreadID, "thread.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	core, err = runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.OwnsThread(child.ThreadID) || len(scope.Contexts()) != 2 {
		t.Fatal("recursive history changed execution context denominator")
	}
	for _, frozen := range scope.Contexts() {
		if frozen.ThreadID == child.ThreadID && frozen.TurnID == parentContext.TurnID {
			t.Fatal("recursive history gained source execution context")
		}
	}
	observed, err := scope.ReadPrimaryThreadSnapshotV1(ctx, child.ThreadID)
	if err != nil || observed.ThreadFileSHA256 != domainsecurity.SHA256Hex(body) {
		t.Fatal("recursive scope failed to preserve complete child history", err)
	}
}

func TestRuntimeDerivedHistoryRejectsReservedContinuationIdentity(t *testing.T) {
	_, target, inherited := runtimeDerivedReportHistoryFixtureV1(t, "fork")
	target.refresh(t)
	ctx := context.Background()
	frozen := target.scope.Contexts()[0]
	primary, err := target.scope.ReadPrimaryThreadSnapshotV1(ctx, frozen.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	manager, err := jobs.NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	// This is a reservation-producer/observer component test. Full case-child
	// provenance admission is tested separately; the reservation never executes.
	record, err := manager.StartChildRun(jobs.StartRequest{ParentGoalID: "goal-continuation", ParentThreadID: frozen.ThreadID, ParentTurnID: frozen.TurnID, ChildThreadID: "thread-continuation-child", ChildTurnID: "turn_921", Kind: "subagent", Status: "completed", Background: true, AutoContinueParent: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, jobs.UpdateRequest{CompletionDeliveryID: "delivery-continuation", CompletionDeliveryItemID: "item-continuation", CompletionDeliveryStatus: "delivered"}); err != nil {
		t.Fatal(err)
	}
	reserved, won, err := manager.ReserveBackgroundAutoContinueV1(record.ID, inherited["id"].(string), func(jobs.Record, []jobs.Record) string { return "" })
	if err != nil || !won || reserved.AutoContinueTurnID != inherited["id"] {
		t.Fatal("native continuation reservation was not persisted", err)
	}
	for _, status := range []string{"starting", "failed"} {
		t.Run(status, func(t *testing.T) {
			if status == "failed" {
				if _, err := manager.UpdateChildRun(record.ID, jobs.UpdateRequest{AutoContinueStatus: "failed"}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, root)
			if err != nil || len(snapshot.Records) != 1 || snapshot.Records[0].AutoContinueTurnID != inherited["id"] {
				t.Fatal("reservation identity was lost", err)
			}
			core := *target.core
			core.jobRecords = snapshot.Records
			proof := &runtimeReportInheritedHistoryV1{core: &core}
			before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir, root)
			if err := proof.ValidateInheritedTurnV1(ctx, primary.Thread, inherited); err == nil {
				t.Fatal("reserved auto-continue execution was accepted as inherited history")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir, root)) {
				t.Fatal("reservation refusal mutated Original bytes")
			}
		})
	}
}

func TestRuntimeCompactedDerivedReportHistory(t *testing.T) {
	for _, derivation := range []string{"fork", "resume"} {
		t.Run(derivation, func(t *testing.T) {
			_, target, _ := runtimeDerivedReportHistoryFixtureV1(t, derivation, true)
			body, err := os.ReadFile(target.primary)
			if err != nil {
				t.Fatal(err)
			}
			var thread map[string]any
			if err := json.Unmarshal(body, &thread); err != nil {
				t.Fatal(err)
			}
			projections := map[string]int{}
			ids := []string{}
			for _, raw := range thread["turns"].([]any) {
				turn := raw.(map[string]any)
				ids = append(ids, turn["id"].(string))
				if marker, present := turn["caseHistoryProjection"]; present {
					projections[marker.(string)]++
					if _, present := turn["securityContext"]; present {
						t.Fatal("compacted history retained execution context")
					}
				}
			}
			if projections["authority_only_v1"] == 0 || projections["compaction_authority_v1"] != 1 {
				t.Fatalf("actual compaction omitted producer shapes: %v", projections)
			}
			t.Logf("actual PrepareCaseCompaction/CommitCompaction -> %s -> unresolved report: projections=%v", derivation, projections)
			before := startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)
			err = target.refreshE()
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
				t.Fatal("compacted history refresh changed original bytes")
			}
			if err != nil {
				t.Fatalf("real compacted %s history blocked report refresh: %v", derivation, err)
			}
			contexts := target.scope.Contexts()
			if len(contexts) != 1 || contexts[0].TurnID != "turn-derived-new-report" {
				t.Fatal("compacted inherited history gained execution context")
			}
			ctx := context.Background()
			store, err := pendingstore.NewStoreContext(ctx, filepath.Join(target.core.roots.DataDir, "private", "pending-work"), target.core.access)
			if err != nil {
				t.Fatal(err)
			}
			service := pendingapp.NewService(target.core.verification, store, runtimeOriginalReportThreadsV1{ctx, target.core.primaries})
			if err := service.PreserveReportRestartScopeV1(ctx, *target.scope); err != nil {
				t.Fatal(err)
			}
			held := service.RestartPreservedTurnIDsV1()
			if len(held) != len(ids) {
				t.Fatal("compacted historical identity inventory changed")
			}
			for _, id := range ids {
				if !containsDerivedHistoryIDV1(held, id) {
					t.Fatal("compacted historical allocation identity was released")
				}
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, target.core.roots.DataDir, target.core.roots.DurableDir)) {
				t.Fatal("preservation changed original compacted bytes")
			}
		})
	}
}
