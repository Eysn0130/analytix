package executiongrant

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestRestartReconcileProjectsApprovedDispatchOutcomeUnknownFromExactAuthority(t *testing.T) {
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-approved", "turn-restart-approved", "/cases/restart-approved", "case-restart-approved", now,
	).Context
	arguments := json.RawMessage(`{"path":"controlled-output"}`)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-restart-approved"), Name: "write_file", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("write-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("write-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	thread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant)
	registry, err := RegistryFromThread(securityContext.ThreadID, thread, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !found {
		t.Fatal("approved grant registry entry is unavailable")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindApprovedToolDispatch, SecurityContext: securityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("approved-payload")), RouteHash: domainsecurity.SHA256Hex([]byte("approved-route")),
		IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(30 * time.Second),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		receipt, domainpendingwork.StatusOutcomeUnknown, "tool_outcome_unknown_after_restart", now.Add(2*time.Second),
		domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := RestartGrantOutcomesV1{grant.GrantID: {Receipt: receipt, Disposition: disposition}}
	store := &restartSettlementStoreStub{thread: thread}
	count, err := ReconcileOpenTurnGrantsOnRestart(
		store, securityContext.ThreadID, securityContext.TurnID, now.Add(3*time.Second), outcomes,
	)
	if err != nil || count != 1 {
		t.Fatalf("outcome-unknown restart reconcile count=%d err=%v", count, err)
	}
	turn, _ := appTurnByIDForRestartTest(store.thread, securityContext.TurnID)
	result := turn["items"].([]any)[1].(map[string]any)
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
	if err != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() ||
		result["isError"] != true || result["hostEvidenceSettlement"] != nil {
		t.Fatalf("approved dispatch restart projection=%#v result=%#v err=%v", projection, result, err)
	}
	currentID := restartString(result, "id")
	legacyID := legacyToolResultItemIDV0(securityContext.TurnID, call.ID)
	if currentID == legacyID {
		t.Fatalf("new restart settlement used legacy public identity: %q", currentID)
	}
	// Simulate an exact pre-migration outcome-unknown record. Compatibility is
	// read-only: a repeated restart may recognize it but must never append it.
	result["id"] = legacyID
	count, err = ReconcileOpenTurnGrantsOnRestart(
		store, securityContext.ThreadID, securityContext.TurnID, now.Add(4*time.Second), outcomes,
	)
	if err != nil || count != 0 || store.appends != 1 {
		t.Fatalf("repeat outcome-unknown reconcile count=%d appends=%d err=%v", count, store.appends, err)
	}
}

func TestRestartReconcileProjectsNotRequiredSideEffectIntentOutcomeUnknown(t *testing.T) {
	now := time.Date(2026, 7, 16, 11, 30, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-side-effect", "turn-restart-side-effect", "/cases/restart-side-effect", "case-restart-side-effect", now,
	).Context
	arguments := json.RawMessage(`{"path":"controlled-output"}`)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-restart-side-effect"), Name: "write_file", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("write-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("write-scope")),
		ReadOnly: false, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	thread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant)
	registry, err := RegistryFromThread(securityContext.ThreadID, thread, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !found {
		t.Fatal("side-effect grant registry entry is unavailable")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: securityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("side-effect-payload")), RouteHash: domainsecurity.SHA256Hex([]byte("side-effect-route")),
		IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(30 * time.Second),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		receipt, domainpendingwork.StatusOutcomeUnknown, "tool_outcome_unknown_after_restart", now.Add(2*time.Second),
		domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &restartSettlementStoreStub{thread: thread}
	count, err := ReconcileOpenTurnGrantsOnRestart(
		store,
		securityContext.ThreadID,
		securityContext.TurnID,
		now.Add(3*time.Second),
		RestartGrantOutcomesV1{grant.GrantID: {Receipt: receipt, Disposition: disposition}},
	)
	if err != nil || count != 1 {
		t.Fatalf("not-required side-effect restart reconcile count=%d err=%v", count, err)
	}
	turn, _ := appTurnByIDForRestartTest(store.thread, securityContext.TurnID)
	result := turn["items"].([]any)[1].(map[string]any)
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
	if err != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() || result["isError"] != true {
		t.Fatalf("not-required side-effect restart projection=%#v result=%#v err=%v", projection, result, err)
	}
}

func TestRestartReconcileSettlesDanglingNativeHealthGrant(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-native", "turn-restart-native", "/cases/restart-native", "case-restart-native", now,
	).Context
	call, grant := restartNativeHealthGrant(t, securityContext, now)
	store := &restartSettlementStoreStub{
		thread: activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant),
	}

	count, err := ReconcileOpenTurnGrantsOnRestart(store, securityContext.ThreadID, securityContext.TurnID, now.Add(time.Second), nil)
	if err != nil || count != 1 {
		t.Fatalf("restart reconcile count=%d err=%v", count, err)
	}
	registry, err := RegistryFromThread(securityContext.ThreadID, store.thread, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(
		registry, securityContext.ThreadID, securityContext.TurnID, grant, domainsecurity.GrantRegistrySettled,
	); err != nil {
		t.Fatalf("dangling grant was not settled: %v", err)
	}
	turn, _ := appTurnByIDForRestartTest(store.thread, securityContext.TurnID)
	items, _ := turn["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("restart settlement items = %#v", items)
	}
	result, _ := items[1].(map[string]any)
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
	if err != nil || projection.Status != "cancelled" || projection.MessageKey != "tool_cancelled" ||
		projection.FactAnswerAllowed || projection.EvidenceAuthority || result["hostEvidenceSettlement"] != nil {
		t.Fatalf("restart settlement projection = %#v err=%v", result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "stderr") || strings.Contains(string(encoded), "registryDigest") {
		t.Fatalf("restart settlement retained private native output: %s", encoded)
	}

	count, err = ReconcileOpenTurnGrantsOnRestart(store, securityContext.ThreadID, securityContext.TurnID, now.Add(2*time.Second), nil)
	if err != nil || count != 0 || store.appends != 1 {
		t.Fatalf("idempotent restart reconcile count=%d appends=%d err=%v", count, store.appends, err)
	}
}

func TestRestartReconcileRejectsApprovedWritableGrantWithoutPendingWorkAuthority(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 15, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-missing-write-authority", "turn-restart-missing-write-authority",
		"/cases/restart-missing-write-authority", "case-restart-missing-write-authority", now,
	).Context
	arguments := json.RawMessage(`{"path":"possibly-written"}`)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-restart-missing-write-authority"), Name: "write_file", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("write-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("write-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	store := &restartSettlementStoreStub{
		thread: activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant),
	}
	if count, err := ReconcileOpenTurnGrantsOnRestart(
		store, securityContext.ThreadID, securityContext.TurnID, now.Add(time.Second), nil,
	); err == nil || count != 0 || store.appends != 0 {
		t.Fatalf("approved writable grant without pending-work authority was downgraded: count=%d appends=%d err=%v", count, store.appends, err)
	}
}

func TestRestartReconcileRejectsOutcomeUnknownResultAfterPrivateAuthorityDeletion(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 20, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-deleted-authority", "turn-restart-deleted-authority",
		"/cases/restart-deleted-authority", "case-restart-deleted-authority", now,
	).Context
	arguments := json.RawMessage(`{"path":"unknown-write"}`)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-restart-deleted-authority"), Name: "write_file", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("write-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("write-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	store := &restartSettlementStoreStub{
		thread: activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant),
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := RegistryFromThread(securityContext.ThreadID, store.thread, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindApprovedToolDispatch, SecurityContext: securityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("deleted-authority-payload")), RouteHash: domainsecurity.SHA256Hex([]byte("deleted-authority-route")),
		IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(30 * time.Second),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		receipt, domainpendingwork.StatusOutcomeUnknown, "tool_outcome_unknown_after_restart", now.Add(2*time.Second),
		domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := RestartGrantOutcomesV1{grant.GrantID: {Receipt: receipt, Disposition: disposition}}
	if count, err := ReconcileOpenTurnGrantsOnRestart(
		store, securityContext.ThreadID, securityContext.TurnID, now.Add(3*time.Second), outcomes,
	); err != nil || count != 1 {
		t.Fatalf("seed outcome-unknown settlement: count=%d err=%v", count, err)
	}
	if count, err := ReconcileOpenTurnGrantsOnRestart(
		store, securityContext.ThreadID, securityContext.TurnID, now.Add(4*time.Second), nil,
	); err == nil || count != 0 || store.appends != 1 {
		t.Fatalf("deleted private outcome authority was accepted: count=%d appends=%d err=%v", count, store.appends, err)
	}
}

func TestRestartReconcileRejectsDuplicateOpenCallIdentity(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 30, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-duplicate", "turn-restart-duplicate", "/cases/restart-duplicate", "case-restart-duplicate", now,
	).Context
	call, first := restartNativeHealthGrant(t, securityContext, now)
	_, second := restartNativeHealthGrant(t, securityContext, now.Add(time.Second))
	thread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, first)
	turn, _ := appTurnByIDForRestartTest(thread, securityContext.TurnID)
	turn["items"] = append(turn["items"].([]any), restartToolCallItem(securityContext, second, call, "item_tool_duplicate"))
	store := &restartSettlementStoreStub{thread: thread}

	if count, err := ReconcileOpenTurnGrantsOnRestart(store, securityContext.ThreadID, securityContext.TurnID, now.Add(2*time.Second), nil); err == nil || count != 0 || store.appends != 0 {
		t.Fatalf("duplicate open call was reconciled: count=%d appends=%d err=%v", count, store.appends, err)
	}
}

func TestRestartReconcilePreflightsEveryOpenGrantBeforeMutation(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 45, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-preflight", "turn-restart-preflight", "/cases/restart-preflight", "case-restart-preflight", now,
	).Context
	firstCall, first := restartNativeHealthGrantForCall(t, securityContext, now, "host-native-health-call-a")
	secondCall, second := restartNativeHealthGrantForCall(t, securityContext, now.Add(time.Second), "host-native-health-call-b")
	thread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, firstCall, first)
	turn, _ := appTurnByIDForRestartTest(thread, securityContext.TurnID)
	turn["items"] = append(turn["items"].([]any),
		restartToolCallItem(securityContext, second, secondCall, "item_tool_second"),
		map[string]any{
			"id": "item_tool_second_collision", "kind": "tool_call", "threadId": securityContext.ThreadID,
			"turnId": securityContext.TurnID, "toolName": secondCall.Name, "callId": secondCall.ID,
		},
	)
	store := &restartSettlementStoreStub{thread: thread}

	if count, err := ReconcileOpenTurnGrantsOnRestart(
		store, securityContext.ThreadID, securityContext.TurnID, now.Add(2*time.Second), nil,
	); err == nil || count != 0 || store.appends != 0 {
		t.Fatalf("invalid later grant caused a partial settlement: count=%d appends=%d err=%v", count, store.appends, err)
	}
	registry, err := RegistryFromThread(securityContext.ThreadID, store.thread, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	for _, grant := range []domainsecurity.ExecutionGrant{first, second} {
		if err := domainsecurity.VerifyExecutionGrantMembership(
			registry, securityContext.ThreadID, securityContext.TurnID, grant, domainsecurity.GrantRegistryActive,
		); err != nil {
			t.Fatalf("preflight changed open grant %s: %v", grant.ToolCallID, err)
		}
	}
}

func TestRestartReconcileFailsClosedWhenSettlementIsNotDurable(t *testing.T) {
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(
		t, "thread-restart-failure", "turn-restart-failure", "/cases/restart-failure", "case-restart-failure", now,
	).Context
	call, grant := restartNativeHealthGrant(t, securityContext, now)
	store := &restartSettlementStoreStub{
		thread:    activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant),
		appendErr: errors.New("durable write failed"),
	}

	if count, err := ReconcileOpenTurnGrantsOnRestart(store, securityContext.ThreadID, securityContext.TurnID, now.Add(time.Second), nil); err == nil || count != 0 {
		t.Fatalf("failed settlement was accepted: count=%d err=%v", count, err)
	}
	registry, err := RegistryFromThread(securityContext.ThreadID, store.thread, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(
		registry, securityContext.ThreadID, securityContext.TurnID, grant, domainsecurity.GrantRegistryActive,
	); err != nil {
		t.Fatalf("failed durable write changed registry authority: %v", err)
	}
}

func restartNativeHealthGrant(t *testing.T, securityContext domainsecurity.TurnSecurityContext, now time.Time) (domainmodel.ToolCall, domainsecurity.ExecutionGrant) {
	return restartNativeHealthGrantForCall(t, securityContext, now, "host-native-health-call")
}

func restartNativeHealthGrantForCall(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	now time.Time,
	callID string,
) (domainmodel.ToolCall, domainsecurity.ExecutionGrant) {
	t.Helper()
	if !domainmodel.IsHostToolCallIDV1(callID) {
		callID = executionGrantTestToolCallID(callID)
	}
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("native health policy is unavailable")
	}
	call := domainmodel.ToolCall{
		ID:        callID,
		Name:      domainnative.ToolName(policy.ComponentID, policy.Operation),
		Arguments: json.RawMessage(`{}`),
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context:         securityContext,
		Provider:        domainnative.NativeProvider,
		ServerIdentity:  domainnative.NativeServerIdentity,
		ToolName:        call.Name,
		ToolCallID:      call.ID,
		ConnectionEpoch: 0,
		ArgsHash:        domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash:      policy.SchemaHash,
		ScopeHash:       domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation),
		ReadOnly:        true,
		ApprovalState:   "not_required",
		IssuedAt:        now,
		ExpiresAt:       now.Add(30 * time.Second),
	})
	if err := domainsecurity.ValidateExecutionGrantForContext(grant, securityContext); err != nil {
		t.Fatal(err)
	}
	return call, grant
}

func restartToolCallItem(securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, call domainmodel.ToolCall, itemID string) map[string]any {
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	return map[string]any{
		"id": itemID, "kind": "tool_call", "threadId": securityContext.ThreadID,
		"turnId": grant.TurnID, "toolName": call.Name, "callId": call.ID, "arguments": map[string]any{},
		"createdAt": grant.IssuedAt, "contextDigest": grant.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
	}
}

type restartSettlementStoreStub struct {
	thread    map[string]any
	appends   int
	appendErr error
}

func (store *restartSettlementStoreStub) GetThread(string) (map[string]any, error) {
	return store.thread, nil
}

func (store *restartSettlementStoreStub) AppendItemToTurn(_ string, turnID string, item map[string]any) error {
	if store.appendErr != nil {
		return store.appendErr
	}
	turn, ok := appTurnByIDForRestartTest(store.thread, turnID)
	if !ok {
		return errors.New("turn unavailable")
	}
	turn["items"] = append(turn["items"].([]any), item)
	store.appends++
	return nil
}

func appTurnByIDForRestartTest(thread map[string]any, turnID string) (map[string]any, bool) {
	for _, raw := range thread["turns"].([]any) {
		turn, _ := raw.(map[string]any)
		if restartString(turn, "id") == turnID {
			return turn, true
		}
	}
	return nil, false
}
