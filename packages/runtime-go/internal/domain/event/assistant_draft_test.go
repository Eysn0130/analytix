package event

import "testing"

func TestLegacyAssistantDraftItemClassification(t *testing.T) {
	for name, item := range map[string]map[string]any{
		"process item":                 {"id": "item_turn-1_assistant_process_2", "turnId": "turn-1", "kind": "assistant_text", "status": "completed"},
		"failed delta materialization": {"id": "item_turn-1_assistant_text", "turnId": "turn-1", "kind": "assistant_text", "status": "failed"},
		"running item":                 {"id": "item_turn-1_assistant", "turnId": "turn-1", "kind": "assistant_text", "status": "running"},
		"missing id":                   {"turnId": "turn-1", "kind": "assistant_text", "status": "completed"},
	} {
		t.Run(name, func(t *testing.T) {
			if !IsLegacyAssistantDraftItem(item) {
				t.Fatalf("legacy assistant draft was not classified: %#v", item)
			}
		})
	}
	for name, item := range map[string]map[string]any{
		"canonical terminal": {"id": "item_turn-1_assistant", "turnId": "turn-1", "kind": "assistant_text", "status": "completed"},
		"error boundary":     {"id": "item_turn-1_error", "turnId": "turn-1", "kind": "error", "status": "failed"},
	} {
		t.Run(name, func(t *testing.T) {
			if IsLegacyAssistantDraftItem(item) {
				t.Fatalf("terminal item was misclassified as a draft: %#v", item)
			}
		})
	}
}
