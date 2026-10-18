package subagent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestSkillResultCaptureV1RealInstructionAndBoundChild(t *testing.T) {
	skill := map[string]any{"id": "synthetic-skill", "name": "Synthetic Skill", "packageDigest": strings.Repeat("a", 64), "prompt": "PRIVATE_PROMPT"}
	input := SkillResultCaptureInputV1{RequestedName: "alias", Skill: skill, RunAs: "inline", Instruction: strings.Repeat("中", 10923)}
	capture, ok := SkillResultCaptureV1(input)
	if !ok || !capture.Truncated || len(capture.Body) != 32766 || capture.Skill.Resolved.Name != "Synthetic Skill" {
		t.Fatal("actual UTF8 instruction observation unavailable")
	}
	security, grant, _ := jobSecurityAuthorityFixture(t, "run_skill", false)
	binding, err := domainjob.NewSecurityBinding(security, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{ID: "job-skill", Kind: "subagent", Status: "failed", ParentThreadID: security.ThreadID, ParentTurnID: security.TurnID, ParentToolCallID: grant.ToolCallID,
		SecurityBinding: binding, SkillPackageDigest: strings.Repeat("a", 64), ChildThreadID: "thread-child", ChildTurnID: "turn-child", Output: "PRIVATE_CHILD_OUTPUT", Error: "PRIVATE_CHILD_ERROR", Prompt: "PRIVATE_PROMPT"}
	input.RunAs, input.Instruction, input.IsError = "subagent", "PRIVATE_INSTRUCTION", true
	input.Job, input.SecurityContext, input.ExecutionGrantID, input.CallID = &record, security, grant.GrantID, grant.ToolCallID
	capture, ok = SkillResultCaptureV1(input)
	if !ok || capture.Status != "failed" || capture.Body != "" || capture.Truncated || capture.Skill.Child == nil || *capture.Skill.Child.ChildTurnID != "turn-child" || capture.Skill.Error != nil {
		t.Fatal("actual child observation unavailable")
	}
	body, _ := json.Marshal(capture)
	if strings.Contains(string(body), "PRIVATE_") || strings.Contains(string(body), grant.GrantID) {
		t.Fatal("child payload/authority entered display")
	}
	input.CallID = "another-call"
	if _, ok := SkillResultCaptureV1(input); ok {
		t.Fatal("another invocation child accepted")
	}
	input.CallID = grant.ToolCallID
	record.SkillPackageDigest = strings.Repeat("b", 64)
	if _, ok := SkillResultCaptureV1(input); ok {
		t.Fatal("another package child accepted")
	}
	input = SkillResultCaptureInputV1{RequestedName: "unknown", IsError: true, ErrorCode: "unknown_skill", ErrorMessage: "unknown skill"}
	if capture, ok := SkillResultCaptureV1(input); !ok || capture.Skill.Resolved != nil || capture.Skill.Child != nil || capture.Skill.Error.Code != "unknown_skill" {
		t.Fatal("unknown skill inferred current state")
	}
}

func TestSkillTaskRequestFromArgs(t *testing.T) {
	skill := map[string]any{
		"id":             "deep-review",
		"name":           "Deep Review",
		"runAs":          "subagent",
		"provider_id":    "anthropic-main",
		"model":          "claude-sonnet",
		"endpointFormat": "messages",
		"variant":        "fast",
		"effort":         "high",
		"allowedTools":   []string{"read", "grep"},
		"packageDigest":  strings.Repeat("a", 64),
	}
	request, err := SkillTaskRequestFromArgs(SkillTaskRequestInput{
		Skill:  skill,
		Args:   map[string]any{"arguments": "Inspect the diff", "model": "override-model"},
		Prompt: "Skill prompt",
	})
	if err != nil {
		t.Fatalf("skill request: %v", err)
	}
	if request.Name != "deep-review" ||
		request.Label != "skill: deep-review" ||
		request.Prompt != "Skill prompt" ||
		request.ProviderID != "anthropic-main" ||
		request.Model != "override-model" ||
		request.EndpointFormat != "messages" ||
		request.Variant != "fast" ||
		request.Effort != "high" ||
		request.ProfileName != "skill:deep-review" ||
		request.ToolPolicy != "" ||
		request.ToolPolicySet ||
		request.SkillPackageDigest != strings.Repeat("a", 64) {
		t.Fatalf("skill task request mismatch: %#v", request)
	}
	if !request.ModelExplicit || request.ProviderExplicit || !request.ProfileExecutionConfigured {
		t.Fatalf("skill task explicit/profile flags mismatch: %#v", request)
	}
	if len(request.Tools) != 2 || request.Tools[0] != "grep" || request.Tools[1] != "read" {
		t.Fatalf("skill request tools mismatch: %#v", request.Tools)
	}
}

func TestSkillTaskRequestRejectsInvalidReasoningEffortExactly(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	skill := map[string]any{
		"id": "deep-review", "packageDigest": strings.Repeat("a", 64), "allowedTools": []string{"read"},
	}
	for _, effort := range []any{" high ", "HIGH", "auto", sentinel, float64(1)} {
		request, err := SkillTaskRequestFromArgs(SkillTaskRequestInput{
			Skill: skill, Args: map[string]any{"arguments": "Inspect", "effort": effort}, Prompt: "Inspect",
		})
		if !errors.Is(err, ErrReasoningEffortInvalid) || request.Prompt != "" || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("invalid skill effort did not fail closed: effort=%#v request=%#v err=%v", effort, request, err)
		}
	}
}

func TestSkillAllowedToolsCannotElevateChildPolicy(t *testing.T) {
	request, err := SkillTaskRequestFromArgs(SkillTaskRequestInput{
		Skill: map[string]any{
			"id": "malicious", "allowedTools": []string{"read", "write_file", "bash", "mcp__fake__write"}, "packageDigest": strings.Repeat("b", 64),
		},
		Args:   map[string]any{"arguments": "inspect"},
		Prompt: "inspect",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.ToolPolicy != "" || request.ToolPolicySet {
		t.Fatalf("skill must not select or mark a child tool policy: %#v", request)
	}
	request, err = ApplyProfile(request, ProfileSettings{DefaultToolPolicy: "readOnly"})
	if err != nil {
		t.Fatal(err)
	}
	if request.ToolPolicy != "readOnly" || request.ToolPolicySource != "default" {
		t.Fatalf("host default must be the only policy source: %#v", request)
	}
	restricted, err := RestrictSkillToolsToHost(request, []string{"read", "grep"})
	if err != nil {
		t.Fatal(err)
	}
	if len(restricted.Tools) != 1 || restricted.Tools[0] != "read" {
		t.Fatalf("skill tools must intersect host authority: %#v", restricted.Tools)
	}
}

func TestSkillWithNoAuthorizedAllowedToolsFailsClosed(t *testing.T) {
	request := TaskRequest{ProfileName: "skill:malicious", Tools: []string{"write_file", "bash"}}
	if _, err := RestrictSkillToolsToHost(request, []string{"read", "grep"}); err == nil {
		t.Fatal("empty skill/host authority intersection must fail closed")
	}
}

func TestSkillContinuationRejectsPackageDigestDrift(t *testing.T) {
	source := domainjob.Record{
		ID: "job-skill", Kind: "subagent", Status: "completed", ChildThreadID: "thread-child",
		SkillPackageDigest: strings.Repeat("a", 64),
		ToolScope:          []string{"read"},
	}
	request := TaskRequest{SkillPackageDigest: strings.Repeat("b", 64), ToolPolicy: "readOnly"}
	err := ValidateSource(source, request, SourceValidation{
		ToolScope:          []string{"read"},
		SkillPackageDigest: request.SkillPackageDigest,
	})
	if err == nil || !strings.Contains(err.Error(), "different skill package snapshot") {
		t.Fatalf("skill digest drift must reject continuation, got %v", err)
	}
}

func TestSkillTaskRequestRejectsMissingArguments(t *testing.T) {
	_, err := SkillTaskRequestFromArgs(SkillTaskRequestInput{
		Skill: map[string]any{"id": "deep-review"},
		Args:  map[string]any{},
	})
	if err == nil || !strings.Contains(err.Error(), "run_skill requires arguments") {
		t.Fatalf("expected missing argument error, got %v", err)
	}
}

func TestRunSkillOutputs(t *testing.T) {
	skill := map[string]any{"id": "deep-review", "name": "Deep Review"}
	inline := InlineSkillOutput(skill, map[string]any{"task": "Inspect"}, "", "Body")
	if inline["runAs"] != "inline" || inline["arguments"] != "Inspect" || inline["instruction"] != "Body" {
		t.Fatalf("inline skill output mismatch: %#v", inline)
	}
	subagent := RunSkillSubagentOutput(map[string]any{"status": "queued"}, skill)
	if subagent["kind"] != "run_skill" || subagent["runAs"] != "subagent" || subagent["skillId"] != "deep-review" {
		t.Fatalf("subagent skill output mismatch: %#v", subagent)
	}
	if SkillNameFromArgs(map[string]any{"skill_id": "deep-review"}) != "deep-review" || !SkillContinueOrForkRequested(map[string]any{"forkFrom": "run_1"}) {
		t.Fatal("skill arg helpers mismatch")
	}
}
