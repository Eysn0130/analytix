package jobs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestJobSecurityBindingPersistsAcrossManagerRestart(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_1"), ContextEpoch: 3, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := jobsTestHostToolCallID("security-binding-restart")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, toolCallID)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	startRequest := StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID, ParentToolCallID: toolCallID,
		SecurityBinding: binding, Kind: "subagent", Status: "running", Background: true,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID,
		ParentToolCallID: toolCallID, SecurityBinding: binding, Kind: "subagent", Status: "completed",
	}); err == nil || !strings.Contains(err.Error(), "completion receipt") {
		t.Fatalf("case-bound child started completed without a trusted receipt: %v", err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "completed"}); err == nil || !strings.Contains(err.Error(), "completion receipt") {
		t.Fatalf("case-bound child updated to completed without a trusted receipt: %v", err)
	}
	withThread, err := manager.UpdateChildRun(record.ID, UpdateRequest{ChildThreadID: "thread-child"})
	if err != nil || withThread.ChildThreadID != "thread-child" {
		t.Fatalf("set immutable child thread: record=%#v err=%v", withThread, err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{ChildThreadID: "thread-other"}); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("child thread identity changed after binding: %v", err)
	}
	withTurn, err := manager.UpdateChildRun(record.ID, UpdateRequest{ChildTurnID: "turn-child"})
	if err != nil || withTurn.ChildTurnID != "turn-child" {
		t.Fatalf("set immutable child turn: record=%#v err=%v", withTurn, err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{ChildTurnID: "turn-other"}); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("child turn identity changed after binding: %v", err)
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reloaded.LoadChildRun(record.ID)
	if err != nil || domainjob.ValidateSecurityBinding(restored.SecurityBinding) != nil || restored.SecurityBinding.BindingDigest != binding.BindingDigest {
		t.Fatalf("durable job security binding was not restored: record=%#v err=%v", restored, err)
	}
	if !domainjob.CaseDelegationContextsEqualV1(restored.CaseDelegation, record.CaseDelegation) ||
		restored.Prompt != record.Prompt || restored.Name != domainjob.CaseDelegationNameV1 || restored.Label != domainjob.CaseDelegationLabelV1 {
		t.Fatalf("durable typed case delegation was not restored exactly: %#v", restored)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Output: "6222020000000000000 private child reasoning"}); err == nil {
		t.Fatal("security-bound child output was persisted")
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Usage: map[string]any{
		"totalTokens": float64(11), "reasoning": "private child reasoning", "account": "6222020000000000000",
	}}); err == nil {
		t.Fatal("security-bound child usage payload was persisted in the job record")
	}
	const poisonedExistingOutput = "HOSTILE_REPLAY_OUTPUT_7B4E"
	manager.mu.Lock()
	manager.jobs[0].Output = poisonedExistingOutput
	manager.mu.Unlock()
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Output: "REJECTED_UPDATE_OUTPUT_9C2A"}); err == nil {
		t.Fatal("hostile security-bound replay output was accepted")
	}
	manager.mu.Lock()
	if manager.jobs[0].Output != poisonedExistingOutput {
		manager.mu.Unlock()
		t.Fatal("rejected security-bound update mutated the in-memory durable record")
	}
	manager.jobs[0].Output = poisonedExistingOutput
	manager.mu.Unlock()
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Usage: map[string]any{"private": "REJECTED_UPDATE_USAGE_50A7"}}); err == nil {
		t.Fatal("hostile security-bound replay usage was accepted")
	}
	manager.mu.Lock()
	if manager.jobs[0].Output != poisonedExistingOutput {
		manager.mu.Unlock()
		t.Fatal("rejected security-bound usage update mutated the in-memory durable record")
	}
	manager.jobs[0].Output = ""
	manager.mu.Unlock()
	failed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusFailed), Error: "SOL_PRIVATE_TRACE_7C private branch alpha 6222020000000000000",
	})
	if err != nil {
		t.Fatalf("typed security-bound failure update: %v", err)
	}
	if failed.Error != "" || failed.FailureCode != domainjob.FailureChildExecutionFailed {
		t.Fatalf("security-bound failure retained raw text: %#v", failed)
	}
	if _, err := os.Stat(filepath.Join(root, record.ID+".log")); !os.IsNotExist(err) {
		t.Fatalf("security-bound child output artifact exists: %v", err)
	}
	memoryOnly, err := NewManager("")
	if err != nil {
		t.Fatal(err)
	}
	missingDelegation := StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID,
		ParentToolCallID: toolCallID, SecurityBinding: binding, Kind: "subagent", Status: "running", Prompt: "raw case prompt acct:1",
	}
	if _, err := memoryOnly.StartChildRun(missingDelegation); err == nil || !strings.Contains(err.Error(), "delegation") {
		t.Fatalf("case-bound child without typed delegation was accepted: %v", err)
	}
	outputRequest := startRequest
	outputRequest.Output = "private"
	if _, err := memoryOnly.StartChildRun(outputRequest); err == nil {
		t.Fatal("rootless manager retained security-bound child output")
	}
	usageRequest := startRequest
	usageRequest.Usage = map[string]any{"privatePayload": "private child reasoning"}
	if _, err := memoryOnly.StartChildRun(usageRequest); err == nil {
		t.Fatal("rootless manager retained security-bound child usage")
	}
	wrong := domainjob.CloneSecurityBinding(binding)
	wrong.ParentTurnID = "turn_other"
	if _, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID, ParentToolCallID: toolCallID,
		SecurityBinding: wrong, Kind: "subagent",
	}); err == nil {
		t.Fatal("tampered/mismatched job security binding was persisted")
	}
	path := filepath.Join(root, record.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	if serialized := string(body); strings.Contains(serialized, "SOL_PRIVATE_TRACE_7C") || strings.Contains(serialized, "6222020000000000000") {
		t.Fatalf("security-bound child job persisted raw failure text: %s", serialized)
	}
	persisted["error"] = "SOL_PRIVATE_TRACE_7C private branch alpha"
	tamperedErrorBody, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tamperedErrorBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil || !strings.Contains(err.Error(), "error text") {
		t.Fatalf("tampered security-bound child error text was accepted during recovery: %v", err)
	}
	beforeSemanticErrorMigration, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil || !strings.Contains(err.Error(), "cannot be migrated") {
		t.Fatalf("semantic migration erased a tampered security-bound child error: %v", err)
	}
	afterSemanticErrorMigration, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeSemanticErrorMigration, afterSemanticErrorMigration) {
		t.Fatal("failed semantic migration modified the security-bound child record")
	}
	delete(persisted, "error")
	persisted["usage"] = map[string]any{"totalTokens": float64(11), "privatePayload": "private child reasoning"}
	body, err = json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil || !strings.Contains(err.Error(), "child usage") {
		t.Fatalf("tampered security-bound child usage was accepted during recovery: %v", err)
	}
	beforeSemanticUsageMigration, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil || !strings.Contains(err.Error(), "cannot be migrated") {
		t.Fatalf("semantic migration erased tampered security-bound child usage: %v", err)
	}
	afterSemanticUsageMigration, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeSemanticUsageMigration, afterSemanticUsageMigration) {
		t.Fatal("failed semantic migration modified the security-bound child usage record")
	}
}

func TestJobRecordUnknownTopLevelFieldFailsClosedOnRestart(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: "thread_1", Kind: "child-run", Status: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["unknownAuthority"] = "fail-closed"
	body, err = json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil || !strings.Contains(err.Error(), "shape") {
		t.Fatalf("unknown durable job field was accepted: %v", err)
	}
}

func TestJobSecurityBindingFailsClosedWhenPersistedBindingIsTampered(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("sources")), ContextEpoch: 3, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := jobsTestHostToolCallID("security-binding-tamper")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, toolCallID)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	startRequest := StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID,
		ParentToolCallID: toolCallID, SecurityBinding: binding, Kind: "subagent", Status: "queued", Background: true,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	securityBinding, _ := persisted["securityBinding"].(map[string]any)
	securityBinding["parentTurnId"] = "turn_tampered"
	body, err = json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("tampered persisted job security binding was accepted during recovery")
	}
}

func TestBoundBackgroundRecordMutationFailsClosedDuringSemanticStartup(t *testing.T) {
	root, path := newBoundBackgroundRecordForSemanticMigrationTest(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["name"] = "curl<think>PRIVATE_BOUND_BACKGROUND</think>"
	tampered, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil || !strings.Contains(err.Error(), "cannot be migrated") {
		t.Fatalf("bound background mutation was silently projected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, tampered) {
		t.Fatalf("rejected bound background migration changed bytes: err=%v", err)
	}
}

func TestBoundBackgroundArtifactPathCannotBeRewrittenDuringSemanticStartup(t *testing.T) {
	root, path := newBoundBackgroundRecordForSemanticMigrationTest(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "job-1.log")
	sentinel := []byte("bound external sentinel")
	if err := os.WriteFile(external, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	persisted["artifactPath"] = filepath.ToSlash(external)
	tampered, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil || !strings.Contains(err.Error(), "cannot be migrated") {
		t.Fatalf("bound artifact path was silently projected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, tampered) {
		t.Fatalf("rejected bound artifact migration changed record bytes: err=%v", err)
	}
	externalAfter, err := os.ReadFile(external)
	if err != nil || !bytes.Equal(externalAfter, sentinel) {
		t.Fatalf("rejected bound artifact migration changed external bytes: err=%v", err)
	}
}

func TestBoundChildSequenceBackfillRejectedDuringSemanticStartup(t *testing.T) {
	root, path := newBoundBackgroundRecordForSemanticMigrationTest(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["childSeq"] = float64(0)
	tampered, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil || !strings.Contains(err.Error(), "cannot be backfilled") {
		t.Fatalf("bound child sequence was silently backfilled: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, tampered) {
		t.Fatalf("rejected bound child sequence migration changed bytes: err=%v", err)
	}
}

func TestBoundSubagentNestedMutationFailsClosedDuringSemanticStartup(t *testing.T) {
	root, path := newBoundSubagentRecordForSemanticMigrationTest(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["childTodoLists"] = []any{map[string]any{
		"id": "todo-list-bound", "parentThreadId": "thread_bound_subagent", "childThreadId": "thread_child",
		"childRunId": "job-1", "jobId": "job-1", "scope": "scope<think>PRIVATE_BOUND_NESTED</think>",
		"items":     []any{map[string]any{"id": "todo-1", "content": "account 6222020202020202020", "status": "pending"}},
		"createdAt": "2026-07-20T08:00:00Z", "updatedAt": "2026-07-20T08:00:00Z",
	}}
	tampered, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil || !strings.Contains(err.Error(), "cannot be migrated") {
		t.Fatalf("bound nested subagent mutation was silently projected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, tampered) {
		t.Fatalf("rejected bound nested migration changed bytes: err=%v", err)
	}
}

func TestBoundSemanticStartupValidationPreservesExactRecordBytes(t *testing.T) {
	root, path := newBoundBackgroundRecordForSemanticMigrationTest(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatalf("valid bound record failed semantic validation: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("semantic validation reformatted a bound record: err=%v", err)
	}
}

func newBoundBackgroundRecordForSemanticMigrationTest(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_bound_background", TurnID: "turn_bound_background", WorkspaceRealPath: "/workspace/bound",
		CaseID: "case_bound_background", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-bound-background")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-bound-background"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("sources-bound-background")), ContextEpoch: 9, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := jobsTestHostToolCallID("bound-background-migration")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, toolCallID)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_bound_background", ParentThreadID: securityContext.ThreadID,
		ParentTurnID: securityContext.TurnID, ParentToolCallID: toolCallID, SecurityBinding: binding,
		Kind: "background-shell", Name: "bash", Label: "background shell", Status: string(domainjob.StatusRunning), Background: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(root, record.ID+".json")
}

func newBoundSubagentRecordForSemanticMigrationTest(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_bound_subagent", TurnID: "turn_bound_subagent", WorkspaceRealPath: "/workspace/bound",
		CaseID: "case_bound_subagent", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-bound-subagent")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-bound-subagent"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("sources-bound-subagent")), ContextEpoch: 10, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := jobsTestHostToolCallID("bound-subagent-migration")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, toolCallID)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	request := StartRequest{
		ParentGoalID: "goal_bound_subagent", ParentThreadID: securityContext.ThreadID,
		ParentTurnID: securityContext.TurnID, ParentToolCallID: toolCallID, SecurityBinding: binding,
		Kind: "subagent", Status: string(domainjob.StatusRunning), Background: true,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &request)
	record, err := manager.StartChildRun(request)
	if err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(root, record.ID+".json")
}
