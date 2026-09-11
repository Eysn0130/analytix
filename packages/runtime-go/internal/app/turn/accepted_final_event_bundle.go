package turn

import (
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

// PrepareAcceptedFinalEventBundle validates one committed accepted-final
// manifest and assigns its contiguous durable sequence range before any write.
func PrepareAcceptedFinalEventBundle(thread map[string]any, drafts []map[string]any, nextSeq int) ([]map[string]any, error) {
	if thread == nil || len(drafts) == 0 || nextSeq <= 0 {
		return nil, errors.New("accepted final event bundle input is invalid")
	}
	threadID := strings.TrimSpace(bundleString(drafts[0], "threadId"))
	commitID := strings.TrimSpace(bundleString(drafts[0], "publicationCommitId"))
	if threadID == "" || commitID == "" || strings.TrimSpace(bundleString(thread, "id")) != threadID {
		return nil, errors.New("accepted final event bundle identity is invalid")
	}
	seenEventIDs := map[string]bool{}
	seenSlots := map[string]bool{}
	events := make([]map[string]any, 0, len(drafts))
	for offset, draft := range drafts {
		if strings.TrimSpace(bundleString(draft, "threadId")) != threadID ||
			strings.TrimSpace(bundleString(draft, "publicationCommitId")) != commitID ||
			strings.TrimSpace(bundleString(draft, "acceptedFinalDigest")) != commitID {
			return nil, errors.New("accepted final event bundle binding is inconsistent")
		}
		eventID := strings.TrimSpace(bundleString(draft, "publicationEventId"))
		slot := strings.TrimSpace(bundleString(draft, "publicationSlot"))
		payloadDigest := strings.TrimSpace(bundleString(draft, "publicationPayloadDigest"))
		if eventID == "" || slot == "" || payloadDigest == "" || seenEventIDs[eventID] || seenSlots[slot] {
			return nil, errors.New("accepted final event bundle manifest is invalid")
		}
		seenEventIDs[eventID] = true
		seenSlots[slot] = true
		projected, err := SanitizeCaseEventPublication(thread, draft)
		if err != nil {
			return nil, err
		}
		event := contracts.CloneMap(projected)
		event["threadId"] = threadID
		event["seq"] = float64(nextSeq + offset)
		if bundleString(event, "publicationPayloadDigest") != payloadDigest || AcceptedFinalPublicationPayloadDigest(event) != payloadDigest {
			return nil, errors.New("accepted final event bundle payload changed during projection")
		}
		events = append(events, event)
	}
	return events, nil
}

func bundleString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
