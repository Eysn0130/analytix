package thread

import "testing"

func TestAppendTurnAppliesAutoTitleProviderAndPatch(t *testing.T) {
	source := map[string]any{
		"id":        "thr_1",
		"title":     "New thread",
		"autoTitle": true,
		"status":    "idle",
		"turns":     []any{},
	}
	turn := map[string]any{
		"id":     "turn_1",
		"prompt": "Analyze the project",
		"items":  []any{map[string]any{"kind": "user_message", "text": "Use cached evidence"}},
	}
	thread := AppendTurn(AppendTurnInput{
		Thread:     source,
		Turn:       turn,
		ProviderID: " provider-a ",
		ThreadPatch: map[string]any{
			"executionPolicyVersion": float64(2),
			"model":                  "model-a",
			"ignored":                "skip",
			"sandboxMode":            "read-only",
			"contextEpochState":      map[string]any{"version": float64(1)},
		},
		UpdatedAt: "now",
	})
	if thread["title"] != "Use cached evidence" || thread["autoTitle"] != false {
		t.Fatalf("auto title mismatch: %#v", thread)
	}
	if thread["status"] != "running" || thread["providerId"] != "provider-a" || thread["model"] != "model-a" {
		t.Fatalf("runtime fields mismatch: %#v", thread)
	}
	if thread["ignored"] != nil || thread["sandboxMode"] != "read-only" || thread["updatedAt"] != "now" {
		t.Fatalf("patch fields mismatch: %#v", thread)
	}
	if thread["executionPolicyVersion"] != float64(2) {
		t.Fatalf("internal turn append must persist the execution policy marker: %#v", thread)
	}
	if thread["contextEpochState"] == nil {
		t.Fatalf("internal turn append must persist context epoch state: %#v", thread)
	}
	turn["prompt"] = "mutated"
	turns, _ := thread["turns"].([]any)
	appended, _ := turns[0].(map[string]any)
	if appended["prompt"] != "Analyze the project" {
		t.Fatalf("appended turn should be cloned: %#v", appended)
	}
	sourceTurns, _ := source["turns"].([]any)
	if len(sourceTurns) != 0 || source["status"] != "idle" {
		t.Fatalf("append should not mutate input: %#v", source)
	}
}

func TestAppendTurnPreservesExistingCustomTitle(t *testing.T) {
	thread := AppendTurn(AppendTurnInput{
		Thread: map[string]any{"title": "Custom", "turns": []any{}},
		Turn:   map[string]any{"id": "turn_1", "prompt": "Prompt title"},
	})
	if thread["title"] != "Custom" {
		t.Fatalf("custom title should be preserved: %#v", thread)
	}
}
