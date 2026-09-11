package turn

import (
	"strings"

	appusage "analytix.local/runtime-go/internal/app/usage"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type FailureRecordInput struct {
	ThreadID         string
	TurnID           string
	Model            string
	Failure          domainfailure.Record
	FinishedAt       string
	Events           []map[string]any
	Result           domainmodel.Result
	CacheDiagnostics map[string]any
	UsageSource      string
	ChildRunID       string
}

type FailureRecord struct {
	Items           []map[string]any
	ErrorItemID     string
	UsageEvent      map[string]any
	TurnFailedEvent map[string]any
}

type FailureRecordEvent struct {
	Event map[string]any
	Label string
}

type RuntimeRestartedAbortRecordInput struct {
	ThreadID   string
	TurnID     string
	FinishedAt string
	Events     []map[string]any
}

type RuntimeRestartedAbortRecord struct {
	Items            []map[string]any
	TurnAbortedEvent map[string]any
	Message          string
}

func BuildFailureRecord(input FailureRecordInput) FailureRecord {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	finishedAt := strings.TrimSpace(input.FinishedAt)
	failure := domainfailure.Normalize(input.Failure)
	message := failure.Message()
	severity := failure.Severity()
	code := failure.Code()
	details := failure.Details()
	errorItemID := "item_" + turnID + "_error"
	errorItem := map[string]any{
		"id":         errorItemID,
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "system",
		"status":     "failed",
		"createdAt":  finishedAt,
		"finishedAt": finishedAt,
		"kind":       "error",
		"message":    message,
		"severity":   severity,
	}
	errorItem["code"] = code
	if details != nil {
		errorItem["details"] = details
	}
	telemetry := appusage.NewTerminalTelemetryV1(input.Result.Usage, input.CacheDiagnostics)
	usageEvent := map[string]any{
		"kind":             "usage",
		"threadId":         threadID,
		"turnId":           turnID,
		"model":            strings.TrimSpace(input.Model),
		"usage":            telemetry.PublicUsageMap(),
		"cacheDiagnostics": telemetry.PublicCacheDiagnosticsMap(),
		"usageFinalStatus": "failed",
	}
	if source := strings.TrimSpace(input.UsageSource); source != "" {
		usageEvent["usageSource"] = source
	}
	if childRunID := strings.TrimSpace(input.ChildRunID); childRunID != "" {
		usageEvent["childRunId"] = childRunID
	}
	items := []map[string]any{errorItem}
	event := map[string]any{
		"kind":     "turn_failed",
		"threadId": threadID,
		"turnId":   turnID,
		"status":   "failed",
		"itemId":   errorItemID,
		"message":  message,
		"error":    message,
		"severity": severity,
	}
	event["code"] = code
	if details != nil {
		event["details"] = failure.Details()
	}
	return FailureRecord{
		Items:           items,
		ErrorItemID:     errorItemID,
		UsageEvent:      usageEvent,
		TurnFailedEvent: event,
	}
}

func (record FailureRecord) RecordEvents() []FailureRecordEvent {
	events := make([]FailureRecordEvent, 0, len(record.Items)+1)
	for _, item := range record.Items {
		label := "record failed turn item event"
		if stringField(item, "id") == strings.TrimSpace(record.ErrorItemID) {
			label = "record failed turn error item event"
		}
		events = append(events, FailureRecordEvent{
			Label: label,
			Event: map[string]any{
				"kind":     "item_completed",
				"threadId": stringField(item, "threadId"),
				"turnId":   stringField(item, "turnId"),
				"itemId":   stringField(item, "id"),
				"item":     item,
			},
		})
	}
	if record.UsageEvent != nil {
		events = append(events, FailureRecordEvent{Label: "record failed turn usage event", Event: record.UsageEvent})
	}
	if record.TurnFailedEvent != nil {
		events = append(events, FailureRecordEvent{Label: "record turn_failed event", Event: record.TurnFailedEvent})
	}
	return events
}

func BuildRuntimeRestartedAbortRecord(input RuntimeRestartedAbortRecordInput) RuntimeRestartedAbortRecord {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	finishedAt := strings.TrimSpace(input.FinishedAt)
	message := "The runtime restarted before this turn completed. The turn was marked aborted so the thread can continue."
	errorItemID := "item_" + turnID + "_runtime_restarted"
	errorItem := map[string]any{
		"id":         errorItemID,
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "system",
		"status":     "aborted",
		"createdAt":  finishedAt,
		"finishedAt": finishedAt,
		"kind":       "error",
		"code":       "runtime_restarted",
		"message":    message,
		"severity":   "warning",
	}
	items := []map[string]any{errorItem}
	return RuntimeRestartedAbortRecord{
		Items:   items,
		Message: message,
		TurnAbortedEvent: map[string]any{
			"kind":     "turn_aborted",
			"threadId": threadID,
			"turnId":   turnID,
			"status":   "aborted",
			"code":     "runtime_restarted",
			"message":  message,
			"severity": "warning",
			"details": map[string]any{
				"runtimeRestarted": true,
			},
		},
	}
}

func (record RuntimeRestartedAbortRecord) RecordEvents() []FailureRecordEvent {
	events := make([]FailureRecordEvent, 0, len(record.Items)+1)
	for _, item := range record.Items {
		events = append(events, FailureRecordEvent{
			Label: "record restarted turn item event",
			Event: map[string]any{
				"kind":     "item_completed",
				"threadId": stringField(item, "threadId"),
				"turnId":   stringField(item, "turnId"),
				"itemId":   stringField(item, "id"),
				"item":     item,
			},
		})
	}
	if record.TurnAbortedEvent != nil {
		events = append(events, FailureRecordEvent{Label: "record restarted turn_aborted event", Event: record.TurnAbortedEvent})
	}
	return events
}
