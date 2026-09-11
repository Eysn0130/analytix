package model_test

import (
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
)

func TestAutomaticCompactionRestoresContinuationAsDynamicUserData(t *testing.T) {
	thread := map[string]any{
		"id": "thr_history_auto", "status": "idle", "model": "test-model",
		"goal":  map[string]any{"id": "goal_1", "objective": "preserve current objective", "status": "active", "evidenceLedger": []any{}},
		"todos": map[string]any{"items": []any{map[string]any{"id": "todo_1", "content": "unfinished check", "status": "pending"}}},
	}
	turns := []any{}
	for index := 1; index <= 5; index++ {
		turnID := "turn_" + string(rune('0'+index))
		turns = append(turns, map[string]any{
			"id": turnID, "threadId": "thr_history_auto", "status": "completed",
			"items": []any{map[string]any{"id": "item_" + turnID, "turnId": turnID, "threadId": "thr_history_auto", "kind": "user_message", "role": "user", "status": "completed", "createdAt": "2026-07-22T00:00:00Z", "finishedAt": "2026-07-22T00:00:00Z", "text": "constraint " + turnID}},
		})
	}
	thread["turns"] = turns
	plan := appturn.BuildThreadCompactionWithMode(thread, "thr_history_auto", "automatic_context_threshold", 100, "2026-07-22T00:00:00Z", true)
	if plan.Error != nil || !plan.Changed {
		t.Fatalf("automatic compaction = %#v err=%v", plan, plan.Error)
	}
	thread["turns"] = plan.NextTurns
	messages := appmodel.ProviderHistoryFromThread(thread)
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Role+":"+message.Content)
	}
	joined := strings.Join(parts, "\n")
	if !strings.Contains(joined, "Analytix task continuation snapshot") || !strings.Contains(joined, "preserve current objective") ||
		!strings.Contains(joined, "unfinished check") || !strings.Contains(joined, "unverified_for_case_facts") {
		t.Fatalf("provider continuation history = %q", joined)
	}
}
