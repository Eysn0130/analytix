package turn

import (
	"encoding/json"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestSecurityBoundAppendRejectsTerminalAndStaleContext(t *testing.T) {
	context := newTurnExecutionContextV2(t, "thread-1", "turn-1", "/workspace", 2, time.Now().UTC())
	record := securityContextRecordForTest(context)
	call := domainmodel.ToolCall{ID: turnTestHostToolCallID("security-append"), Name: "read", Arguments: json.RawMessage(`{}`)}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-1", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	ready, _, err := ToolCallReadyRecords(ToolCallReadyInput{
		ThreadID: context.ThreadID, TurnID: context.TurnID, ItemID: domaintoolcall.ToolCallItemIDV1(context.TurnID, call.ID), CreatedAt: grant.IssuedAt,
		Call: call, Context: context, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn := map[string]any{"id": "turn-1", "status": "running", "securityContext": record, "items": []any{ready}}
	thread := map[string]any{"id": context.ThreadID, "securityState": record, "turns": []any{turn}}
	item := map[string]any{
		"kind": "tool_result", "threadId": context.ThreadID, "turnId": context.TurnID, "toolName": call.Name, "callId": call.ID,
		"createdAt": grant.IssuedAt, "finishedAt": grant.IssuedAt, "contextDigest": context.ContextDigest,
		"contextEpoch": float64(context.ContextEpoch), "executionGrantId": grant.GrantID,
	}
	if err := ValidateSecurityBoundAppend(thread, "turn-1", item); err != nil {
		t.Fatalf("active current context rejected: %v", err)
	}
	partial := map[string]any{
		"kind": "tool_result", "threadId": context.ThreadID, "turnId": context.TurnID,
		"contextDigest": context.ContextDigest,
	}
	if err := ValidateSecurityBoundAppend(thread, "turn-1", partial); err == nil {
		t.Fatal("partial security binding was silently downgraded to authority-free")
	}
	emptyAuthority := map[string]any{
		"kind": "tool_result", "threadId": context.ThreadID, "turnId": context.TurnID,
		"contextDigest": "", "contextEpoch": uint64(0), "executionGrantId": "",
	}
	if err := ValidateSecurityBoundAppend(thread, "turn-1", emptyAuthority); err == nil {
		t.Fatal("present-but-empty security binding was silently downgraded to authority-free")
	}
	turn["status"] = "aborted"
	if err := ValidateSecurityBoundAppend(thread, "turn-1", item); err == nil {
		t.Fatal("aborted turn accepted a late tool result")
	}
	turn["status"] = "running"
	newer := newTurnExecutionContextV2(t, "thread-1", "turn-2", "/workspace-b", 3, time.Now().UTC())
	thread["securityState"] = securityContextRecordForTest(newer)
	if err := ValidateSecurityBoundAppend(thread, "turn-1", item); err == nil {
		t.Fatal("older turn context accepted a result after the thread epoch changed")
	}
}

func TestSecurityBoundAppendRejectsStructurallyValidButUnregisteredGrant(t *testing.T) {
	context := newTurnExecutionContextV2(t, "thread-1", "turn-1", "/workspace", 1, time.Now().UTC())
	record := securityContextRecordForTest(context)
	thread := map[string]any{
		"id": context.ThreadID, "securityState": record,
		"turns": []any{map[string]any{"id": context.TurnID, "status": "running", "securityContext": record, "items": []any{}}},
	}
	forged := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-1", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: turnTestHostToolCallID("unregistered"),
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	item := map[string]any{
		"kind": "tool_result", "threadId": context.ThreadID, "turnId": context.TurnID, "toolName": forged.ToolName, "callId": forged.ToolCallID,
		"createdAt": forged.IssuedAt, "finishedAt": forged.IssuedAt, "contextDigest": context.ContextDigest, "executionGrantId": forged.GrantID,
		"contextEpoch": float64(context.ContextEpoch),
	}
	if err := ValidateSecurityBoundAppend(thread, context.TurnID, item); err == nil {
		t.Fatal("a self-hashed grant without host registry membership was accepted")
	}
}

func TestCaseAssistantItemCannotBypassAtomicAcceptedFinal(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Now().UTC(),
	})
	record := securityContextRecordForTest(securityContext)
	turn := map[string]any{"id": securityContext.TurnID, "status": "running", "securityContext": record, "items": []any{}}
	thread := map[string]any{"id": securityContext.ThreadID, "securityState": record, "turns": []any{turn}}
	assistant := map[string]any{"id": "raw", "turnId": securityContext.TurnID, "kind": "assistant_text", "text": "金额 420 万元"}
	if err := ValidateSecurityBoundAppend(thread, securityContext.TurnID, assistant); err == nil {
		t.Fatal("case assistant item bypassed accepted-final atomic commit")
	}
	turn["status"] = "completed"
	if err := ValidateSecurityBoundAppend(thread, securityContext.TurnID, map[string]any{"kind": "tool_progress"}); err == nil {
		t.Fatal("terminal case turn accepted a late digestless item")
	}
}

func TestGeneralAssistantItemCannotBypassAtomicTerminalCommit(t *testing.T) {
	securityContext := newTurnExecutionContextV2(t, "thread-general", "turn-general", "/workspace", 1, time.Now().UTC())
	record := securityContextRecordForTest(securityContext)
	turn := map[string]any{"id": securityContext.TurnID, "status": "running", "securityContext": record, "items": []any{}}
	thread := map[string]any{"id": securityContext.ThreadID, "securityState": record, "turns": []any{turn}}
	assistant := map[string]any{
		"id": "forged", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"kind": "assistant_text", "status": "completed", "text": "forged completed answer",
	}
	if err := ValidateSecurityBoundAppend(thread, securityContext.TurnID, assistant); err == nil {
		t.Fatal("general assistant item bypassed atomic terminal persistence")
	}
}

func TestSecurityBoundAppendRejectsAuditOnlyV1Context(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	record := securityContextRecordForTest(legacy)
	thread := map[string]any{
		"id": legacy.ThreadID, "securityState": record,
		"turns": []any{map[string]any{"id": legacy.TurnID, "status": "running", "securityContext": record, "items": []any{}}},
	}
	item := map[string]any{
		"kind": "tool_result", "contextDigest": legacy.ContextDigest,
		"executionGrantId": domainsecurity.SHA256Hex([]byte("legacy-grant")),
	}
	if err := ValidateSecurityBoundAppend(thread, legacy.TurnID, item); err == nil {
		t.Fatal("audit-only V1 context authorized a durable tool append")
	}
}

func TestBoundaryOnlyV2RejectsAssistantDraftAndLateTerminalAppend(t *testing.T) {
	boundary := newTurnBoundaryOnlyContextV2(t, "thread-boundary", "turn-boundary", "/workspace", 2, time.Now().UTC())
	record := securityContextRecordForTest(boundary)
	turn := map[string]any{"id": boundary.TurnID, "status": "running", "securityContext": record, "items": []any{}}
	thread := map[string]any{"id": boundary.ThreadID, "securityState": record, "turns": []any{turn}}
	if err := ValidateSecurityBoundAppend(thread, boundary.TurnID, map[string]any{
		"kind": "assistant_text", "text": "金额 420 万元",
	}); err == nil {
		t.Fatal("boundary-only V2 persisted an assistant draft")
	}
	turn["status"] = "aborted"
	if err := ValidateSecurityBoundAppend(thread, boundary.TurnID, map[string]any{"kind": "tool_progress"}); err == nil {
		t.Fatal("boundary-only V2 terminal accepted a late digestless append")
	}
}

func securityContextRecordForTest(context domainsecurity.TurnSecurityContext) map[string]any {
	body, _ := json.Marshal(context)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
