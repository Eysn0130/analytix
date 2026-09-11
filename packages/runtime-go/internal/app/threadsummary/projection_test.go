package threadsummary

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestTaskFromJobProjectsTerminalAndOutputState(t *testing.T) {
	now := time.Date(2026, 7, 2, 7, 0, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:                "job-1",
		Kind:              "background-shell",
		ParentThreadID:    "thread-1",
		ParentTurnID:      "turn-1",
		ParentToolCallID:  "call-1",
		ChildThreadID:     "child-thread",
		Label:             "npm test",
		Workspace:         "/repo",
		Status:            "completed",
		Output:            "all tests passed",
		StartedAt:         now.Add(-time.Minute).Format(time.RFC3339Nano),
		FinishedAt:        now.Format(time.RFC3339Nano),
		Usage:             map[string]any{"exitCode": 0.0},
		ArtifactPath:      "/private/child/output.log",
		AutoContinueError: "PRIVATE_CHILD_ERROR_SENTINEL",
		RecoveryReason:    "PRIVATE_CHILD_RECOVERY_SENTINEL",
	}
	task := TaskFromJob(record, now)
	if stringValue(task, "id") != "taskjob:job-1" || stringValue(task, "kind") != "background-shell" ||
		stringValue(task, "status") != "completed" ||
		boolValue(task, "canReadOutput") ||
		!boolValue(task, "outputWithheld") ||
		boolValue(task, "factAnswerAllowed") ||
		!boolValue(task, "terminal") {
		t.Fatalf("task projection mismatch: %#v", task)
	}
	body, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"all tests passed", "/private/child/output.log", "PRIVATE_CHILD_ERROR_SENTINEL", "PRIVATE_CHILD_RECOVERY_SENTINEL", "outputSnippet", "errorSnippet", "resultSnippet"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("task summary leaked child diagnostic %q: %s", forbidden, body)
		}
	}

	record.Status = "running"
	record.Output = "private child output"
	task = TaskFromJob(record, now)
	if stringValue(task, "status") != "running" || boolValue(task, "terminal") ||
		boolValue(task, "canReadOutput") || !boolValue(task, "outputWithheld") {
		t.Fatalf("running task projection mismatch: %#v", task)
	}
}

func TestTaskFromJobDoesNotReflectUnknownLifecycleControlText(t *testing.T) {
	const hostileKind = "PRIVATE_PROVIDER_KIND_SENTINEL"
	const hostileStatus = "PRIVATE_PROVIDER_STATUS_SENTINEL"
	task := TaskFromJob(domainjob.Record{
		ID:             "job-hostile",
		Kind:           hostileKind,
		ParentThreadID: "thread-1",
		Status:         hostileStatus,
	}, time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC))
	if task["kind"] != "unknown" || task["status"] != "unknown" || task["canReadOutput"] != false {
		t.Fatalf("unknown durable lifecycle was not closed: %#v", task)
	}
	body, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), hostileKind) || strings.Contains(string(body), hostileStatus) {
		t.Fatalf("unknown durable lifecycle text was reflected: %s", body)
	}
}

func TestSummaryProjectionHelpers(t *testing.T) {
	if !IsSubagentJob(domainjob.Record{Kind: "subagent"}) ||
		IsSubagentJob(domainjob.Record{Kind: "background-shell"}) ||
		!IsChildTaskSubagentJob(domainjob.Record{Kind: "task", ChildThreadID: "child"}) {
		t.Fatalf("job kind classification mismatch")
	}
	if NormalizeActiveStatus("running") != "active" ||
		NormalizeActiveStatus("completed") != "done" ||
		NormalizeActiveStatus("") != "inactive" ||
		NormalizeActiveStatus("provider_controlled_status") != "inactive" ||
		NormalizeActiveStatus("pause_requested") != "active" ||
		NormalizeActiveStatus("paused") != "active" ||
		NormalizeActiveStatus("resume_requested") != "active" ||
		NormalizeActiveStatus("resuming") != "active" ||
		NormalizeActiveStatus("killed") != "terminal" ||
		PublicSummaryJobStatusV1("provider_controlled_status") != "unknown" ||
		!CanKillJobStatus("starting") ||
		!CanKillJobStatus("running") ||
		CanKillJobStatus("completed") ||
		NormalizeCommandStatus("pending", false) != "active" ||
		NormalizeCommandStatus("", true) != "terminal" ||
		NormalizeCommandStatus("provider_controlled_status", false) != "inactive" ||
		PublicCommandStatusV1("provider_controlled_status", false) != "unknown" ||
		PublicCommandStatusV1("", true) != "error" {
		t.Fatalf("status normalization mismatch")
	}
}

func TestSummaryDisplayNameHelpers(t *testing.T) {
	if ParallelIndexSeed(2) != "2" || FloatParallelIndexSeed(3.2) != "3" {
		t.Fatalf("parallel index seed mismatch")
	}
	if DistinctSubagentNameCandidate("Reviewer", "Reviewer", "") != "" ||
		DistinctSubagentNameCandidate("Reviewer", "audit", "profile") != "Reviewer" {
		t.Fatalf("distinct name candidate mismatch")
	}
	if NormalizeChildThreadTitle("Child agent: Reviewer") != "" ||
		NormalizeChildThreadTitle("Reviewer fork") != "Reviewer" ||
		DisplayNameCandidate("profile", []string{"profile"}) != "" ||
		CleanSubagentDisplayLabel("Child agent: Reviewer fork") != "reviewer" {
		t.Fatalf("display name helper mismatch")
	}
	if SubagentRecordDisplayLabel(domainjob.Record{Name: "Reviewer", Label: "audit"}) != "Reviewer" {
		t.Fatalf("record display label should prefer distinct explicit name")
	}
	if SubagentSourceDisplayLabel(domainjob.Record{ProfileName: "reviewer", ID: "run-1"}, "Investigation fork") != "Investigation" {
		t.Fatalf("source display label should use normalized child title")
	}
}

func TestSummaryUpsertAndSort(t *testing.T) {
	values := map[string]map[string]any{}
	UpsertByUpdatedAt(values, map[string]any{"key": "a", "id": "a", "updatedAt": "2026-07-02T07:00:00Z", "label": "old"})
	UpsertByUpdatedAt(values, map[string]any{"key": "a", "id": "a", "updatedAt": "2026-07-02T07:01:00Z", "label": "new"})
	UpsertByUpdatedAt(values, map[string]any{"key": "b", "id": "b", "updatedAt": "2026-07-02T07:02:00Z"})
	sorted := SortedValues(values)
	if len(sorted) != 2 || stringValue(sorted[0].(map[string]any), "id") != "b" || stringValue(sorted[1].(map[string]any), "label") != "new" {
		t.Fatalf("upsert/sort mismatch: %#v", sorted)
	}
}

func boolValue(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func stringValue(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func numericValue(record map[string]any, key string) float64 {
	switch value := record[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}
