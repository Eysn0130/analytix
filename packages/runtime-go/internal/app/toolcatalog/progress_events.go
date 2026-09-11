package toolcatalog

import domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"

type ToolProgressEventInput struct {
	ThreadID string
	TurnID   string
	ItemID   string
	CallID   string
	ToolName string
	Summary  string
	Status   string
	Message  string
	Child    map[string]any
}

type RuntimeEventRecorder func(map[string]any, string)

func RecordToolProgressEvent(recorder RuntimeEventRecorder, input ToolProgressEventInput) {
	event, ok := BuildToolProgressEvent(input)
	if ok && recorder != nil {
		recorder(event, "tool progress")
	}
}

func BuildToolProgressEvent(input ToolProgressEventInput) (map[string]any, bool) {
	if input.ItemID == "" || input.CallID == "" {
		return nil, false
	}
	return buildToolProgressEvent(input), true
}

func buildToolProgressEvent(input ToolProgressEventInput) map[string]any {
	summary := input.Summary
	if summary == "" {
		summary = input.ToolName
	}
	event := map[string]any{
		"kind":     "tool_progress",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"itemId":   input.ItemID,
		"callId":   input.CallID,
		"toolName": input.ToolName,
		"summary":  summary,
		"status":   input.Status,
		"message":  input.Message,
	}
	if input.Child != nil {
		event["child"] = input.Child
	}
	return event
}

type ToolStartedEventInput struct {
	ThreadID  string
	TurnID    string
	ItemID    string
	CallID    string
	ToolName  string
	CreatedAt string
}

func BuildToolStartedEvent(input ToolStartedEventInput) (map[string]any, bool) {
	if input.ItemID == "" || input.CallID == "" {
		return nil, false
	}
	runningItem := map[string]any{
		"id":        input.ItemID,
		"turnId":    input.TurnID,
		"threadId":  input.ThreadID,
		"role":      "tool",
		"status":    "running",
		"createdAt": input.CreatedAt,
		"kind":      "tool_call",
		"toolName":  input.ToolName,
		"callId":    input.CallID,
		"toolKind":  ToolKind(input.ToolName),
		"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
	}
	return map[string]any{
		"kind":     "tool_call_started",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"itemId":   input.ItemID,
		"item":     runningItem,
	}, true
}
