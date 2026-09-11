package turn

import "testing"

func TestPendingGateCancellationEvents(t *testing.T) {
	events := PendingGateCancellationEvents([]PendingGateCancellation{
		{
			Kind:     "approval",
			ID:       "approval-a",
			ThreadID: "thread-a",
			TurnID:   "turn-a",
			ItemID:   "item-approval",
			ToolName: "bash",
		},
		{
			Kind:     "user_input",
			ID:       "input-a",
			ThreadID: "thread-a",
			TurnID:   "turn-a",
			ItemID:   "item-input",
			Prompt:   "Continue?",
		},
	}, "turn_aborted")
	if len(events) != 2 {
		t.Fatalf("expected approval and user-input events, got %#v", events)
	}
	if events[0]["kind"] != "approval_resolved" ||
		events[0]["status"] != "expired" ||
		events[0]["summary"] != "Approve bash" {
		t.Fatalf("approval cancellation event mismatch: %#v", events[0])
	}
	if events[1]["kind"] != "user_input_resolved" ||
		events[1]["status"] != "cancelled" ||
		events[1]["prompt"] != "Continue?" {
		t.Fatalf("user-input cancellation event mismatch: %#v", events[1])
	}
}

func TestPendingGateCancellationEventsUseDefaults(t *testing.T) {
	events := PendingGateCancellationEvents([]PendingGateCancellation{
		{Kind: "approval"},
		{Kind: "user_input"},
		{Kind: "ignored"},
	}, "")
	if len(events) != 2 {
		t.Fatalf("expected unsupported gate kind to be skipped, got %#v", events)
	}
	if events[0]["toolName"] != "tool" || events[0]["summary"] != "Approve tool" {
		t.Fatalf("approval default mismatch: %#v", events[0])
	}
	if events[1]["prompt"] != "User input required" {
		t.Fatalf("input default mismatch: %#v", events[1])
	}
}
