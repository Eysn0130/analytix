package thread

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCaseTerminalTodoAuditProjectionRetainsOnlyBoundedLifecycleState(t *testing.T) {
	projected, err := CaseTerminalTodoAuditProjectionV1(map[string]any{
		"items": []any{
			map[string]any{
				"id": "todo_6222021234567890", "content": "核验完整账号 6222021234567890",
				"status": "failed", "statusReasonCode": "tool_failed",
			},
			map[string]any{
				"id": "todo_cancelled", "content": "CASE_FACT_SENTINEL_7654321",
				"status": "canceled", "statusReasonCode": "user_canceled",
			},
			map[string]any{"id": "pending", "content": "must not copy", "status": "pending"},
			map[string]any{"id": "done", "content": "must not copy", "status": "completed"},
		},
	}, "thr_case", "2026-07-26T08:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	items := listAny(projected["items"])
	if projected["threadId"] != "thr_case" || len(items) != 2 {
		t.Fatalf("case terminal Todo audit projection mismatch: %#v", projected)
	}
	encoded, _ := json.Marshal(projected)
	body := string(encoded)
	for _, sentinel := range []string{
		"6222021234567890", "CASE_FACT_SENTINEL_7654321", "todo_cancelled", "must not copy",
	} {
		if strings.Contains(body, sentinel) {
			t.Fatalf("case terminal Todo audit leaked %q: %s", sentinel, body)
		}
	}
	failed := items[0].(map[string]any)
	canceled := items[1].(map[string]any)
	if failed["status"] != "failed" || failed["statusReasonCode"] != "tool_failed" ||
		canceled["status"] != "canceled" || canceled["statusReasonCode"] != "user_canceled" {
		t.Fatalf("terminal lifecycle state was not retained: %#v", items)
	}
	for _, item := range items {
		id := stringField(item.(map[string]any), "id")
		if !strings.HasPrefix(id, "todo_case_audit_") || len(id) != len("todo_case_audit_")+64 {
			t.Fatalf("case Todo audit id is not pseudonymous: %q", id)
		}
	}
}

func TestCaseTerminalTodoAuditProjectionRejectsMalformedTerminalRecords(t *testing.T) {
	for name, item := range map[string]map[string]any{
		"missing reason": {
			"id": "failed", "content": "failed", "status": "failed",
		},
		"wrong reason class": {
			"id": "failed", "content": "failed", "status": "failed", "statusReasonCode": "user_canceled",
		},
		"unknown field": {
			"id": "failed", "content": "failed", "status": "failed", "statusReasonCode": "tool_failed",
			"privateDetail": "must not pass",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if projected, err := CaseTerminalTodoAuditProjectionV1(
				map[string]any{"items": []any{item}},
				"thr_case",
				"2026-07-26T08:00:00Z",
			); err == nil || projected != nil {
				t.Fatalf("malformed terminal Todo became an audit projection: %#v err=%v", projected, err)
			}
		})
	}
}

func TestCaseTerminalTodoAuditProjectionIgnoresLegacyNonTerminalContent(t *testing.T) {
	projected, err := CaseTerminalTodoAuditProjectionV1(map[string]any{
		"items": []any{
			map[string]any{"content": "legacy item without lifecycle"},
			map[string]any{"id": "unknown", "content": "unknown", "status": "unknown"},
		},
	}, "thr_case", "2026-07-26T08:00:00Z")
	if err != nil || projected != nil {
		t.Fatalf("legacy non-terminal content should remain withheld: %#v err=%v", projected, err)
	}
}
