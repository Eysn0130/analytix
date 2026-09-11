package loop

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestRepeatSuccessBlockGuardsRepeatedWriteLikeCalls(t *testing.T) {
	call := domainmodel.ToolCall{
		Name:      "edit_file",
		Arguments: json.RawMessage(`{"path":"a.txt","old":"x","new":"y"}`),
	}
	counts := map[string]int{}
	RecordRepeatSuccess(call, counts)
	if _, blocked := RepeatSuccessBlock(call, counts); blocked {
		t.Fatal("first repeated write-like call should not be blocked")
	}
	RecordRepeatSuccess(call, counts)
	output, blocked := RepeatSuccessBlock(call, counts)
	if !blocked {
		t.Fatal("second repeated write-like success should be blocked")
	}
	if output["code"] != "loop_guard" || output["toolName"] != "edit_file" {
		t.Fatalf("unexpected repeat-success guard output: %#v", output)
	}
}

func TestRepeatSuccessBlockDetectsForegroundBashWritesOnly(t *testing.T) {
	foreground := domainmodel.ToolCall{
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"printf hi > out.txt"}`),
	}
	background := domainmodel.ToolCall{
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"printf hi > out.txt","run_in_background":true}`),
	}
	counts := map[string]int{}
	RecordRepeatSuccess(foreground, counts)
	RecordRepeatSuccess(foreground, counts)
	if _, blocked := RepeatSuccessBlock(foreground, counts); !blocked {
		t.Fatal("foreground bash write should be guarded after repeated success")
	}
	RecordRepeatSuccess(background, counts)
	RecordRepeatSuccess(background, counts)
	if _, blocked := RepeatSuccessBlock(background, counts); blocked {
		t.Fatal("background bash writes should not use foreground repeat guard")
	}
}

func TestFailureStormGuardAddsDurableModelNudge(t *testing.T) {
	call := domainmodel.ToolCall{Name: "read_file", Arguments: json.RawMessage(`{"path":"missing"}`)}
	signature := ""
	count := 0
	output := map[string]any{"code": "not_found", "error": "file missing\nextra detail"}

	for attempt := 1; attempt <= FailureStormBreakThreshold; attempt++ {
		guardedOutput, guarded := ApplyFailureStormGuard(call, output, true, &signature, &count)
		if attempt < FailureStormBreakThreshold && guarded {
			t.Fatalf("attempt %d should not be guarded", attempt)
		}
		if attempt == FailureStormBreakThreshold {
			if !guarded {
				t.Fatal("failure storm threshold should be guarded")
			}
			record, ok := guardedOutput.(map[string]any)
			if !ok {
				t.Fatalf("guard output should remain a map: %#v", guardedOutput)
			}
			if record["loop_guard"] != true || record["storm_count"] != float64(FailureStormBreakThreshold) {
				t.Fatalf("missing storm guard metadata: %#v", record)
			}
			if !strings.Contains(record["error"].(string), "file missing") || !strings.Contains(record["error"].(string), "[loop guard]") {
				t.Fatalf("guard error should preserve cause and nudge: %#v", record["error"])
			}
		}
	}
}

func TestFailureStormGuardIgnoresPolicySettlementFailures(t *testing.T) {
	call := domainmodel.ToolCall{Name: "bash", Arguments: json.RawMessage(`{"command":"pwd"}`)}
	signature := ""
	count := 0
	output := map[string]any{"code": "approval_denied", "error": "denied"}

	for attempt := 0; attempt < FailureStormBreakThreshold+1; attempt++ {
		if _, guarded := ApplyFailureStormGuard(call, output, true, &signature, &count); guarded {
			t.Fatal("policy settlement failures should not trigger loop guard")
		}
	}
	if signature != "" || count != 0 {
		t.Fatalf("excluded failures should reset storm state, got signature=%q count=%d", signature, count)
	}
}

func TestFailureStormTurnFailureStopsRepeatedEmptyRequiredArguments(t *testing.T) {
	call := domainmodel.ToolCall{Name: "bash", Arguments: json.RawMessage(`{}`)}
	output := map[string]any{"code": "validation_error", "error": "command is required"}
	if _, terminal := FailureStormTurnFailure(call, output, true, InvalidToolArgumentsHardStopThreshold-1); terminal {
		t.Fatal("empty required arguments should not hard-stop before threshold")
	}
	err, terminal := FailureStormTurnFailure(call, output, true, InvalidToolArgumentsHardStopThreshold)
	if !terminal {
		t.Fatal("empty required argument storm should hard-stop at threshold")
	}
	if err.Code != "tool_invalid_arguments_storm" || err.Details["toolName"] != "bash" {
		t.Fatalf("unexpected turn failure error: %#v", err)
	}
}

func TestFailureStormTurnFailureAllowsGenericFailuresToNudgeFirst(t *testing.T) {
	call := domainmodel.ToolCall{Name: "read_file", Arguments: json.RawMessage(`{"path":"missing"}`)}
	output := map[string]any{"code": "not_found", "error": "file missing"}
	if _, terminal := FailureStormTurnFailure(call, output, true, FailureStormBreakThreshold); terminal {
		t.Fatal("generic failures should remain a model nudge at the first loop-guard threshold")
	}
	if _, terminal := FailureStormTurnFailure(call, output, true, FailureStormHardStopThreshold); !terminal {
		t.Fatal("generic failure storm should eventually hard-stop")
	}
}
