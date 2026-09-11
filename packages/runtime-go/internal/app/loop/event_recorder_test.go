package loop

import (
	"errors"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type runtimeLoopEventRecorderStub struct {
	events     []map[string]any
	batchCalls int
	batchSizes []int
	batchErr   error
	batchErrAt int
}

type runtimeNonAtomicEventRecorderStub struct {
	events []map[string]any
}

func (stub *runtimeNonAtomicEventRecorderStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, nil, nil
}

func (stub *runtimeLoopEventRecorderStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, nil, nil
}

func (stub *runtimeLoopEventRecorderStub) RecordEventsAtomic(events []map[string]any) ([]map[string]any, []string, error) {
	stub.batchCalls++
	if stub.batchErr != nil && (stub.batchErrAt == 0 || stub.batchCalls == stub.batchErrAt) {
		return nil, nil, stub.batchErr
	}
	stub.batchSizes = append(stub.batchSizes, len(events))
	stub.events = append(stub.events, events...)
	return events, nil, nil
}

func TestRuntimeEventRecorderPersistsLoopEvents(t *testing.T) {
	stub := &runtimeLoopEventRecorderStub{}
	recorder := NewRuntimeEventRecorder(stub)
	if err := recorder.AssistantTextDelta("thr_1", "turn_1", "hello"); err != nil {
		t.Fatalf("assistant delta: %v", err)
	}
	if err := recorder.ToolCallPartial("thr_1", "turn_1", domainmodel.ToolCall{ID: "call_1", Name: "read"}); err != nil {
		t.Fatalf("tool partial: %v", err)
	}
	if len(stub.events) != 2 {
		t.Fatalf("expected two events, got %#v", stub.events)
	}
	first := stub.events[0]
	if first["kind"] != "assistant_text_delta" || first["threadId"] != "thr_1" || first["turnId"] != "turn_1" {
		t.Fatalf("assistant delta identity mismatch: %#v", first)
	}
	item, _ := first["item"].(map[string]any)
	if item["createdAt"] == "" || item["text"] != "hello" {
		t.Fatalf("assistant delta item mismatch: %#v", item)
	}
	second := stub.events[1]
	if second["kind"] != "tool_call_ready" || second["callId"] != "call_1" || second["toolName"] != "read" {
		t.Fatalf("tool partial event mismatch: %#v", second)
	}
}

func TestRuntimeEventRecorderAddsTraceFieldsOnlyWhenEnabled(t *testing.T) {
	stub := &runtimeLoopEventRecorderStub{}
	recorder := NewRuntimeEventRecorder(stub)
	if err := recorder.AssistantTextDelta("thr_1", "turn_1", "hello"); err != nil {
		t.Fatalf("assistant delta: %v", err)
	}
	if _, ok := stub.events[0]["trace"]; ok {
		t.Fatalf("trace should be disabled by default: %#v", stub.events[0])
	}

	stub = &runtimeLoopEventRecorderStub{}
	recorder = NewRuntimeEventRecorderWithTrace(stub, true)
	base := time.Unix(100, 0).UTC()
	if err := recorder.AssistantTextDeltaWithTrace("thr_1", "turn_1", "hello", domainmodel.ChunkTrace{
		ProviderRequestSentAt:     base,
		ProviderResponseHeadersAt: base.Add(10 * time.Millisecond),
		ProviderRawSSEChunkAt:     base.Add(20 * time.Millisecond),
		ProviderChunkParsedAt:     base.Add(21 * time.Millisecond),
		LoopChunkCallbackAt:       base.Add(22 * time.Millisecond),
	}); err != nil {
		t.Fatalf("assistant delta: %v", err)
	}
	trace, _ := stub.events[0]["trace"].(map[string]any)
	for _, key := range []string{
		"provider_request_sent_at",
		"provider_response_headers_at",
		"provider_raw_sse_chunk_at",
		"provider_chunk_parsed_at",
		"loop_chunk_callback_at",
		"event_record_started_at",
		"event_recorded_at",
	} {
		if _, ok := trace[key].(float64); !ok {
			t.Fatalf("missing trace key %s in %#v", key, trace)
		}
	}
}

func TestRuntimeEventRecorderSanitizesProviderErrors(t *testing.T) {
	stub := &runtimeLoopEventRecorderStub{}
	recorder := NewRuntimeEventRecorder(stub)
	if err := recorder.ProviderRetrying("thr_1", "turn_1", 2, 3, errors.New("failed bearer secret-token")); err != nil {
		t.Fatalf("provider retrying: %v", err)
	}
	details, _ := stub.events[0]["details"].(map[string]any)
	if strings.Contains(strings.ToLower(details["message"].(string)), "secret-token") {
		t.Fatalf("provider retry event leaked secret: %#v", details)
	}
}

func TestRuntimeEventRecorderPersistsPipelineStagesAsOneValidatedBatch(t *testing.T) {
	stub := &runtimeLoopEventRecorderStub{}
	recorder := NewRuntimeEventRecorder(stub)
	base := time.Unix(100, 0).UTC()
	err := recorder.PipelineStages("thr_1", "turn_1", []domainmodel.PipelineStage{
		{Stage: "setup", At: base, Details: map[string]any{"workspaceBound": true, "caseBound": false}},
		{Stage: "pre_start", At: base.Add(time.Millisecond), Details: map[string]any{
			"approvalPolicy": "on-request",
			"sandboxMode":    "workspace-write",
		}},
	})
	if err != nil {
		t.Fatalf("pipeline batch: %v", err)
	}
	if stub.batchCalls != 1 || len(stub.batchSizes) != 1 || stub.batchSizes[0] != 2 || len(stub.events) != 2 {
		t.Fatalf("pipeline stages were not recorded as one batch: calls=%d sizes=%#v events=%#v", stub.batchCalls, stub.batchSizes, stub.events)
	}
	if stub.events[0]["stage"] != "setup" || stub.events[1]["stage"] != "pre_start" {
		t.Fatalf("pipeline batch order changed: %#v", stub.events)
	}
	if stub.events[0]["timestamp"] != base.Format(time.RFC3339Nano) ||
		stub.events[1]["timestamp"] != base.Add(time.Millisecond).Format(time.RFC3339Nano) {
		t.Fatalf("pipeline batch timestamps changed: %#v", stub.events)
	}

	beforeCalls, beforeEvents := stub.batchCalls, len(stub.events)
	if err := recorder.PipelineStages("thr_1", "turn_1", []domainmodel.PipelineStage{
		{Stage: "setup", Details: map[string]any{"workspaceBound": true, "caseBound": false}},
		{Stage: "unknown", Details: map[string]any{}},
	}); err == nil {
		t.Fatal("invalid pipeline batch should fail closed")
	}
	if stub.batchCalls != beforeCalls || len(stub.events) != beforeEvents {
		t.Fatalf("invalid pipeline batch mutated the recorder: calls=%d events=%#v", stub.batchCalls, stub.events)
	}
}

func TestRuntimeEventRecorderRequiresAtomicStoreForCriticalPipelineStage(t *testing.T) {
	stub := &runtimeNonAtomicEventRecorderStub{}
	err := NewRuntimeEventRecorder(stub).PipelineStageAtomic("thr_1", "turn_1", domainmodel.PipelineStage{
		Stage: "pre_send", At: time.Unix(100, 0).UTC(),
	})
	if err == nil || len(stub.events) != 0 {
		t.Fatalf("critical pipeline stage used a non-atomic fallback: events=%#v err=%v", stub.events, err)
	}
}

func TestRuntimeEventRecorderRequiresAtomicStoreForCriticalPipelineStageBatch(t *testing.T) {
	stub := &runtimeNonAtomicEventRecorderStub{}
	err := NewRuntimeEventRecorder(stub).PipelineStagesAtomic("thr_1", "turn_1", []domainmodel.PipelineStage{
		{Stage: "setup", At: time.Unix(100, 0).UTC(), Details: map[string]any{"workspaceBound": true, "caseBound": false}},
		{Stage: "pre_send", At: time.Unix(101, 0).UTC()},
	})
	if err == nil || len(stub.events) != 0 {
		t.Fatalf("critical pipeline stage batch used a non-atomic fallback: events=%#v err=%v", stub.events, err)
	}
}

func TestRuntimeEventRecorderRejectsMissingRecorder(t *testing.T) {
	if err := NewRuntimeEventRecorder(nil).ProviderError("thr_1", "turn_1", errors.New("boom")); err == nil {
		t.Fatal("expected missing recorder error")
	}
}
