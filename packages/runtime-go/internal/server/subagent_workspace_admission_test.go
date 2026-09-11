package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestCrossWorkspaceForegroundSubagentRejectsBeforeDurableJobOrThread(t *testing.T) {
	parentWorkspace := t.TempDir()
	childWorkspace := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(), Host: "127.0.0.1",
		ProviderID: "workspace-admission-provider", BaseURL: "https://provider.invalid/v1", APIKey: "test-key",
		Model: "test-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	t.Cleanup(func() { _ = handler.Shutdown(context.Background()) })

	args := map[string]any{"prompt": "inspect another workspace", "workspace": childWorkspace}
	arguments, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_workspace_parent", TurnID: "turn_workspace_parent", WorkspaceRealPath: parentWorkspace,
		CaseID: "case_workspace_parent", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-workspace-parent")),
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil),
		ContextEpoch:       1, IssuedAt: now,
	})
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("cross_workspace_foreground"), Name: "task", Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "workspace-admission-provider", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema-workspace-task")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope-workspace-task")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	request, err := subagentapp.TaskRequestFromArgs("task", args)
	if err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: parentWorkspace,
		ProviderID: "workspace-admission-provider", Model: "test-model", ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, call.ID),
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", Call: call,
		SecurityContext: securityContext, ExecutionGrant: grant,
	}

	result := handler.runRuntimeSubagentTask(context.Background(), pending, request)
	if !result.IsError || stringField(result.Output, "code") != "subagent_failed" ||
		stringField(result.Output, "failureCode") != "child_execution_failed" ||
		strings.Contains(fmt.Sprint(result.Output), childWorkspace) {
		t.Fatalf("cross-workspace foreground admission did not fail closed: %#v", result)
	}
	if records, err := handler.jobs.List(securityContext.ThreadID); err != nil || len(records) != 0 {
		t.Fatalf("rejected foreground admission created durable child jobs: records=%#v err=%v", records, err)
	}
	if threads, err := handler.store.ListThreads(false, true, true, ""); err != nil || len(threads) != 0 {
		t.Fatalf("rejected foreground admission created durable child threads: threads=%#v err=%v", threads, err)
	}
}
