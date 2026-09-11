package turn

import "testing"

func TestApplyTerminalUpdateSettlesItemsAndAppendsTerminalItem(t *testing.T) {
	source := map[string]any{
		"status": "running",
		"turns": []any{
			map[string]any{
				"id":       "turn_1",
				"status":   "running",
				"steering": []any{map[string]any{"id": "steer_1", "status": "pending"}},
				"items": []any{
					map[string]any{"id": "approval_1", "kind": "approval", "status": "pending"},
					map[string]any{"id": "tool_1", "kind": "tool_call", "status": "running"},
				},
			},
		},
	}
	result := ApplyTerminalUpdate(TerminalUpdateInput{
		Thread:     source,
		TurnID:     "turn_1",
		Status:     "failed",
		FinishedAt: "done",
		AppendItems: []map[string]any{
			{"id": "error_1", "kind": "error"},
			{"id": "tool_1", "kind": "tool_call", "status": "failed"},
		},
		Fields:       map[string]any{"discard": true},
		ThreadFields: map[string]any{"terminalArchive": map[string]any{"digest": "archive"}},
	})
	if !result.Found || !result.Applied || result.CurrentStatus != "failed" {
		t.Fatalf("terminal update result mismatch: %#v", result)
	}
	turns, _ := result.Thread["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	if turn["status"] != "failed" || turn["finishedAt"] != "done" || turn["discard"] != true {
		t.Fatalf("terminal turn fields mismatch: %#v", turn)
	}
	if archive, _ := result.Thread["terminalArchive"].(map[string]any); archive["digest"] != "archive" {
		t.Fatalf("terminal root fields were not committed atomically: %#v", result.Thread)
	}
	steering, _ := turn["steering"].([]any)
	cancelled, _ := steering[0].(map[string]any)
	if cancelled["status"] != "cancelled" || cancelled["cancelReason"] != "failed" {
		t.Fatalf("steering not cancelled: %#v", steering)
	}
	items, _ := turn["items"].([]any)
	approval, _ := items[0].(map[string]any)
	tool, _ := items[1].(map[string]any)
	errorItem, _ := items[2].(map[string]any)
	if approval["status"] != "expired" || tool["status"] != "failed" || errorItem["status"] != "failed" {
		t.Fatalf("terminal item settlement mismatch: %#v", items)
	}
	sourceTurns, _ := source["turns"].([]any)
	sourceTurn, _ := sourceTurns[0].(map[string]any)
	if sourceTurn["status"] != "running" || source["status"] != "running" {
		t.Fatalf("terminal update should not mutate input: %#v", source)
	}
	if source["terminalArchive"] != nil {
		t.Fatalf("terminal root fields mutated the input: %#v", source)
	}
}

func TestApplyTerminalUpdateProtectsTerminalTurn(t *testing.T) {
	result := ApplyTerminalUpdate(TerminalUpdateInput{
		Thread:          map[string]any{"turns": []any{map[string]any{"id": "turn_1", "status": "completed"}}},
		TurnID:          "turn_1",
		Status:          "failed",
		ProtectTerminal: true,
	})
	if !result.Found || result.Applied || result.CurrentStatus != "completed" {
		t.Fatalf("terminal protection mismatch: %#v", result)
	}
}

func TestApplyTerminalUpdateReportsMissingTurn(t *testing.T) {
	result := ApplyTerminalUpdate(TerminalUpdateInput{
		Thread: map[string]any{"turns": []any{map[string]any{"id": "turn_1"}}},
		TurnID: "missing",
	})
	if result.Found || result.Applied {
		t.Fatalf("missing turn should not be found: %#v", result)
	}
}
