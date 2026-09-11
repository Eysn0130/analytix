package subagent

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type jobSecurityThreadStub struct {
	thread map[string]any
}

type jobSecurityUpdateStub struct {
	request domainjob.UpdateRequest
	err     error
}

func (stub *jobSecurityUpdateStub) UpdateChildRun(_ string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	stub.request = request
	return domainjob.Record{}, stub.err
}

func (stub jobSecurityThreadStub) GetThread(string) (map[string]any, error) {
	return stub.thread, nil
}

func TestJobSecurityBindingRequiresDurableParentGrantMembershipBeforeResume(t *testing.T) {
	context, grant, thread := jobSecurityAuthorityFixture(t, "task", false)
	binding, err := domainjob.NewSecurityBinding(context, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: context.ThreadID, ParentTurnID: context.TurnID,
		ParentToolCallID: grant.ToolCallID, Kind: "subagent", SecurityBinding: binding,
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	authorizer := JobSecurityAuthorizer{Threads: jobSecurityThreadStub{thread: thread}}
	checkAt := mustJobSecurityTime(t, grant.IssuedAt).Add(time.Minute)
	if blocker := authorizer.BlockerAt(record, checkAt); blocker != "" {
		t.Fatalf("durable parent grant was rejected: %s", blocker)
	}

	otherGrant := grant
	otherGrant.ScopeHash = domainsecurity.SHA256Hex([]byte("other-scope"))
	otherGrant.GrantID = ""
	otherGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: grant.Provider, ServerIdentity: grant.ServerIdentity, ToolName: grant.ToolName,
		ToolCallID: grant.ToolCallID, ArgsHash: grant.ArgsHash, SchemaHash: grant.SchemaHash, ScopeHash: otherGrant.ScopeHash,
		ReadOnly: grant.ReadOnly, ApprovalState: grant.ApprovalState, IssuedAt: mustJobSecurityTime(t, grant.IssuedAt), ExpiresAt: mustJobSecurityTime(t, grant.ExpiresAt),
	})
	tamperedBinding, err := domainjob.NewSecurityBinding(context, otherGrant, otherGrant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record.SecurityBinding = tamperedBinding
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	if blocker := authorizer.BlockerAt(record, checkAt); blocker != "parent_execution_grant_missing" {
		t.Fatalf("recomputed binding for an unknown grant did not fail closed: %q", blocker)
	}
}

func TestJobSecurityBindingRejectsWrongParentGrantTool(t *testing.T) {
	context, grant, thread := jobSecurityAuthorityFixture(t, "read", false)
	binding, err := domainjob.NewSecurityBinding(context, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: context.ThreadID, ParentTurnID: context.TurnID,
		ParentToolCallID: grant.ToolCallID, Kind: "subagent", SecurityBinding: binding,
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	if blocker := (JobSecurityAuthorizer{Threads: jobSecurityThreadStub{thread: thread}}).BlockerAt(record, mustJobSecurityTime(t, grant.IssuedAt).Add(time.Minute)); blocker != "parent_execution_grant_mismatch" {
		t.Fatalf("non-subagent grant authorized a subagent job: %q", blocker)
	}
}

func TestJobSecurityBindingAcceptsSettledParentGrantForCompletionDelivery(t *testing.T) {
	context, grant, thread := jobSecurityAuthorityFixture(t, "bash", true)
	binding, err := domainjob.NewSecurityBinding(context, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: context.ThreadID, ParentTurnID: context.TurnID,
		ParentToolCallID: grant.ToolCallID, Kind: "background-shell", SecurityBinding: binding, Background: true,
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	if blocker := (JobSecurityAuthorizer{Threads: jobSecurityThreadStub{thread: thread}}).BlockerAt(record, mustJobSecurityTime(t, grant.IssuedAt).Add(time.Minute)); blocker != "" {
		t.Fatalf("settled parent grant lost durable completion authority: %q", blocker)
	}
}

func TestJobSecurityBindingRejectsExpiredParentGrant(t *testing.T) {
	context, grant, thread := jobSecurityAuthorityFixture(t, "task", false)
	binding, err := domainjob.NewSecurityBinding(context, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: context.ThreadID, ParentTurnID: context.TurnID,
		ParentToolCallID: grant.ToolCallID, Kind: "subagent", SecurityBinding: binding,
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	expiresAt := mustJobSecurityTime(t, grant.ExpiresAt)
	if blocker := (JobSecurityAuthorizer{Threads: jobSecurityThreadStub{thread: thread}}).BlockerAt(record, expiresAt); blocker != "parent_execution_grant_expired" {
		t.Fatalf("expired parent grant did not fail closed: %q", blocker)
	}
}

func TestRecoveredSecurityBoundSubagentWithoutToolManifestDeadLetters(t *testing.T) {
	context, grant, thread := jobSecurityAuthorityFixture(t, "task", false)
	binding, err := domainjob.NewSecurityBinding(context, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "job-recovered", ParentThreadID: context.ThreadID, ParentTurnID: context.TurnID,
		ParentToolCallID: grant.ToolCallID, Kind: "subagent", Status: string(domainjob.StatusRunning), SecurityBinding: binding,
	}
	updater := &jobSecurityUpdateStub{}
	authorizer := JobSecurityAuthorizer{Threads: jobSecurityThreadStub{thread: thread}, Jobs: updater}
	blocker := authorizer.BlockerAt(
		record, mustJobSecurityTime(t, grant.IssuedAt).Add(time.Minute),
	)
	if blocker != "job_delegated_tool_manifest_invalid" {
		t.Fatalf("recovered job without delegated manifest blocker=%q", blocker)
	}
	if err := authorizer.MarkDeadLetter(record, blocker); err != nil {
		t.Fatal(err)
	}
	if updater.request.DeadLetterReason != blocker || updater.request.RecoveryStatus != "dead_lettered" ||
		updater.request.CompletionDeliveryStatus != "dead_letter" || updater.request.LateCompletionSuppressed == nil || !*updater.request.LateCompletionSuppressed {
		t.Fatalf("invalid recovered manifest was not dead-lettered: %#v", updater.request)
	}
}

func TestJobSecurityDeadLetterPersistenceFailurePropagates(t *testing.T) {
	sentinel := errors.New("durable write failed")
	updater := &jobSecurityUpdateStub{err: sentinel}
	err := (JobSecurityAuthorizer{Jobs: updater}).MarkDeadLetter(domainjob.Record{ID: "job-1"}, "parent_security_context_mismatch")
	if !errors.Is(err, sentinel) {
		t.Fatalf("dead-letter persistence failure was swallowed: %v", err)
	}
}

func jobSecurityAuthorityFixture(t *testing.T, toolName string, settled bool) (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant, map[string]any) {
	t.Helper()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	context, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("sources")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"prompt": "work"}
	argumentsBody, _ := json.Marshal(arguments)
	callID := subagentTestHostToolCallID("security-authority-" + toolName)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentsBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: toolName != "bash", ApprovalState: "not_required", IssuedAt: now,
	})
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	items := []any{map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(context.TurnID, grant.ToolCallID), "threadId": context.ThreadID, "turnId": context.TurnID, "kind": "tool_call",
		"toolName": toolName, "callId": grant.ToolCallID, "arguments": arguments, "createdAt": now.Format(time.RFC3339Nano),
		"contextDigest": context.ContextDigest, "contextEpoch": float64(context.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
	}}
	if settled {
		items = append(items, map[string]any{
			"id": domaintoolresult.ToolResultItemIDV1(context.TurnID, grant.ToolCallID), "threadId": context.ThreadID, "turnId": context.TurnID, "kind": "tool_result",
			"toolName": toolName, "callId": grant.ToolCallID, "createdAt": now.Add(time.Second).Format(time.RFC3339Nano),
			"finishedAt": now.Add(time.Second).Format(time.RFC3339Nano), "contextDigest": context.ContextDigest,
			"contextEpoch": float64(context.ContextEpoch), "executionGrantId": grant.GrantID,
		})
	}
	return context, grant, map[string]any{
		"id": context.ThreadID, "securityState": jobSecurityContractRecord(context),
		"turns": []any{map[string]any{"id": context.TurnID, "threadId": context.ThreadID, "securityContext": jobSecurityContractRecord(context), "items": items}},
	}
}

func subagentTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("app-subagent-test-tool-call:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func jobSecurityContractRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func mustJobSecurityTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
