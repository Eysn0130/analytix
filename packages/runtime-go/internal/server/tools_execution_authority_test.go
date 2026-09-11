package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	sideeffectidentityapp "analytix.local/runtime-go/internal/app/sideeffectidentity"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

func TestRuntimeSubagentSideEffectIdentityUsesEffectiveProfileAndIgnoresDeadRawAliases(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "profile-provider", BaseURL: "http://127.0.0.1:18997/v1", APIKey: "test-placeholder",
		EndpointFormat: "chat_completions", Model: "profile-model",
	}).(*runtimeServerHandler)
	handler.subagents = subagentapp.ProfileSettings{
		Enabled: true, DefaultToolPolicy: "readOnly", MaxParallel: 2, MaxChildRuns: 4,
		Profiles: map[string]subagentapp.ProfileConfig{
			"reviewer": {Name: "reviewer", ToolPolicy: "readOnly", MaxSteps: 4, MaxStepsSet: true},
		},
	}
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-profile-identity", TurnID: "turn-profile-identity", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	callID := serverTestHostToolCallID("profile-side-effect-identity")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "profile-provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("profile-side-effect-args")), SchemaHash: domainsecurity.SHA256Hex([]byte("profile-side-effect-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("profile-side-effect-scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	pending := runtimePendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, SecurityContext: securityContext,
		ProviderID: "profile-provider", Model: "profile-model", Effort: "auto", Workspace: workspace,
		Call: provider.ToolCall{ID: callID, Name: "task"}, ExecutionGrant: grant,
	}
	resolve := func(arguments string) (sideeffectidentityapp.IdentityV1, error) {
		return sideeffectidentityapp.ResolveV1(sideeffectidentityapp.Input{
			ToolName: "task", Arguments: json.RawMessage(arguments), WorkspaceRealPath: workspace,
			ResolveTask: func(request subagentapp.TaskRequest) (any, error) {
				return handler.resolveRuntimeSubagentSideEffectProjection(context.Background(), pending, request)
			},
		})
	}
	fromProfile, err := resolve(`{"prompt":"inspect","profile":"reviewer"}`)
	if err != nil {
		t.Fatal(err)
	}
	explicitSameEffect, err := resolve(`{"prompt":"inspect","profile":"reviewer","toolPolicy":"readOnly","maxSteps":4,"blockedSkills":["unused-b","unused-a"]}`)
	if err != nil || fromProfile != explicitSameEffect {
		t.Fatalf("effective-identical profile requests diverged: profile=%#v explicit=%#v err=%v", fromProfile, explicitSameEffect, err)
	}
	changed, err := resolve(`{"prompt":"inspect changed","profile":"reviewer"}`)
	if err != nil || fromProfile == changed {
		t.Fatalf("meaningful child prompt change collapsed: first=%#v changed=%#v err=%v", fromProfile, changed, err)
	}
}

func TestRuntimeSideEffectIntentDispatcherSelection(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		readOnly bool
		override any
		want     bool
	}{
		{name: "local write", toolName: "write_file", want: true},
		{name: "non-read-only MCP", toolName: "mcp__funds__query", want: true},
		{name: "subagent", toolName: "task", want: true},
		{name: "job kill", toolName: "kill_shell", want: true},
		{name: "job restart", toolName: "restart_job", want: true},
		{name: "foreground or background bash", toolName: "bash", want: true},
		{name: "read", toolName: "read", readOnly: true},
		{name: "job wait", toolName: "wait", readOnly: true},
		{name: "job list", toolName: "list_jobs", readOnly: true},
		{name: "job output", toolName: "bash_output", readOnly: true},
		{name: "report staging has its own lease", toolName: pendingworkapp.ReportStageToolName},
		{name: "host override performs no provider side effect", toolName: "write_file", override: map[string]any{"code": "blocked"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pending := runtimePendingToolCall{ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: test.toolName, ReadOnly: test.readOnly}}
			if got := apploop.RequiresSideEffectIntent(pending, test.override); got != test.want {
				t.Fatalf("requiresRuntimeSideEffectIntent() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRuntimeRunSkillSideEffectClassificationUsesResolvedRunMode(t *testing.T) {
	handler := &runtimeServerHandler{skills: runtimeSkillCatalog{
		Enabled: true,
		Skills: []map[string]any{
			{"id": "inline-review", "name": "Inline Review", "runAs": "inline"},
			{"id": "child-review", "name": "Child Review", "runAs": "subagent"},
		},
	}}
	pending := runtimePendingToolCall{
		Call:           provider.ToolCall{Name: "run_skill"},
		ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: "run_skill"},
	}

	pending.Call.Arguments = json.RawMessage(`{"name":"inline-review","arguments":"inspect"}`)
	if !handler.runtimeRunSkillIsInline(pending) {
		t.Fatal("resolved inline skill was incorrectly classified as a child side effect")
	}
	pending.Call.Arguments = json.RawMessage(`{"name":"child-review","arguments":"inspect"}`)
	if handler.runtimeRunSkillIsInline(pending) {
		t.Fatal("resolved subagent skill bypassed side-effect intent admission")
	}
	pending.Call.Arguments = json.RawMessage(`{"name":"missing-review","arguments":"inspect"}`)
	if handler.runtimeRunSkillIsInline(pending) {
		t.Fatal("unknown skill bypassed fail-closed side-effect intent admission")
	}
}

func TestProviderToolEffectGatewayReauthorizesBeforeOverride(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: "deepseek-key",
		EndpointFormat: "chat_completions", Model: "deepseek-v4-pro",
	}).(*runtimeServerHandler)
	pending := prepareAuthorizedVisionBridgePending(
		t, handler, provider.TurnConfig{SupportsImageInput: false, Model: "deepseek-v4-pro"},
	)

	// The grant and durable registry still carry the original complete tool
	// scope. A caller-substituted scope must be rejected before even a host
	// override can be returned, because overrides share the effect boundary.
	pending.ToolScope = []string{pending.Call.Name}
	sentinel := map[string]any{"forbiddenEffect": "executed"}
	output, isError := handler.executeRuntimeToolWithEffectAuthority(context.Background(), pending, sentinel)
	if !isError {
		t.Fatalf("stale provider tool authority reached the dispatcher override: %#v", output)
	}
	record, ok := output.(map[string]any)
	if !ok || record["code"] != "execution_grant_schema_unavailable" {
		t.Fatalf("stale provider tool authority did not return the closed grant rejection: %#v", output)
	}
	body, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "forbiddenEffect") || strings.Contains(string(body), "executed") {
		t.Fatalf("dispatcher override escaped after current grant rejection: %s", body)
	}
}
