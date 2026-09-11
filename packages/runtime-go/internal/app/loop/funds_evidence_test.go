package loop

import "testing"

func TestConcreteCaseFundClaimRequiresHostEvidenceReceipt(t *testing.T) {
	thread := map[string]any{"turns": []any{
		map[string]any{"id": "turn_other", "items": []any{map[string]any{"kind": "tool_result", "toolName": "mcp__analytix_funds__count", "status": "completed"}}},
		map[string]any{"id": "turn_1", "items": []any{
			map[string]any{"kind": "tool_result", "toolName": "mcp__analytix_funds__count", "status": "completed", "isError": true},
			map[string]any{"kind": "tool_result", "toolName": "mcp__other__count", "status": "completed"},
		}},
	}}
	if CaseFundHasSuccessfulToolResult(thread, "turn_1") {
		t.Fatal("error, other-turn, or other-server result must not satisfy the funds boundary")
	}
	items := thread["turns"].([]any)[1].(map[string]any)["items"].([]any)
	thread["turns"].([]any)[1].(map[string]any)["items"] = append(items, map[string]any{
		"kind": "tool_result", "toolName": "mcp__analytix_funds__count", "status": "completed", "isError": false,
	})
	if CaseFundHasSuccessfulToolResult(thread, "turn_1") {
		t.Fatal("transport success without a host EvidenceReceipt registry match must not publish case facts")
	}
}
