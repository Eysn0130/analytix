package subagent

import (
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestTaskJobDiagnosticsMarksStalledAndTerminalJobs(t *testing.T) {
	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:             "job-stalled",
		Kind:           "background-shell",
		ParentGoalID:   "goal-stalled",
		ParentThreadID: "thread-stalled",
		Status:         "running",
		Background:     true,
		ArtifactPath:   "/tmp/analytix/job-stalled.log",
		StartedAt:      now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		UpdatedAt:      now.Add(-45 * time.Second).Format(time.RFC3339Nano),
	}

	diagnostics := taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if !boolValue(diagnostics, "stalled") || boolValue(diagnostics, "terminal") {
		t.Fatalf("running idle background job should be stalled: %#v", diagnostics)
	}
	if stringValue(diagnostics, "warningCode") != "task_job_stale" ||
		stringValue(diagnostics, "heartbeatStatus") != "stale" ||
		numericValue(diagnostics, "idleMs") < 45000 ||
		numericValue(diagnostics, "heartbeatAgeMs") < 45000 ||
		!listContains(diagnostics, "suggestedTools", "bash_output") ||
		listContains(diagnostics, "suggestedTools", "steer_job") ||
		!strings.Contains(stringValue(diagnostics, "suggestion"), "job-stalled") {
		t.Fatalf("stalled diagnostics should include warning and idle duration: %#v", diagnostics)
	}

	record.Kind = "subagent"
	controlDiagnostics := taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if !listContains(controlDiagnostics, "suggestedTools", "steer_job") ||
		!listContains(controlDiagnostics, "suggestedTools", "pause_job") ||
		!strings.Contains(stringValue(controlDiagnostics, "suggestion"), "append guidance") {
		t.Fatalf("internal stalled subagent diagnostics should retain steer/pause tools: %#v", controlDiagnostics)
	}
	publicDiagnostics := TaskJobDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if publicDiagnostics["outputWithheld"] != true || publicDiagnostics["canReadOutput"] != false {
		t.Fatalf("public task-job diagnostics must fail closed: %#v", publicDiagnostics)
	}

	record.Kind = "background-shell"
	record.Status = "completed"
	record.FinishedAt = now.Format(time.RFC3339Nano)
	diagnostics = taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if boolValue(diagnostics, "stalled") || !boolValue(diagnostics, "terminal") {
		t.Fatalf("terminal job must not be stalled: %#v", diagnostics)
	}

	record.Status = string(domainjob.StatusPaused)
	record.FinishedAt = ""
	record.PauseState = domainjob.ChildRunPauseState{
		PauseRequestID: "pause-1",
		Status:         "paused",
		Paused:         true,
		LastPausedAt:   now.Add(-40 * time.Second).Format(time.RFC3339Nano),
	}
	diagnostics = taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if boolValue(diagnostics, "stalled") || !boolValue(diagnostics, "paused") ||
		stringValue(diagnostics, "pauseStatus") != "paused" ||
		stringValue(diagnostics, "heartbeatStatus") != "paused" {
		t.Fatalf("paused job should not be marked stalled: %#v", diagnostics)
	}
}

func TestTaskJobDiagnosticsClassifiesLeaseRecoveryAndDeadLetter(t *testing.T) {
	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:              "job-lease",
		Status:          "running",
		Background:      true,
		StartedAt:       now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
		UpdatedAt:       now.Add(-10 * time.Second).Format(time.RFC3339Nano),
		LastHeartbeatAt: now.Add(-10 * time.Second).Format(time.RFC3339Nano),
		LeaseOwner:      "analytix-runtime",
		LeaseExpiresAt:  now.Add(-time.Second).Format(time.RFC3339Nano),
		StaleAfterMs:    int64((30 * time.Second).Milliseconds()),
	}
	diagnostics := taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if stringValue(diagnostics, "heartbeatStatus") != "lease_expired" ||
		!boolValue(diagnostics, "leaseExpired") ||
		stringValue(diagnostics, "warningCode") != "task_job_lease_expired" ||
		stringValue(diagnostics, "leaseOwner") != "analytix-runtime" {
		t.Fatalf("lease-expired diagnostics mismatch: %#v", diagnostics)
	}

	record.LeaseExpiresAt = now.Add(time.Minute).Format(time.RFC3339Nano)
	record.Orphaned = true
	diagnostics = taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if stringValue(diagnostics, "heartbeatStatus") != "orphaned" ||
		!boolValue(diagnostics, "orphaned") ||
		stringValue(diagnostics, "warningCode") != "task_job_orphaned" {
		t.Fatalf("orphaned diagnostics mismatch: %#v", diagnostics)
	}

	record.RecoveryStatus = "recovered"
	record.RecoveryAttempt = 2
	record.RecoveryReason = "runtime_restart_orphaned_running_job"
	diagnostics = taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if stringValue(diagnostics, "heartbeatStatus") != "recovered" ||
		numericValue(diagnostics, "recoveryAttempt") != 2 ||
		stringValue(diagnostics, "recoveryReason") != "runtime_restart_orphaned_running_job" {
		t.Fatalf("recovered diagnostics mismatch: %#v", diagnostics)
	}

	record.CompletionDeliveryStatus = "dead_letter"
	record.DeadLetterReason = "parent_missing"
	diagnostics = taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if stringValue(diagnostics, "heartbeatStatus") != "dead_lettered" ||
		stringValue(diagnostics, "deadLetterReason") != "parent_missing" ||
		stringValue(diagnostics, "warningCode") != "task_job_dead_lettered" {
		t.Fatalf("dead-letter diagnostics mismatch: %#v", diagnostics)
	}
}

func TestTaskJobOutputViewWithholdsWithoutParsingPrivateBytes(t *testing.T) {
	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:        "job-output",
		Status:    "running",
		Output:    "alpha\nbeta\ngamma\n",
		StartedAt: now.Add(-time.Second).Format(time.RFC3339Nano),
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
	view, err := TaskJobOutputViewWithFilter(record, -10, 12, "^alpha$", now, DefaultTaskJobStalledAfter)
	if err != nil {
		t.Fatalf("output view should filter: %v", err)
	}
	assertTaskJobOutputWithheldV1(t, view, record.ID, record.Status)
	if _, ok := view["output"]; ok {
		t.Fatalf("task-job output view exposed private bytes: %#v", view)
	}
	invalidFilter, err := TaskJobOutputViewWithFilter(record, 0, 0, "[", now, DefaultTaskJobStalledAfter)
	if err != nil {
		t.Fatalf("withheld output must not parse a caller filter: %v", err)
	}
	assertTaskJobOutputWithheldV1(t, invalidFilter, record.ID, record.Status)
}

func TestTaskJobOutputViewSurfacesWorktreeIsolationMetadata(t *testing.T) {
	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:             "job-iso",
		Status:         "completed",
		Output:         "done",
		IsolationMode:  "worktree",
		WorktreePath:   "/tmp/analytix/subagent-worktrees/thr/job-iso",
		WorktreeBranch: "codex/subagent/thr/job-iso",
		BaseCommit:     "base",
		CurrentCommit:  "current",
		MergeStatus:    "not_requested",
		DiffSummary:    "1 file changed",
		ChangedFiles:   []domainjob.ChangedFile{{Path: "src/app.go", Status: "M"}},
		StartedAt:      now.Format(time.RFC3339Nano),
		UpdatedAt:      now.Format(time.RFC3339Nano),
	}
	view, _, err := TaskJobOutputResult(record, TaskJobOutputRequest{JobID: "job-iso"}, 0, now, DefaultTaskJobStalledAfter)
	if err != nil {
		t.Fatalf("output result: %v", err)
	}
	assertTaskJobOutputWithheldV1(t, view, record.ID, record.Status)
	for _, forbidden := range []string{"isolationMode", "worktreePath", "worktreeBranch", "changedFiles", "changedFileCount", "output"} {
		if _, ok := view[forbidden]; ok {
			t.Fatalf("withheld task output exposed %q: %#v", forbidden, view)
		}
	}
}

func TestBackgroundJobContextRecordsAndNote(t *testing.T) {
	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	records := []domainjob.Record{
		{ID: "ignore", Kind: "subagent", Background: false, Status: "completed", UpdatedAt: now.Format(time.RFC3339Nano)},
		{ID: "old", Kind: "subagent", Background: true, Status: "completed", Output: "old output", UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano)},
		{ID: "new", Kind: "background-shell", Background: true, Status: "running", Label: "PRIVATE_LABEL", ArtifactPath: "/tmp/PRIVATE_PATH", Output: "hidden until terminal PRIVATE_OUTPUT", Error: "shell failed PRIVATE_ERROR", StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), UpdatedAt: now.Add(-45 * time.Second).Format(time.RFC3339Nano)},
	}
	filtered := BackgroundJobContextRecords(records, 1)
	if len(filtered) != 1 || filtered[0].ID != "new" {
		t.Fatalf("background context should keep newest background record: %#v", filtered)
	}
	note := BackgroundJobContextNote(filtered, now, DefaultTaskJobStalledAfter, 32)
	if !strings.Contains(note, "<background-jobs>") ||
		!strings.Contains(note, "terminal=false") ||
		!strings.Contains(note, "outputWithheld=true") ||
		!strings.Contains(note, "factAnswerAllowed=false") {
		t.Fatalf("background context note mismatch:\n%s", note)
	}
	for _, forbidden := range []string{"PRIVATE_LABEL", "PRIVATE_PATH", "PRIVATE_OUTPUT", "PRIVATE_ERROR", "hidden until terminal", "shell failed", "artifactPath=", " output=", " error=", "errorBytes=", "outputBytes=", "stalled=", "heartbeatStatus=", "diagnostics"} {
		if strings.Contains(note, forbidden) {
			t.Fatalf("background context note leaked untrusted content %q:\n%s", forbidden, note)
		}
	}
}

func TestTaskJobsTerminalAndBoundedText(t *testing.T) {
	if !TaskJobsTerminal(nil) || !TaskJobsTerminal([]domainjob.Record{{Status: "timeout"}}) {
		t.Fatalf("empty and timeout jobs should be terminal")
	}
	if TaskJobsTerminal([]domainjob.Record{{Status: "running"}}) {
		t.Fatalf("running job should not be terminal")
	}
	bounded := BoundedText(strings.Repeat("x", 50), 30)
	if !strings.Contains(bounded, "[truncated]") || len([]rune(bounded)) > 50 {
		t.Fatalf("bounded text should include truncation marker: %q", bounded)
	}
}

func TestTaskJobToolRequestsAndResponses(t *testing.T) {
	wait := TaskJobWaitRequestFromArgs(map[string]any{
		"jobIds":          []any{"job-a", "job-b", "job-a"},
		"timeout_seconds": float64(99),
	})
	if wait.TimeoutMS != DefaultTaskJobWaitTimeoutMS || len(wait.JobIDs) != 2 || wait.JobIDs[0] != "job-a" || wait.JobIDs[1] != "job-b" {
		t.Fatalf("wait request mismatch: %#v", wait)
	}
	output, validation, invalid := TaskJobOutputRequestFromArgs(map[string]any{
		"job_id": "job-a",
		"cursor": float64(4),
		"limit":  float64(8),
		"filter": "alpha",
		"tail":   true,
		"since":  "2026-06-26T05:30:00Z",
	})
	if invalid || validation != nil || output.JobID != "job-a" || output.Offset != 4 || output.Limit != 8 ||
		output.Filter != "alpha" || !output.ExplicitOffset || !output.Tail || output.Since != "2026-06-26T05:30:00Z" {
		t.Fatalf("output request mismatch: %#v validation=%#v invalid=%v", output, validation, invalid)
	}
	background := true
	list, validation, invalid := TaskJobListRequestFromArgs(map[string]any{"status": "running", "background": true, "limit": float64(2)})
	if invalid || validation != nil || list.Status != "running" || list.Background == nil || *list.Background != background || list.Limit != 2 {
		t.Fatalf("list request mismatch: %#v", list)
	}
	if _, validation, invalid := TaskJobListRequestFromArgs(map[string]any{"status": "running PRIVATE_STATUS"}); !invalid || stringValue(validation, "code") != "validation_error" {
		t.Fatalf("unknown list status should fail closed: %#v invalid=%v", validation, invalid)
	}
	if _, validation, invalid := TaskJobOutputRequestFromArgs(map[string]any{}); !invalid || stringValue(validation, "code") != "validation_error" {
		t.Fatalf("missing output job_id should validate: %#v invalid=%v", validation, invalid)
	}
	kill, validation, invalid := TaskJobKillRequestFromArgs(map[string]any{"id": "job-a"})
	if invalid || validation != nil || kill.JobID != "job-a" || kill.Reason != "killed by parent" {
		t.Fatalf("kill request mismatch: %#v validation=%#v invalid=%v", kill, validation, invalid)
	}

	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := domainjob.Record{ID: "job-a", ParentThreadID: "thread-a", Kind: "background-shell", Status: "running", Output: "0123456789", StartedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	if !TaskJobRecordAllowed(record, "thread-a") || TaskJobRecordAllowed(record, "thread-b") {
		t.Fatalf("parent thread allow check mismatch")
	}
	view, nextOffset, err := TaskJobOutputResult(record, TaskJobOutputRequest{JobID: "job-a", Limit: 3}, 2, now, DefaultTaskJobStalledAfter)
	if err != nil || nextOffset != 0 {
		t.Fatalf("output result mismatch: view=%#v next=%d err=%v", view, nextOffset, err)
	}
	assertTaskJobOutputWithheldV1(t, view, record.ID, record.Status)
	tail, nextOffset, err := TaskJobOutputResult(record, TaskJobOutputRequest{JobID: "job-a", Limit: 4, Tail: true}, 0, now, DefaultTaskJobStalledAfter)
	if err != nil || nextOffset != 0 {
		t.Fatalf("tail output result mismatch: view=%#v next=%d err=%v", tail, nextOffset, err)
	}
	assertTaskJobOutputWithheldV1(t, tail, record.ID, record.Status)
	since, nextOffset, err := TaskJobOutputResult(record, TaskJobOutputRequest{JobID: "job-a", Limit: 4, Since: now.Add(time.Second).Format(time.RFC3339Nano)}, 0, now, DefaultTaskJobStalledAfter)
	if err != nil || nextOffset != 0 {
		t.Fatalf("since output result mismatch: view=%#v next=%d err=%v", since, nextOffset, err)
	}
	assertTaskJobOutputWithheldV1(t, since, record.ID, record.Status)
	if response, isError := TaskJobWaitResult(nil, []string{"job-a"}, now, DefaultTaskJobStalledAfter); !isError || stringValue(response, "code") != "not_found" {
		t.Fatalf("empty wait result should be not_found: %#v error=%v", response, isError)
	}
	if killView := TaskJobKillResult(record, now, DefaultTaskJobStalledAfter); killView["job"] == nil {
		t.Fatalf("kill result should include job view: %#v", killView)
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
	case int64:
		return float64(value)
	case int:
		return float64(value)
	default:
		return 0
	}
}

func listContains(record map[string]any, key string, expected string) bool {
	values, _ := record[key].([]any)
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
