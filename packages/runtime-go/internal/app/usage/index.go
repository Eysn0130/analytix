package usage

import (
	"sort"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type IndexRecord struct {
	ThreadID    string   `json:"threadId"`
	TurnID      string   `json:"turnId"`
	Seq         int      `json:"seq"`
	Model       string   `json:"model"`
	Provider    string   `json:"provider"`
	CompletedAt string   `json:"completedAt"`
	UsageSource string   `json:"usageSource,omitempty"`
	ChildRunID  string   `json:"childRunId,omitempty"`
	Usage       Snapshot `json:"usage"`
	RawUsage    Snapshot `json:"rawUsage"`
}

type IndexRecordInput struct {
	Event               map[string]any
	Thread              map[string]any
	Previous            Snapshot
	HasPrevious         bool
	CompletedAtFallback string
}

type IndexRecordResult struct {
	Record         IndexRecord
	OK             bool
	Current        Snapshot
	UpdatePrevious bool
}

func RecordsFromIndex(records []IndexRecord, completedAtFallback string) []Record {
	completedAtFallback = strings.TrimSpace(completedAtFallback)
	if completedAtFallback == "" {
		completedAtFallback = time.Now().UTC().Format(time.RFC3339Nano)
	}
	out := make([]Record, 0, len(records))
	for _, record := range records {
		if !record.Usage.HasUsage() {
			continue
		}
		out = append(out, Record{
			ThreadID:    record.ThreadID,
			TurnID:      record.TurnID,
			Model:       firstNonEmptyAnyString(record.Model, "unknown"),
			Provider:    firstNonEmptyAnyString(record.Provider, "unknown"),
			CompletedAt: firstNonEmptyAnyString(record.CompletedAt, completedAtFallback),
			UsageSource: record.UsageSource,
			ChildRunID:  record.ChildRunID,
			Usage:       record.Usage,
		})
	}
	SortRecords(out)
	return out
}

func SortIndexRecords(records []IndexRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CompletedAt == records[j].CompletedAt {
			if records[i].ThreadID == records[j].ThreadID {
				return records[i].Seq < records[j].Seq
			}
			return records[i].ThreadID < records[j].ThreadID
		}
		return records[i].CompletedAt < records[j].CompletedAt
	})
}

func BuildIndexRecord(input IndexRecordInput) IndexRecordResult {
	event := input.Event
	threadID := strings.TrimSpace(stringField(event, "threadId"))
	if threadID == "" {
		return IndexRecordResult{}
	}
	rawUsageMap, _ := event["usage"].(map[string]any)
	if rawUsageMap == nil {
		return IndexRecordResult{}
	}
	current := SnapshotFromMap(rawUsageMap)
	delta := current
	if input.HasPrevious && LooksCumulativeUsage(current, input.Previous) {
		delta = current.Diff(input.Previous)
	}
	updatePrevious := current.Turns > input.Previous.Turns || current.TotalTokens >= input.Previous.TotalTokens
	if !delta.HasUsage() {
		return IndexRecordResult{Current: current, UpdatePrevious: updatePrevious}
	}
	seq, _ := contracts.NumericSeq(event["seq"])
	completedAtFallback := strings.TrimSpace(input.CompletedAtFallback)
	if completedAtFallback == "" {
		completedAtFallback = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return IndexRecordResult{
		Record: IndexRecord{
			ThreadID:    threadID,
			TurnID:      strings.TrimSpace(stringField(event, "turnId")),
			Seq:         seq,
			Model:       firstNonEmptyAnyString(event["model"], RecordModel(input.Thread, event)),
			Provider:    RecordProvider(input.Thread, event),
			CompletedAt: firstNonEmptyAnyString(event["timestamp"], stringField(input.Thread, "updatedAt"), completedAtFallback),
			UsageSource: strings.TrimSpace(firstNonEmptyAnyString(event["usageSource"])),
			ChildRunID:  strings.TrimSpace(firstNonEmptyAnyString(event["childRunId"])),
			Usage:       delta,
			RawUsage:    current,
		},
		OK:             true,
		Current:        current,
		UpdatePrevious: updatePrevious,
	}
}
