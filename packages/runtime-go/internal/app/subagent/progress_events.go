package subagent

import (
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type SubagentProgressEventInput struct {
	ThreadID       string
	TurnID         string
	ItemID         string
	CallID         string
	ToolName       string
	Record         domainjob.Record
	Status         string
	ProgressStatus string
	Message        string
}

func BuildSubagentProgressEvents(input SubagentProgressEventInput) []map[string]any {
	if ChildOutputRequiresWithholding(input.Record) {
		input.Record.Status = normalizedChildStatus(firstNonEmptyAnyString(input.Status, input.Record.Status))
		progressStatus := normalizedToolProgressStatus(firstNonEmptyAnyString(input.ProgressStatus, input.Record.Status))
		child := SecurityBoundChildOutputProjection(input.Record)
		events := []map[string]any{
			{
				"kind":     "pipeline_stage",
				"threadId": input.ThreadID,
				"turnId":   input.TurnID,
				"stage":    "subagent_" + input.Record.Status,
				"message":  securityBoundOutputMessage,
				"child":    child,
			},
			{
				"kind":     "tool_progress",
				"threadId": input.ThreadID,
				"turnId":   input.TurnID,
				"itemId":   input.ItemID,
				"callId":   input.CallID,
				"toolName": input.ToolName,
				"status":   progressStatus,
				"message":  securityBoundOutputMessage,
				"child":    child,
			},
		}
		if event, ok := BuildBackgroundJobCompletionNotificationEvent(JobLifecycleEventInput{
			ThreadID: input.ThreadID, TurnID: input.TurnID, ItemID: input.ItemID, CallID: input.CallID,
			ToolName: input.ToolName, Record: input.Record, Status: input.Record.Status, ProgressStatus: progressStatus,
		}); ok {
			events = append(events, event)
		}
		return events
	}
	status := normalizedChildStatus(firstNonEmptyAnyString(input.Status, input.Record.Status))
	progressStatus := normalizedToolProgressStatus(firstNonEmptyAnyString(input.ProgressStatus, status))
	input.Record.Status = status
	safeMessage := jobProgressMessageV1("subagent", status, progressStatus, input.Message)
	safeLabel := ProjectOrdinaryJobText(input.Record.Label)
	child := EventChild(input.Record, status, safeMessage)
	events := []map[string]any{
		{
			"kind":     "pipeline_stage",
			"threadId": input.ThreadID,
			"turnId":   input.TurnID,
			"stage":    "subagent_" + status,
			"label":    safeLabel,
			"child":    child,
		},
		{
			"kind":     "tool_progress",
			"threadId": input.ThreadID,
			"turnId":   input.TurnID,
			"itemId":   input.ItemID,
			"callId":   input.CallID,
			"toolName": input.ToolName,
			"summary":  safeLabel,
			"status":   progressStatus,
			"message":  safeMessage,
			"child":    child,
		},
	}
	if event, ok := BuildBackgroundJobCompletionNotificationEvent(JobLifecycleEventInput{
		ThreadID:       input.ThreadID,
		TurnID:         input.TurnID,
		ItemID:         input.ItemID,
		CallID:         input.CallID,
		ToolName:       input.ToolName,
		Record:         input.Record,
		Status:         status,
		ProgressStatus: progressStatus,
		Message:        input.Message,
	}); ok {
		events = append(events, event)
	}
	return events
}

type TaskJobProgressEventInput struct {
	ThreadID       string
	TurnID         string
	ItemID         string
	CallID         string
	ToolName       string
	Record         domainjob.Record
	Status         string
	ProgressStatus string
	Message        string
}

type BackgroundJobCompletionNotificationItemInput struct {
	ThreadID  string
	TurnID    string
	ItemID    string
	CallID    string
	ToolName  string
	CreatedAt string
	Event     map[string]any
}

type BackgroundJobDeliveryLedgerItemInput struct {
	ThreadID  string
	TurnID    string
	ItemID    string
	CallID    string
	CreatedAt string
	Record    domainjob.Record
	Status    string
	Reason    string
	Error     string
}

func BuildTaskJobProgressEvent(input TaskJobProgressEventInput) (map[string]any, bool) {
	if input.ItemID == "" || input.CallID == "" || strings.TrimSpace(input.Record.ID) == "" {
		return nil, false
	}
	status := normalizedChildStatus(firstNonEmptyAnyString(input.Status, input.Record.Status))
	progressStatus := normalizedToolProgressStatus(firstNonEmptyAnyString(input.ProgressStatus, status))
	input.Record.Status = status
	child := TaskJobProgressChild(input.Record, status)
	if ChildOutputRequiresWithholding(input.Record) {
		return map[string]any{
			"kind":     "tool_progress",
			"threadId": input.ThreadID,
			"turnId":   input.TurnID,
			"itemId":   input.ItemID,
			"callId":   input.CallID,
			"toolName": input.ToolName,
			"status":   normalizedToolProgressStatus(firstNonEmptyAnyString(input.ProgressStatus, input.Record.Status)),
			"message":  securityBoundOutputMessage,
			"child":    child,
		}, true
	}
	safeMessage := jobProgressMessageV1("background job", status, progressStatus, input.Message)
	return map[string]any{
		"kind":     "tool_progress",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"itemId":   input.ItemID,
		"callId":   input.CallID,
		"toolName": input.ToolName,
		"summary":  ProjectOrdinaryJobText(input.Record.Label),
		"status":   progressStatus,
		"message":  safeMessage,
		"child":    child,
	}, true
}

func jobProgressMessageV1(prefix, status, progressStatus, message string) string {
	if progressStatus == "error" {
		return prefix + " " + status
	}
	switch status {
	case string(domainjob.StatusQueued), string(domainjob.StatusRunning), string(domainjob.StatusPauseRequested),
		string(domainjob.StatusPaused), string(domainjob.StatusResumeRequested), string(domainjob.StatusResuming),
		string(domainjob.StatusCompleted):
		if projected := ProjectOrdinaryJobText(message); projected != "" {
			return projected
		}
	}
	return prefix + " " + status
}

func BuildBackgroundJobCompletionNotificationItem(input BackgroundJobCompletionNotificationItemInput) (map[string]any, bool) {
	if strings.TrimSpace(input.ThreadID) == "" ||
		strings.TrimSpace(input.TurnID) == "" ||
		strings.TrimSpace(input.ItemID) == "" ||
		strings.TrimSpace(input.CallID) == "" ||
		strings.TrimSpace(input.ToolName) == "" {
		return nil, false
	}
	if input.Event == nil || input.Event["kind"] != "pipeline_stage" {
		return nil, false
	}
	stage := strings.TrimSpace(firstNonEmptyAnyString(input.Event["stage"]))
	if !strings.HasPrefix(stage, "background_job_") {
		return nil, false
	}
	details, _ := input.Event["details"].(map[string]any)
	if details == nil || details["notificationKind"] != "background_job_completion" {
		return nil, false
	}
	child, _ := input.Event["child"].(map[string]any)
	if SecurityBoundOutputMap(child) || SecurityBoundOutputMap(details) {
		status := normalizedChildStatus(firstNonEmptyAnyString(details["status"], strings.TrimPrefix(stage, "background_job_")))
		stage = "background_job_" + status
		itemStatus := "completed"
		if status != string(domainjob.StatusCompleted) {
			itemStatus = "failed"
		}
		safeDetails := PublicChildOutputProjection(details)
		if safeDetails == nil {
			safeDetails = PublicChildOutputProjection(child)
		}
		arguments := map[string]any{
			"runtimeStatus": "tool_progress",
			"stage":         stage,
			"message":       securityBoundOutputMessage,
			"status":        status,
			"diagnostics":   safeDetails,
		}
		if child != nil {
			arguments["child"] = PublicChildOutputProjection(child)
		}
		return map[string]any{
			"id":         strings.TrimSpace(input.ItemID),
			"turnId":     strings.TrimSpace(input.TurnID),
			"threadId":   strings.TrimSpace(input.ThreadID),
			"role":       "tool",
			"status":     itemStatus,
			"createdAt":  strings.TrimSpace(input.CreatedAt),
			"finishedAt": strings.TrimSpace(input.CreatedAt),
			"kind":       "tool_progress",
			"toolName":   strings.TrimSpace(input.ToolName),
			"callId":     strings.TrimSpace(input.CallID),
			"toolKind":   "subagent",
			"message":    securityBoundOutputMessage,
			"arguments":  arguments,
		}, true
	}
	status := normalizedChildStatus(firstNonEmptyAnyString(details["status"], strings.TrimPrefix(stage, "background_job_")))
	stage = "background_job_" + status
	itemStatus := "completed"
	if status != string(domainjob.StatusCompleted) {
		itemStatus = "failed"
	}
	message := "background job " + normalizedChildStatus(status)
	details = ProjectOrdinaryJobMap(details)
	if details == nil {
		return nil, false
	}
	details["status"] = status
	delete(details, "outputPreview")
	delete(details, "error")
	delete(details, "autoContinueError")
	delete(details, "deliveryError")
	summary := strings.TrimSpace(firstNonEmptyAnyString(details["jobId"], input.ToolName))
	arguments := map[string]any{
		"runtimeStatus": "tool_progress",
		"stage":         stage,
		"message":       message,
		"status":        status,
		"diagnostics":   details,
	}
	if child != nil {
		safeChild := ProjectOrdinaryJobMap(child)
		if safeChild != nil {
			if _, exists := safeChild["status"]; exists {
				safeChild["status"] = status
			}
			if _, exists := safeChild["childStatus"]; exists {
				safeChild["childStatus"] = status
			}
			arguments["child"] = safeChild
		}
	}
	if message != "" {
		arguments["summary"] = message
	}
	toolKind := "subagent"
	if childKind := strings.TrimSpace(firstNonEmptyAnyString(child["kind"])); childKind == "background-shell" {
		toolKind = "command_execution"
	}
	return map[string]any{
		"id":         strings.TrimSpace(input.ItemID),
		"turnId":     strings.TrimSpace(input.TurnID),
		"threadId":   strings.TrimSpace(input.ThreadID),
		"role":       "tool",
		"status":     itemStatus,
		"createdAt":  strings.TrimSpace(input.CreatedAt),
		"finishedAt": strings.TrimSpace(input.CreatedAt),
		"kind":       "tool_progress",
		"toolName":   strings.TrimSpace(input.ToolName),
		"callId":     strings.TrimSpace(input.CallID),
		"toolKind":   toolKind,
		"summary":    summary,
		"message":    message,
		"arguments":  arguments,
	}, true
}

func BuildBackgroundJobDeliveryLedgerItem(input BackgroundJobDeliveryLedgerItemInput) (map[string]any, bool) {
	if strings.TrimSpace(input.ThreadID) == "" ||
		strings.TrimSpace(input.TurnID) == "" ||
		strings.TrimSpace(input.ItemID) == "" ||
		strings.TrimSpace(input.CallID) == "" ||
		strings.TrimSpace(input.Record.ID) == "" {
		return nil, false
	}
	status := normalizedBackgroundDeliveryStatus(input.Status)
	if ChildOutputRequiresWithholding(input.Record) {
		diagnostics := SecurityBoundChildOutputProjection(input.Record)
		diagnostics["notificationKind"] = "background_job_delivery"
		diagnostics["deliveryStatus"] = status
		itemStatus := "completed"
		if status == "dead_letter" {
			itemStatus = "failed"
		}
		return map[string]any{
			"id":         strings.TrimSpace(input.ItemID),
			"turnId":     strings.TrimSpace(input.TurnID),
			"threadId":   strings.TrimSpace(input.ThreadID),
			"role":       "tool",
			"status":     itemStatus,
			"createdAt":  strings.TrimSpace(input.CreatedAt),
			"finishedAt": strings.TrimSpace(input.CreatedAt),
			"kind":       "tool_progress",
			"toolName":   "background_delivery",
			"callId":     strings.TrimSpace(input.CallID),
			"message":    securityBoundOutputMessage,
			"arguments": map[string]any{
				"runtimeStatus": "tool_progress",
				"stage":         "background_job_delivery_" + status,
				"message":       securityBoundOutputMessage,
				"status":        status,
				"diagnostics":   diagnostics,
			},
		}, true
	}
	reason := domainjob.NormalizeOperationalReasonV1(input.Reason)
	errorText := ""
	message := BackgroundJobDeliveryLedgerMessage(status, reason, errorText)
	diagnostics := map[string]any{
		"notificationKind":          "background_job_delivery",
		"jobId":                     input.Record.ID,
		"childRunId":                input.Record.ID,
		"status":                    normalizedChildStatus(input.Record.Status),
		"deliveryStatus":            status,
		"completionDeliveryStatus":  status,
		"parentThreadId":            strings.TrimSpace(input.Record.ParentThreadID),
		"parentTurnId":              strings.TrimSpace(input.Record.ParentTurnID),
		"background":                input.Record.Background,
		"terminal":                  TaskJobTerminal(input.Record),
		"completionDeliveryAttempt": float64(input.Record.CompletionDeliveryAttempts),
	}
	if strings.TrimSpace(input.Record.CompletionDeliveryID) != "" {
		diagnostics["deliveryId"] = input.Record.CompletionDeliveryID
	}
	if strings.TrimSpace(input.Record.CompletionDeliveryItemID) != "" {
		diagnostics["deliveryItemId"] = input.Record.CompletionDeliveryItemID
	}
	if reason != "" {
		diagnostics["deliveryReason"] = reason
	}
	if errorText != "" {
		diagnostics["deliveryError"] = errorText
	}
	if strings.TrimSpace(input.Record.ChildThreadID) != "" {
		diagnostics["childThreadId"] = input.Record.ChildThreadID
	}
	if strings.TrimSpace(input.Record.ChildTurnID) != "" {
		diagnostics["childTurnId"] = input.Record.ChildTurnID
	}
	if strings.TrimSpace(input.Record.CompletionDeliveryAt) != "" {
		diagnostics["completionDeliveryAt"] = input.Record.CompletionDeliveryAt
	}
	if strings.TrimSpace(input.Record.CompletionDeadLetterAt) != "" {
		diagnostics["completionDeadLetterAt"] = input.Record.CompletionDeadLetterAt
	}
	if strings.TrimSpace(input.Record.RecoveryStatus) != "" {
		diagnostics["recoveryStatus"] = domainjob.PublicRecoveryStatusV1(input.Record.RecoveryStatus)
	}
	if input.Record.RecoveryAttempt > 0 {
		diagnostics["recoveryAttempt"] = float64(input.Record.RecoveryAttempt)
	}
	if value := domainjob.NormalizeOperationalReasonV1(input.Record.RecoveryReason); value != "" {
		diagnostics["recoveryReason"] = value
	}
	if value := domainjob.NormalizeOperationalReasonV1(input.Record.DeadLetterReason); value != "" {
		diagnostics["deadLetterReason"] = value
	}
	if value := domainjob.NormalizeOperationalReasonV1(input.Record.LateCompletionReason); value != "" {
		diagnostics["lateCompletionReason"] = value
	}
	if input.Record.LateCompletionSuppressed {
		diagnostics["lateCompletionSuppressed"] = true
	}
	if input.Record.SecurityBinding != nil {
		diagnostics["outputTrustStatus"] = input.Record.SecurityBinding.OutputTrustStatus
	} else if outputBytes := len([]byte(input.Record.Output)); outputBytes > 0 {
		diagnostics["outputBytes"] = float64(outputBytes)
	}
	itemStatus := "completed"
	if status == "dead_letter" {
		itemStatus = "failed"
	}
	return map[string]any{
		"id":         strings.TrimSpace(input.ItemID),
		"turnId":     strings.TrimSpace(input.TurnID),
		"threadId":   strings.TrimSpace(input.ThreadID),
		"role":       "tool",
		"status":     itemStatus,
		"createdAt":  strings.TrimSpace(input.CreatedAt),
		"finishedAt": strings.TrimSpace(input.CreatedAt),
		"kind":       "tool_progress",
		"toolName":   "background_delivery",
		"callId":     strings.TrimSpace(input.CallID),
		"summary":    message,
		"message":    message,
		"arguments": map[string]any{
			"runtimeStatus": "tool_progress",
			"stage":         "background_job_delivery_" + status,
			"message":       message,
			"status":        status,
			"diagnostics":   diagnostics,
		},
	}, true
}

func BackgroundJobDeliveryLedgerMessage(status string, reason string, errorText string) string {
	status = normalizedBackgroundDeliveryStatus(status)
	detail := domainjob.NormalizeOperationalReasonV1(reason)
	errorText = ""
	switch status {
	case "pending":
		return "background job completion delivery pending"
	case "retry":
		if detail != "" {
			return "background job completion delivery retry: " + detail
		}
		return "background job completion delivery retry"
	case "delivered":
		if detail != "" {
			return "background job completion delivered: " + detail
		}
		return "background job completion delivered"
	case "skipped":
		if detail != "" {
			return "background job completion delivery skipped: " + detail
		}
		return "background job completion delivery skipped"
	case "dead_letter":
		if detail != "" {
			return "background job completion delivery dead-lettered: " + detail
		}
		return "background job completion delivery dead-lettered"
	default:
		return "background job completion delivery status unavailable"
	}
}

func normalizedBackgroundDeliveryStatus(status string) string {
	status = domainjob.PublicCompletionDeliveryStatusV1(status)
	if status == "" {
		return domainjob.PublicOperationalStatusUnknownV1
	}
	return status
}

func BuildBackgroundJobCompletionNotificationEvent(input JobLifecycleEventInput) (map[string]any, bool) {
	if strings.TrimSpace(input.Record.ID) == "" || !input.Record.Background || !TaskJobTerminal(input.Record) {
		return nil, false
	}
	status := normalizedChildStatus(firstNonEmptyAnyString(input.Status, input.Record.Status))
	if ChildOutputRequiresWithholding(input.Record) {
		input.Record.Status = normalizedChildStatus(status)
		status = input.Record.Status
		child := SecurityBoundChildOutputProjection(input.Record)
		details := SecurityBoundChildOutputProjection(input.Record)
		details["notificationKind"] = "background_job_completion"
		details["canContinueParent"] = false
		return map[string]any{
			"kind":     "pipeline_stage",
			"threadId": input.ThreadID,
			"turnId":   input.TurnID,
			"stage":    "background_job_" + status,
			"message":  securityBoundOutputMessage,
			"child":    child,
			"details":  details,
		}, true
	}
	message := "background job " + status
	child := backgroundJobNotificationChild(input.Record, status, message)
	details := map[string]any{
		"notificationKind":         "background_job_completion",
		"jobId":                    input.Record.ID,
		"childRunId":               input.Record.ID,
		"status":                   status,
		"terminal":                 true,
		"background":               true,
		"kind":                     input.Record.Kind,
		"canContinueParent":        status == string(domainjob.StatusCompleted) && !input.Record.LateCompletionSuppressed,
		"lateCompletionSuppressed": input.Record.LateCompletionSuppressed,
	}
	if input.Record.AutoContinueParent {
		details["autoContinueParent"] = true
	}
	// Auto-continue is a later, independently durable lifecycle. Keeping its
	// mutable closed status out of the byte-exact completion bundle makes live
	// duplicate callbacks and startup replay reproduce the original delivery
	// instead of rewriting an already-published completion event.
	if value := domainjob.NormalizeOperationalReasonV1(input.Record.LateCompletionReason); value != "" {
		details["lateCompletionReason"] = value
	}
	if input.Record.CompletionDeliveryID != "" {
		details["deliveryId"] = input.Record.CompletionDeliveryID
	}
	if input.Record.CompletionDeliveryStatus != "" {
		details["deliveryStatus"] = domainjob.PublicCompletionDeliveryStatusV1(input.Record.CompletionDeliveryStatus)
	}
	if input.Record.CompletionDeliveryItemID != "" {
		details["deliveryItemId"] = input.Record.CompletionDeliveryItemID
	}
	if value := domainjob.NormalizeOperationalReasonV1(input.Record.CompletionDeliveryReason); value != "" {
		details["deliveryReason"] = value
	}
	if input.Record.ChildThreadID != "" {
		details["childThreadId"] = input.Record.ChildThreadID
	}
	if input.Record.ChildTurnID != "" {
		details["childTurnId"] = input.Record.ChildTurnID
	}
	if input.Record.SecurityBinding != nil {
		details["outputTrustStatus"] = input.Record.SecurityBinding.OutputTrustStatus
	} else if outputBytes := len([]byte(input.Record.Output)); outputBytes > 0 {
		details["outputBytes"] = float64(outputBytes)
	}
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"stage":    "background_job_" + status,
		"label":    input.Record.ID,
		"message":  message,
		"child":    child,
		"details":  details,
	}, true
}

func backgroundJobNotificationChild(record domainjob.Record, status string, message string) map[string]any {
	if JobIsSubagent(record) {
		if status == string(domainjob.StatusCompleted) {
			return EventChild(record, status, "")
		}
		return EventChild(record, status, message)
	}
	return TaskJobProgressChild(record, status)
}

func RecordTaskJobProgressEvent(recorder RuntimeEventRecorder, input TaskJobProgressEventInput) {
	event, ok := BuildTaskJobProgressEvent(input)
	if ok && recorder != nil {
		recorder(event, "task job progress")
	}
}
