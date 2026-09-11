package turn

import (
	"fmt"
	"strings"
	"testing"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestBuildFailureRecordDiscardsUnverifiedAssistantDraft(t *testing.T) {
	record := BuildFailureRecord(FailureRecordInput{
		ThreadID:   " thread-a ",
		TurnID:     " turn-a ",
		Failure:    domainfailure.New("provider_error", map[string]any{"status": float64(500)}),
		FinishedAt: "2026-07-02T00:00:00Z",
		Model:      "gpt-5",
		Result: domainmodel.Result{Usage: domainmodel.Usage{
			PromptTokens:     10,
			CompletionTokens: 2,
			TotalTokens:      12,
		}},
		UsageSource: "subagent",
		ChildRunID:  "job-1",
		Events: []map[string]any{
			{
				"kind":     "assistant_text_delta",
				"threadId": "thread-a",
				"turnId":   "turn-a",
				"item": map[string]any{
					"id":   "item_text",
					"kind": "assistant_text",
					"text": "partial",
				},
			},
		},
	})
	if record.ErrorItemID != "item_turn-a_error" {
		t.Fatalf("error item id mismatch: %q", record.ErrorItemID)
	}
	if len(record.Items) != 1 {
		t.Fatalf("expected only the fixed error item, got %#v", record.Items)
	}
	if strings.Contains(fmt.Sprint(record.Items), "partial") {
		t.Fatalf("unverified assistant draft was materialized: %#v", record.Items)
	}
	errorItem := record.Items[0]
	if errorItem["kind"] != "error" ||
		errorItem["message"] != "The turn failed before a verified response was available." ||
		errorItem["code"] != "provider_error" ||
		errorItem["severity"] != "error" {
		t.Fatalf("error item mismatch: %#v", errorItem)
	}
	if record.TurnFailedEvent["kind"] != "turn_failed" ||
		record.TurnFailedEvent["message"] != "The turn failed before a verified response was available." ||
		record.TurnFailedEvent["code"] != "provider_error" {
		t.Fatalf("turn_failed event mismatch: %#v", record.TurnFailedEvent)
	}
	if record.UsageEvent["kind"] != "usage" ||
		record.UsageEvent["model"] != "gpt-5" ||
		record.UsageEvent["usageSource"] != "subagent" ||
		record.UsageEvent["childRunId"] != "job-1" ||
		record.UsageEvent["usageFinalStatus"] != "failed" {
		t.Fatalf("failure usage event mismatch: %#v", record.UsageEvent)
	}
	events := record.RecordEvents()
	if len(events) != 3 {
		t.Fatalf("expected error item, usage, and turn_failed events: %#v", events)
	}
	if events[0].Event["kind"] != "item_completed" ||
		events[0].Event["threadId"] != "thread-a" ||
		events[0].Event["turnId"] != "turn-a" ||
		events[0].Label != "record failed turn error item event" {
		t.Fatalf("error completed event mismatch: %#v", events[0])
	}
	if events[0].Label != "record failed turn error item event" ||
		events[1].Label != "record failed turn usage event" ||
		events[2].Label != "record turn_failed event" {
		t.Fatalf("failure event labels mismatch: %#v", events)
	}
}

func TestBuildFailureRecordDefaultsMessageAndSeverity(t *testing.T) {
	record := BuildFailureRecord(FailureRecordInput{
		ThreadID:   "thread-a",
		TurnID:     "turn-a",
		FinishedAt: "2026-07-02T00:00:00Z",
	})
	if len(record.Items) != 1 {
		t.Fatalf("expected only error item, got %#v", record.Items)
	}
	if record.Items[0]["message"] != "The turn failed before a verified response was available." || record.Items[0]["severity"] != "error" || record.Items[0]["code"] != "turn_failed" {
		t.Fatalf("default failure item mismatch: %#v", record.Items[0])
	}
	if record.TurnFailedEvent["message"] != "The turn failed before a verified response was available." || record.TurnFailedEvent["severity"] != "error" || record.TurnFailedEvent["code"] != "turn_failed" {
		t.Fatalf("default turn_failed event mismatch: %#v", record.TurnFailedEvent)
	}
}

func TestBuildRuntimeRestartedAbortRecord(t *testing.T) {
	record := BuildRuntimeRestartedAbortRecord(RuntimeRestartedAbortRecordInput{
		ThreadID:   "thread-a",
		TurnID:     "turn-a",
		FinishedAt: "2026-07-02T00:00:00Z",
		Events: []map[string]any{
			{
				"kind":     "assistant_text_delta",
				"threadId": "thread-a",
				"turnId":   "turn-a",
				"item": map[string]any{
					"id":   "item_text",
					"kind": "assistant_text",
					"text": "partial",
				},
			},
		},
	})
	if len(record.Items) != 1 {
		t.Fatalf("expected only the fixed restart item, got %#v", record.Items)
	}
	if record.Items[0]["status"] != "aborted" || record.Items[0]["code"] != "runtime_restarted" || strings.Contains(fmt.Sprint(record.Items), "partial") {
		t.Fatalf("restart abort items mismatch: %#v", record.Items)
	}
	if record.TurnAbortedEvent["kind"] != "turn_aborted" || record.TurnAbortedEvent["code"] != "runtime_restarted" {
		t.Fatalf("turn_aborted event mismatch: %#v", record.TurnAbortedEvent)
	}
	events := record.RecordEvents()
	if len(events) != 2 {
		t.Fatalf("expected restart item and turn_aborted events: %#v", events)
	}
	if events[0].Label != "record restarted turn item event" ||
		events[0].Event["kind"] != "item_completed" ||
		events[0].Event["itemId"] != "item_turn-a_runtime_restarted" {
		t.Fatalf("restart error event mismatch: %#v", events[0])
	}
	if events[1].Label != "record restarted turn_aborted event" || events[1].Event["code"] != "runtime_restarted" {
		t.Fatalf("restart terminal event mismatch: %#v", events[1])
	}
}
