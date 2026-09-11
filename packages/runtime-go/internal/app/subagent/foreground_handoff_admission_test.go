package subagent

import (
	"encoding/json"
	"testing"
	"time"

	modelapp "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestForegroundHandoffAdmissionBindsOnlyExplicitBoundedRequest(t *testing.T) {
	workspace := t.TempDir()
	parent, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "parent-thread", TurnID: "parent-turn", WorkspaceRealPath: workspace,
		ContextEpoch: 3, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BindForegroundHandoffRequest(parent, TaskRequest{
		Prompt:   "inspect one bounded question",
		MaxSteps: 2, MaxStepsSet: true,
		TokenBudget: 512, TokenBudgetSet: true,
		TimeBudgetMS: 30000, TimeBudgetMSSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !request.ForegroundHandoff || request.ToolPolicy != "readOnly" || len(request.Tools) != 1 || request.Tools[0] != toolcatalogForegroundSubmitToolName {
		t.Fatalf("foreground request was not closed by the host: %#v", request)
	}
	parentExecution := ParentExecution{ProviderID: "provider", Model: "model", Workspace: workspace}
	if err := ValidateForegroundHandoffExecution(request, ExecutionResult{
		ProviderID: "provider", Model: "model", Workspace: workspace,
	}, parentExecution); err != nil {
		t.Fatalf("matching execution was rejected: %v", err)
	}
	for name, mutate := range map[string]func(*TaskRequest){
		"background":           func(value *TaskRequest) { value.RunInBackground = true },
		"profile":              func(value *TaskRequest) { value.ProfileName = "reviewer" },
		"workspace":            func(value *TaskRequest) { value.Workspace = workspace },
		"provider":             func(value *TaskRequest) { value.ProviderID = "other" },
		"replay":               func(value *TaskRequest) { value.ContinueFrom = "old-run" },
		"nested tool":          func(value *TaskRequest) { value.Tools = []string{"submit_child_result", "task"} },
		"missing token budget": func(value *TaskRequest) { value.TokenBudgetSet = false },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if err := ValidateForegroundHandoffRequest(candidate); err == nil {
				t.Fatalf("%s authority widening was accepted: %#v", name, candidate)
			}
		})
	}
	for name, execution := range map[string]ExecutionResult{
		"cross workspace":   {ProviderID: "provider", Model: "model", Workspace: workspace + "/other"},
		"cross provider":    {ProviderID: "other", Model: "model", Workspace: workspace},
		"cross model":       {ProviderID: "provider", Model: "other", Workspace: workspace},
		"explicit override": {ProviderID: "provider", Model: "model", Workspace: workspace, ExplicitExecutionOverride: true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateForegroundHandoffExecution(request, execution, parentExecution); err == nil {
				t.Fatalf("%s execution widening was accepted", name)
			}
		})
	}
}

func TestForegroundHandoffPendingAdmissionOwnsTheHostGeneralOnlyBoundary(t *testing.T) {
	parent, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "parent-thread", TurnID: "parent-turn", WorkspaceRealPath: t.TempDir(),
		ContextEpoch: 3, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{
		Prompt: "inspect one bounded question", MaxSteps: 2, MaxStepsSet: true,
		TokenBudget: 512, TokenBudgetSet: true, TimeBudgetMS: 30000, TimeBudgetMSSet: true,
	}
	pending := modelapp.PendingToolCall{
		SecurityContext: parent,
		Call: domainmodel.ToolCall{
			ID: "call-foreground", Name: "task",
			Arguments: json.RawMessage(`{"prompt":"inspect one bounded question","max_steps":2,"token_budget":512,"time_budget_ms":30000}`),
		},
	}
	bound, err := BindForegroundHandoffPendingRequest(pending, request)
	if err != nil {
		t.Fatal(err)
	}
	if !bound.ForegroundHandoff || bound.ToolPolicy != "readOnly" || len(bound.Tools) != 1 ||
		bound.Tools[0] != toolcatalogForegroundSubmitToolName {
		t.Fatalf("pending foreground request was not host-bound: %#v", bound)
	}
	profileRequest := TaskRequest{
		Prompt: "review the repository", ProfileName: "milestone-a-readonly",
		ToolPolicy: "readOnly", ToolPolicySet: true,
	}
	pending.Call.Arguments = json.RawMessage(`{"prompt":"review the repository","profile":"milestone-a-readonly","toolPolicy":"readOnly"}`)
	ordinary, err := BindForegroundHandoffPendingRequest(pending, profileRequest)
	if err != nil || ordinary.ForegroundHandoff || ordinary.ProfileName != profileRequest.ProfileName ||
		ordinary.ToolPolicy != profileRequest.ToolPolicy {
		t.Fatalf("profile-bound ordinary child request was not preserved: request=%#v err=%v", ordinary, err)
	}
	pending.SubagentDepth = 1
	ordinary, err = BindForegroundHandoffPendingRequest(pending, request)
	if err != nil || ordinary.ForegroundHandoff {
		t.Fatalf("non-matching child request should retain ordinary host policy: request=%#v err=%v", ordinary, err)
	}
}

func TestForegroundHandoffAdmissionRequiresHostGeneralOnlyParent(t *testing.T) {
	parent := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "audit-parent", TurnID: "audit-turn", WorkspaceRealPath: t.TempDir(),
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	_, err := BindForegroundHandoffRequest(parent, TaskRequest{
		Prompt: "x", MaxSteps: 1, MaxStepsSet: true,
		TokenBudget: 1, TokenBudgetSet: true, TimeBudgetMS: 1, TimeBudgetMSSet: true,
	})
	if err == nil {
		t.Fatal("non-general-only parent acquired foreground child authority")
	}
}
