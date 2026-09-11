package subagent

import (
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func JobToolIdentity(thread map[string]any, record domainjob.Record) (string, string, string, bool) {
	itemID, callID, toolName := JobToolIdentityFromThread(thread, record)
	if itemID == "" || callID == "" || toolName == "" {
		return "", "", "", false
	}
	return itemID, callID, toolName, true
}

func JobToolIdentityFromThread(thread map[string]any, record domainjob.Record) (string, string, string) {
	if thread == nil {
		return "", "", ""
	}
	parentThreadID := strings.TrimSpace(record.ParentThreadID)
	parentTurnID := strings.TrimSpace(record.ParentTurnID)
	parentCallID := strings.TrimSpace(record.ParentToolCallID)
	if parentThreadID == "" || parentTurnID == "" || !domainmodel.IsHostToolCallIDV1(parentCallID) {
		return "", "", ""
	}
	expectedItemID := domaintoolcall.ToolCallItemIDV1(parentTurnID, parentCallID)
	if expectedItemID == "" {
		return "", "", ""
	}
	if recordedItemID := strings.TrimSpace(record.ParentToolItemID); recordedItemID != "" && recordedItemID != expectedItemID {
		return "", "", ""
	}
	if threadID := mapString(thread, "id"); threadID != "" && threadID != parentThreadID {
		return "", "", ""
	}
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil || mapString(turn, "id") != parentTurnID {
			continue
		}
		if turnThreadID := mapString(turn, "threadId"); turnThreadID != "" && turnThreadID != parentThreadID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item == nil || mapString(item, "kind") != "tool_call" {
				continue
			}
			callID := mapString(item, "callId")
			if callID != parentCallID || mapString(item, "id") != expectedItemID ||
				mapString(item, "turnId") != parentTurnID || mapString(item, "threadId") != parentThreadID {
				continue
			}
			toolName := mapString(item, "toolName")
			if toolName == "" {
				return "", "", ""
			}
			return expectedItemID, parentCallID, toolName
		}
	}
	return "", "", ""
}

func ToolResultExistsInThread(thread map[string]any, turnID, callID string) bool {
	if thread == nil {
		return false
	}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if mapString(turn, "id") != turnID {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if item == nil || mapString(item, "kind") != "tool_result" {
				continue
			}
			if mapString(item, "callId") == callID {
				return true
			}
		}
	}
	return false
}

type JobLifecycleEventInput struct {
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

func BuildJobLifecycleEvents(input JobLifecycleEventInput) []map[string]any {
	appendBackgroundCompletion := func(events []map[string]any) []map[string]any {
		event, ok := BuildBackgroundJobCompletionNotificationEvent(input)
		if ok {
			events = append(events, event)
		}
		return events
	}
	if JobIsSubagent(input.Record) {
		return BuildSubagentProgressEvents(SubagentProgressEventInput{
			ThreadID:       input.ThreadID,
			TurnID:         input.TurnID,
			ItemID:         input.ItemID,
			CallID:         input.CallID,
			ToolName:       input.ToolName,
			Record:         input.Record,
			Status:         input.Status,
			ProgressStatus: input.ProgressStatus,
			Message:        input.Message,
		})
	}
	event, ok := BuildTaskJobProgressEvent(TaskJobProgressEventInput{
		ThreadID:       input.ThreadID,
		TurnID:         input.TurnID,
		ItemID:         input.ItemID,
		CallID:         input.CallID,
		ToolName:       input.ToolName,
		Record:         input.Record,
		Status:         input.Status,
		ProgressStatus: input.ProgressStatus,
		Message:        input.Message,
	})
	if !ok {
		return nil
	}
	return appendBackgroundCompletion([]map[string]any{event})
}

func JobIsSubagent(record domainjob.Record) bool {
	kind := strings.ToLower(strings.TrimSpace(record.Kind))
	return strings.Contains(kind, "subagent") || strings.Contains(kind, "child-run") || strings.TrimSpace(record.ChildThreadID) != ""
}

func listAny(value any) []any {
	values, _ := value.([]any)
	return values
}

func JobDefaultToolName(record domainjob.Record) string {
	kind := strings.ToLower(strings.TrimSpace(record.Kind))
	switch {
	case kind == "background-shell":
		return "bash"
	case strings.Contains(kind, "subagent"), strings.Contains(kind, "child-run"):
		return "task"
	default:
		return firstNonEmptyAnyString(record.Name, record.Kind, "task")
	}
}
