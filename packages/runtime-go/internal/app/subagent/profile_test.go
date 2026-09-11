package subagent

import (
	"errors"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestApplyProfileFillsExecutionToolScopeAndPreamble(t *testing.T) {
	request, err := ApplyProfile(TaskRequest{Prompt: "Review this", ProfileName: "reviewer"}, ProfileSettings{
		DefaultToolPolicy: "readOnly",
		Profiles: map[string]ProfileConfig{
			"reviewer": {
				ProviderID:     "anthropic-main",
				Model:          "claude-sonnet",
				EndpointFormat: "messages",
				Variant:        "fast",
				Effort:         "high",
				PromptPreamble: "Stay read-only.",
				ToolPolicy:     "inherit",
				Tools:          []string{"read", "grep"},
			},
		},
	})
	if err != nil {
		t.Fatalf("profile should apply: %v", err)
	}
	if request.ProviderID != "anthropic-main" || request.Model != "claude-sonnet" || request.EndpointFormat != "messages" || request.Variant != "fast" {
		t.Fatalf("profile execution fields mismatch: %#v", request)
	}
	if !request.ProfileExecutionConfigured || request.Effort != "high" || request.ToolPolicy != "inherit" || !request.ToolPolicySet || request.ToolPolicySource != "profile" {
		t.Fatalf("profile effort/tool policy mismatch: %#v", request)
	}
	if len(request.Tools) != 2 || request.Tools[0] != "read" || !strings.HasPrefix(request.Prompt, "Stay read-only.\n\n") {
		t.Fatalf("profile tools/preamble mismatch: %#v", request)
	}
}

func TestApplyProfilePreservesExplicitOverridesAndDefaultsPolicy(t *testing.T) {
	request, err := ApplyProfile(TaskRequest{
		Prompt:        "Inspect",
		ProviderID:    "openai-main",
		Model:         "gpt-explicit",
		ToolPolicy:    "",
		ToolPolicySet: false,
	}, ProfileSettings{
		DefaultProfile:    "reviewer",
		DefaultToolPolicy: "readOnly",
		Profiles: map[string]ProfileConfig{
			"reviewer": {ProviderID: "anthropic-main", Model: "claude-sonnet", ToolPolicy: "inherit"},
		},
	})
	if err != nil {
		t.Fatalf("profile should apply: %v", err)
	}
	if request.ProviderID != "openai-main" || request.Model != "gpt-explicit" || request.ProfileExecutionConfigured {
		t.Fatalf("explicit execution should win: %#v", request)
	}
	if request.ProfileName != "reviewer" || request.ToolPolicy != "inherit" || request.ToolPolicySource != "profile" {
		t.Fatalf("default profile policy should apply: %#v", request)
	}
	if _, err := ApplyProfile(TaskRequest{Prompt: "Inspect", ProfileName: "missing"}, ProfileSettings{}); err == nil {
		t.Fatalf("unknown profile should fail")
	}
}

func TestApplyProfileModeVisibilityAndReadOnlyDefaults(t *testing.T) {
	settings := ProfileSettings{
		DefaultToolPolicy: "readOnly",
		Profiles: map[string]ProfileConfig{
			"primary": {Mode: "primary", ToolPolicy: "inherit"},
			"invalid": {Mode: "PRIMARY", ToolPolicy: "inherit"},
			"hidden":  {Mode: "subagent", Hidden: true, Description: "internal workflow"},
			"general": {Mode: "all", Description: "general reviewer"},
			"worker":  {Mode: "subagent", Description: "worker reviewer"},
		},
	}
	description := ProfileDescription(settings)
	if strings.Contains(description, "primary") || strings.Contains(description, "hidden") {
		t.Fatalf("primary and hidden profiles must not be advertised: %q", description)
	}
	if !strings.Contains(description, "general") || !strings.Contains(description, "worker") {
		t.Fatalf("all and subagent profiles should be advertised: %q", description)
	}
	if !ProfileVisibleForDelegation(settings.Profiles["general"]) || !ProfileVisibleForDelegation(settings.Profiles["worker"]) ||
		ProfileVisibleForDelegation(settings.Profiles["primary"]) || ProfileVisibleForDelegation(settings.Profiles["hidden"]) {
		t.Fatalf("profile visibility mismatch")
	}
	request, err := ApplyProfile(TaskRequest{Prompt: "Inspect files.", ProfileName: "worker"}, settings)
	if err != nil {
		t.Fatalf("worker profile should apply: %v", err)
	}
	if request.ToolPolicy != "readOnly" || request.ProfileMode != "subagent" || request.ProfileDescription != "worker reviewer" {
		t.Fatalf("subagent profile metadata/default policy mismatch: %#v", request)
	}
	if _, err := ApplyProfile(TaskRequest{Prompt: "Inspect files.", ProfileName: "primary"}, settings); err == nil ||
		!strings.Contains(err.Error(), "cannot be delegated") {
		t.Fatalf("primary profile should reject delegation, got %v", err)
	}
	if _, err := ApplyProfile(TaskRequest{Prompt: "Inspect files.", ProfileName: "invalid"}, settings); err == nil ||
		!strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("invalid profile mode should reject delegation, got %v", err)
	}
}

func TestApplyProfileRejectsBuiltinProfileDrift(t *testing.T) {
	_, err := ApplyProfile(TaskRequest{
		Prompt:      "Scan the Desktop and summarize suspicious files.",
		ProfileName: "design-reviewer",
	}, DefaultProfileSettings())
	if err == nil || !strings.Contains(err.Error(), "only for frontend/design review tasks") {
		t.Fatalf("design-reviewer should reject non-design file analysis, got %v", err)
	}
	request, err := ApplyProfile(TaskRequest{
		Prompt:      "Review the React UI spacing and accessibility in this screen.",
		ProfileName: "design-reviewer",
	}, DefaultProfileSettings())
	if err != nil {
		t.Fatalf("design-reviewer should accept UI review tasks: %v", err)
	}
	if !strings.HasPrefix(request.Prompt, "Review frontend code and prototypes") {
		t.Fatalf("design-reviewer preamble should apply to matching tasks: %#v", request)
	}
}

func TestApplyProfileRejectsReadOnlyTasksThatRequireWritesOrShell(t *testing.T) {
	_, err := ApplyProfile(TaskRequest{
		Prompt: "Inspect the directory and write file with the final findings.",
	}, ProfileSettings{DefaultToolPolicy: "readOnly"})
	if err == nil || !strings.Contains(err.Error(), "requires file write/edit capability") {
		t.Fatalf("read-only write task should fail preflight, got %v", err)
	}
	_, err = ApplyProfile(TaskRequest{
		Prompt: "Inspect the directory and run python to read metadata.",
	}, ProfileSettings{DefaultToolPolicy: "readOnly"})
	if err == nil || !strings.Contains(err.Error(), "requires shell/command capability") {
		t.Fatalf("read-only shell task should fail preflight, got %v", err)
	}
	if _, err = ApplyProfile(TaskRequest{
		Prompt: "只读取 package.json 并报告 name；不要修改文件。",
	}, ProfileSettings{DefaultToolPolicy: "readOnly"}); err != nil {
		t.Fatalf("read-only prompt with explicit no-modify instruction should pass: %v", err)
	}
	if _, err = ApplyProfile(TaskRequest{
		Prompt:        "Inspect the directory and write file with the final findings.",
		ToolPolicy:    "inherit",
		ToolPolicySet: true,
	}, ProfileSettings{DefaultToolPolicy: "readOnly"}); err != nil {
		t.Fatalf("inherit policy should allow mutable task: %v", err)
	}
}

func TestApplyProfileAcceptsExplicitlyProhibitedCommandReview(t *testing.T) {
	request, err := ApplyProfile(TaskRequest{
		Prompt:          "Review the named source bytes against the required test command without executing it. Do not mutate state. Do not run commands. Do not delegate or inspect any other path.",
		ProfileName:     "milestone-a-readonly",
		ToolPolicy:      "readOnly",
		ToolPolicySet:   true,
		RunInBackground: false,
	}, ProfileSettings{
		DefaultToolPolicy: "readOnly",
		Profiles: map[string]ProfileConfig{
			"milestone-a-readonly": {ToolPolicy: "readOnly", Tools: []string{"read"}},
		},
	})
	if err != nil {
		t.Fatalf("explicit command prohibition must remain a read-only review: %v", err)
	}
	if request.ToolPolicy != "readOnly" || request.RunInBackground || request.ProfileName != "milestone-a-readonly" {
		t.Fatalf("read-only review binding mismatch: %#v", request)
	}
}

func TestValidateSourceRejectsIdentityDriftAndAcceptsMatchingSource(t *testing.T) {
	source := domainjob.Record{
		ID:               "child-1",
		Kind:             "subagent",
		Status:           "completed",
		ParentThreadID:   "parent",
		ChildThreadID:    "child-thread",
		Workspace:        "/work",
		ProviderID:       "openai-main",
		Model:            "gpt-5",
		EndpointFormat:   "chat",
		Variant:          "fast",
		Effort:           "medium",
		ToolPolicy:       "readOnly",
		ToolScope:        []string{"grep", "read"},
		ToolSchemaHash:   "tools",
		SystemPromptHash: "persona",
	}
	spec := SourceValidation{
		ParentThreadID:   "parent",
		Workspace:        "/work",
		ProviderID:       "openai-main",
		Model:            "gpt-5",
		EndpointFormat:   "chat",
		Variant:          "fast",
		Effort:           "medium",
		ToolScope:        []string{"grep", "read"},
		ToolSchemaHash:   "tools",
		SystemPromptHash: "persona",
	}
	if err := ValidateSource(source, TaskRequest{ToolPolicy: "readOnly"}, spec); err != nil {
		t.Fatalf("matching source should validate: %v", err)
	}
	spec.ProviderID = "anthropic-main"
	if err := ValidateSource(source, TaskRequest{ToolPolicy: "readOnly"}, spec); err == nil || !strings.Contains(err.Error(), "uses provider") {
		t.Fatalf("provider drift should fail, got %v", err)
	}
	spec.ProviderID = source.ProviderID
	spec.ToolScope = []string{"read"}
	if err := ValidateSource(source, TaskRequest{ToolPolicy: "readOnly"}, spec); !errors.Is(err, ErrSourceToolScopeMismatch) {
		t.Fatalf("tool-scope drift should return its typed host error, got %v", err)
	}
}

func TestValidateSourceRejectsNonReplayableLifecycleStates(t *testing.T) {
	source := domainjob.Record{
		ID:             "child-1",
		Kind:           "subagent",
		ParentThreadID: "parent",
		ChildThreadID:  "child-thread",
		Workspace:      "/work",
		Status:         "completed",
	}
	spec := SourceValidation{ParentThreadID: "parent", Workspace: "/work"}
	if err := ValidateSource(source, TaskRequest{}, spec); err != nil {
		t.Fatalf("completed source should remain replayable: %v", err)
	}
	for _, status := range []string{"running", "queued", "failed", "interrupted", "aborted", "killed"} {
		source.Status = status
		err := ValidateSource(source, TaskRequest{}, spec)
		if err == nil || !strings.Contains(err.Error(), "subagent reference") {
			t.Fatalf("status %q should reject continue/fork, got %v", status, err)
		}
		if status != "running" && status != "queued" && !errors.Is(err, ErrSourceNotReplayable) {
			t.Fatalf("terminal status %q should return the typed replay boundary, got %v", status, err)
		}
	}
}

func TestValidateSourceRejectsReasoningEffortPoisonWithoutReflection(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	base := domainjob.Record{
		ID: "child-1", Kind: "subagent", Status: "completed", ParentThreadID: "parent", ChildThreadID: "child-thread",
	}
	for name, values := range map[string][2]string{
		"source poison":  {sentinel, "high"},
		"spec poison":    {"high", sentinel},
		"valid mismatch": {"low", "high"},
	} {
		t.Run(name, func(t *testing.T) {
			source := base
			source.Effort = values[0]
			err := ValidateSource(source, TaskRequest{}, SourceValidation{ParentThreadID: "parent", Effort: values[1]})
			if err == nil || strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), values[0]+`"`) {
				t.Fatalf("reasoning effort authority was accepted or reflected: %v", err)
			}
		})
	}
}

func TestProfileHelpersMirrorRuntimeContracts(t *testing.T) {
	if got := ToolScope([]string{"bash", " read ", "missing"}, []string{"read", "grep", "bash"}); len(got) != 2 || got[0] != "bash" || got[1] != "read" {
		t.Fatalf("tool scope should filter and sort: %#v", got)
	}
	if ApprovalPolicy("inherit", "") != "on-request" || ApprovalPolicy("inherit", "never") != "never" || ApprovalPolicy("readOnly", "on-request") != "never" {
		t.Fatalf("approval policy mismatch")
	}
	if ProfileSource(TaskRequest{ProfileName: "reviewer"}) != "profile:reviewer" || ProfileSource(TaskRequest{}) != "parent-default" {
		t.Fatalf("profile source mismatch")
	}
	if ModelSource(false, false, true) != "session" || ModelSource(false, true, true) != "subagent-profile" || ModelSource(true, false, false) != "explicit-input" {
		t.Fatalf("model source precedence mismatch")
	}
	parent := 20
	if DefaultMaxSteps(nil) != 12 || DefaultMaxSteps(&parent) != 10 || ResolvedMaxSteps(TaskRequest{MaxStepsSet: true, MaxSteps: 3}, &parent) != 3 {
		t.Fatalf("max step helper mismatch")
	}
	parent = 0
	if DefaultMaxSteps(&parent) != 12 || ResolvedMaxSteps(TaskRequest{Prompt: "扫描桌面目录并生成报告"}, &parent) != 20 {
		t.Fatalf("unbounded parent budget should not create unbounded child runs")
	}
	if ResolvedMaxSteps(TaskRequest{Prompt: "扫描桌面目录并生成报告"}, nil) != 20 {
		t.Fatalf("complex child task should get larger default budget")
	}
	parent = 64
	if ResolvedMaxSteps(TaskRequest{Prompt: "扫描桌面目录并生成报告"}, &parent) != 32 {
		t.Fatalf("complex child task should inherit from effective parent budget")
	}
	if len(ReadOnlyTools()) == 0 || !strings.Contains(SystemPrompt, "Analytix sub-agent") {
		t.Fatalf("subagent tool/persona defaults missing")
	}
	readOnly := ReadOnlyToolScope(false)
	for _, name := range readOnly {
		if name == "web_fetch" {
			t.Fatalf("web_fetch should be filtered when disabled: %#v", readOnly)
		}
	}
	inherited := InheritableToolScope(true, []string{"mcp_search"})
	if !containsString(inherited, "web_fetch") || !containsString(inherited, "mcp_search") || !containsString(inherited, "write_file") {
		t.Fatalf("inheritable tool scope mismatch: %#v", inherited)
	}
	if scoped := ToolScopeForPolicy(TaskRequest{ToolPolicy: "readOnly", Tools: []string{"write_file", "grep"}}, true, []string{"mcp_search"}); len(scoped) != 1 || scoped[0] != "grep" {
		t.Fatalf("read-only policy scope should filter writes: %#v", scoped)
	}
	if scoped := ToolScopeForPolicy(TaskRequest{ToolPolicy: "inherit", Tools: []string{"write_file", "mcp_search"}}, true, []string{"mcp_search"}); len(scoped) != 2 || scoped[0] != "mcp_search" || scoped[1] != "write_file" {
		t.Fatalf("inherit policy scope should include write and MCP tools: %#v", scoped)
	}
	defaultInherited := AllowedToolScopeForPolicy(TaskRequest{ToolPolicy: "inherit"}, true, []string{"mcp_search"})
	if containsString(defaultInherited, "mcp_search") || !containsString(defaultInherited, "write_file") {
		t.Fatalf("default inherited scope should keep core tools without unbounded mcp fanout: %#v", defaultInherited)
	}
	explicitInherited := AllowedToolScopeForPolicy(TaskRequest{ToolPolicy: "inherit", Tools: []string{"mcp_search"}}, true, []string{"mcp_search"})
	if !containsString(explicitInherited, "mcp_search") {
		t.Fatalf("explicit inherited scope should allow requested mcp tools: %#v", explicitInherited)
	}
	if dropped := DroppedRequestedTools(TaskRequest{Tools: []string{"write_file", "grep"}}, ReadOnlyToolScope(true)); len(dropped) != 1 || dropped[0] != "write_file" {
		t.Fatalf("unavailable requested tools should be reported: %#v", dropped)
	}
	blockedReadOnly := ReadOnlyToolScope(false)
	request := TaskRequest{ToolPolicy: "readOnly", BlockedTools: append([]string(nil), blockedReadOnly...)}
	if allowed := AllowedToolScopeForPolicy(request, false, []string{"mcp__docs__lookup"}); len(allowed) != 0 {
		t.Fatalf("blocking every delegated read tool must leave an explicit empty scope: %#v", allowed)
	}
	if scoped := ToolScopeForPolicy(request, false, []string{"mcp__docs__lookup"}); len(scoped) != 0 {
		t.Fatalf("empty delegated authority must not regain builtin or MCP tools: %#v", scoped)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
