package subagent

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const ordinaryTaskJobSummarySentinel = "PRIVATE_TASK_JOB_SUMMARY_6E1B"

func TestTaskJobRecordViewUsesClosedOrdinarySummaryAllowlist(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 19, 9, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:                         "job-summary",
		ChildSeq:                   4,
		ParentGoalID:               ordinaryTaskJobSummarySentinel,
		ParentGoalObjective:        ordinaryTaskJobSummarySentinel,
		ParentThreadID:             ordinaryTaskJobSummarySentinel,
		ParentTurnID:               ordinaryTaskJobSummarySentinel,
		ParentToolItemID:           ordinaryTaskJobSummarySentinel,
		ParentToolCallID:           ordinaryTaskJobSummarySentinel,
		Kind:                       "background-shell",
		Name:                       ordinaryTaskJobSummarySentinel,
		Label:                      ordinaryTaskJobSummarySentinel,
		Prompt:                     ordinaryTaskJobSummarySentinel,
		Status:                     string(domainjob.StatusFailed),
		Workspace:                  "/" + ordinaryTaskJobSummarySentinel,
		Model:                      ordinaryTaskJobSummarySentinel,
		ProviderID:                 ordinaryTaskJobSummarySentinel,
		ModelExecution:             map[string]any{"privatePayload": ordinaryTaskJobSummarySentinel},
		Background:                 true,
		AutoContinueStatus:         "PRIVATE_STATUS_" + ordinaryTaskJobSummarySentinel,
		AutoContinueTurnID:         ordinaryTaskJobSummarySentinel,
		AutoContinueReason:         ordinaryTaskJobSummarySentinel,
		AutoContinueError:          ordinaryTaskJobSummarySentinel,
		CompletionDeliveryID:       ordinaryTaskJobSummarySentinel,
		CompletionDeliveryStatus:   "PRIVATE_STATUS_" + ordinaryTaskJobSummarySentinel,
		CompletionDeliveryItemID:   ordinaryTaskJobSummarySentinel,
		CompletionDeliveryReason:   ordinaryTaskJobSummarySentinel,
		CompletionDeliveryError:    ordinaryTaskJobSummarySentinel,
		CompletionDeliveryAt:       ordinaryTaskJobSummarySentinel,
		CompletionDeadLetterAt:     ordinaryTaskJobSummarySentinel,
		LeaseOwner:                 ordinaryTaskJobSummarySentinel,
		LeaseExpiresAt:             ordinaryTaskJobSummarySentinel,
		RecoveryStatus:             "PRIVATE_STATUS_" + ordinaryTaskJobSummarySentinel,
		RecoveryReason:             ordinaryTaskJobSummarySentinel,
		RecoveryUpdatedAt:          ordinaryTaskJobSummarySentinel,
		DeadLetterReason:           ordinaryTaskJobSummarySentinel,
		ArtifactPath:               "/" + ordinaryTaskJobSummarySentinel,
		IsolationMode:              "worktree",
		WorktreePath:               "/" + ordinaryTaskJobSummarySentinel,
		WorktreeBranch:             ordinaryTaskJobSummarySentinel,
		DiffSummary:                ordinaryTaskJobSummarySentinel,
		ChangedFiles:               []domainjob.ChangedFile{{Path: ordinaryTaskJobSummarySentinel, Status: "M"}},
		RepairPatchReviews:         []domainjob.RepairPatchReview{{ID: ordinaryTaskJobSummarySentinel, RepairPatch: ordinaryTaskJobSummarySentinel}},
		ChildTodoLists:             []domainjob.ChildTodoList{{ID: ordinaryTaskJobSummarySentinel, Items: []domainjob.ChildTodoItem{{Content: ordinaryTaskJobSummarySentinel}}}},
		ChildTodoProjections:       []domainjob.ChildTodoProjection{{ID: ordinaryTaskJobSummarySentinel, Summary: ordinaryTaskJobSummarySentinel}},
		ProjectionDecisions:        []domainjob.ProjectionDecision{{ID: ordinaryTaskJobSummarySentinel}},
		StartedAt:                  now.Add(-time.Minute).Format(time.RFC3339Nano),
		UpdatedAt:                  now.Format(time.RFC3339Nano),
		Output:                     "completed output " + ordinaryTaskJobSummarySentinel,
		Error:                      ordinaryTaskJobSummarySentinel,
		FailureCode:                ordinaryTaskJobSummarySentinel,
		Usage:                      map[string]any{"privatePayload": ordinaryTaskJobSummarySentinel, "totalTokens": float64(17)},
		Steers:                     []domainjob.SteerMessage{{ID: ordinaryTaskJobSummarySentinel, Text: ordinaryTaskJobSummarySentinel, ContentDigest: ordinaryTaskJobSummarySentinel}},
		PauseRequests:              []domainjob.PauseRequest{{ID: ordinaryTaskJobSummarySentinel, ResumeToken: ordinaryTaskJobSummarySentinel}},
		PauseState:                 domainjob.ChildRunPauseState{PauseRequestID: ordinaryTaskJobSummarySentinel, Status: ordinaryTaskJobSummarySentinel, LastPausedAt: ordinaryTaskJobSummarySentinel},
		CompletionDeliveryAttempts: 3,
	}

	view := TaskJobRecordView(record, now, DefaultTaskJobStalledAfter)
	wantKeys := []string{
		"background", "canContinueParent", "canReadOutput", "evidenceAuthority", "factAnswerAllowed", "id", "kind",
		"outputTrustStatus", "outputWithheld", "schemaVersion", "status", "terminal",
	}
	if got := sortedTaskJobSummaryKeys(view); !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("ordinary task-job summary must stay an exact allowlist: got=%v want=%v", got, wantKeys)
	}
	if view["schemaVersion"] != 1 || view["id"] != record.ID || view["kind"] != "background-shell" || view["status"] != string(domainjob.StatusFailed) ||
		view["factAnswerAllowed"] != false || view["evidenceAuthority"] != false ||
		view["canReadOutput"] != false || view["canContinueParent"] != false || view["outputWithheld"] != true {
		t.Fatalf("ordinary task-job summary identity or authority mismatch: %#v", view)
	}
	forbiddenKeys := map[string]bool{
		"output": true, "error": true, "artifactPath": true, "prompt": true, "workspace": true,
		"modelExecution": true, "usage": true, "name": true, "label": true,
		"worktreePath": true, "worktreeBranch": true, "changedFiles": true, "mergeDecisions": true,
		"repairPatchReviews": true, "repairPatch": true, "childTodoLists": true, "childTodoProjections": true,
		"projectionDecisions": true, "steers": true, "pauseRequests": true, "resumeToken": true,
		"pauseRequestId": true, "deliveryId": true, "deliveryItemId": true, "autoContinueTurnId": true,
		"privatePayload": true,
	}
	assertTaskJobSummaryHasNoKeys(t, view, forbiddenKeys)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal ordinary summary: %v", err)
	}
	if strings.Contains(string(encoded), ordinaryTaskJobSummarySentinel) {
		t.Fatalf("ordinary task-job summary leaked a poisoned record field: %s", encoded)
	}
	wait, waitIsError := TaskJobWaitResult([]domainjob.Record{record}, []string{record.ID}, now, DefaultTaskJobStalledAfter)
	if waitIsError {
		t.Fatalf("ordinary task-job wait summary unexpectedly failed: %#v", wait)
	}
	for surfaceName, surface := range map[string]any{
		"list": map[string]any{"jobs": TaskJobRecordsAnyWithState([]domainjob.Record{record}, now, DefaultTaskJobStalledAfter, nil)},
		"wait": wait,
		"kill": TaskJobKillResult(record, now, DefaultTaskJobStalledAfter),
	} {
		assertTaskJobSummaryHasNoKeys(t, surface, forbiddenKeys)
		encoded, err := json.Marshal(surface)
		if err != nil || strings.Contains(string(encoded), ordinaryTaskJobSummarySentinel) {
			t.Fatalf("ordinary %s response leaked poisoned record data: encoded=%s err=%v", surfaceName, encoded, err)
		}
	}
}

func TestTaskJobRecordsAnyWithStateAddsActiveDuringOrdinaryConstruction(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	ordinary := domainjob.Record{ID: "ordinary", ParentThreadID: "thread-a", Kind: "background-shell", Status: "running", Background: true}
	withheld := poisonedSecurityBoundRecord()
	state := NewRuntimeState()
	state.RegisterBackgroundJob(ordinary.ID, func() {})
	state.RegisterBackgroundJob(withheld.ID, func() {})

	views := TaskJobRecordsAnyWithState([]domainjob.Record{ordinary, withheld}, now, DefaultTaskJobStalledAfter, state)
	if len(views) != 2 {
		t.Fatalf("task-job summary count mismatch: %#v", views)
	}
	ordinaryView, _ := views[0].(map[string]any)
	if ordinaryView["active"] != true {
		t.Fatalf("runtime-active state was not carried as closed metadata: %#v", ordinaryView)
	}
	if !reflect.DeepEqual(ordinaryView, TaskJobMetadataProjectionV1(ordinary, boolPointerV1(true))) {
		t.Fatalf("ordinary task output did not use the fixed metadata projection: got=%#v", ordinaryView)
	}
	withheldView, _ := views[1].(map[string]any)
	if withheldView["active"] != true {
		t.Fatalf("security-bound active state was not closed metadata: %#v", withheldView)
	}
	if !reflect.DeepEqual(withheldView, TaskJobMetadataProjectionV1(withheld, boolPointerV1(true))) {
		t.Fatalf("security-bound projection changed under runtime state: got=%#v", withheldView)
	}
}

func boolPointerV1(value bool) *bool { return &value }

func TestTaskJobDiagnosticsRejectsOpenLeaseAndTimestampText(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC)
	record := domainjob.Record{
		ID: "job-closed-diagnostics", Kind: "background-shell", Status: "running", Background: true,
		StartedAt: now.Add(-time.Second).Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
		LeaseOwner: ordinaryTaskJobSummarySentinel, LeaseExpiresAt: ordinaryTaskJobSummarySentinel,
		RecoveryUpdatedAt:    ordinaryTaskJobSummarySentinel,
		PauseState:           domainjob.ChildRunPauseState{LastPausedAt: ordinaryTaskJobSummarySentinel, LastResumedAt: ordinaryTaskJobSummarySentinel},
		CompletionDeliveryAt: ordinaryTaskJobSummarySentinel, CompletionDeadLetterAt: ordinaryTaskJobSummarySentinel,
	}
	diagnostics := TaskJobDiagnostics(record, now, DefaultTaskJobStalledAfter)
	for _, key := range []string{"leaseOwner", "leaseExpiresAt", "recoveryUpdatedAt", "lastPausedAt", "lastResumedAt", "completionDeliveryAt", "completionDeadLetterAt"} {
		if _, exists := diagnostics[key]; exists {
			t.Fatalf("invalid open diagnostic field %q was reflected: %#v", key, diagnostics)
		}
	}
	encoded, _ := json.Marshal(diagnostics)
	if strings.Contains(string(encoded), ordinaryTaskJobSummarySentinel) {
		t.Fatalf("closed diagnostics reflected poisoned text: %s", encoded)
	}

	record.LeaseOwner = "analytix-runtime:4711"
	diagnostics = taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	if diagnostics["leaseOwner"] != "analytix-runtime" {
		t.Fatalf("runtime lease owner should expose only its closed class: %#v", diagnostics)
	}
}

func TestTaskJobRecordViewRejectsMalformedOpenIdentifierText(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC)
	record := domainjob.Record{
		ID:   "<think>" + ordinaryTaskJobSummarySentinel + "</think>",
		Kind: "background-shell", Status: "running", Background: true,
		StartedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
	view := TaskJobRecordView(record, now, DefaultTaskJobStalledAfter)
	if view["id"] != "" || view["canReadOutput"] != false {
		t.Fatalf("malformed task-job identifier must fail closed: %#v", view)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), ordinaryTaskJobSummarySentinel) || strings.Contains(string(encoded), "<think>") {
		t.Fatalf("malformed task-job identifier was reflected: %s", encoded)
	}
}

func sortedTaskJobSummaryKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertTaskJobSummaryHasNoKeys(t *testing.T, value any, forbidden map[string]bool) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if forbidden[key] {
				t.Fatalf("ordinary task-job summary exposed forbidden key %q: %#v", key, typed)
			}
			assertTaskJobSummaryHasNoKeys(t, nested, forbidden)
		}
	case []any:
		for _, nested := range typed {
			assertTaskJobSummaryHasNoKeys(t, nested, forbidden)
		}
	}
}
