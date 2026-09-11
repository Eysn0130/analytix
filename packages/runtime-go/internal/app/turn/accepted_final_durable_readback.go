package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
)

// ValidateAcceptedFinalDurableReadbackV1 proves that expected is the one and
// only exact durable group for its publication commit. When requireTail is
// true, no later event may have acquired a sequence number.
func ValidateAcceptedFinalDurableReadbackV1(all, expected []map[string]any, requireTail bool) error {
	if len(all) == 0 || len(expected) == 0 || domainevent.ValidateAcceptedFinalDeliveryEventsV2(expected) != nil {
		return errors.New("accepted final durable readback manifest is invalid")
	}
	threadID := strings.TrimSpace(contracts.StringField(expected[0], "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(expected[0], "turnId"))
	commitID := strings.TrimSpace(contracts.StringField(expected[0], "publicationCommitId"))
	if threadID == "" || turnID == "" || commitID == "" {
		return errors.New("accepted final durable readback identity is invalid")
	}
	matched := make([]map[string]any, 0, len(expected))
	firstMatch := -1
	lastMatch := -1
	previousSeq := 0
	for index, event := range all {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= 0 || (previousSeq != 0 && seq != previousSeq+1) ||
			strings.TrimSpace(contracts.StringField(event, "threadId")) != threadID {
			return errors.New("accepted final durable event log is not exact and contiguous")
		}
		previousSeq = seq
		eventCommitID := strings.TrimSpace(contracts.StringField(event, "publicationCommitId"))
		acceptedFinalDigest := strings.TrimSpace(contracts.StringField(event, "acceptedFinalDigest"))
		if (eventCommitID == "") != (acceptedFinalDigest == "") ||
			(eventCommitID != "" && acceptedFinalDigest != eventCommitID) {
			return errors.New("accepted final durable readback contains a partial publication marker")
		}
		if eventCommitID != commitID {
			continue
		}
		if strings.TrimSpace(contracts.StringField(event, "turnId")) != turnID {
			return errors.New("accepted final durable readback commit crosses turns")
		}
		if firstMatch < 0 {
			firstMatch = index
		}
		lastMatch = index
		matched = append(matched, event)
	}
	if firstMatch < 0 || len(matched) != len(expected) || lastMatch-firstMatch+1 != len(expected) ||
		(requireTail && lastMatch != len(all)-1) {
		return errors.New("accepted final durable readback is not one exact commit group")
	}
	for index := range expected {
		matchedJSON, matchedErr := json.Marshal(matched[index])
		expectedJSON, expectedErr := json.Marshal(expected[index])
		if matchedErr != nil || expectedErr != nil || !bytes.Equal(matchedJSON, expectedJSON) {
			return errors.New("accepted final durable readback differs from expected events")
		}
	}
	return nil
}
