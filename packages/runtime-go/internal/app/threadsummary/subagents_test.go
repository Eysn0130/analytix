package threadsummary

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestSubagentFromJobProjectsModelAndDuration(t *testing.T) {
	maxModelSteps := 8
	record := domainjob.Record{
		ID:             "run-1",
		Kind:           "subagent",
		ParentThreadID: "thread-1",
		ParentTurnID:   "turn-1",
		ChildThreadID:  "child-thread",
		Status:         "completed",
		Model:          "model-a",
		ProviderID:     "provider-a",
		EndpointFormat: "chat_completions",
		Variant:        "fast",
		ProfileName:    "reviewer",
		ToolPolicy:     "readOnly",
		MaxModelSteps:  &maxModelSteps,
		TimeBudgetMs:   180000,
		StartedAt:      "2026-01-01T00:00:00Z",
		FinishedAt:     "2026-01-01T00:00:02Z",
		Usage:          map[string]any{"totalTokens": float64(42)},
	}
	projected := SubagentFromJob(SubagentContext{
		ParentThreadID:    "thread-1",
		ChildThreadTitles: map[string]string{"child-thread": "Investigator"},
	}, record, time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC))
	if projected["displayName"] != "Investigator" {
		t.Fatalf("unexpected displayName: %#v", projected)
	}
	if projected["status"] != "done" || projected["durationMs"] != float64(2000) || projected["totalTokens"] != float64(42) {
		t.Fatalf("unexpected projection: %#v", projected)
	}
	if projected["taskJobId"] != "run-1" || projected["taskKind"] != "subagent" || projected["canReadOutput"] != false || projected["outputWithheld"] != true || projected["factAnswerAllowed"] != false || projected["canRestart"] != false || projected["canKill"] != false {
		t.Fatalf("subagent projection should expose metadata without child output authority: %#v", projected)
	}
	if projected["profile"] != "reviewer" || projected["toolPolicy"] != "readOnly" ||
		projected["maxModelSteps"] != float64(8) || projected["timeBudgetMs"] != float64(180000) {
		t.Fatalf("subagent projection lost durable execution bounds: %#v", projected)
	}
	record.Status = "starting"
	projected = SubagentFromJob(SubagentContext{ParentThreadID: "thread-1"}, record, time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC))
	if projected["status"] != "active" || projected["canKill"] != false {
		t.Fatalf("starting historical subagent should be active metadata without control authority: %#v", projected)
	}
	record.Status = "killed"
	record.Prompt = "Inspect again"
	projected = SubagentFromJob(SubagentContext{ParentThreadID: "thread-1"}, record, time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC))
	if projected["status"] != "terminal" || projected["canRestart"] != false || projected["canKill"] != false {
		t.Fatalf("terminal historical subagent should not advertise control authority: %#v", projected)
	}
}

func TestSubagentFromTaskJobProjectsBackgroundChildControls(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-1",
		Kind:           "task",
		ParentThreadID: "thread-1",
		ChildThreadID:  "child-thread",
		Status:         "running",
		Label:          "Audit source",
		Background:     true,
		Output:         "partial output",
		UpdatedAt:      "2026-01-01T00:00:01Z",
	}
	projected := SubagentFromTaskJob(SubagentContext{
		ParentThreadID:    "thread-1",
		ChildThreadTitles: map[string]string{"child-thread": "Child agent: task"},
	}, record, time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC))
	if projected["taskJobId"] != "job-1" ||
		projected["taskKind"] != "task" ||
		projected["status"] != "active" ||
		projected["canKill"] != false ||
		projected["canReadOutput"] != false ||
		projected["canRestart"] != false ||
		projected["outputWithheld"] != true ||
		projected["factAnswerAllowed"] != false ||
		projected["background"] != true {
		t.Fatalf("child task job should project metadata-only controls without restart: %#v", projected)
	}
	if projected["displayName"] == "task" || projected["displayName"] == "Audit source" {
		t.Fatalf("prompt-like/generic child task labels should be replaced by generated display names: %#v", projected)
	}
}

func TestSubagentFromEventChildRejectsForeignParent(t *testing.T) {
	projected := SubagentFromEventChild(SubagentContext{ParentThreadID: "thread-1"}, map[string]any{
		"parentThreadId": "thread-2",
		"childRunId":     "run-1",
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if projected != nil {
		t.Fatalf("expected nil for foreign parent, got %#v", projected)
	}
}

func TestSubagentFromEventChildUsesClosedMetadataProjection(t *testing.T) {
	const sentinel = "PRIVATE_CHILD_REASONING_SENTINEL"
	projected := SubagentFromEventChild(SubagentContext{ParentThreadID: "thread-1"}, map[string]any{
		"parentThreadId":      "thread-1",
		"childRunId":          "run-1",
		"childStatus":         "completed",
		"maxModelSteps":       float64(8),
		"timeBudgetMs":        float64(180000),
		"output":              sentinel,
		"summary":             sentinel,
		"error":               sentinel,
		"evidenceLedgerError": sentinel,
		"childModelExecution": map[string]any{
			"providerId": "provider-a",
			"reasoning":  sentinel,
			"arbitrary":  map[string]any{"secret": sentinel},
		},
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), sentinel) || projected["canReadOutput"] != false || projected["outputWithheld"] != true || projected["factAnswerAllowed"] != false {
		t.Fatalf("event child projection leaked non-metadata content: %s", body)
	}
	if projected["maxModelSteps"] != float64(8) || projected["timeBudgetMs"] != float64(180000) {
		t.Fatalf("event child projection lost closed execution bounds: %#v", projected)
	}
}

func TestSubagentReasoningEffortProjectionIsClosedAndNonReflective(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	record := domainjob.Record{
		ID: "run-1", Kind: "subagent", ParentThreadID: "thread-1", Status: "running",
		Effort: sentinel,
	}
	projected := SubagentFromJob(SubagentContext{ParentThreadID: "thread-1"}, record, now)
	if _, exists := projected["effort"]; exists {
		t.Fatalf("invalid persisted effort entered job summary: %#v", projected)
	}
	projected = SubagentFromEventChild(SubagentContext{ParentThreadID: "thread-1"}, map[string]any{
		"parentThreadId": "thread-1", "childRunId": "run-2", "childStatus": "running", "effort": sentinel,
	}, now)
	if _, exists := projected["effort"]; exists {
		t.Fatalf("invalid event effort entered summary: %#v", projected)
	}
	body, err := json.Marshal(projected)
	if err != nil || strings.Contains(string(body), sentinel) {
		t.Fatalf("invalid event effort was reflected: body=%s err=%v", body, err)
	}
	record.Effort = "auto"
	projected = SubagentFromJob(SubagentContext{ParentThreadID: "thread-1"}, record, now)
	if projected["effort"] != "auto" {
		t.Fatalf("valid auto effort was not preserved: %#v", projected)
	}
}

func TestSubagentStatusProjectionDoesNotReflectUnknownLifecycleText(t *testing.T) {
	const hostileStatus = "PRIVATE_PROVIDER_STATUS_SENTINEL"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:             "run-unknown",
		Kind:           "provider_kind_sentinel",
		ParentThreadID: "thread-1",
		Status:         hostileStatus,
	}
	projected := SubagentFromJob(SubagentContext{ParentThreadID: "thread-1"}, record, now)
	if projected["taskKind"] != "unknown" || projected["rawStatus"] != "unknown" || projected["status"] != "inactive" {
		t.Fatalf("unknown durable lifecycle was not closed: %#v", projected)
	}
	diagnostics, _ := projected["diagnostics"].(map[string]any)
	if diagnostics["status"] != "unknown" || diagnostics["terminal"] != false || diagnostics["paused"] != false {
		t.Fatalf("unknown durable lifecycle gained control semantics: %#v", diagnostics)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), hostileStatus) {
		t.Fatalf("unknown durable lifecycle text was reflected: %s", body)
	}

	projected = SubagentFromEventChild(SubagentContext{ParentThreadID: "thread-1"}, map[string]any{
		"parentThreadId": "thread-1",
		"childRunId":     "run-event-unknown",
		"childStatus":    hostileStatus,
	}, now)
	if projected["rawStatus"] != "unknown" || projected["status"] != "inactive" || projected["canKill"] == true || projected["canRestart"] == true {
		t.Fatalf("unknown event lifecycle gained public control semantics: %#v", projected)
	}
	body, err = json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), hostileStatus) {
		t.Fatalf("unknown event lifecycle text was reflected: %s", body)
	}
}

func TestChildThreadIDHelpers(t *testing.T) {
	ids := ChildThreadIDs([]domainjob.Record{{ChildThreadID: "record-child"}}, []map[string]any{
		{"child": map[string]any{"childThreadId": "event-child"}},
	})
	if !ids["record-child"] || !ids["event-child"] {
		t.Fatalf("unexpected ids: %#v", ids)
	}
	byKey := map[string]map[string]any{
		"run": {"childThreadId": "record-child"},
	}
	if !SubagentChildThreadIDs(byKey)["record-child"] {
		t.Fatalf("unexpected projected child ids")
	}
}

func TestApplySubagentJobThreadAuthoritySurvivesMetadataOnlyEventMerge(t *testing.T) {
	maxModelSteps := 8
	byKey := map[string]map[string]any{
		"run:job-1": {"key": "run:job-1", "canOpenThread": false},
	}
	ApplySubagentJobThreadAuthority(byKey, []domainjob.Record{{
		ID: "job-1", ParentThreadID: "parent-1", ParentTurnID: "turn-1", ParentToolCallID: "call-1",
		ChildThreadID: "child-thread-1", ChildTurnID: "child-turn-1", ProfileName: "reviewer",
		ToolPolicy: "readOnly", MaxModelSteps: &maxModelSteps, TimeBudgetMs: 180000,
	}})
	projected := byKey["run:job-1"]
	if projected["childThreadId"] != "child-thread-1" || projected["canOpenThread"] != true {
		t.Fatalf("durable child-thread authority was not restored: %#v", projected)
	}
	if projected["parentThreadId"] != "parent-1" || projected["parentTurnId"] != "turn-1" ||
		projected["parentToolCallId"] != "call-1" || projected["childRunId"] != "job-1" ||
		projected["childTurnId"] != "child-turn-1" || projected["profile"] != "reviewer" ||
		projected["toolPolicy"] != "readOnly" || projected["maxModelSteps"] != float64(8) ||
		projected["timeBudgetMs"] != float64(180000) {
		t.Fatalf("durable child execution authority was not restored: %#v", projected)
	}
}
