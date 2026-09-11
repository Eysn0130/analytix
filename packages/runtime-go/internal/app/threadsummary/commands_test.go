package threadsummary

import (
	"testing"
	"time"
)

func TestCommandTasksWithholdArgumentsAndDisableHistoricalRestart(t *testing.T) {
	thread := map[string]any{
		"id":             "thread-1",
		"workspace":      "/tmp/work",
		"sandboxMode":    "workspace-write",
		"approvalPolicy": "on-request",
		"turns": []any{
			map[string]any{
				"items": []any{
					map[string]any{
						"id":        "call-item",
						"turnId":    "turn-1",
						"kind":      "tool_call",
						"toolName":  "bash",
						"callId":    "call-1",
						"createdAt": "2026-01-01T00:00:00Z",
						"arguments": map[string]any{
							"command": "npm test",
							"cwd":     "/tmp/work/pkg",
						},
					},
					map[string]any{
						"id":        "result-item",
						"turnId":    "turn-1",
						"kind":      "tool_result",
						"toolKind":  "command_execution",
						"toolName":  "bash",
						"callId":    "call-1",
						"createdAt": "2026-01-01T00:00:01Z",
						"output": map[string]any{
							"status":    "completed",
							"output":    "ok",
							"exit_code": float64(0),
						},
					},
				},
			},
		},
	}
	tasks := CommandTasks(thread, time.Date(2026, 1, 1, 0, 0, 2, 0, time.UTC))
	if len(tasks) != 1 {
		t.Fatalf("expected one command task, got %d", len(tasks))
	}
	task := tasks[0].Task
	if task["id"] != "command:call-1" {
		t.Fatalf("unexpected id: %#v", task["id"])
	}
	if task["schemaVersion"] != 1 || task["status"] != "completed" || task["kind"] != "command" ||
		task["background"] != false || task["active"] != false || task["terminal"] != true ||
		task["canReadOutput"] != false || task["outputWithheld"] != true ||
		task["outputTrustStatus"] != "private_tool_output" || task["canContinueParent"] != false {
		t.Fatalf("unexpected task projection: %#v", task)
	}
	if task["command"] != nil || task["cwd"] != nil || task["outputSnippet"] != nil || task["outputBytes"] != nil || task["canRestart"] != nil {
		t.Fatalf("historical tool arguments entered summary projection: %#v", task)
	}
}

func TestCommandTasksUseClosedLifecycleStatuses(t *testing.T) {
	thread := map[string]any{
		"id": "thread-1",
		"turns": []any{map[string]any{"items": []any{
			map[string]any{
				"id":       "unknown-result",
				"kind":     "tool_result",
				"toolKind": "command_execution",
				"toolName": "bash",
				"callId":   "call-unknown",
				"status":   "PRIVATE_PROVIDER_STATUS_SENTINEL",
			},
			map[string]any{
				"id":       "error-result",
				"kind":     "tool_result",
				"toolKind": "command_execution",
				"toolName": "bash",
				"callId":   "call-error",
				"isError":  true,
			},
		}}},
	}
	tasks := CommandTasks(thread, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(tasks) != 2 {
		t.Fatalf("expected two command tasks, got %#v", tasks)
	}
	byID := map[string]map[string]any{}
	for _, task := range tasks {
		byID[task.Task["id"].(string)] = task.Task
	}
	unknown := byID["command:call-unknown"]
	if unknown["status"] != "unknown" || unknown["active"] != false || unknown["terminal"] != false || unknown["canReadOutput"] != false {
		t.Fatalf("unknown command lifecycle gained control semantics: %#v", unknown)
	}
	errorTask := byID["command:call-error"]
	if errorTask["status"] != "error" || errorTask["active"] != false || errorTask["terminal"] != true || errorTask["canReadOutput"] != false {
		t.Fatalf("blank error lifecycle was not projected as terminal: %#v", errorTask)
	}
}

func TestCommandTasksSemanticErrorOverridesCompletedStatus(t *testing.T) {
	tasks := CommandTasks(map[string]any{
		"turns": []any{
			map[string]any{
				"items": []any{
					map[string]any{
						"id": "result", "kind": "tool_result", "toolKind": "command_execution",
						"callId": "call-error", "status": "completed", "isError": true,
					},
				},
			},
		},
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(tasks) != 1 || tasks[0].Task["status"] != "error" || tasks[0].Task["terminal"] != true {
		t.Fatalf("semantic command error was upgraded to success: %#v", tasks)
	}
}

func TestItemsSort(t *testing.T) {
	thread := map[string]any{
		"turns": []any{
			map[string]any{
				"items": []any{
					map[string]any{"id": "b", "kind": "tool_call", "createdAt": "2026-01-01T00:00:01Z"},
					map[string]any{"id": "a", "kind": "user_message", "createdAt": "2026-01-01T00:00:00Z", "displayText": " hello "},
				},
			},
		},
	}
	items := Items(thread)
	if len(items) != 2 || items[0]["id"] != "a" {
		t.Fatalf("items not sorted: %#v", items)
	}
}
