package server

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	jobs "analytix.local/runtime-go/internal/jobs"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
)

type serverBoundJobParent struct {
	Context domainsecurity.TurnSecurityContext
	Grant   domainsecurity.ExecutionGrant
	Binding *domainjob.SecurityBinding
	ItemID  string
	CallID  string
}

func appendServerBoundJobParent(
	t *testing.T,
	store *DurableEventSessionStore,
	threadID, turnID, workspace, kind string,
) serverBoundJobParent {
	t.Helper()
	if store == nil {
		t.Fatal("job parent store is unavailable")
	}
	now := time.Now().UTC().Truncate(time.Second)
	workspaceRealPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspaceRealPath, ContextEpoch: 1, IssuedAt: now,
	})
	toolName := "task"
	if strings.TrimSpace(kind) == "background-shell" {
		toolName = "bash"
	}
	callID := serverTestHostToolCallID("job-parent-" + turnID + "-" + toolName)
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	arguments := map[string]any{"prompt": "bounded background work"}
	if toolName == "bash" {
		arguments = map[string]any{"command": "true"}
	}
	argumentsBody, _ := json.Marshal(arguments)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentsBody), SchemaHash: domainsecurity.SHA256Hex([]byte("job-parent-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("job-parent-scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	})
	grantRecord := map[string]any{}
	grantBody, _ := json.Marshal(grant)
	_ = json.Unmarshal(grantBody, &grantRecord)
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": turnsecurityapp.PublicRecord(securityContext),
		"items": []any{map[string]any{
			"id": itemID, "turnId": turnID, "threadId": threadID, "kind": "tool_call", "toolName": toolName,
			"toolKind": kind, "callId": callID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano),
			"arguments": arguments, "contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
			"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
		}},
	}, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(securityContext)}); err != nil {
		t.Fatal(err)
	}
	storedThread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executiongrantapp.RegistryFromThread(threadID, storedThread, turnID); err != nil {
		t.Fatalf("bound parent execution-grant registry is invalid: %v", err)
	}
	return serverBoundJobParent{Context: securityContext, Grant: grant, Binding: binding, ItemID: itemID, CallID: callID}
}

func TestStartupRecoveryStaleEpochDeadLettersBeforeParentMutation(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{"id": "thr_stale_startup_job", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	parent := appendServerBoundJobParent(t, store, threadID, "turn_epoch_a", workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	startRequest := jobs.StartRequest{
		ParentGoalID: "goal-stale-startup", ParentThreadID: threadID, ParentTurnID: parent.Context.TurnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID, Kind: "subagent",
		Status: "running", Background: true, SecurityBinding: parent.Binding,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	contextB := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_epoch_b", WorkspaceRealPath: parent.Context.WorkspaceRealPath,
		CaseID: "case-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")),
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil), ContextEpoch: parent.Context.ContextEpoch + 1,
		IssuedAt: time.Now().UTC().Add(time.Minute),
	})
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": contextB.TurnID, "threadId": threadID, "status": "completed", "createdAt": contextB.IssuedAt,
		"securityContext": turnsecurityapp.PublicRecord(contextB), "items": []any{},
	}, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(contextB)}); err != nil {
		t.Fatal(err)
	}
	cleaned, err := manager.CleanupStaleRunningRecords()
	if err != nil || len(cleaned) != 1 {
		t.Fatalf("cleanup stale job: records=%#v err=%v", cleaned, err)
	}
	beforeThread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeBody, _ := json.Marshal(beforeThread)
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	if err := runtimeStartupRecoveryForTest(handler).RecoverInterrupted(cleaned); err != nil {
		t.Fatal(err)
	}
	afterThread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := json.Marshal(afterThread)
	afterSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBody) != string(afterBody) || beforeSeq != afterSeq {
		t.Fatalf("stale recovery mutated parent authority: beforeSeq=%d afterSeq=%d before=%s after=%s", beforeSeq, afterSeq, beforeBody, afterBody)
	}
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RecoveryStatus != "dead_lettered" || updated.CompletionDeliveryStatus != "dead_letter" ||
		updated.DeadLetterReason != "parent_security_context_mismatch" || !updated.LateCompletionSuppressed {
		t.Fatalf("stale recovery did not close as dead-lettered: %#v", updated)
	}
	jobBody, err := json.Marshal(updated)
	if err != nil || strings.Contains(string(jobBody), `"recoveryStatus":"recovered"`) {
		t.Fatalf("stale recovery ever claimed recovered authority: err=%v body=%s", err, jobBody)
	}
}

func TestBackgroundJobCaseSwitchRejectsLateDelivery(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{"id": "thr_job_security", "title": "Job security", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC().Truncate(time.Second)
	contextA := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_case_a", WorkspaceRealPath: workspace, CaseID: "case_a",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-a")),
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil), ContextEpoch: 1, IssuedAt: now,
	})
	arguments := map[string]any{"prompt": "background"}
	argumentsBody, _ := json.Marshal(arguments)
	callID := serverTestHostToolCallID("case-switch-task")
	itemID := domaintoolcall.ToolCallItemIDV1(contextA.TurnID, callID)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextA, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentsBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	grantRecord := map[string]any{}
	grantBody, _ := json.Marshal(grant)
	_ = json.Unmarshal(grantBody, &grantRecord)
	binding, err := domainjob.NewSecurityBinding(contextA, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": "turn_case_a", "threadId": threadID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": turnsecurityapp.PublicRecord(contextA),
		"items": []any{map[string]any{
			"id": itemID, "turnId": "turn_case_a", "threadId": threadID, "kind": "tool_call", "toolName": "task",
			"callId": callID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano), "arguments": arguments,
			"contextDigest": contextA.ContextDigest, "contextEpoch": float64(contextA.ContextEpoch),
			"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
		}},
	}, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(contextA)}); err != nil {
		t.Fatal(err)
	}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const secret = "CASE_A_SECRET_AMOUNT_123"
	startRequest := jobs.StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: threadID, ParentTurnID: contextA.TurnID, ParentToolItemID: itemID,
		ParentToolCallID: callID, SecurityBinding: binding, Kind: "subagent", Status: "queued", Background: true, Output: secret,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	if _, unsafeErr := manager.StartChildRun(startRequest); unsafeErr == nil || !strings.Contains(unsafeErr.Error(), "security-bound child output") {
		t.Fatalf("security-bound child raw output was accepted: %v", unsafeErr)
	}
	startRequest.Output = ""
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	events := subagentapp.BuildJobLifecycleEvents(subagentapp.JobLifecycleEventInput{
		ThreadID: threadID, TurnID: contextA.TurnID, ItemID: itemID, CallID: callID, ToolName: "task",
		Record: record, Status: "completed", ProgressStatus: "success",
	})
	eventBytes, _ := json.Marshal(events)
	if strings.Contains(string(eventBytes), secret) || !strings.Contains(string(eventBytes), "untrusted_child_output") {
		t.Fatalf("security-bound child output leaked into persisted/SSE lifecycle event: %s", eventBytes)
	}
	if messages := handler.runtimeBackgroundJobContextMessages(threadID, 0, contextA); len(messages) != 1 || strings.Contains(messages[0].Content, secret) {
		t.Fatalf("security-bound child output was injected into provider context: %#v", messages)
	}
	contextB := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_case_b", WorkspaceRealPath: workspace, CaseID: "case_b",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-b")),
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil), ContextEpoch: 2, IssuedAt: now.Add(time.Minute),
	})
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": contextB.TurnID, "threadId": threadID, "status": "completed", "createdAt": contextB.IssuedAt,
		"securityContext": turnsecurityapp.PublicRecord(contextB), "items": []any{},
	}, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(contextB)}); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.authorizeRuntimeJobStart(record); err == nil || !strings.Contains(err.Error(), "parent_security_context_mismatch") {
		t.Fatalf("queued case A job was not rejected before execution after switching to case B: %v", err)
	}
	if messages := handler.runtimeBackgroundJobContextMessages(threadID, 0, contextB); len(messages) != 0 {
		t.Fatalf("case A job output entered case B provider context: %#v", messages)
	}
	handler.recordRuntimeJobLifecycleEvent(record, "completed", "")
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.LateCompletionSuppressed || updated.DeadLetterReason != "parent_security_context_mismatch" || updated.CompletionDeliveryStatus != "dead_letter" {
		t.Fatalf("late case A completion was not dead-lettered: %#v", updated)
	}
	output, isError := handler.executeRuntimeTaskJobOutput(runtimePendingToolCall{
		ThreadID: threadID, TurnID: contextB.TurnID, SecurityContext: contextB,
		Call: domainmodel.ToolCall{ID: "call_output", Name: "bash_output"},
	}, map[string]any{"job_id": record.ID})
	encoded, _ := json.Marshal(output)
	if !isError || !strings.Contains(string(encoded), subagentapp.TaskJobErrorNotFound) || strings.Contains(string(encoded), secret) {
		t.Fatalf("case B task-job output endpoint exposed case A data: %s", encoded)
	}
	diagnostics, _ := json.Marshal(handler.runtimeSubagentToolDiagnostics(threadID))
	if strings.Contains(string(diagnostics), secret) || strings.Contains(string(diagnostics), record.ID) {
		t.Fatalf("runtime tools exposed stale case A membership in case B: %s", diagnostics)
	}
	if reason := handler.runtimeBackgroundDeliveryService().AutoContinueGate(record); reason != "parent_security_context_mismatch" {
		t.Fatalf("case A auto-continue was not blocked after case switch: %q", reason)
	}
}

func TestRuntimeToolDiagnosticsAreThreadScopedAndNeverExposeChildOutput(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	threadA := ""
	jobA := ""
	jobB := ""
	for _, fixture := range []struct{ threadID, turnID, secret string }{
		{"thread-tools-a", "turn-tools-a", "THREAD_A_OUTPUT"},
		{"thread-tools-b", "turn-tools-b", "THREAD_B_SECRET_SENTINEL"},
	} {
		thread, createErr := store.CreateThread(map[string]any{"id": fixture.threadID, "workspace": t.TempDir()}, t.TempDir())
		if createErr != nil {
			t.Fatal(createErr)
		}
		threadID := stringField(thread, "id")
		if fixture.secret == "THREAD_A_OUTPUT" {
			threadA = threadID
		}
		parent := appendServerBoundJobParent(t, store, threadID, fixture.turnID, stringField(thread, "workspace"), "subagent")
		startRequest := jobs.StartRequest{
			ParentGoalID: "goal-tools", ParentThreadID: threadID, ParentTurnID: fixture.turnID,
			ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID, SecurityBinding: parent.Binding,
			Kind: "subagent", Status: "completed", Output: fixture.secret,
		}
		if _, unsafeErr := manager.StartChildRun(startRequest); unsafeErr == nil || !strings.Contains(unsafeErr.Error(), "security-bound child output") {
			t.Fatalf("security-bound child raw output was accepted for %s: %v", fixture.threadID, unsafeErr)
		}
		startRequest.Output = ""
		record, startErr := manager.StartChildRun(startRequest)
		if startErr != nil {
			t.Fatal(startErr)
		}
		if fixture.secret == "THREAD_A_OUTPUT" {
			jobA = record.ID
		} else {
			jobB = record.ID
		}
	}
	withoutScope, _ := json.Marshal(handler.runtimeSubagentToolDiagnostics(""))
	if strings.Contains(string(withoutScope), "THREAD_A_OUTPUT") || strings.Contains(string(withoutScope), "THREAD_B_SECRET_SENTINEL") {
		t.Fatalf("runtime tools exposed child runs without a thread scope: %s", withoutScope)
	}
	scoped, _ := json.Marshal(handler.runtimeSubagentToolDiagnostics(threadA))
	if strings.Contains(string(scoped), "THREAD_A_OUTPUT") || strings.Contains(string(scoped), "THREAD_B_SECRET_SENTINEL") ||
		!strings.Contains(string(scoped), jobA) || strings.Contains(string(scoped), jobB) ||
		!strings.Contains(string(scoped), "untrusted_child_output") {
		t.Fatalf("runtime tools crossed thread scope: %s", scoped)
	}
}
