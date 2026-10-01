package model_test

import (
	"fmt"
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
)

func TestAutomaticCompactionKeepsEarlyUserRequirementsWithoutGoal(t *testing.T) {
	thread := map[string]any{"id": "thread-early", "workspace": "/synthetic-workspace", "turns": []any{}}
	turns := []any{}
	for i := 0; i < 8; i++ {
		text := fmt.Sprintf("ordinary follow-up %d", i)
		if i == 0 {
			text = "Only modify A.txt; never modify B.txt."
		}
		id := fmt.Sprintf("turn-%d", i)
		turns = append(turns, map[string]any{"id": id, "threadId": "thread-early", "status": "completed", "items": []any{map[string]any{
			"id": "user-" + id, "turnId": id, "threadId": "thread-early", "kind": "user_message", "role": "user", "text": text,
		}}})
	}
	thread["turns"] = turns
	plan := appturn.BuildThreadCompactionWithMode(thread, "thread-early", "automatic_context_threshold", 100, "2026-09-26T00:00:00Z", true)
	if plan.Error != nil || !plan.Changed {
		t.Fatal("automatic compaction failed", plan.Error)
	}
	thread["turns"] = plan.NextTurns
	var body strings.Builder
	for _, message := range appmodel.ProviderHistoryFromThread(thread) {
		body.WriteString(message.Content)
	}
	if !strings.Contains(body.String(), "Only modify A.txt; never modify B.txt.") {
		t.Fatal("early user requirement disappeared from real Provider history")
	}
	// The whole new snapshot is fenced, including its legacy summary and
	// latest-four constraints, not only the new userHistory field.
	thread["workspace"] = "/another-synthetic-workspace"
	for _, message := range appmodel.ProviderHistoryFromThread(thread) {
		if strings.Contains(message.Content, "[Compacted conversation summary]") || strings.Contains(message.Content, "[Analytix task continuation snapshot;") {
			t.Fatal("a scoped compaction revived after workspace change")
		}
	}
}

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
