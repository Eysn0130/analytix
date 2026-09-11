package usage

import "testing"

func TestBuildIndexRecordConvertsCumulativeUsageToDelta(t *testing.T) {
	result := BuildIndexRecord(IndexRecordInput{
		Event: map[string]any{
			"threadId":    "thread-a",
			"turnId":      "turn-2",
			"seq":         float64(7),
			"timestamp":   "2026-07-01T10:00:00Z",
			"usageSource": SourceSubagent,
			"childRunId":  "job-1",
			"usage": map[string]any{
				"promptTokens":     float64(150),
				"completionTokens": float64(25),
				"totalTokens":      float64(175),
				"turns":            float64(2),
			},
		},
		Thread: map[string]any{
			"model":      "thread-model",
			"providerId": "thread-provider",
		},
		Previous: Snapshot{
			PromptTokens:     100,
			CompletionTokens: 10,
			TotalTokens:      110,
			Turns:            1,
		},
		HasPrevious: true,
	})
	if !result.OK || !result.UpdatePrevious {
		t.Fatalf("usage index record should be usable and advance previous state: %#v", result)
	}
	if result.Record.Seq != 7 || result.Record.ThreadID != "thread-a" || result.Record.TurnID != "turn-2" {
		t.Fatalf("record identity mismatch: %#v", result.Record)
	}
	if result.Record.Usage.PromptTokens != 50 || result.Record.Usage.CompletionTokens != 15 || result.Record.Usage.TotalTokens != 65 || result.Record.Usage.Turns != 1 {
		t.Fatalf("usage delta mismatch: %#v", result.Record.Usage)
	}
	if result.Record.RawUsage.TotalTokens != 175 || result.Current.TotalTokens != 175 {
		t.Fatalf("raw cumulative usage mismatch: %#v current=%#v", result.Record.RawUsage, result.Current)
	}
	if result.Record.Model != "thread-model" || result.Record.Provider != "thread-provider" || result.Record.UsageSource != SourceSubagent || result.Record.ChildRunID != "job-1" {
		t.Fatalf("record metadata mismatch: %#v", result.Record)
	}
}

func TestBuildIndexRecordUsesTurnModelAndProviderDiagnostics(t *testing.T) {
	result := BuildIndexRecord(IndexRecordInput{
		Event: map[string]any{
			"threadId": "thread-a",
			"turnId":   "turn-1",
			"seq":      float64(1),
			"cacheDiagnostics": map[string]any{
				"providerId": "deepseek",
			},
			"usage": map[string]any{
				"promptTokens":     float64(12),
				"completionTokens": float64(3),
			},
		},
		Thread: map[string]any{
			"updatedAt":  "2026-07-01T09:00:00Z",
			"model":      "thread-model",
			"providerId": "thread-provider",
			"turns": []any{
				map[string]any{"id": "turn-1", "model": "turn-model"},
			},
		},
	})
	if !result.OK {
		t.Fatalf("usage index record should be usable: %#v", result)
	}
	if result.Record.Model != "turn-model" || result.Record.Provider != "deepseek" || result.Record.CompletedAt != "2026-07-01T09:00:00Z" {
		t.Fatalf("record metadata should prefer turn and provider diagnostics: %#v", result.Record)
	}
}

func TestBuildIndexRecordRejectsMissingAndZeroUsage(t *testing.T) {
	missing := BuildIndexRecord(IndexRecordInput{Event: map[string]any{"threadId": "thread-a"}})
	if missing.OK || missing.UpdatePrevious {
		t.Fatalf("missing usage should not produce a record: %#v", missing)
	}
	zero := BuildIndexRecord(IndexRecordInput{
		Event: map[string]any{
			"threadId": "thread-a",
			"usage":    map[string]any{},
		},
		HasPrevious: true,
	})
	if zero.OK || !zero.UpdatePrevious || zero.Current.HasUsage() {
		t.Fatalf("zero usage should only advance raw state: %#v", zero)
	}
}

func TestRecordsFromIndexFiltersDefaultsAndSorts(t *testing.T) {
	records := RecordsFromIndex([]IndexRecord{
		{
			ThreadID:    "thread-b",
			TurnID:      "turn-b",
			Model:       "model-b",
			Provider:    "provider-b",
			CompletedAt: "2026-07-01T10:00:00Z",
			Usage:       Snapshot{TotalTokens: 2},
		},
		{
			ThreadID: "thread-z",
			TurnID:   "turn-zero",
		},
		{
			ThreadID:    "thread-a",
			TurnID:      "turn-a",
			CompletedAt: "",
			Usage:       Snapshot{TotalTokens: 1},
		},
	}, "2026-07-01T09:00:00Z")

	if len(records) != 2 {
		t.Fatalf("index records should filter zero-usage records: %#v", records)
	}
	if records[0].ThreadID != "thread-a" || records[0].Model != "unknown" || records[0].Provider != "unknown" || records[0].CompletedAt != "2026-07-01T09:00:00Z" {
		t.Fatalf("defaulted first record mismatch: %#v", records[0])
	}
	if records[1].ThreadID != "thread-b" {
		t.Fatalf("records should sort by completedAt: %#v", records)
	}
}

func TestSortIndexRecordsOrdersByCompletionThreadAndSeq(t *testing.T) {
	records := []IndexRecord{
		{ThreadID: "thread-b", Seq: 1, CompletedAt: "2026-07-01T10:00:00Z"},
		{ThreadID: "thread-a", Seq: 2, CompletedAt: "2026-07-01T10:00:00Z"},
		{ThreadID: "thread-a", Seq: 1, CompletedAt: "2026-07-01T10:00:00Z"},
		{ThreadID: "thread-c", Seq: 1, CompletedAt: "2026-07-01T09:00:00Z"},
	}
	SortIndexRecords(records)
	got := []string{
		records[0].ThreadID,
		records[1].ThreadID,
		records[2].ThreadID,
		records[3].ThreadID,
	}
	want := []string{"thread-c", "thread-a", "thread-a", "thread-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort order mismatch: got %#v want %#v", got, want)
		}
	}
	if records[1].Seq != 1 || records[2].Seq != 2 {
		t.Fatalf("same-thread records should sort by seq: %#v", records)
	}
}
