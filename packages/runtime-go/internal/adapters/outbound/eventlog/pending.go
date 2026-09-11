package eventlog

import (
	"sort"

	contracts "analytix.local/runtime-go/internal/contracts"
)

func SnapshotPendingEvents(pending map[int]map[string]any, afterSeq int) []map[string]any {
	events := make([]map[string]any, 0, len(pending))
	for seq, event := range pending {
		if seq > afterSeq {
			events = append(events, contracts.CloneMap(event))
		}
	}
	return events
}

// MergePendingEvents merges an immutable pending snapshot into a durable
// replay without duplicating sequence numbers.
func MergePendingEvents(afterSeq int, result LoadResult, pending []map[string]any) LoadResult {
	if len(pending) == 0 {
		return result
	}
	seenSeq := map[int]bool{}
	for _, event := range result.Events {
		if seq, ok := contracts.NumericSeq(event["seq"]); ok {
			seenSeq[seq] = true
		}
	}
	for _, event := range pending {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= afterSeq || seenSeq[seq] {
			continue
		}
		result.Events = append(result.Events, contracts.CloneMap(event))
	}
	sort.SliceStable(result.Events, func(i, j int) bool {
		left, _ := contracts.NumericSeq(result.Events[i]["seq"])
		right, _ := contracts.NumericSeq(result.Events[j]["seq"])
		return left < right
	})
	return result
}
