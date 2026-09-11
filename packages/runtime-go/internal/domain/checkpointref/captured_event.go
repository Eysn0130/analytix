package checkpointref

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

type CapturedEventIdentity struct {
	ThreadID string
	EventID  string
}

func ParseCapturedEventIdentity(event map[string]any) (CapturedEventIdentity, error) {
	if exactCapturedString(event["kind"]) != "checkpoint_captured" {
		return CapturedEventIdentity{}, errors.New("checkpoint captured event kind is invalid")
	}
	threadID := exactCapturedString(event["threadId"])
	if threadID == "" {
		return CapturedEventIdentity{}, errors.New("checkpoint captured event thread identity is invalid")
	}
	checkpoint, _ := event["checkpoint"].(map[string]any)
	if checkpoint == nil || !CapturedPayloadDigestMatches(checkpoint) {
		return CapturedEventIdentity{}, errors.New("checkpoint captured event payload digest is invalid")
	}
	eventID := exactCapturedString(checkpoint["captureEventId"])
	if !IsCaptureEventIDV2(eventID) {
		return CapturedEventIdentity{}, errors.New("checkpoint captured event id is invalid")
	}
	return CapturedEventIdentity{ThreadID: threadID, EventID: eventID}, nil
}

func ExactCapturedEventCount(events []map[string]any, eventID string, expected map[string]any) (int, bool, map[string]any) {
	count := 0
	var persisted map[string]any
	expectedJSON, err := json.Marshal(CapturedEventComparable(expected))
	if err != nil {
		return 0, true, nil
	}
	for _, event := range events {
		checkpoint, _ := event["checkpoint"].(map[string]any)
		if checkpoint == nil || exactCapturedString(checkpoint["captureEventId"]) != eventID {
			continue
		}
		persistedJSON, marshalErr := json.Marshal(CapturedEventComparable(event))
		if _, err := ParseCapturedEventIdentity(event); err != nil || marshalErr != nil || !bytes.Equal(persistedJSON, expectedJSON) {
			return count, true, nil
		}
		count++
		persisted = event
	}
	return count, false, persisted
}

func CapturedEventComparable(event map[string]any) map[string]any {
	comparable := make(map[string]any, len(event))
	for key, value := range event {
		if key != "seq" && key != "timestamp" {
			comparable[key] = value
		}
	}
	return comparable
}

func exactCapturedString(value any) string {
	text, _ := value.(string)
	if text == "" || text != strings.TrimSpace(text) {
		return ""
	}
	return text
}
