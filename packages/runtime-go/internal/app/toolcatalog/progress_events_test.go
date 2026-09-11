package toolcatalog

import (
	"testing"

	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestBuildToolProgressEventUsesStableRuntimeShape(t *testing.T) {
	event, ok := BuildToolProgressEvent(ToolProgressEventInput{
		ThreadID: "thread-1",
		TurnID:   "turn-1",
		ItemID:   "item-1",
		CallID:   "call-1",
		ToolName: "grep",
		Status:   "running",
		Message:  "grep running",
		Child:    map[string]any{"job": "job-1"},
	})
	if !ok {
		t.Fatalf("progress event should be built")
	}
	if event["kind"] != "tool_progress" ||
		event["threadId"] != "thread-1" ||
		event["turnId"] != "turn-1" ||
		event["itemId"] != "item-1" ||
		event["callId"] != "call-1" ||
		event["toolName"] != "grep" ||
		event["summary"] != "grep" ||
		event["status"] != "running" ||
		event["message"] != "grep running" {
		t.Fatalf("progress event shape mismatch: %#v", event)
	}
	if child, ok := event["child"].(map[string]any); !ok || child["job"] != "job-1" {
		t.Fatalf("progress event child mismatch: %#v", event)
	}
}

func TestBuildToolProgressEventRejectsMissingIdentity(t *testing.T) {
	if event, ok := BuildToolProgressEvent(ToolProgressEventInput{ItemID: "", CallID: "call-1"}); ok || event != nil {
		t.Fatalf("missing item id should reject event: %#v", event)
	}
	if event, ok := BuildToolProgressEvent(ToolProgressEventInput{ItemID: "item-1", CallID: ""}); ok || event != nil {
		t.Fatalf("missing call id should reject event: %#v", event)
	}
}

func TestBuildToolStartedEventMaterializesRunningItem(t *testing.T) {
	event, ok := BuildToolStartedEvent(ToolStartedEventInput{
		ThreadID:  "thread-1",
		TurnID:    "turn-1",
		ItemID:    "item-1",
		CallID:    "call-1",
		ToolName:  "write",
		CreatedAt: "2026-07-03T00:00:00Z",
	})
	if !ok {
		t.Fatalf("tool started event should be built")
	}
	if event["kind"] != "tool_call_started" || event["itemId"] != "item-1" {
		t.Fatalf("started event shape mismatch: %#v", event)
	}
	item, ok := event["item"].(map[string]any)
	if !ok {
		t.Fatalf("started event item missing: %#v", event)
	}
	args, ok := item["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("started event arguments missing: %#v", item)
	}
	if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(args); err != nil {
		t.Fatalf("started event exposed a non-closed argument shape: %#v err=%v", args, err)
	}
	if item["status"] != "running" ||
		item["kind"] != "tool_call" ||
		item["toolKind"] != "file_change" ||
		item["createdAt"] != "2026-07-03T00:00:00Z" {
		t.Fatalf("started item mismatch: %#v", item)
	}
}

func TestBuildToolStartedEventRejectsMissingIdentity(t *testing.T) {
	if event, ok := BuildToolStartedEvent(ToolStartedEventInput{ItemID: "", CallID: "call-1"}); ok || event != nil {
		t.Fatalf("missing item id should reject started event: %#v", event)
	}
	if event, ok := BuildToolStartedEvent(ToolStartedEventInput{ItemID: "item-1", CallID: ""}); ok || event != nil {
		t.Fatalf("missing call id should reject started event: %#v", event)
	}
}
