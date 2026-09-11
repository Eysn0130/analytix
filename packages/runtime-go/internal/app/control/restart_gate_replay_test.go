package control

import "testing"

func TestReplayPendingGatesAppliesTerminalAndResolutionEvents(t *testing.T) {
	replay := ReplayPendingGates("thread-a", []map[string]any{
		{"kind": "approval_requested", "approvalId": "approval-old", "turnId": "turn-old", "toolName": "write_file"},
		{"kind": "turn_aborted", "turnId": "turn-old"},
		{"kind": "approval_requested", "approvalId": "approval-live", "turnId": "turn-live", "toolName": "read_file"},
		{"kind": "user_input_requested", "inputId": "input-closed", "turnId": "turn-live", "prompt": "secret"},
		{"kind": "user_input_resolved", "inputId": "input-closed", "turnId": "turn-live", "status": "submitted"},
		{"kind": "user_input_requested", "inputId": "input-live", "turnId": "turn-live", "prompt": "confirm"},
	})
	if len(replay.Approvals) != 1 || replay.Approvals["approval-live"].ToolName != "read_file" ||
		len(replay.Inputs) != 1 || replay.Inputs["input-live"].Prompt != "confirm" {
		t.Fatalf("restart gate replay mismatch: %#v", replay)
	}
	if replay.ClosedApprovals["approval-old"].TurnID != "turn-old" {
		t.Fatalf("terminal gate must remain as a non-executable tombstone: %#v", replay.ClosedApprovals)
	}
	if replay.ClosedInputs["input-closed"].TurnID != "turn-live" || replay.ClosedInputResolutions["input-closed"].Status != "submitted" {
		t.Fatalf("resolved input must remain a non-executable auditable tombstone: %#v", replay.ClosedInputs)
	}
}

func TestReplayPendingGatesPreservesHostCancelledTombstones(t *testing.T) {
	replay := ReplayPendingGates("thread-a", []map[string]any{
		{"kind": "approval_requested", "threadId": "thread-a", "approvalId": "approval-closed", "turnId": "turn-a", "itemId": "item-a", "toolName": "write_file", "continuationReceiptId": "receipt-a"},
		{"kind": "approval_resolved", "threadId": "thread-a", "approvalId": "approval-closed", "turnId": "turn-a", "itemId": "item-a", "status": "expired", "cancelledBy": "runtime_shutdown"},
		{"kind": "user_input_requested", "threadId": "thread-a", "inputId": "input-closed", "turnId": "turn-a", "itemId": "item-i", "prompt": "confirm", "continuationReceiptId": "receipt-i"},
		{"kind": "user_input_resolved", "threadId": "thread-a", "inputId": "input-closed", "turnId": "turn-a", "itemId": "item-i", "status": "cancelled", "cancelledBy": "runtime_shutdown"},
		{"kind": "turn_aborted", "threadId": "thread-a", "turnId": "turn-a"},
	})
	if len(replay.Approvals) != 0 || len(replay.Inputs) != 0 {
		t.Fatalf("closed gates must never replay as pending: %#v", replay)
	}
	if got := replay.ClosedApprovals["approval-closed"]; got.ContinuationReceiptID != "receipt-a" || got.ToolName != "write_file" {
		t.Fatalf("approval tombstone lost immutable request identity: %#v", got)
	}
	if got := replay.ClosedInputs["input-closed"]; got.ContinuationReceiptID != "receipt-i" || got.Prompt != "confirm" {
		t.Fatalf("input tombstone lost immutable request identity: %#v", got)
	}
}

func TestReplayPendingGatesPreservesEverySignedResolutionClass(t *testing.T) {
	replay := ReplayPendingGates("thread-a", []map[string]any{
		{"kind": "approval_requested", "threadId": "thread-a", "approvalId": "approval-allowed", "turnId": "turn-a", "itemId": "item-aa"},
		{"kind": "approval_resolved", "threadId": "thread-a", "approvalId": "approval-allowed", "turnId": "turn-a", "itemId": "item-aa", "status": "allowed"},
		{"kind": "approval_requested", "threadId": "thread-a", "approvalId": "approval-denied", "turnId": "turn-a", "itemId": "item-ad"},
		{"kind": "approval_resolved", "threadId": "thread-a", "approvalId": "approval-denied", "turnId": "turn-a", "itemId": "item-ad", "status": "denied"},
		{"kind": "user_input_requested", "threadId": "thread-a", "inputId": "input-submitted", "turnId": "turn-a", "itemId": "item-is"},
		{"kind": "user_input_resolved", "threadId": "thread-a", "inputId": "input-submitted", "turnId": "turn-a", "itemId": "item-is", "status": "submitted"},
		{"kind": "user_input_requested", "threadId": "thread-a", "inputId": "input-cancelled", "turnId": "turn-a", "itemId": "item-ic"},
		{"kind": "user_input_resolved", "threadId": "thread-a", "inputId": "input-cancelled", "turnId": "turn-a", "itemId": "item-ic", "status": "cancelled"},
	})
	if len(replay.Approvals) != 0 || len(replay.Inputs) != 0 ||
		len(replay.ClosedApprovals) != 2 || len(replay.ClosedInputs) != 2 || len(replay.InvalidGateIDs) != 0 {
		t.Fatalf("resolution tombstone replay mismatch: %#v", replay)
	}
}

func TestReplayPendingGatesRejectsResolutionIdentityMismatch(t *testing.T) {
	replay := ReplayPendingGates("thread-a", []map[string]any{
		{"kind": "approval_requested", "threadId": "thread-a", "approvalId": "approval-a", "turnId": "turn-a", "itemId": "item-a"},
		{"kind": "approval_resolved", "threadId": "thread-a", "approvalId": "approval-a", "turnId": "turn-other", "itemId": "item-a", "status": "allowed"},
	})
	if !replay.InvalidGateIDs["approval-a"] || len(replay.Approvals)+len(replay.ClosedApprovals) != 0 {
		t.Fatalf("mismatched resolution retained authority: %#v", replay)
	}
}

func TestReplayPendingGatesInvalidatesReusedClosedGateID(t *testing.T) {
	replay := ReplayPendingGates("thread-a", []map[string]any{
		{"kind": "approval_requested", "threadId": "thread-a", "approvalId": "gate-reused", "turnId": "turn-a"},
		{"kind": "turn_aborted", "threadId": "thread-a", "turnId": "turn-a"},
		{"kind": "user_input_requested", "threadId": "thread-a", "inputId": "gate-reused", "turnId": "turn-b"},
	})
	if !replay.InvalidGateIDs["gate-reused"] || restartGateIDExists(replay, "gate-reused") == false {
		t.Fatalf("reused closed gate id must remain invalid: %#v", replay)
	}
	if len(replay.Approvals)+len(replay.Inputs)+len(replay.ClosedApprovals)+len(replay.ClosedInputs) != 0 {
		t.Fatalf("invalid gate id must not retain executable or display authority: %#v", replay)
	}
}
