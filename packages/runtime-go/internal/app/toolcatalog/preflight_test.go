package toolcatalog

import (
	"encoding/json"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestPreflightToolFiltersSubagentAndJobToolsInsideSubagents(t *testing.T) {
	for _, toolName := range []string{
		"delegate_task", "task", "parallel_tasks", "run_skill",
		"wait", "bash_output", "kill_shell", "restart_job", "list_jobs",
		"get_goal", "create_goal", "complete_step", "update_goal",
		"todo_list", "todo_write", "todo_ops", "todo_patch", "create_plan", "user_input", "request_user_input",
	} {
		decision := PreflightTool(ToolPreflightInput{ToolName: toolName, SubagentDepth: 1})
		if !decision.Blocked || !decision.IsError {
			t.Fatalf("%s should be blocked inside subagents: %#v", toolName, decision)
		}
		if got := decision.Output["code"]; got != "subagent_tool_filtered" {
			t.Fatalf("%s code = %v", toolName, got)
		}
	}
}

func TestPreflightToolBlocksApprovalNeverPolicy(t *testing.T) {
	decision := PreflightTool(ToolPreflightInput{
		ToolName:               "write_file",
		ApprovalPolicy:         "never",
		BlockedByApprovalNever: true,
	})
	if !decision.Blocked || !decision.IsError {
		t.Fatalf("approval-never mutation should be blocked: %#v", decision)
	}
	if got := decision.Output["code"]; got != "approval_policy_blocked" {
		t.Fatalf("code = %v", got)
	}
}

func TestPreflightToolBlocksBashBeforeApprovalOutsideDangerFullAccess(t *testing.T) {
	for _, sandboxMode := range []string{"", "read-only", "workspace-write", "external-sandbox"} {
		decision := PreflightTool(ToolPreflightInput{
			ToolName:       "bash",
			ApprovalPolicy: "on-request",
			SandboxMode:    sandboxMode,
		})
		if !decision.Blocked || !decision.IsError || decision.Output["code"] != "sandbox_blocked" {
			t.Fatalf("bash should be blocked for sandbox %q: %#v", sandboxMode, decision)
		}
	}
	decision := PreflightTool(ToolPreflightInput{
		ToolName:       "bash",
		ApprovalPolicy: "on-request",
		SandboxMode:    "danger-full-access",
	})
	if decision.Blocked {
		t.Fatalf("danger-full-access should pass shell sandbox preflight: %#v", decision)
	}
}

func TestPreflightToolBlocksCaseFundReportUnderApprovalNever(t *testing.T) {
	decision := PreflightTool(ToolPreflightInput{
		ToolName:               "mcp__analytix_funds__run_full_case_analysis",
		ApprovalPolicy:         "never",
		BlockedByApprovalNever: true,
		CaseBound:              true,
	})
	if !decision.Blocked || !decision.IsError || decision.Output["code"] != "publication_receipt_required" {
		t.Fatalf("case fund report must require host publication authority before approval policy: %#v", decision)
	}
}

func TestPreflightToolBlocksOnlyCasePublicationEffectsBeforeApproval(t *testing.T) {
	for _, toolName := range []string{
		"mcp__spoofed__run_full_case_analysis", "mcp__spoofed__create_case_notebook", "mcp__spoofed__export_cleaned_case_data",
	} {
		decision := PreflightTool(ToolPreflightInput{
			ToolName: toolName, CaseBound: true, ApprovalPolicy: "auto", SandboxMode: "danger-full-access",
		})
		if !decision.Blocked || !decision.IsError || decision.Output["code"] != "publication_receipt_required" || decision.Output["executed"] != false {
			t.Fatalf("case-bound artifact tool %s escaped host publication preflight: %#v", toolName, decision)
		}
	}
	for _, toolName := range []string{
		"read", "grep", "write", "write_file", "edit", "bash", "run_skill", "task", "parallel_tasks",
		ReportDeliveryToolName, "mcp__analytix_funds__count_case_rows", "mcp__docs__write", "wait", "bash_output", "kill_shell", "list_jobs",
	} {
		decision := PreflightTool(ToolPreflightInput{
			ToolName: toolName, CaseBound: true,
			HostReadOnly:   toolName == "mcp__analytix_funds__count_case_rows",
			ApprovalPolicy: "auto", SandboxMode: "danger-full-access",
		})
		if decision.Blocked {
			t.Fatalf("case context replaced an ordinary tool %s: %#v", toolName, decision)
		}
	}
}

func TestPreflightToolBlocksCaseDataWritesButKeepsOrdinaryMCPWrites(t *testing.T) {
	for _, approvalPolicy := range []string{"auto", "on-request", "never"} {
		decision := PreflightTool(ToolPreflightInput{
			ToolName: "mcp__analytix_funds__generate_bundle", CaseBound: true, HostReadOnly: false,
			ApprovalPolicy: approvalPolicy,
		})
		if !decision.Blocked || !decision.IsError || decision.Output["code"] != "publication_receipt_required" || decision.Output["executed"] != false {
			t.Fatalf("case-data MCP write escaped under %s: %#v", approvalPolicy, decision)
		}
	}
	decision := PreflightTool(ToolPreflightInput{
		ToolName: "mcp__server__generate_bundle", CaseBound: true, HostReadOnly: false, ApprovalPolicy: "auto",
	})
	if decision.Blocked {
		t.Fatalf("ordinary MCP write was mistaken for case publication: %#v", decision)
	}
}

func TestCaseArtifactCallDoesNotTreatOrdinarySubagentsAsPublication(t *testing.T) {
	for _, call := range []domainmodel.ToolCall{
		{ID: "read-only-default", Name: "task", Arguments: json.RawMessage(`{"prompt":"inspect"}`)},
		{ID: "read-only-explicit", Name: "delegate_task", Arguments: json.RawMessage(`{"prompt":"inspect","toolPolicy":"readOnly"}`)},
		{ID: "parallel-read-only", Name: "parallel_tasks", Arguments: json.RawMessage(`{"tasks":[{"prompt":"a"},{"prompt":"b","tool_policy":"readOnly"}]}`)},
		{ID: "inherit", Name: "task", Arguments: json.RawMessage(`{"prompt":"write","toolPolicy":"inherit"}`)},
		{ID: "parallel-inherit", Name: "parallel_tasks", Arguments: json.RawMessage(`{"tasks":[{"prompt":"a"},{"prompt":"write","tool_policy":"inherit"}]}`)},
		{ID: "invalid", Name: "task", Arguments: json.RawMessage(`{`)},
	} {
		if CaseArtifactCallRequiresAuthority(call) {
			t.Fatalf("ordinary subagent was mistaken for a publication effect: %#v", call)
		}
	}
	if CaseArtifactCallRequiresAuthority(domainmodel.ToolCall{Name: ReportDeliveryToolName}) {
		t.Fatal("typed host report delivery was mistaken for a legacy artifact MCP")
	}
	if !CaseArtifactCallRequiresAuthority(domainmodel.ToolCall{Name: "mcp__spoofed__run_full_case_analysis"}) {
		t.Fatal("legacy artifact MCP lost its host quarantine")
	}
}

func TestPreflightToolRequiresReadGuardsForFreshReadMutations(t *testing.T) {
	tests := map[string]string{
		"edit":          ReadGuardEdit,
		"edit_file":     ReadGuardEdit,
		"multi_edit":    ReadGuardEdit,
		"delete_range":  ReadGuardDeleteRange,
		"delete_symbol": ReadGuardDeleteSymbol,
	}
	for toolName, want := range tests {
		decision := PreflightTool(ToolPreflightInput{ToolName: toolName})
		if decision.Blocked || !decision.RequiresReadGuard || decision.ReadGuard != want {
			t.Fatalf("%s read guard decision = %#v, want guard %q", toolName, decision, want)
		}
	}
}

func TestPreflightToolAllowsSafeTools(t *testing.T) {
	decision := PreflightTool(ToolPreflightInput{ToolName: "read", ApprovalPolicy: "never"})
	if decision.Blocked || decision.RequiresReadGuard {
		t.Fatalf("read should not be blocked or read-guarded: %#v", decision)
	}
}

func TestHostGeneralOnlyForegroundTaskRequiresExactExplicitBudgets(t *testing.T) {
	valid := domainmodel.ToolCall{Name: ForegroundTaskToolName, Arguments: json.RawMessage(`{"prompt":"summarize the bounded question","max_steps":2,"token_budget":512,"time_budget_ms":30000}`)}
	if !HostGeneralOnlyForegroundTaskCall(valid) {
		t.Fatalf("exact foreground task was not admitted: %#v", valid)
	}
	invalid := []json.RawMessage{
		json.RawMessage(`{"prompt":"x","max_steps":2,"token_budget":512}`),
		json.RawMessage(`{"prompt":"x","max_steps":5,"token_budget":512,"time_budget_ms":30000}`),
		json.RawMessage(`{"prompt":"x","max_steps":2,"token_budget":4097,"time_budget_ms":30000}`),
		json.RawMessage(`{"prompt":"x","max_steps":2,"token_budget":512,"time_budget_ms":60001}`),
		json.RawMessage(`{"prompt":"x","max_steps":2,"token_budget":512,"time_budget_ms":30000,"run_in_background":true}`),
		json.RawMessage(`{"prompt":"x","max_steps":2.5,"token_budget":512,"time_budget_ms":30000}`),
	}
	for _, arguments := range invalid {
		call := domainmodel.ToolCall{Name: ForegroundTaskToolName, Arguments: arguments}
		if HostGeneralOnlyForegroundTaskCall(call) {
			t.Fatalf("foreground task authority widened for %s", arguments)
		}
	}
}
