package turn

import "testing"

func TestIsTerminalStatus(t *testing.T) {
	for _, status := range []string{"completed", "failed", "aborted", "interrupted", "killed"} {
		if !IsTerminalStatus(status) {
			t.Fatalf("%s should be terminal", status)
		}
	}
	if IsTerminalStatus("running") || IsTerminalStatus("pending") {
		t.Fatal("non-terminal statuses should not be terminal")
	}
}

func TestSettleItemsForTerminalStatus(t *testing.T) {
	turn := map[string]any{
		"items": []any{
			map[string]any{"kind": "approval", "status": "pending"},
			map[string]any{"kind": "user_input", "status": "pending"},
			map[string]any{"kind": "tool_call", "status": "running"},
			map[string]any{"kind": "assistant_text", "status": "completed"},
		},
	}
	SettleItemsForTerminalStatus(turn, "failed", "done")
	items, _ := turn["items"].([]any)
	if items[0].(map[string]any)["status"] != "expired" {
		t.Fatalf("approval not expired: %#v", items[0])
	}
	if items[1].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("user input not cancelled: %#v", items[1])
	}
	if items[2].(map[string]any)["status"] != "failed" {
		t.Fatalf("running item not failed: %#v", items[2])
	}
	if items[3].(map[string]any)["status"] != "completed" {
		t.Fatalf("completed item should not change: %#v", items[3])
	}
	if items[0].(map[string]any)["finishedAt"] != "done" ||
		items[1].(map[string]any)["finishedAt"] != "done" ||
		items[2].(map[string]any)["finishedAt"] != "done" {
		t.Fatalf("finishedAt not stamped: %#v", items)
	}
}

func TestSettleItemsForCompletedTurnDoesNothing(t *testing.T) {
	turn := map[string]any{"items": []any{map[string]any{"kind": "tool_call", "status": "running"}}}
	SettleItemsForTerminalStatus(turn, "completed", "done")
	item := turn["items"].([]any)[0].(map[string]any)
	if item["status"] != "running" || item["finishedAt"] != nil {
		t.Fatalf("completed settlement should not mutate items: %#v", item)
	}
}
