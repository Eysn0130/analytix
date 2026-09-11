package thread

import (
	"reflect"
	"testing"
)

func TestPendingGateIDsFromEventsV1PreservesOpenOrderAndClosesTerminalTurns(t *testing.T) {
	approvals, inputs := PendingGateIDsFromEventsV1([]map[string]any{
		{"kind": "approval_requested", "approvalId": "approval-closed", "turnId": "turn-1"},
		{"kind": "approval_requested", "approvalId": "approval-open", "turnId": "turn-2"},
		{"kind": "approval_resolved", "approvalId": "approval-closed", "turnId": "turn-1"},
		{"kind": "approval_requested", "approvalId": "approval-open", "turnId": "turn-2"},
		{"kind": "user_input_requested", "inputId": "input-terminal", "turnId": "turn-3"},
		{"kind": "user_input_requested", "inputId": "input-open", "turnId": "turn-4"},
		{"kind": "turn_failed", "turnId": "turn-3"},
		{"kind": "user_input_requested", "inputId": "", "turnId": "turn-5"},
	})
	if !reflect.DeepEqual(approvals, []string{"approval-open"}) ||
		!reflect.DeepEqual(inputs, []string{"input-open"}) {
		t.Fatalf("pending gate replay mismatch: approvals=%#v inputs=%#v", approvals, inputs)
	}
}
