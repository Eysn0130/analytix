package goal

import "testing"

func TestGoalEventBuilders(t *testing.T) {
	goal := map[string]any{"id": "goal_1", "status": "active"}
	goalEvent := BuildUpdatedEvent(" thread_1 ", goal)
	if goalEvent["kind"] != "goal_updated" || goalEvent["threadId"] != "thread_1" {
		t.Fatalf("goal event identity mismatch: %#v", goalEvent)
	}
	payload, _ := goalEvent["goal"].(map[string]any)
	payload["status"] = "complete"
	if goal["status"] != "active" {
		t.Fatalf("goal event did not clone payload: %#v", goal)
	}

	todos := map[string]any{"items": []any{map[string]any{"content": "ship"}}}
	todoEvent := BuildTodosUpdatedEvent(" thread_1 ", todos)
	if todoEvent["kind"] != "todos_updated" || todoEvent["threadId"] != "thread_1" {
		t.Fatalf("todo event identity mismatch: %#v", todoEvent)
	}
	todoPayload, _ := todoEvent["todos"].(map[string]any)
	todoPayload["items"] = []any{}
	if len(listAny(todos["items"])) != 1 {
		t.Fatalf("todo event did not clone payload: %#v", todos)
	}
}
