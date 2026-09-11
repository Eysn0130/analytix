package turn

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type CompletionRecordInput struct {
	ThreadID         string
	TurnID           string
	Model            string
	AssistantText    string
	CreatedAt        string
	FinishedAt       string
	Usage            map[string]any
	CacheDiagnostics map[string]any
	UsageSource      string
	ChildRunID       string
}

type AssistantTextItemInput struct {
	ThreadID   string
	TurnID     string
	ItemID     string
	Status     string
	Text       string
	CreatedAt  string
	FinishedAt string
}

type CompletionRecord struct {
	AssistantItem      map[string]any
	ItemCompletedEvent map[string]any
	UsageEvent         map[string]any
	TurnCompletedEvent map[string]any
}

func BuildCompletionRecord(input CompletionRecordInput) CompletionRecord {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	createdAt := strings.TrimSpace(input.CreatedAt)
	finishedAt := strings.TrimSpace(input.FinishedAt)
	if finishedAt == "" {
		finishedAt = createdAt
	}
	assistantItem := BuildAssistantTextItem(AssistantTextItemInput{
		ThreadID:   threadID,
		TurnID:     turnID,
		ItemID:     "item_" + turnID + "_assistant",
		Status:     "completed",
		Text:       input.AssistantText,
		CreatedAt:  createdAt,
		FinishedAt: finishedAt,
	})
	usageEvent := map[string]any{
		"kind":             "usage",
		"threadId":         threadID,
		"turnId":           turnID,
		"model":            strings.TrimSpace(input.Model),
		"usage":            contracts.CloneMap(input.Usage),
		"cacheDiagnostics": contracts.CloneMap(input.CacheDiagnostics),
	}
	if source := strings.TrimSpace(input.UsageSource); source != "" {
		usageEvent["usageSource"] = source
	}
	if childRunID := strings.TrimSpace(input.ChildRunID); childRunID != "" {
		usageEvent["childRunId"] = childRunID
	}
	return CompletionRecord{
		AssistantItem:      assistantItem,
		ItemCompletedEvent: itemCompletedEvent(assistantItem),
		UsageEvent:         usageEvent,
		TurnCompletedEvent: map[string]any{
			"kind":     "turn_completed",
			"threadId": threadID,
			"turnId":   turnID,
			"status":   "completed",
		},
	}
}

func BuildAssistantTextItem(input AssistantTextItemInput) map[string]any {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	itemID := strings.TrimSpace(input.ItemID)
	if itemID == "" {
		itemID = "item_" + turnID + "_assistant"
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "completed"
	}
	createdAt := strings.TrimSpace(input.CreatedAt)
	finishedAt := strings.TrimSpace(input.FinishedAt)
	if finishedAt == "" {
		finishedAt = createdAt
	}
	return map[string]any{
		"id":         itemID,
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "assistant",
		"status":     status,
		"createdAt":  createdAt,
		"finishedAt": finishedAt,
		"kind":       "assistant_text",
		"text":       input.Text,
	}
}

func AssistantItemCompletedEvent(item map[string]any) map[string]any {
	return itemCompletedEvent(item)
}

func itemCompletedEvent(item map[string]any) map[string]any {
	return map[string]any{
		"kind":     "item_completed",
		"threadId": stringField(item, "threadId"),
		"turnId":   stringField(item, "turnId"),
		"itemId":   item["id"],
		"item":     contracts.CloneMap(item),
	}
}
