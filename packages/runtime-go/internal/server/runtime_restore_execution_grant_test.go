package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	casepublicationtest "analytix.local/runtime-go/internal/testsupport/casepublication"
)

func TestRuntimeRestoreMarksOpenApprovedDispatchOutcomeUnknownWithoutResend(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_restore_approved_dispatch", "title": "Approved dispatch", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_restore_approved_dispatch"
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("approved-manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"items": []any{}, "securityContext": securityRecord,
	}, "host", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"path":"controlled-artifact"}`)
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call_restore_approved_dispatch"), Name: "write_file", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("approved-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("approved-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	callItem, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: domaintoolcall.ToolCallItemIDV1(turnID, call.ID),
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "file_change", Context: securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, callItem); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !found {
		t.Fatal("approved grant registry entry is unavailable")
	}

	authorityRoot := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindApprovedToolDispatch, SecurityContext: securityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("exact-approved-dispatch")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("exact-approved-route")), IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(45 * time.Second),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	if err := pendingStore.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	continuations, err := newServerTestContinuationService(t, filepath.Join(t.TempDir(), "continuations"), authority)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{}}
	store.SetCaseThreadAuthority(caseAuthority)
	handler := &runtimeServerHandler{
		store: store, pendingWork: pendingworkapp.NewService(authority, pendingStore, store),
		continuations: continuations, caseThreads: caseAuthority,
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("restore approved dispatch: %v", err)
	}

	dispositions, err := pendingStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].WorkID != receipt.WorkID ||
		dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown || dispositions[0].ReasonCode != "tool_outcome_unknown_after_restart" {
		t.Fatalf("approved dispatch restart disposition=%#v err=%v", dispositions, err)
	}
	reloaded, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(
		registry, threadID, turnID, grant, domainsecurity.GrantRegistrySettled,
	); err != nil {
		t.Fatalf("outcome-unknown grant was not closed: %v", err)
	}
	turn, _ := turnappTurnByIDForRestoreTest(reloaded, turnID)
	unknownResults := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") != "tool_result" || stringField(item, "executionGrantId") != grant.GrantID {
			continue
		}
		unknownResults++
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if err != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() || item["isError"] != true || item["hostEvidenceSettlement"] != nil {
			t.Fatalf("restart outcome-unknown result=%#v projection=%#v err=%v", item, projection, err)
		}
	}
	if unknownResults != 1 || stringField(turn, "status") != "aborted" {
		t.Fatalf("restart outcome-unknown terminal state is invalid: results=%d turn=%#v", unknownResults, turn)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("repeat approved dispatch restore: %v", err)
	}
	dispositionsAfter, err := pendingStore.ListDispositions(context.Background())
	if err != nil || len(dispositionsAfter) != 1 || dispositionsAfter[0].DispositionID != dispositions[0].DispositionID {
		t.Fatalf("repeat restore changed the private disposition: dispositions=%#v err=%v", dispositionsAfter, err)
	}
	reloaded, _ = store.GetThread(threadID)
	turn, _ = turnappTurnByIDForRestoreTest(reloaded, turnID)
	repeatedResults := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" && stringField(item, "executionGrantId") == grant.GrantID {
			repeatedResults++
		}
	}
	if repeatedResults != 1 {
		t.Fatalf("repeat restore duplicated outcome-unknown result: %#v", turn["items"])
	}
}

func TestRuntimeRestoreMarksOpenNotRequiredSideEffectIntentOutcomeUnknownWithoutResend(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_restore_side_effect_intent", "title": "Side-effect intent", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_restore_side_effect_intent"
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("side-effect-manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"items": []any{}, "securityContext": securityRecord,
	}, "host", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"command":"printf x >> effect.txt"}`)
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call_restore_side_effect_intent"), Name: "bash", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("side-effect-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("side-effect-scope")),
		ReadOnly: false, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	callItem, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: domaintoolcall.ToolCallItemIDV1(turnID, call.ID),
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "command", Context: securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, callItem); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !found {
		t.Fatal("side-effect grant registry entry is unavailable")
	}
	authorityRoot := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: securityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("exact-side-effect-intent")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("exact-side-effect-route")), IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(45 * time.Second),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	if err := pendingStore.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	continuations, err := newServerTestContinuationService(t, filepath.Join(t.TempDir(), "continuations"), authority)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{}}
	store.SetCaseThreadAuthority(caseAuthority)
	handler := &runtimeServerHandler{
		store: store, pendingWork: pendingworkapp.NewService(authority, pendingStore, store),
		continuations: continuations, caseThreads: caseAuthority,
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("restore side-effect intent: %v", err)
	}
	dispositions, err := pendingStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].WorkID != receipt.WorkID ||
		dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown || dispositions[0].ReasonCode != "tool_outcome_unknown_after_restart" {
		t.Fatalf("side-effect restart disposition=%#v err=%v", dispositions, err)
	}
	reloaded, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, grant, domainsecurity.GrantRegistrySettled); err != nil {
		t.Fatalf("outcome-unknown side-effect grant was not closed: %v", err)
	}
	turn, _ := turnappTurnByIDForRestoreTest(reloaded, turnID)
	resultCount := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") != "tool_result" || stringField(item, "executionGrantId") != grant.GrantID {
			continue
		}
		resultCount++
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if err != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() || item["isError"] != true || item["hostEvidenceSettlement"] != nil {
			t.Fatalf("side-effect restart result=%#v projection=%#v err=%v", item, projection, err)
		}
	}
	if resultCount != 1 || stringField(turn, "status") != "aborted" {
		t.Fatalf("side-effect restart terminal state is invalid: results=%d turn=%#v", resultCount, turn)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("repeat side-effect restore: %v", err)
	}
	reloaded, _ = store.GetThread(threadID)
	turn, _ = turnappTurnByIDForRestoreTest(reloaded, turnID)
	repeatedResults := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" && stringField(item, "executionGrantId") == grant.GrantID {
			repeatedResults++
		}
	}
	if repeatedResults != 1 {
		t.Fatalf("repeat restore duplicated side-effect outcome: %#v", turn["items"])
	}
}

func TestRuntimeRestoreMarksOpenReportStageOutcomeUnknownWithoutRepublish(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_restore_report_stage", "title": "Report stage", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_restore_report_stage"
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("report-stage-manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"items": []any{}, "securityContext": securityRecord,
	}, "host", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"report":"case"}`)
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call_restore_report_stage"), Name: pendingworkapp.ReportStageToolName, Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("report-stage-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("report-stage-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	callItem, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: domaintoolcall.ToolCallItemIDV1(turnID, call.ID),
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "report", Context: securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, callItem); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !found {
		t.Fatal("report stage grant registry entry is unavailable")
	}
	authorityRoot := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("exact-report-stage-input")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("analytix/pending-work-route/report-stage/v1")), IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(45 * time.Second),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	if err := pendingStore.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	continuations, err := newServerTestContinuationService(t, filepath.Join(t.TempDir(), "continuations"), authority)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{threadID: true}}
	store.SetCaseThreadAuthority(caseAuthority)
	caseFinalizer, err := casepublicationtest.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{
		store: store, pendingWork: pendingworkapp.NewService(authority, pendingStore, store),
		continuations: continuations, caseThreads: caseAuthority, caseFinalizer: caseFinalizer,
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("restore report stage: %v", err)
	}
	dispositions, err := pendingStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].WorkID != receipt.WorkID ||
		dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown || dispositions[0].ReasonCode != "report_stage_outcome_unknown_after_restart" {
		t.Fatalf("report stage restart disposition=%#v err=%v", dispositions, err)
	}
	reloaded, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, grant, domainsecurity.GrantRegistrySettled); err != nil {
		t.Fatalf("outcome-unknown report grant was not closed: %v", err)
	}
	turn, _ := turnappTurnByIDForRestoreTest(reloaded, turnID)
	unknownResults := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") != "tool_result" || stringField(item, "executionGrantId") != grant.GrantID {
			continue
		}
		unknownResults++
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if err != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() || item["isError"] != true || item["hostEvidenceSettlement"] != nil {
			t.Fatalf("restart report outcome-unknown result=%#v projection=%#v err=%v", item, projection, err)
		}
	}
	if unknownResults != 1 || stringField(turn, "status") != "aborted" {
		t.Fatalf("restart report outcome-unknown terminal state is invalid: results=%d turn=%#v", unknownResults, turn)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("repeat report stage restore: %v", err)
	}
	reloaded, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ = turnappTurnByIDForRestoreTest(reloaded, turnID)
	repeatedResults := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" && stringField(item, "executionGrantId") == grant.GrantID {
			repeatedResults++
		}
	}
	if repeatedResults != 1 {
		t.Fatalf("repeat restore duplicated report outcome-unknown result: %#v", turn["items"])
	}
}

func TestRuntimeRestoreSettlesDanglingExecutionGrantBeforeAbort(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_restore_dangling_grant", "title": "Dangling grant", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_restore_dangling_grant"
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"items": []any{}, "securityContext": securityRecord,
	}, "host", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}

	arguments := json.RawMessage(`{}`)
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call_restore_dangling_grant"), Name: "host_test_probe", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	callItem, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: domaintoolcall.ToolCallItemIDV1(turnID, call.ID),
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "host", Context: securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, callItem); err != nil {
		t.Fatal(err)
	}

	authorityRoot := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	continuations, err := newServerTestContinuationService(t, filepath.Join(t.TempDir(), "continuations"), authority)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{}}
	store.SetCaseThreadAuthority(caseAuthority)
	handler := &runtimeServerHandler{
		store: store, pendingWork: pendingworkapp.NewService(authority, pendingStore, store),
		continuations: continuations, caseThreads: caseAuthority,
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("restore runtime state: %v", err)
	}

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(
		registry, threadID, turnID, grant, domainsecurity.GrantRegistrySettled,
	); err != nil {
		t.Fatalf("restart left the durable grant open: %v", err)
	}
	turn, ok := turnappTurnByIDForRestoreTest(reloaded, turnID)
	if !ok || stringField(turn, "status") != "aborted" {
		t.Fatalf("restart did not abort the stale turn: %#v", turn)
	}
	items := listAny(turn["items"])
	if len(items) != 3 {
		t.Fatalf("expected call, private settlement, and restart boundary: %#v", items)
	}
	if stringField(items[0].(map[string]any), "kind") != "tool_call" ||
		stringField(items[1].(map[string]any), "kind") != "tool_result" ||
		stringField(items[2].(map[string]any), "code") != "runtime_restarted" {
		t.Fatalf("restart settlement order is invalid: %#v", items)
	}
	result := items[1].(map[string]any)
	if stringField(result, "executionGrantId") != grant.GrantID || result["hostEvidenceSettlement"] != nil {
		t.Fatalf("restart settlement authority is invalid: %#v", result)
	}

	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range replay.Events {
		if kind := stringField(event, "kind"); kind == "tool_call_ready" || kind == "tool_call_finished" {
			t.Fatalf("private restart grant records leaked into replay: %#v", event)
		}
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("repeat restore runtime state: %v", err)
	}
	reloaded, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ = turnappTurnByIDForRestoreTest(reloaded, turnID)
	resultCount := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" && stringField(item, "executionGrantId") == grant.GrantID {
			resultCount++
		}
	}
	if resultCount != 1 {
		t.Fatalf("repeat restore duplicated the grant settlement: %#v", turn["items"])
	}
}

func turnappTurnByIDForRestoreTest(thread map[string]any, turnID string) (map[string]any, bool) {
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID {
			return turn, true
		}
	}
	return nil, false
}
