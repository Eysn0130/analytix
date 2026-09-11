package turn

import "testing"

func TestBuildCompletionRecordShapesDurableTerminalEvents(t *testing.T) {
	record := BuildCompletionRecord(CompletionRecordInput{
		ThreadID:         " thread-a ",
		TurnID:           " turn-1 ",
		Model:            " model-a ",
		AssistantText:    "done",
		CreatedAt:        "2026-01-02T03:04:05Z",
		FinishedAt:       "2026-01-02T03:04:06Z",
		Usage:            map[string]any{"totalTokens": float64(12)},
		CacheDiagnostics: map[string]any{"cacheHitRate": float64(0.5)},
		UsageSource:      "subagent",
		ChildRunID:       "job-1",
	})
	if got := stringField(record.AssistantItem, "id"); got != "item_turn-1_assistant" {
		t.Fatalf("assistant id = %q", got)
	}
	if got := stringField(record.AssistantItem, "threadId"); got != "thread-a" {
		t.Fatalf("assistant thread id = %q", got)
	}
	if got := stringField(record.ItemCompletedEvent, "kind"); got != "item_completed" {
		t.Fatalf("item event kind = %q", got)
	}
	if got := stringField(record.UsageEvent, "model"); got != "model-a" {
		t.Fatalf("usage model = %q", got)
	}
	if got := stringField(record.UsageEvent, "usageSource"); got != "subagent" {
		t.Fatalf("usageSource = %q", got)
	}
	if got := stringField(record.UsageEvent, "childRunId"); got != "job-1" {
		t.Fatalf("childRunId = %q", got)
	}
	if got := stringField(record.TurnCompletedEvent, "status"); got != "completed" {
		t.Fatalf("turn status = %q", got)
	}
}

func TestBuildAssistantTextItemShapesProcessItemAndCompletionEvent(t *testing.T) {
	item := BuildAssistantTextItem(AssistantTextItemInput{
		ThreadID:  " thread-a ",
		TurnID:    " turn-1 ",
		ItemID:    " item_turn-1_assistant_process_1 ",
		Text:      "I will inspect the workspace first.",
		CreatedAt: "2026-01-02T03:04:05Z",
	})
	if got := stringField(item, "id"); got != "item_turn-1_assistant_process_1" {
		t.Fatalf("assistant process id = %q", got)
	}
	if got := stringField(item, "kind"); got != "assistant_text" {
		t.Fatalf("assistant process kind = %q", got)
	}
	if got := stringField(item, "status"); got != "completed" {
		t.Fatalf("assistant process status = %q", got)
	}
	if got := stringField(item, "finishedAt"); got != "2026-01-02T03:04:05Z" {
		t.Fatalf("assistant process finishedAt = %q", got)
	}
	event := AssistantItemCompletedEvent(item)
	if got := stringField(event, "kind"); got != "item_completed" {
		t.Fatalf("event kind = %q", got)
	}
	if got := stringField(event, "itemId"); got != "item_turn-1_assistant_process_1" {
		t.Fatalf("event item id = %q", got)
	}
}

func TestBuildCompletionRecordClonesUsagePayloads(t *testing.T) {
	usage := map[string]any{"totalTokens": float64(12)}
	cache := map[string]any{"cacheHitRate": float64(0.5)}
	record := BuildCompletionRecord(CompletionRecordInput{
		ThreadID:         "thread-a",
		TurnID:           "turn-1",
		Model:            "model-a",
		CreatedAt:        "2026-01-02T03:04:05Z",
		Usage:            usage,
		CacheDiagnostics: cache,
	})
	usage["totalTokens"] = float64(99)
	cache["cacheHitRate"] = float64(0)
	clonedUsage, _ := record.UsageEvent["usage"].(map[string]any)
	if got := clonedUsage["totalTokens"]; got != float64(12) {
		t.Fatalf("usage not cloned: %#v", got)
	}
	clonedCache, _ := record.UsageEvent["cacheDiagnostics"].(map[string]any)
	if got := clonedCache["cacheHitRate"]; got != float64(0.5) {
		t.Fatalf("cache diagnostics not cloned: %#v", got)
	}
}
