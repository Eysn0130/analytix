package subagent

import (
	"errors"
	"strings"
	"testing"
)

func TestTaskRequestFromArgsNormalizesExecutionAndPolicy(t *testing.T) {
	request, err := TaskRequestFromArgs("task", map[string]any{
		"prompt":               "# Inspect\n\nRead the runtime split",
		"name":                 "Reviewer",
		"provider_id":          "moonshot-cn",
		"model":                "kimi-k2",
		"endpoint_format":      "v1/messages",
		"model_variant":        "fast",
		"effort":               "high",
		"profile":              "inherit",
		"tools":                []any{"read", "grep"},
		"blockedTools":         []any{"bash"},
		"blockedMcpServers":    []any{"github"},
		"blockedSkills":        []any{"imagegen"},
		"max_steps":            4.0,
		"tokenBudget":          2048.0,
		"timeBudgetMs":         6000.0,
		"returnFormat":         "transcript_ref",
		"run_in_background":    true,
		"auto_continue_parent": true,
		"task_id":              "child-1",
	})
	if err != nil {
		t.Fatalf("task request should parse: %v", err)
	}
	if request.Label != "Reviewer" || !request.LabelExplicit || !request.NameExplicit {
		t.Fatalf("explicit label/name not preserved: %#v", request)
	}
	if request.ProviderID != "moonshot-cn" || request.Model != "kimi-k2" || request.EndpointFormat != "messages" || request.Variant != "fast" {
		t.Fatalf("execution ref fields mismatch: %#v", request)
	}
	if !request.ProviderExplicit || !request.ModelExplicit || !request.EndpointExplicit || !request.VariantExplicit {
		t.Fatalf("explicit execution flags mismatch: %#v", request)
	}
	if request.Effort != "high" || request.ToolPolicy != "inherit" || !request.ToolPolicySet {
		t.Fatalf("effort/tool policy mismatch: %#v", request)
	}
	if request.ToolPolicySource != "request" {
		t.Fatalf("explicit tool policy source mismatch: %#v", request)
	}
	if request.MaxSteps != 4 || !request.MaxStepsSet || request.TokenBudget != 2048 || !request.TokenBudgetSet ||
		request.TimeBudgetMS != 6000 || !request.TimeBudgetMSSet || request.ReturnFormat != "transcriptRef" ||
		!request.RunInBackground || !request.AutoContinueParent || request.ContinueFrom != "child-1" {
		t.Fatalf("limits/background/source mismatch: %#v", request)
	}
	if len(request.Tools) != 2 || request.Tools[0] != "read" || request.Tools[1] != "grep" {
		t.Fatalf("tool scope mismatch: %#v", request.Tools)
	}
	if len(request.BlockedTools) != 1 || request.BlockedTools[0] != "bash" ||
		len(request.BlockedMCPServers) != 1 || request.BlockedMCPServers[0] != "github" ||
		len(request.BlockedSkills) != 1 || request.BlockedSkills[0] != "imagegen" {
		t.Fatalf("blocklists mismatch: %#v", request)
	}
}

func TestWithSystemPromptHintOwnsDeterministicPromptComposition(t *testing.T) {
	request := WithSystemPromptHint(TaskRequest{SystemPrompt: "base"}, "  local filesystem hint  ")
	if request.SystemPrompt != "base\nlocal filesystem hint" {
		t.Fatalf("system prompt hint composition drifted: %q", request.SystemPrompt)
	}
	unchanged := WithSystemPromptHint(request, " \n ")
	if unchanged.SystemPrompt != request.SystemPrompt {
		t.Fatalf("blank system prompt hint changed the request: %#v", unchanged)
	}
}

func TestTaskRequestFromArgsRejectsInvalidReasoningEffortExactly(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []any{" high ", "HIGH", "auto", sentinel, float64(1)} {
		request, err := TaskRequestFromArgs("task", map[string]any{"prompt": "Inspect", "effort": effort})
		if !errors.Is(err, ErrReasoningEffortInvalid) || request.Prompt != "" || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("invalid effort did not fail closed: effort=%#v request=%#v err=%v", effort, request, err)
		}
	}
	for _, effort := range []string{"off", "low", "medium", "high", "max"} {
		request, err := TaskRequestFromArgs("task", map[string]any{"prompt": "Inspect", "effort": effort})
		if err != nil || request.Effort != effort {
			t.Fatalf("valid explicit effort %q failed: request=%#v err=%v", effort, request, err)
		}
	}
}

func TestParallelTaskRequestsValidateDependencies(t *testing.T) {
	valid, err := ParallelTaskRequestsFromArgs(map[string]any{"tasks": []any{
		map[string]any{"id": "first", "prompt": "First"},
		map[string]any{"id": "second", "prompt": "Second", "depends_on": []any{"first"}},
	}})
	if err != nil || len(valid) != 2 || valid[1].Request.DependsOn[0] != "first" {
		t.Fatalf("valid DAG rejected: tasks=%#v err=%v", valid, err)
	}
	for label, args := range map[string]map[string]any{
		"duplicate": {"tasks": []any{
			map[string]any{"id": "same", "prompt": "First"},
			map[string]any{"id": "same", "prompt": "Second"},
		}},
		"unknown": {"tasks": []any{
			map[string]any{"id": "first", "prompt": "First", "depends_on": []any{"missing"}},
			map[string]any{"id": "second", "prompt": "Second"},
		}},
		"cycle": {"tasks": []any{
			map[string]any{"id": "first", "prompt": "First", "depends_on": []any{"second"}},
			map[string]any{"id": "second", "prompt": "Second", "depends_on": []any{"first"}},
		}},
		"single": {"tasks": []any{
			map[string]any{"id": "only", "prompt": "Only"},
		}},
	} {
		if _, err := ParallelTaskRequestsFromArgs(args); err == nil {
			t.Fatalf("expected %s DAG to be rejected", label)
		}
	}
}

func TestTaskRequestFromArgsAcceptsCamelCaseLimits(t *testing.T) {
	request, err := TaskRequestFromArgs("task", map[string]any{
		"prompt":             "Read the workspace and summarize package scripts.",
		"toolPolicy":         "readOnly",
		"maxSteps":           9.0,
		"runInBackground":    true,
		"autoContinueParent": true,
		"continueFrom":       "job-1",
		"forkFrom":           "job-0",
	})
	if err != nil {
		t.Fatalf("camelCase task request should parse: %v", err)
	}
	if request.ToolPolicy != "readOnly" || !request.ToolPolicySet {
		t.Fatalf("camelCase toolPolicy mismatch: %#v", request)
	}
	if request.MaxSteps != 9 || !request.MaxStepsSet || !request.RunInBackground || !request.AutoContinueParent ||
		request.ContinueFrom != "job-1" || request.ForkFrom != "job-0" {
		t.Fatalf("camelCase fields mismatch: %#v", request)
	}
}

func TestParallelDependencyContextAndSummary(t *testing.T) {
	context := ParallelDependencyContext([]string{"first", "missing"}, map[string]string{"first": strings.Repeat("x", 3000)})
	if !strings.HasPrefix(context, "- first: ") || !strings.Contains(context, "[truncated]") {
		t.Fatalf("dependency context should include bounded summary: %q", context)
	}
	if summary := DependencySummary(map[string]any{"error": "failed"}); summary != "error: failed" {
		t.Fatalf("error summary mismatch: %q", summary)
	}
	if !ParallelDependenciesComplete([]string{"first"}, map[string]bool{"first": true}) {
		t.Fatalf("dependencies should be complete")
	}
}

func TestDisplayNamesAndGeneratedNicknames(t *testing.T) {
	if got := PromptLabel("## 1. Inspect runtime files\nthen report"); got != "Inspect runtime files" {
		t.Fatalf("prompt label mismatch: %q", got)
	}
	if got := DisplayNameCandidate("Child agent: Reviewer fork"); got != "Reviewer" {
		t.Fatalf("display candidate mismatch: %q", got)
	}
	if got := DisplayNameCandidate("please inspect the full runtime tree now"); got != "" {
		t.Fatalf("prompt-like display name should be rejected: %q", got)
	}
	if got := GeneratedNickname(ParallelIndexSeed(2)); got != "Nash" {
		t.Fatalf("parallel nickname mismatch: %q", got)
	}
	if NormalizeToolPolicy("read_only") != "readOnly" || NormalizeToolPolicy("write") != "" {
		t.Fatalf("tool policy normalization mismatch")
	}
	if NormalizeIsolationMode("worktree") != "worktree" || NormalizeIsolationMode("container") != "" {
		t.Fatalf("isolation mode normalization mismatch")
	}
}

func TestValidateWorktreeIsolationRequestRequiresDurableMutationAuthority(t *testing.T) {
	for label, request := range map[string]TaskRequest{
		"previouslyValidRequest": {Prompt: "edit safely", RunInBackground: true, IsolationMode: "worktree", ToolPolicy: "inherit", ToolPolicySource: "request"},
		"previouslyValidProfile": {Prompt: "edit safely", RunInBackground: true, IsolationMode: "worktree", ToolPolicy: "inherit", ToolPolicySource: "profile"},
		"readOnly":               {Prompt: "inspect", RunInBackground: true, IsolationMode: "worktree", ToolPolicy: "readOnly", ToolPolicySource: "request"},
		"foreground":             {Prompt: "edit", IsolationMode: "worktree", ToolPolicy: "inherit", ToolPolicySource: "request"},
		"defaultPolicy":          {Prompt: "edit", RunInBackground: true, IsolationMode: "worktree", ToolPolicy: "inherit", ToolPolicySource: "default"},
		"continue":               {Prompt: "edit", RunInBackground: true, IsolationMode: "worktree", ToolPolicy: "inherit", ToolPolicySource: "request", ContinueFrom: "job-1"},
	} {
		err := ValidateWorktreeIsolationRequest(request)
		if !errors.Is(err, ErrWorktreeIsolationAuthorityRequired) {
			t.Fatalf("%s worktree isolation should require durable mutation authority: %v", label, err)
		}
		if err.Error() != ErrWorktreeIsolationAuthorityRequired.Error() {
			t.Fatalf("%s worktree isolation returned a non-deterministic boundary: %q", label, err)
		}
	}
	if err := ValidateWorktreeIsolationRequest(TaskRequest{}); err != nil {
		t.Fatalf("request without isolation should remain valid: %v", err)
	}
	if err := ValidateWorktreeIsolationRequest(TaskRequest{IsolationMode: "container"}); err == nil || errors.Is(err, ErrWorktreeIsolationAuthorityRequired) {
		t.Fatalf("unknown isolation mode should retain its distinct validation error: %v", err)
	}
}
