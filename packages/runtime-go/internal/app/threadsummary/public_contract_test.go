package threadsummary

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateSummaryResponseV1AcceptsOnlyClosedMetadata(t *testing.T) {
	summary := validSummaryContractTestV1()
	summary["subagents"] = []any{validSummarySubagentContractTestV1()}
	summary["tasks"] = []any{
		validSummaryTaskJobContractTestV1(),
		validSummaryCommandTaskContractTestV1(),
	}
	summary["sideChats"] = []any{map[string]any{
		"threadId": "side-1", "title": "Side", "status": "idle", "relation": "side",
		"parentThreadId": "thread-1", "messageCount": float64(0), "turnCount": float64(0),
		"createdAt": "2026-07-20T00:00:00Z", "updatedAt": "2026-07-20T00:00:01Z",
	}}
	if err := ValidateSummaryResponseV1(summary, "thread-1"); err != nil {
		t.Fatalf("valid summary failed: %v", err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"unknown root field": func(value map[string]any) { value["reasoning"] = "private" },
		"route identity":     func(value map[string]any) { value["threadId"] = "thread-2" },
		"invalid timestamp":  func(value map[string]any) { value["generatedAt"] = "not-a-time" },
		"fractional seq":     func(value map[string]any) { value["latestSeq"] = 1.5 },
		"output authority":   func(value map[string]any) { value["outputs"] = []any{map[string]any{"path": "/private"}} },
		"source authority":   func(value map[string]any) { value["sources"] = []any{map[string]any{"id": "fake"}} },
		"process authority": func(value map[string]any) {
			value["backgroundProcesses"] = []any{map[string]any{"command": "cat case.csv"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneSummaryContractTestV1(summary)
			mutate(candidate)
			if err := ValidateSummaryResponseV1(candidate, "thread-1"); err == nil {
				t.Fatalf("unsafe summary was accepted: %#v", candidate)
			}
		})
	}
}

func TestValidateSummaryResponseV1RejectsPrivateSemanticCarriers(t *testing.T) {
	for name, sentinel := range map[string]string{
		"reasoning":  "<think>PRIVATE_SUMMARY_SENTINEL</think>",
		"credential": "Authorization: Bearer private-summary-token",
		"account":    "6222020202020202020",
	} {
		t.Run(name, func(t *testing.T) {
			summary := validSummaryContractTestV1()
			subagent := validSummarySubagentContractTestV1()
			subagent["title"] = sentinel
			summary["subagents"] = []any{subagent}
			if err := ValidateSummaryResponseV1(summary, "thread-1"); err == nil {
				t.Fatalf("private semantic carrier was accepted: %#v", summary)
			}
		})
	}
}

func TestValidateSummaryResponseV1UsesClosedReasoningEffort(t *testing.T) {
	for _, effort := range []string{"auto", "off", "low", "medium", "high", "max"} {
		summary := validSummaryContractTestV1()
		subagent := validSummarySubagentContractTestV1()
		subagent["effort"] = effort
		summary["subagents"] = []any{subagent}
		if err := ValidateSummaryResponseV1(summary, "thread-1"); err != nil {
			t.Fatalf("valid reasoning effort %q failed: %v", effort, err)
		}
	}
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []any{"", " high ", "HIGH", sentinel, map[string]any{"reasoning": sentinel}} {
		summary := validSummaryContractTestV1()
		subagent := validSummarySubagentContractTestV1()
		subagent["effort"] = effort
		summary["subagents"] = []any{subagent}
		err := ValidateSummaryResponseV1(summary, "thread-1")
		if err == nil || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("invalid reasoning effort was accepted or reflected: effort=%#v err=%v", effort, err)
		}
	}
}

func TestValidateSummaryResponseV1UsesClosedChildExecutionBounds(t *testing.T) {
	summary := validSummaryContractTestV1()
	subagent := validSummarySubagentContractTestV1()
	subagent["maxModelSteps"] = float64(8)
	subagent["timeBudgetMs"] = float64(180000)
	summary["subagents"] = []any{subagent}
	if err := ValidateSummaryResponseV1(summary, "thread-1"); err != nil {
		t.Fatalf("valid child execution bounds failed: %v", err)
	}
	for _, candidate := range []any{float64(-1), 1.5, "8"} {
		for _, field := range []string{"maxModelSteps", "timeBudgetMs"} {
			invalid := cloneSummaryContractTestV1(summary)
			invalidSubagent := invalid["subagents"].([]any)[0].(map[string]any)
			invalidSubagent[field] = candidate
			if err := ValidateSummaryResponseV1(invalid, "thread-1"); err == nil {
				t.Fatalf("invalid %s=%#v was accepted", field, candidate)
			}
		}
	}
}

func TestValidateSummaryResponseV1AllowsOnlyClosedSubagentsAtCaseBoundary(t *testing.T) {
	summary := validSummaryContractTestV1()
	summary["historyAuthority"] = "case_boundary_only_v1"
	if err := ValidateSummaryResponseV1(summary, "thread-1"); err != nil {
		t.Fatalf("empty case boundary failed: %v", err)
	}
	summary["subagents"] = []any{validSummarySubagentContractTestV1()}
	if err := ValidateSummaryResponseV1(summary, "thread-1"); err != nil {
		t.Fatalf("closed case-bound subagent lifecycle failed: %v", err)
	}
	summary["tasks"] = []any{validSummaryTaskJobContractTestV1()}
	if err := ValidateSummaryResponseV1(summary, "thread-1"); err == nil {
		t.Fatalf("case boundary accepted task metadata: %#v", summary)
	}
	summary["tasks"] = []any{}
	summary["sideChats"] = []any{map[string]any{
		"threadId": "side-1", "title": "Side", "status": "idle", "relation": "side",
		"parentThreadId": "thread-1", "messageCount": float64(0), "turnCount": float64(0),
		"createdAt": "2026-07-20T00:00:00Z", "updatedAt": "2026-07-20T00:00:01Z",
	}}
	if err := ValidateSummaryResponseV1(summary, "thread-1"); err == nil {
		t.Fatalf("case boundary accepted side-chat metadata: %#v", summary)
	}
}

func TestValidateTaskProjectionV1EnforcesLifecycleAndExactShape(t *testing.T) {
	for _, task := range []map[string]any{validSummaryTaskJobContractTestV1(), validSummaryCommandTaskContractTestV1()} {
		if err := ValidateTaskProjectionV1(task); err != nil {
			t.Fatalf("valid task failed: %#v err=%v", task, err)
		}
	}

	taskJob := validSummaryTaskJobContractTestV1()
	taskJob["terminal"] = true
	if err := ValidateTaskProjectionV1(taskJob); err == nil {
		t.Fatalf("task-job lifecycle mismatch was accepted")
	}
	command := validSummaryCommandTaskContractTestV1()
	command["command"] = "cat /private/case.csv"
	if err := ValidateTaskProjectionV1(command); err == nil {
		t.Fatalf("command authority entered metadata projection")
	}
	command = validSummaryCommandTaskContractTestV1()
	command["active"] = false
	if err := ValidateTaskProjectionV1(command); err == nil {
		t.Fatalf("command lifecycle mismatch was accepted")
	}
}

func TestValidateTaskOutputResponseV1PreservesDistinctWithheldVariants(t *testing.T) {
	child := validSummaryTaskOutputContractTestV1(
		"taskjob:job-1", "running", "security_bound_child_output", "untrusted_child_output",
	)
	command := validSummaryTaskOutputContractTestV1(
		"command:call-1", "error", "tool_output_private", "private_tool_output",
	)
	if err := ValidateTaskOutputResponseV1(child, "taskjob:job-1"); err != nil {
		t.Fatalf("valid child output failed: %v", err)
	}
	if err := ValidateTaskOutputResponseV1(command, "command:call-1"); err != nil {
		t.Fatalf("valid command output failed: %v", err)
	}

	for name, candidate := range map[string]map[string]any{
		"crossed trust": validSummaryTaskOutputContractTestV1(
			"command:call-1", "completed", "tool_output_private", "untrusted_child_output",
		),
		"wrong identity": validSummaryTaskOutputContractTestV1(
			"command:call-2", "completed", "tool_output_private", "private_tool_output",
		),
		"readable output": func() map[string]any {
			value := validSummaryTaskOutputContractTestV1(
				"command:call-1", "completed", "tool_output_private", "private_tool_output",
			)
			value["output"] = "PRIVATE_OUTPUT"
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateTaskOutputResponseV1(candidate, "command:call-1"); err == nil {
				t.Fatalf("invalid output variant was accepted: %#v", candidate)
			}
		})
	}
}

func TestValidateTaskMutationResponseV1BindsCanonicalIdentity(t *testing.T) {
	response := map[string]any{"task": validSummaryTaskJobContractTestV1()}
	if err := ValidateTaskMutationResponseV1(response, "run:job-1"); err != nil {
		t.Fatalf("canonical task identity failed: %v", err)
	}
	if err := ValidateTaskMutationResponseV1(response, "taskjob:job-2"); err == nil {
		t.Fatalf("foreign mutation task was accepted")
	}
	response["error"] = "PRIVATE_MUTATION"
	if err := ValidateTaskMutationResponseV1(response, "taskjob:job-1"); err == nil {
		t.Fatalf("mixed mutation envelope was accepted")
	}
}

func validSummaryContractTestV1() map[string]any {
	return map[string]any{
		"threadId": "thread-1", "generatedAt": "2026-07-20T00:00:00Z", "latestSeq": float64(1),
		"subagents": []any{}, "tasks": []any{}, "outputs": []any{}, "sources": []any{},
		"sideChats": []any{}, "backgroundProcesses": []any{},
	}
}

func validSummarySubagentContractTestV1() map[string]any {
	return map[string]any{
		"schemaVersion": 1, "id": "run:child-1", "key": "run:child-1", "parentThreadId": "thread-1",
		"childRunId": "child-1", "childThreadId": "child-thread-1", "status": "active", "rawStatus": "running",
		"outputWithheld": true, "outputTrustStatus": "untrusted_child_output", "factAnswerAllowed": false,
		"evidenceAuthority": false, "canContinueParent": false, "canReadOutput": false,
		"canOpenThread": true, "canKill": false, "canRestart": false,
		"diagnostics": map[string]any{
			"status": "running", "terminal": false, "background": false, "paused": false,
			"startedAt": "2026-07-20T00:00:00Z", "updatedAt": "2026-07-20T00:00:01Z",
		},
		"updatedAt": "2026-07-20T00:00:01Z",
	}
}

func validSummaryTaskJobContractTestV1() map[string]any {
	return map[string]any{
		"schemaVersion": 1, "id": "taskjob:job-1", "kind": "task", "status": "running",
		"background": false, "terminal": false, "outputWithheld": true,
		"outputTrustStatus": "untrusted_child_output", "factAnswerAllowed": false,
		"evidenceAuthority": false, "canReadOutput": false, "canContinueParent": false,
	}
}

func validSummaryCommandTaskContractTestV1() map[string]any {
	return map[string]any{
		"schemaVersion": 1, "id": "command:call-1", "kind": "command", "status": "running",
		"background": false, "active": true, "terminal": false, "outputWithheld": true,
		"outputTrustStatus": "private_tool_output", "factAnswerAllowed": false,
		"evidenceAuthority": false, "canReadOutput": false, "canContinueParent": false,
	}
}

func validSummaryTaskOutputContractTestV1(taskID string, status string, reason string, trust string) map[string]any {
	return map[string]any{
		"schemaVersion": 1, "availability": "withheld", "taskId": taskID, "status": status,
		"reasonCode": reason, "outputWithheld": true, "outputTrustStatus": trust,
		"factAnswerAllowed": false, "evidenceAuthority": false, "canReadOutput": false,
		"canContinueParent": false,
	}
}

func cloneSummaryContractTestV1(value map[string]any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		panic(err)
	}
	return cloned
}

func TestSummaryContractErrorsNeverContainPrivateSentinel(t *testing.T) {
	const sentinel = "PRIVATE_CONTRACT_ERROR_SENTINEL"
	summary := validSummaryContractTestV1()
	summary["reasoning"] = sentinel
	err := ValidateSummaryResponseV1(summary, "thread-1")
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("contract error reflected private content: %v", err)
	}
}
