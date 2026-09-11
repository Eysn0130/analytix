package evidence

import (
	"errors"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
)

// ValidateHistoricalAcceptedFinalDeliveryEventsV1 is the evidence-owner
// admission boundary for the frozen private Batch V1 tuple. The event owner
// first validates the exact delivery framing; this owner then strictly parses
// the embedded signed V5 and its expanded V2 view. Passing the event framing
// validator alone never grants migration, hydration, replay, or publication
// authority.
func ValidateHistoricalAcceptedFinalDeliveryEventsV1(events []map[string]any) error {
	if err := domainevent.ValidateAcceptedFinalDeliveryEventsV1(events); err != nil {
		return err
	}
	if len(events) == 0 {
		return errors.New("historical accepted final delivery is empty")
	}
	item, itemOK := events[0]["item"].(map[string]any)
	if !itemOK {
		return errors.New("historical accepted final delivery item is invalid")
	}
	record, err := ParseAcceptedFinalRecord(item["acceptedFinal"])
	if err != nil || record.SchemaVersion != AcceptedFinalRecordVersion ||
		record.ThreadID != contracts.StringField(events[0], "threadId") ||
		record.TurnID != contracts.StringField(events[0], "turnId") ||
		record.RecordDigest != contracts.StringField(events[0], "publicationCommitId") ||
		record.AcceptedAt != contracts.StringField(events[0], "timestamp") {
		return errors.Join(errors.New("historical accepted final delivery record authority is invalid"), err)
	}
	if _, err := ParseAcceptedFinalPublicViewV2ForRecord(item["acceptedFinalView"], record); err != nil {
		return errors.Join(errors.New("historical accepted final delivery public view is invalid"), err)
	}
	return nil
}
