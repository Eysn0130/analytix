package executiongrant

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestExecutionGrantRegistryAcceptsExactPrimaryJSONNumberEpoch(t *testing.T) {
	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	frozen := newExecutionGrantGeneralFixture(t, "thread_1", "turn_1", "/workspace/case-a", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "write", Arguments: json.RawMessage(`{"path":"report.md"}`)}
	grant, err := IssueProvider(frozen, "provider_1", call, executionGrantTestSchemas(call), []string{"write"}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(activeExecutionGrantThread(frozen, frozen.WorkspaceRealPath, call, grant))
	if err != nil {
		t.Fatal(err)
	}
	var primary map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&primary); err != nil {
		t.Fatal(err)
	}
	if err := VerifyThreadGrantMembership(frozen.ThreadID, primary, frozen.TurnID, grant, domainsecurity.GrantRegistryPending); err != nil {
		t.Fatalf("exact primary epoch was rejected: %v", err)
	}
	item := primary["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	for _, invalid := range []json.Number{"-1", "0", "1.5", "18446744073709551616"} {
		item["contextEpoch"] = invalid
		if err := VerifyThreadGrantMembership(frozen.ThreadID, primary, frozen.TurnID, grant, domainsecurity.GrantRegistryPending); err == nil {
			t.Fatalf("invalid epoch %q was accepted", invalid)
		}
	}
}

func TestExecutionGrantRegistryProjectionRequiresDurableHostRecord(t *testing.T) {
	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread_1", "turn_1", "/workspace/case-a", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "write", Arguments: json.RawMessage(`{"path":"report.md"}`)}
	schemas := executionGrantTestSchemas(call)
	pending, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"write"}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	thread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, pending)
	if err := VerifyThreadGrantMembership(securityContext.ThreadID, thread, securityContext.TurnID, pending, domainsecurity.GrantRegistryPending); err != nil {
		t.Fatalf("durable host-issued grant was not projected: %v", err)
	}
	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["items"] = []any{}
	if err := VerifyThreadGrantMembership(securityContext.ThreadID, thread, securityContext.TurnID, pending, domainsecurity.GrantRegistryPending); err == nil {
		t.Fatal("self-hashed grant without durable host registry record was accepted")
	}
}

func TestExecutionGrantRegistryProjectionTracksApprovedAndSettledStatus(t *testing.T) {
	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread_1", "turn_1", "/workspace/case-a", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "write", Arguments: json.RawMessage(`{"path":"report.md"}`)}
	schemas := executionGrantTestSchemas(call)
	pending, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"write"}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := Approve(pending, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	transitionAuthority, err := domainsecurity.NewApprovalGrantTransitionV1(domainsecurity.ApprovalGrantTransitionInputV1{
		Context: securityContext, ApprovalID: "appr_registry", ApprovalItemID: "item_appr_registry", ContinuationReceiptID: domainsecurity.SHA256Hex([]byte("receipt")),
		ContinuationDispositionID: domainsecurity.SHA256Hex([]byte("disposition")), PendingGrant: pending, ApprovedGrant: approved,
		TransitionedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, pending)
	transition, _, err := ApprovalTransitionRecords(securityContext, transitionAuthority, pending, approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRegistryAppend(securityContext.ThreadID, thread, securityContext.TurnID, transition); err != nil {
		t.Fatalf("valid approval transition rejected: %v", err)
	}
	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["items"] = append(turn["items"].([]any), transition)
	if err := VerifyThreadGrantMembership(securityContext.ThreadID, thread, securityContext.TurnID, pending, domainsecurity.GrantRegistryPending); err == nil {
		t.Fatal("revoked pending grant was replayable after approval")
	}
	if err := VerifyThreadGrantMembership(securityContext.ThreadID, thread, securityContext.TurnID, approved, domainsecurity.GrantRegistryActive); err != nil {
		t.Fatalf("approved registry member rejected: %v", err)
	}
	result := map[string]any{
		"id": "item_result_turn_1_call_1", "kind": "tool_result", "role": "tool", "status": "completed", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"toolName": call.Name, "callId": call.ID, "createdAt": now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		"finishedAt": now.Add(2 * time.Minute).Format(time.RFC3339Nano), "contextDigest": securityContext.ContextDigest,
		"contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": approved.GrantID,
	}
	if err := ValidateRegistryAppend(securityContext.ThreadID, thread, securityContext.TurnID, result); err != nil {
		t.Fatalf("first registered settlement rejected: %v", err)
	}
	turn["items"] = append(turn["items"].([]any), result)
	authority, err := DurableSettlementFromThread(securityContext.ThreadID, thread, securityContext.TurnID, "item_result_turn_1_call_1", approved)
	if err != nil {
		t.Fatalf("durable settlement prefix was not recovered: %v", err)
	}
	if domainsecurity.VerifyExecutionGrantMembership(authority.ActiveRegistry, securityContext.ThreadID, securityContext.TurnID, approved, domainsecurity.GrantRegistryActive) != nil ||
		domainsecurity.VerifyExecutionGrantMembership(authority.SettledRegistry, securityContext.ThreadID, securityContext.TurnID, approved, domainsecurity.GrantRegistrySettled) != nil ||
		authority.ResultItem["id"] != "item_result_turn_1_call_1" {
		t.Fatalf("durable settlement did not prove active-before/settled-after: %#v", authority)
	}
	if err := VerifyThreadGrantMembership(securityContext.ThreadID, thread, securityContext.TurnID, approved, domainsecurity.GrantRegistryActive); err == nil {
		t.Fatal("settled grant remained executable")
	}
	if err := ValidateRegistryAppend(securityContext.ThreadID, thread, securityContext.TurnID, result); err == nil {
		t.Fatal("duplicate tool-result settlement replay was accepted")
	}
	if _, err := DurableSettlementFromThread(securityContext.ThreadID, thread, securityContext.TurnID, "missing_result", approved); err == nil {
		t.Fatal("historical active grant without the exact durable result became settlement authority")
	}
	reference := domainsecurity.SettledToolReference{GrantID: approved.GrantID, ResultItemID: "item_result_turn_1_call_1"}
	laterCall := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_2"), Name: "write", Arguments: json.RawMessage(`{"path":"later.md"}`)}
	laterGrant, err := IssueProvider(securityContext, "provider_1", laterCall, schemas, []string{"write"}, false, "pending", 0, "", now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	laterThread := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, laterCall, laterGrant)
	laterTurn := laterThread["turns"].([]any)[0].(map[string]any)
	laterTurn["items"] = append(append([]any(nil), turn["items"].([]any)...), laterTurn["items"].([]any)...)
	if err := ValidatePriorSettledToolReferences(securityContext.ThreadID, laterThread, securityContext, laterGrant, []domainsecurity.SettledToolReference{reference}); err != nil {
		t.Fatalf("durable prior settlement reference rejected: %v", err)
	}
	result["contextEpoch"] = float64(securityContext.ContextEpoch + 1)
	if err := ValidatePriorSettledToolReferences(securityContext.ThreadID, laterThread, securityContext, laterGrant, []domainsecurity.SettledToolReference{reference}); err == nil {
		t.Fatal("cross-epoch prior settlement reference was accepted")
	}
}

func TestApprovalTransitionRecordsRejectAuditOnlyV1Context(t *testing.T) {
	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1-audit", TurnID: "turn-v1-audit", WorkspaceRealPath: "/workspace/audit", ContextEpoch: 1, IssuedAt: now,
	})
	pending := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "write", ToolCallID: "call-v1-audit",
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(DefaultTTL),
	})
	approved, err := Approve(pending, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	item, event, err := ApprovalTransitionRecords(securityContext, domainsecurity.ApprovalGrantTransitionV1{}, pending, approved)
	if err == nil || item != nil || event != nil {
		t.Fatalf("audit-only V1 authority produced an approval transition: item=%#v event=%#v err=%v", item, event, err)
	}
}

func TestRegistryRejectsSourceGrantReboundIntoDerivedThread(t *testing.T) {
	now := time.Date(2026, 7, 14, 4, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread_source", "turn_source", "/workspace/source", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_source"), Name: "read", Arguments: json.RawMessage(`{"path":"source.md"}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"read"}, true, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	source := activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant)
	body, _ := json.Marshal(source)
	derived := map[string]any{}
	if err := json.Unmarshal(body, &derived); err != nil {
		t.Fatal(err)
	}
	derived["id"] = "thread_derived"
	turn := derived["turns"].([]any)[0].(map[string]any)
	turn["threadId"] = "thread_derived"
	callItem := turn["items"].([]any)[0].(map[string]any)
	callItem["threadId"] = "thread_derived"

	if _, err := RegistryFromThread("thread_derived", derived, securityContext.TurnID); err == nil {
		t.Fatal("source grant was re-registered as derived-thread authority")
	}
	result := map[string]any{
		"id": "result_source", "kind": "tool_result", "threadId": "thread_derived", "turnId": securityContext.TurnID,
		"toolName": call.Name, "callId": call.ID, "status": "completed", "isError": false,
		"createdAt": now.Add(time.Second).Format(time.RFC3339Nano), "finishedAt": now.Add(time.Second).Format(time.RFC3339Nano),
		"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": grant.GrantID,
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.LegacyWithheldProjectionV1()),
	}
	turn["items"] = append(turn["items"].([]any), result)
	if _, err := DurableSettlementFromThread("thread_derived", derived, securityContext.TurnID, "result_source", grant); err == nil {
		t.Fatal("durable settlement accepted source authority rebound into a derived thread")
	}
	delete(turn, "securityContext")
	if _, err := RegistryFromThread("thread_derived", derived, securityContext.TurnID); err == nil {
		t.Fatal("authority-bearing derived history without a frozen context was accepted")
	}
	if _, err := DurableSettlementFromThread("thread_derived", derived, securityContext.TurnID, "result_source", grant); err == nil {
		t.Fatal("durable settlement accepted derived source authority without a frozen context")
	}
}

func TestRegistryRejectsGrantBoundToDifferentFrozenContext(t *testing.T) {
	now := time.Date(2026, 7, 14, 4, 30, 0, 0, time.UTC)
	frozen := newExecutionGrantGeneralFixture(t, "thread_context", "turn_context", "/workspace/context", now).Context
	other := newExecutionGrantGeneralFixture(t, "thread_context", "turn_context", "/workspace/context", now.Add(time.Second)).Context
	if frozen.ContextDigest == other.ContextDigest {
		t.Fatal("context fixtures unexpectedly share a digest")
	}
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_context"), Name: "read", Arguments: json.RawMessage(`{"path":"context.md"}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(other, "provider_1", call, schemas, []string{"read"}, true, "not_required", 0, "", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	thread := activeExecutionGrantThread(other, other.WorkspaceRealPath, call, grant)
	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["securityContext"] = frozen
	if _, err := RegistryFromThread(frozen.ThreadID, thread, frozen.TurnID); err == nil {
		t.Fatal("grant bound to a different frozen context was accepted")
	}
}

func TestRegistryRejectsAuditOnlyV1Authority(t *testing.T) {
	now := time.Date(2026, 7, 14, 4, 45, 0, 0, time.UTC)
	contextV1 := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_v1", TurnID: "turn_v1", WorkspaceRealPath: "/workspace/v1", ContextEpoch: 1, IssuedAt: now,
	})
	arguments := map[string]any{"path": "v1.md"}
	argumentBody, _ := json.Marshal(arguments)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextV1, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: "call_v1",
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	thread := map[string]any{
		"id": contextV1.ThreadID,
		"turns": []any{map[string]any{
			"id": contextV1.TurnID, "threadId": contextV1.ThreadID, "securityContext": contextV1,
			"items": []any{map[string]any{
				"id": "call_v1_item", "kind": "tool_call", "threadId": contextV1.ThreadID, "turnId": contextV1.TurnID,
				"toolName": "read", "callId": "call_v1", "arguments": arguments, "createdAt": now.Format(time.RFC3339Nano),
				"contextDigest": contextV1.ContextDigest, "contextEpoch": float64(contextV1.ContextEpoch),
				"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
			}},
		}},
	}
	if _, err := RegistryFromThread(contextV1.ThreadID, thread, contextV1.TurnID); err == nil {
		t.Fatal("audit-only V1 context became execution registry authority")
	}
}

func TestRegistryAllowsAuthorityFreeHistoricalToolPair(t *testing.T) {
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionHostStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "tool_completed", Status: "completed", Code: "tool_completed",
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
	}
	thread := map[string]any{
		"id": "thread_derived",
		"turns": []any{map[string]any{
			"id": "turn_source", "threadId": "thread_derived", "status": "completed",
			"items": []any{
				map[string]any{
					"id": "call_item", "kind": "tool_call", "threadId": "thread_derived", "turnId": "turn_source",
					"toolName": "read", "callId": "call_source", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
				},
				map[string]any{
					"id": "result_item", "kind": "tool_result", "threadId": "thread_derived", "turnId": "turn_source",
					"toolName": "read", "callId": "call_source", "isError": false,
					"output": domaintoolresult.PublicToolResultProjectionRecordV1(projection),
				},
			},
		}},
	}
	registry, err := RegistryFromThread("thread_derived", thread, "turn_source")
	if err != nil {
		t.Fatalf("authority-free provider pairing history was rejected: %v", err)
	}
	if registry.ThreadID != "thread_derived" || len(registry.Entries) != 0 {
		t.Fatalf("authority-free history created registry authority: %#v", registry)
	}
	now := time.Date(2026, 7, 14, 5, 0, 0, 0, time.UTC)
	sourceContext := newExecutionGrantGeneralFixture(t, "thread_source", "turn_source", "/workspace/source", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_source"), Name: "read", Arguments: json.RawMessage(`{"path":"source.md"}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(sourceContext, "provider_1", call, schemas, []string{"read"}, true, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	source := activeExecutionGrantThread(sourceContext, sourceContext.WorkspaceRealPath, call, grant)
	appendItem := source["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	appendItem["threadId"] = "thread_derived"
	appendItem["turnId"] = "turn_source"
	if err := ValidateRegistryAppend("thread_derived", thread, "turn_source", appendItem); err == nil {
		t.Fatal("authority item was appended to authority-free history without a frozen context")
	}
}
