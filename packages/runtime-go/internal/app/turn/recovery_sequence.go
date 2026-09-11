package turn

import domainjob "analytix.local/runtime-go/internal/domain/job"

func RestoredRuntimeSequenceV1(current int, events []map[string]any, jobs []domainjob.Record) int {
	for _, event := range events {
		turnID, _ := event["turnId"].(string)
		if sequence, ok := SequenceID(turnID); ok && sequence > current {
			current = sequence
		}
	}
	for _, record := range jobs {
		if sequence, ok := SequenceID(record.AutoContinueTurnID); ok && sequence > current {
			current = sequence
		}
	}
	return current
}
