package checkpointref

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactCapturedEventCountUsesCanonicalJSONBytes(t *testing.T) {
	eventID := CaptureEventID(strings.Repeat("d", 64))
	expected := comparableCapturedEventFixture(eventID, float64(1), "captured")
	persisted := comparableCapturedEventFixture(eventID, json.Number("1"), "captured")
	persisted["seq"] = json.Number("7")
	persisted["timestamp"] = "2026-07-26T00:00:00Z"

	count, conflict, found := ExactCapturedEventCount([]map[string]any{persisted}, eventID, expected)
	if conflict || count != 1 || found == nil {
		t.Fatalf("canonical numeric JSON replay did not match: count=%d conflict=%v found=%#v", count, conflict, found)
	}
	if found["seq"] != persisted["seq"] {
		t.Fatalf("persisted event authority was not returned: %#v", found)
	}
}

func TestExactCapturedEventCountTreatsDivergenceAndMarshalFailureAsConflict(t *testing.T) {
	eventID := CaptureEventID(strings.Repeat("e", 64))
	expected := comparableCapturedEventFixture(eventID, float64(1), "captured")
	for _, test := range []struct {
		name     string
		events   []map[string]any
		expected map[string]any
	}{
		{
			name:     "payload-divergence",
			events:   []map[string]any{comparableCapturedEventFixture(eventID, json.Number("1"), "different")},
			expected: expected,
		},
		{
			name: "persisted-marshal-failure",
			events: []map[string]any{func() map[string]any {
				event := comparableCapturedEventFixture(eventID, json.Number("1"), "captured")
				event["unsupported"] = func() {}
				return event
			}()},
			expected: expected,
		},
		{
			name:   "expected-marshal-failure",
			events: []map[string]any{comparableCapturedEventFixture(eventID, json.Number("1"), "captured")},
			expected: func() map[string]any {
				event := comparableCapturedEventFixture(eventID, float64(1), "captured")
				event["unsupported"] = func() {}
				return event
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			count, conflict, found := ExactCapturedEventCount(test.events, eventID, test.expected)
			if !conflict || count != 0 || found != nil {
				t.Fatalf("unsafe comparison was not a conflict: count=%d conflict=%v found=%#v", count, conflict, found)
			}
		})
	}
}

func comparableCapturedEventFixture(eventID string, schemaVersion any, status string) map[string]any {
	payload := map[string]any{
		"schemaVersion": schemaVersion, "captureEventId": eventID, "status": status,
	}
	payload["capturePayloadDigest"] = CapturedPayloadDigest(payload)
	return map[string]any{
		"kind": "checkpoint_captured", "threadId": "thread-capture", "turnId": "turn-capture", "checkpoint": payload,
	}
}
