package subagent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestRunOutputCarriesOnlyClosedTaskMetadata(t *testing.T) {
	started := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:             "child-1",
		Name:           "review",
		ChildThreadID:  "thread-child",
		ChildTurnID:    "turn-child",
		Status:         "completed",
		Label:          "Review",
		Model:          "mimo-v2",
		ProviderID:     "xiaomi",
		EndpointFormat: "openai-chat",
		Variant:        "fast",
		ModelSource:    "explicit-input",
		ToolPolicy:     "read-only",
		ToolScope:      []string{"read", "grep"},
		ModelExecution: map[string]any{
			"providerId": "xiaomi", "modelId": "mimo-v2", "source": "explicit-input",
			"resolvedAt": "2026-06-26T05:29:59Z", "capabilityFingerprint": strings.Repeat("a", 64),
		},
		ParallelGroupID: "parallel-1",
		ParallelIndex:   2,
		StartedAt:       started.Format(time.RFC3339Nano),
		FinishedAt:      started.Add(1500 * time.Millisecond).Format(time.RFC3339Nano),
	}
	output := RunOutput(record, "done", map[string]any{"totalTokens": float64(7)}, nil)
	assertSecurityBoundFlags(t, output)
	if output["id"] != record.ID || output["status"] != "completed" || output["usage"] != nil || output["modelExecution"] != nil ||
		output["providerId"] != nil || output["parallelGroupId"] != nil || output["durationMs"] != nil {
		t.Fatalf("task output escaped its closed metadata projection: %#v", output)
	}
}

func TestRunOutputCarriesEvidenceBundleWithoutCompletingParent(t *testing.T) {
	summary := "```json\n{\"evidence\":[{\"kind\":\"test\",\"receipt\":\"child-receipt\"}]}\n```"
	output := RunOutput(domainjob.Record{
		ID:           "child-1",
		Status:       "completed",
		ReturnFormat: "evidence",
	}, summary, nil, nil)
	if output["evidenceBundleStatus"] != nil || output["evidenceCount"] != nil || output["evidence"] != nil || output["outputWithheld"] != true {
		t.Fatalf("unverified child evidence escaped through task output: %#v", output)
	}
	if _, ok := output["goalStatus"]; ok {
		t.Fatalf("child evidence must not auto-complete parent goal: %#v", output)
	}
	if evidence, ok := EvidenceBundleFromSummary(summary); !ok || len(evidence) != 1 {
		t.Fatalf("evidence bundle parser mismatch: evidence=%#v ok=%v", evidence, ok)
	}
	missing := RunOutput(domainjob.Record{ID: "child-2", Status: "completed", ReturnFormat: "evidence"}, "no json evidence", nil, nil)
	assertSecurityBoundFlags(t, missing)
}

func TestEventChildAndTaskJobProgressChildWithholdUnboundChildPayload(t *testing.T) {
	maxSteps := 7
	record := domainjob.Record{
		ID:                    "child-1",
		ParentGoalID:          "goal-1",
		ParentGoalObjective:   "ship",
		ParentThreadID:        "thread-parent",
		ParentTurnID:          "turn-parent",
		ParentToolCallID:      "call-parent",
		Name:                  "review",
		ChildThreadID:         "thread-child",
		ChildTurnID:           "turn-child",
		Label:                 "Review",
		Model:                 "mimo-v2",
		ProviderID:            "xiaomi",
		EndpointFormat:        "openai-chat",
		Variant:               "fast",
		ModelSource:           "explicit-input",
		ProfileName:           "reviewer",
		ProfileMode:           "subagent",
		ProfileDescription:    "Review profile",
		ProfileColor:          "blue",
		ProfileIcon:           "search",
		ProfileSource:         "subagent-profile",
		ToolPolicy:            "read-only",
		ToolScope:             []string{"read"},
		ReturnFormat:          "evidence",
		TokenBudget:           2048,
		TimeBudgetMs:          6000,
		LineageKey:            "lineage",
		DefaultModelInherited: true,
		ContinueFrom:          "source",
		SourceRef:             "source",
		Background:            true,
		ArtifactPath:          "/tmp/artifact.log",
		MaxModelSteps:         &maxSteps,
		ModelExecution:        map[string]any{"providerId": "xiaomi"},
		Usage: map[string]any{
			"totalTokens":      float64(12),
			"cacheDiagnostics": map[string]any{"prefix": "stable"},
			"exitCode":         float64(0),
			"durationMs":       float64(250),
			"truncated":        true,
		},
		Output:          "```json\n{\"evidence\":[{\"kind\":\"test\",\"receipt\":\"child-receipt\"}]}\n```",
		ToolInvocations: 3,
	}
	child := EventChild(record, "completed", "done")
	assertSecurityBoundFlags(t, child)
	if child["jobId"] != "child-1" || child["status"] != "completed" {
		t.Fatalf("event child metadata mismatch: %#v", child)
	}
	encoded, _ := json.Marshal(child)
	for _, forbidden := range []string{"child-receipt", "xiaomi", "Review profile", "/tmp/artifact.log", "done"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("event child leaked %q: %s", forbidden, encoded)
		}
	}
	running := EventChild(domainjob.Record{
		ID:             "child-running",
		ParentThreadID: "thread-parent",
		ParentTurnID:   "turn-parent",
		ReturnFormat:   "evidence",
		Status:         "running",
	}, "running", "")
	assertSecurityBoundFlags(t, running)
	record.Output = "hello"
	progress := TaskJobProgressChild(record, "completed")
	assertSecurityBoundFlags(t, progress)
	if progress["childStatus"] != "completed" || progress["outputBytes"] != nil || progress["exitCode"] != nil || progress["truncated"] != nil {
		t.Fatalf("task job progress child leaked private output metadata: %#v", progress)
	}
}

func TestSummaryAndToolInvocationProjectionFromThread(t *testing.T) {
	thread := map[string]any{"turns": []any{
		map[string]any{"id": "turn-a", "items": []any{
			map[string]any{"kind": "assistant_text", "text": " first "},
			map[string]any{"kind": "tool_call", "callId": "call-a"},
		}},
		map[string]any{"id": "turn-b", "items": []any{
			map[string]any{"kind": "assistant_text", "text": " old "},
			map[string]any{"kind": "tool_call", "callId": "call-b"},
			map[string]any{"kind": "assistant_text", "text": " final "},
		}},
	}}
	if got := SummaryFromThread(thread, "turn-b"); got != "final" {
		t.Fatalf("summary should use the last assistant text for the target turn, got %q", got)
	}
	if got := ToolInvocationCountFromThread(thread, "turn-b"); got != 1 {
		t.Fatalf("target turn tool count mismatch: %d", got)
	}
	if got := ToolInvocationCountFromThread(thread, ""); got != 2 {
		t.Fatalf("all-turn tool count mismatch: %d", got)
	}
}

func TestFailedOutputAndParallelStatus(t *testing.T) {
	request := TaskRequest{Label: "Review", ProfileName: "reviewer", ToolPolicy: "read-only"}
	failed := FailedOutput(request, "boom")
	if failed["code"] != "subagent_failed" || failed["status"] != "failed" ||
		failed["failureCode"] != domainjob.FailureChildExecutionFailed || failed["error"] != nil ||
		failed["message"] != nil || failed["reasonCode"] != nil || failed["maxChildRuns"] != nil {
		t.Fatalf("failed output mismatch: %#v", failed)
	}
	limited := FailedOutput(request, "subagent child run limit reached: maxChildRuns=1")
	if limited["reasonCode"] != "subagent_child_run_limit_reached" ||
		limited["message"] != "subagent child run limit reached: maxChildRuns=1" ||
		limited["maxChildRuns"] != float64(1) || limited["error"] != nil {
		t.Fatalf("child-run limit output must retain only the closed host diagnostic: %#v", limited)
	}
	private := FailedOutput(request, "subagent child run limit reached: maxChildRuns=1 PRIVATE_TRACE")
	if private["message"] != nil || private["reasonCode"] != nil || private["maxChildRuns"] != nil {
		t.Fatalf("non-canonical child-run limit diagnostic leaked: %#v", private)
	}
	mismatch := SourceToolScopeMismatchOutput(request)
	if mismatch["reasonCode"] != "subagent_reference_tool_scope_mismatch" ||
		mismatch["message"] != ErrSourceToolScopeMismatch.Error() || mismatch["error"] != nil {
		t.Fatalf("tool-scope mismatch output must use the closed host reason: %#v", mismatch)
	}
	notReplayable := SourceNotReplayableOutput(request, "job-1", "interrupted")
	if notReplayable["reasonCode"] != "subagent_reference_not_replayable" ||
		notReplayable["sourceRef"] != "job-1" || notReplayable["sourceStatus"] != "interrupted" ||
		notReplayable["message"] != `subagent reference "job-1" is interrupted and cannot be continued or forked` ||
		notReplayable["error"] != nil {
		t.Fatalf("non-replayable source output must use the closed host reason: %#v", notReplayable)
	}
	if ParallelStatusFromErrors([]bool{false, true}) != "failed" || ParallelStatusFromErrors([]bool{false}) != "completed" {
		t.Fatalf("parallel status should aggregate child errors")
	}
	if output := RunOutput(domainjob.Record{ID: "child"}, "", nil, errors.New("boom")); output["status"] != "failed" ||
		output["failureCode"] != nil || output["error"] != nil || output["outputWithheld"] != true {
		t.Fatalf("run output should fail empty-status error records: %#v", output)
	}
}

func TestAddUsageMetadataKeepsOnlyNumericFields(t *testing.T) {
	child := map[string]any{}
	AddUsageMetadata(child, map[string]any{
		"totalTokens":      float64(10),
		"reasoningTokens":  float64(2),
		"cacheDiagnostics": map[string]any{"shape": "PRIVATE_USAGE_DIAGNOSTIC"},
		"cacheSuggestions": []any{"PRIVATE_USAGE_SUGGESTION"},
	})
	if child["totalTokens"] != float64(10) || child["reasoningTokens"] != float64(2) {
		t.Fatalf("usage fields should be copied: %#v", child)
	}
	if child["cacheDiagnostics"] != nil || child["cacheSuggestions"] != nil || strings.Contains(strings.ToLower(strings.TrimSpace(firstNonEmptyAnyString(child))), "private") {
		t.Fatalf("usage projection retained non-numeric diagnostics: %#v", child)
	}
}

func TestTaskJobFailureResponsesAreCodeOnly(t *testing.T) {
	const sentinel = "PRIVATE_RUNTIME_ERROR_6222020202020202020"
	for name, test := range map[string]struct {
		response map[string]any
		code     string
	}{
		"validation": {response: ValidationErrorResponse(sentinel), code: "validation_error"},
		"forbidden":  {response: ForbiddenTaskJobResponse(), code: "forbidden"},
		"not found":  {response: TaskJobNotFoundResponse(sentinel), code: "not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			if len(test.response) != 1 || test.response["code"] != test.code || test.response["error"] != nil ||
				strings.Contains(firstNonEmptyAnyString(test.response), sentinel) {
				t.Fatalf("task-job failure response reflected private error text: %#v", test.response)
			}
		})
	}
}
