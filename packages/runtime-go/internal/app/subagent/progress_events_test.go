package subagent

import (
	"fmt"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestBuildSubagentProgressEventsMaterializesPipelineAndToolProgress(t *testing.T) {
	record := domainjob.Record{
		ID:               "child-1",
		Label:            "Review",
		Name:             "review",
		ChildThreadID:    "thread-child",
		ParentThreadID:   "thread-parent",
		ParentTurnID:     "turn-parent",
		ParentToolCallID: "call-parent",
	}
	events := BuildSubagentProgressEvents(SubagentProgressEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "task",
		Record:         record,
		Status:         "completed",
		ProgressStatus: "success",
		Message:        "done",
	})
	if len(events) != 2 {
		t.Fatalf("expected pipeline and progress events, got %#v", events)
	}
	stage := events[0]
	if stage["kind"] != "pipeline_stage" ||
		stage["stage"] != "subagent_completed" ||
		stage["message"] != securityBoundOutputMessage || stage["label"] != nil {
		t.Fatalf("pipeline event mismatch: %#v", stage)
	}
	progress := events[1]
	if progress["kind"] != "tool_progress" ||
		progress["itemId"] != "item-1" ||
		progress["callId"] != "call-1" ||
		progress["toolName"] != "task" ||
		progress["summary"] != nil ||
		progress["status"] != "success" ||
		progress["message"] != securityBoundOutputMessage {
		t.Fatalf("progress event mismatch: %#v", progress)
	}
	child, ok := progress["child"].(map[string]any)
	if !ok || child["childId"] != "child-1" || child["outputWithheld"] != true || child["factAnswerAllowed"] != false || child["contract"] != nil {
		t.Fatalf("progress child mismatch: %#v", progress)
	}
}

func TestBuildSubagentProgressEventsAddsBackgroundCompletionNotification(t *testing.T) {
	record := domainjob.Record{
		ID:               "child-1",
		Label:            "Background child",
		Kind:             "subagent",
		Status:           string(domainjob.StatusCompleted),
		Background:       true,
		ChildThreadID:    "thread-child",
		ChildTurnID:      "turn-child",
		ParentThreadID:   "thread-parent",
		ParentTurnID:     "turn-parent",
		ParentToolCallID: "call-parent",
		Output:           "done",
	}
	events := BuildSubagentProgressEvents(SubagentProgressEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "task",
		Record:         record,
		Status:         string(domainjob.StatusCompleted),
		ProgressStatus: "success",
	})
	if len(events) != 3 {
		t.Fatalf("expected subagent progress events plus background notification, got %#v", events)
	}
	if events[2]["stage"] != "background_job_completed" {
		t.Fatalf("background notification missing: %#v", events)
	}
}

func TestBuildTaskJobProgressEventMaterializesChildAndDefaultsMessage(t *testing.T) {
	record := domainjob.Record{
		ID:               "job-1",
		Label:            "Background Job",
		Name:             "bash",
		ParentThreadID:   "thread-parent",
		ParentTurnID:     "turn-parent",
		ParentToolCallID: "call-parent",
		Kind:             "bash",
		Output:           "hello",
	}
	event, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "wait",
		Record:         record,
		Status:         "completed",
		ProgressStatus: "success",
	})
	if !ok {
		t.Fatalf("task job progress event should be built")
	}
	if event["kind"] != "tool_progress" ||
		event["status"] != "success" ||
		event["message"] != securityBoundOutputMessage || event["summary"] != nil {
		t.Fatalf("task job progress event mismatch: %#v", event)
	}
	child, ok := event["child"].(map[string]any)
	if !ok || child["childId"] != "job-1" || child["outputBytes"] != nil || child["outputWithheld"] != true {
		t.Fatalf("task job child mismatch: %#v", event)
	}
}

func TestBuildTaskJobProgressEventRejectsMissingIdentity(t *testing.T) {
	record := domainjob.Record{ID: "job-1"}
	if event, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{ItemID: "", CallID: "call-1", Record: record}); ok || event != nil {
		t.Fatalf("missing item id should reject event: %#v", event)
	}
	if event, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{ItemID: "item-1", CallID: "", Record: record}); ok || event != nil {
		t.Fatalf("missing call id should reject event: %#v", event)
	}
	if event, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{ItemID: "item-1", CallID: "call-1", Record: domainjob.Record{ID: " "}}); ok || event != nil {
		t.Fatalf("missing record id should reject event: %#v", event)
	}
}

func TestFailedJobProgressUsesClosedFailureProjection(t *testing.T) {
	const sentinel = "PRIVATE_PROVIDER_ERROR_6222020202020202020"
	record := domainjob.Record{
		ID: "job-failed", Kind: "background-shell", Status: string(domainjob.StatusFailed), FailureCode: domainjob.FailureChildExecutionFailed,
		ParentThreadID: "thread", ParentTurnID: "turn", ParentToolCallID: "call",
	}
	event, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", ToolName: "wait",
		Record: record, Status: string(domainjob.StatusFailed), ProgressStatus: "error", Message: sentinel,
	})
	if !ok || event["message"] != securityBoundOutputMessage || strings.Contains(fmt.Sprint(event), sentinel) {
		t.Fatalf("failed task progress reflected raw error: %#v", event)
	}
	child, _ := event["child"].(map[string]any)
	if child["failureCode"] != nil || child["error"] != nil || child["outputWithheld"] != true {
		t.Fatalf("failed task child escaped the fixed metadata projection: %#v", child)
	}
	events := BuildSubagentProgressEvents(SubagentProgressEventInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", ToolName: "task",
		Record: record, Status: string(domainjob.StatusFailed), ProgressStatus: "error", Message: sentinel,
	})
	if len(events) != 2 || events[0]["message"] != securityBoundOutputMessage || events[1]["message"] != securityBoundOutputMessage || strings.Contains(fmt.Sprint(events), sentinel) {
		t.Fatalf("failed subagent progress reflected raw error: %#v", events)
	}
}

func TestEveryJobProgressBuilderUsesClosedStatus(t *testing.T) {
	privateStatus := "completed<think>PRIVATE_STATUS</think>"
	record := domainjob.Record{
		ID: "job-closed-status", Kind: "background-shell", Status: string(domainjob.StatusCompleted), Background: true,
		ParentThreadID: "thread", ParentTurnID: "turn", ParentToolCallID: "call", Label: "ordinary diagnostic",
	}
	events := BuildSubagentProgressEvents(SubagentProgressEventInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", ToolName: "task",
		Record: record, Status: privateStatus, ProgressStatus: privateStatus,
	})
	if len(events) < 2 || events[0]["stage"] != "subagent_unknown" || events[1]["status"] != "unknown" ||
		strings.Contains(fmt.Sprint(events), "PRIVATE_STATUS") {
		t.Fatalf("subagent progress reflected unknown status: %#v", events)
	}

	task, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", ToolName: "wait",
		Record: record, Status: privateStatus, ProgressStatus: privateStatus,
	})
	if !ok || task["status"] != "unknown" || strings.Contains(fmt.Sprint(task), "PRIVATE_STATUS") {
		t.Fatalf("task progress reflected unknown status: %#v", task)
	}

	notification, ok := BuildBackgroundJobCompletionNotificationEvent(JobLifecycleEventInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", ToolName: "task",
		Record: record, Status: privateStatus,
	})
	if !ok || notification["stage"] != "background_job_unknown" || strings.Contains(fmt.Sprint(notification), "PRIVATE_STATUS") {
		t.Fatalf("completion notification reflected unknown status: %#v", notification)
	}
	item, ok := BuildBackgroundJobCompletionNotificationItem(BackgroundJobCompletionNotificationItemInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", ToolName: "task",
		Event: map[string]any{
			"kind": "pipeline_stage", "stage": "background_job_" + privateStatus,
			"details": map[string]any{"notificationKind": "background_job_completion", "jobId": record.ID, "status": privateStatus},
		},
	})
	if !ok || strings.Contains(fmt.Sprint(item), "PRIVATE_STATUS") {
		t.Fatalf("completion replay item reflected unknown status: %#v", item)
	}
	arguments, _ := item["arguments"].(map[string]any)
	if arguments["stage"] != "background_job_unknown" || arguments["status"] != "unknown" {
		t.Fatalf("completion replay status was not closed: %#v", item)
	}

	delivery, ok := BuildBackgroundJobDeliveryLedgerItem(BackgroundJobDeliveryLedgerItemInput{
		ThreadID: "thread", TurnID: "turn", ItemID: "item", CallID: "call", Record: record, Status: privateStatus,
	})
	if !ok || strings.Contains(fmt.Sprint(delivery), "PRIVATE_STATUS") ||
		BackgroundJobDeliveryLedgerMessage(privateStatus, "", "") != "background job completion delivery status unavailable" ||
		strings.Contains(BackgroundAutoContinueStatusMessage(privateStatus, "", ""), "PRIVATE_STATUS") {
		t.Fatalf("background lifecycle reflected unknown status: delivery=%#v", delivery)
	}
	if value := BackgroundCompletionDeliveryItemID("turn", "delivery", privateStatus); strings.Contains(value, "PRIVATE") || !strings.Contains(value, "unknown") {
		t.Fatalf("delivery item id reflected unknown status: %q", value)
	}
	if value := BackgroundAutoContinueItemID("turn", record.ID, privateStatus); strings.Contains(value, "PRIVATE") || !strings.Contains(value, "unknown") {
		t.Fatalf("auto-continue item id reflected unknown status: %q", value)
	}
}

func TestBackgroundAutoContinueIDsPreserveEveryClosedStatus(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, status := range []string{"starting", "started", "skipped", "failed"} {
		itemID := BackgroundAutoContinueItemID("turn", "job", status)
		callID := BackgroundAutoContinueCallID("job", status)
		if !strings.Contains(itemID, status) || !strings.Contains(callID, status) || seen[itemID] {
			t.Fatalf("closed auto-continue status lost identity: status=%q item=%q call=%q", status, itemID, callID)
		}
		seen[itemID] = true
	}
}

func TestBuildJobLifecycleEventsAddsBackgroundCompletionNotification(t *testing.T) {
	record := domainjob.Record{
		ID:               "job-1",
		Label:            "Research",
		Name:             "research",
		Kind:             "subagent",
		Status:           string(domainjob.StatusCompleted),
		Background:       true,
		ParentThreadID:   "thread-parent",
		ParentTurnID:     "turn-parent",
		ParentToolItemID: "item-1",
		ParentToolCallID: "call-1",
		ChildThreadID:    "thread-child",
		ChildTurnID:      "turn-child",
		Output:           "completed child result",
	}
	events := BuildJobLifecycleEvents(JobLifecycleEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "task",
		Record:         record,
		Status:         string(domainjob.StatusCompleted),
		ProgressStatus: "success",
	})
	if len(events) != 3 {
		t.Fatalf("expected subagent lifecycle plus background completion notification, got %#v", events)
	}
	notification := events[2]
	if notification["kind"] != "pipeline_stage" ||
		notification["stage"] != "background_job_completed" ||
		notification["label"] != nil ||
		notification["message"] != securityBoundOutputMessage {
		t.Fatalf("notification mismatch: %#v", notification)
	}
	details, ok := notification["details"].(map[string]any)
	if !ok ||
		details["notificationKind"] != "background_job_completion" ||
		details["jobId"] != "job-1" ||
		details["status"] != string(domainjob.StatusCompleted) ||
		details["terminal"] != true ||
		details["canContinueParent"] != false ||
		details["outputWithheld"] != true ||
		details["outputPreview"] != nil {
		t.Fatalf("notification details mismatch: %#v", notification)
	}
	child, ok := notification["child"].(map[string]any)
	if !ok || child["childRunId"] != "job-1" || child["childThreadId"] != nil || child["outputWithheld"] != true || child["canReadOutput"] != false {
		t.Fatalf("notification child mismatch: %#v", notification)
	}
	if strings.Contains(fmt.Sprint(notification), "completed child result") {
		t.Fatalf("legacy unbound completion notification leaked raw output: %#v", notification)
	}
	if _, hasError := child["error"]; hasError {
		t.Fatalf("completed background notification child should not be marked as error: %#v", notification)
	}
}

func TestBuildBackgroundJobCompletionNotificationReflectsSuppressionAndAutoContinue(t *testing.T) {
	record := domainjob.Record{
		ID:                       "job-2",
		Label:                    "Late child",
		Kind:                     "subagent",
		Status:                   string(domainjob.StatusCompleted),
		Background:               true,
		AutoContinueParent:       true,
		AutoContinueStatus:       "skipped",
		AutoContinueReason:       "parent_turn_not_latest",
		LateCompletionSuppressed: true,
		LateCompletionReason:     "terminal update completed suppressed after killed",
		Output:                   "late output",
	}
	event, ok := BuildBackgroundJobCompletionNotificationEvent(JobLifecycleEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "task",
		Record:         record,
		Status:         string(domainjob.StatusCompleted),
		ProgressStatus: "success",
	})
	if !ok {
		t.Fatalf("expected notification event")
	}
	details, ok := event["details"].(map[string]any)
	if !ok || details["canContinueParent"] != false || details["outputWithheld"] != true ||
		details["lateCompletionSuppressed"] != nil || details["autoContinueParent"] != nil ||
		details["autoContinueStatus"] != nil || details["autoContinueReason"] != nil || details["lateCompletionReason"] != nil {
		t.Fatalf("suppressed notification details mismatch: %#v", event)
	}
}

func TestBuildBackgroundJobCompletionNotificationItemMaterializesReplayableToolProgress(t *testing.T) {
	event := map[string]any{
		"kind":    "pipeline_stage",
		"stage":   "background_job_completed",
		"label":   "Research",
		"message": "background job completed",
		"child": map[string]any{
			"parentThreadId": "thread-parent",
			"parentTurnId":   "turn-parent",
			"childId":        "job-1",
			"childRunId":     "job-1",
			"childStatus":    "completed",
		},
		"details": map[string]any{
			"notificationKind":  "background_job_completion",
			"jobId":             "job-1",
			"status":            "completed",
			"terminal":          true,
			"background":        true,
			"canContinueParent": true,
			"outputPreview":     "child summary",
		},
	}
	item, ok := BuildBackgroundJobCompletionNotificationItem(BackgroundJobCompletionNotificationItemInput{
		ThreadID:  "thread-parent",
		TurnID:    "turn-parent",
		ItemID:    "item-progress",
		CallID:    "call-task",
		ToolName:  "task",
		CreatedAt: "2026-07-07T00:00:00Z",
		Event:     event,
	})
	if !ok {
		t.Fatalf("expected replay item")
	}
	if item["kind"] != "tool_progress" ||
		item["status"] != "completed" ||
		item["toolKind"] != "subagent" ||
		item["summary"] != "job-1" ||
		item["callId"] != "call-task" {
		t.Fatalf("item mismatch: %#v", item)
	}
	args, ok := item["arguments"].(map[string]any)
	if !ok ||
		args["runtimeStatus"] != "tool_progress" ||
		args["stage"] != "background_job_completed" ||
		args["summary"] != "background job completed" {
		t.Fatalf("arguments mismatch: %#v", item)
	}
	diagnostics, ok := args["diagnostics"].(map[string]any)
	if !ok || diagnostics["canContinueParent"] != true || diagnostics["outputPreview"] != nil {
		t.Fatalf("diagnostics mismatch: %#v", item)
	}
	child, ok := args["child"].(map[string]any)
	if !ok || child["childRunId"] != "job-1" || child["childStatus"] != "completed" {
		t.Fatalf("child mismatch: %#v", item)
	}
}

func TestBuildJobLifecycleEventsSkipsCompletionNotificationForForegroundJob(t *testing.T) {
	record := domainjob.Record{
		ID:               "job-1",
		Label:            "Foreground",
		Kind:             "subagent",
		Status:           string(domainjob.StatusCompleted),
		ParentThreadID:   "thread-parent",
		ParentTurnID:     "turn-parent",
		ParentToolItemID: "item-1",
		ParentToolCallID: "call-1",
	}
	events := BuildJobLifecycleEvents(JobLifecycleEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "task",
		Record:         record,
		Status:         string(domainjob.StatusCompleted),
		ProgressStatus: "success",
	})
	if len(events) != 2 {
		t.Fatalf("foreground subagent should not add background notification: %#v", events)
	}
}

func TestBuildJobLifecycleEventsAddsBackgroundKilledNotificationForShell(t *testing.T) {
	record := domainjob.Record{
		ID:               "shell-1",
		Label:            "Shell",
		Kind:             "background-shell",
		Status:           string(domainjob.StatusKilled),
		Background:       true,
		ParentThreadID:   "thread-parent",
		ParentTurnID:     "turn-parent",
		ParentToolItemID: "item-1",
		ParentToolCallID: "call-1",
		Output:           "partial output",
		Error:            "stopped by user",
	}
	events := BuildJobLifecycleEvents(JobLifecycleEventInput{
		ThreadID:       "thread-parent",
		TurnID:         "turn-parent",
		ItemID:         "item-1",
		CallID:         "call-1",
		ToolName:       "bash_output",
		Record:         record,
		Status:         string(domainjob.StatusKilled),
		ProgressStatus: "error",
	})
	if len(events) != 2 {
		t.Fatalf("expected shell progress plus background notification, got %#v", events)
	}
	notification := events[1]
	if notification["stage"] != "background_job_killed" {
		t.Fatalf("killed notification stage mismatch: %#v", notification)
	}
	details, ok := notification["details"].(map[string]any)
	if !ok || details["canContinueParent"] != false || details["error"] != nil || details["outputPreview"] != nil || details["outputBytes"] != nil || details["outputWithheld"] != true {
		t.Fatalf("killed notification details mismatch: %#v", notification)
	}
	child, ok := notification["child"].(map[string]any)
	if !ok || child["kind"] != "subagent_task" || child["childStatus"] != string(domainjob.StatusKilled) {
		t.Fatalf("killed notification child mismatch: %#v", notification)
	}
}

func TestBackgroundDeliveryLedgerWithholdsRawErrorAndUnknownReason(t *testing.T) {
	item, ok := BuildBackgroundJobDeliveryLedgerItem(BackgroundJobDeliveryLedgerItemInput{
		ThreadID: "thread-parent", TurnID: "turn-parent", ItemID: "item-delivery", CallID: "call-delivery",
		CreatedAt: "2026-07-07T00:00:00Z", Status: "dead_letter",
		Record: domainjob.Record{
			ID: "job-raw-error", Kind: "background-shell", Status: string(domainjob.StatusFailed), Background: true,
		},
		Reason: "PRIVATE database reason account=6222020202020202020",
		Error:  `<think>PRIVATE_DELIVERY_REASONING</think>`,
	})
	if !ok {
		t.Fatal("expected metadata-only delivery ledger item")
	}
	encoded := fmt.Sprint(item)
	for _, forbidden := range []string{"PRIVATE", "6222020202020202020", "<think>", "deliveryError", "outputPreview"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("delivery ledger leaked %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, securityBoundOutputMessage) || strings.Contains(encoded, domainjob.OperationalReasonWithheld) {
		t.Fatalf("delivery ledger did not use the fixed metadata-only message: %s", encoded)
	}
}
