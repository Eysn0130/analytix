package subagent

import "testing"

func TestChildTodoItemsFromTodosQuarantinesUnknownStatus(t *testing.T) {
	items := ChildTodoItemsFromTodos(map[string]any{"items": []any{
		map[string]any{"id": "valid", "content": "valid", "status": "completed"},
		map[string]any{"id": "missing", "content": "missing"},
		map[string]any{"id": "unknown", "content": "unknown", "status": "model_claimed_complete"},
	}})
	if len(items) != 1 || items[0].ID != "valid" || items[0].Status != "completed" {
		t.Fatalf("unknown child todo status entered projection authority: %#v", items)
	}
}
