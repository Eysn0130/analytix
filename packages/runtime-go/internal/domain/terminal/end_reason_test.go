package terminal

import "testing"

func TestEndReasonIsClosedSuccessfulDisplayMetadata(t *testing.T) {
	for _, value := range []any{map[string]any{"kind": "max-tokens"}} {
		reason, err := ParseEndReason(value)
		if err != nil || reason == nil || ValidateEndReason(reason) != nil {
			t.Fatal("valid end reason rejected")
		}
	}
	for _, value := range []any{"max-tokens", map[string]any{}, map[string]any{"kind": "stop"}, map[string]any{"kind": "max-tokens", "raw": "private"}} {
		if _, err := ParseEndReason(value); err == nil {
			t.Fatal("invalid end reason accepted")
		}
	}
	for _, disposition := range AllDispositionsV1() {
		record := map[string]any{"status": disposition.Status, "terminalReason": disposition.Reason, "endReason": map[string]any{"kind": "max-tokens"}}
		if ValidEndReasonRecord(record) != disposition.CandidateAllowed {
			t.Fatalf("end reason changed terminal priority for %q", disposition.Reason)
		}
	}
	if !ValidEndReasonRecord(map[string]any{}) || ValidEndReasonRecord(map[string]any{"endReason": nil}) {
		t.Fatal("old omission/null semantics changed")
	}
}
