package control

import "testing"

func TestApprovalManagerEnsurePendingNeverReopensTerminalProjection(t *testing.T) {
	manager := NewApprovalUserInputManager()
	if !manager.EnsureApprovalPending("approval", "write_file") || !manager.EnsureApprovalPending("approval", "write_file") {
		t.Fatal("exact pending approval projection was not idempotent")
	}
	if manager.EnsureApprovalPending("approval", "other_tool") {
		t.Fatal("conflicting pending approval projection was accepted")
	}
	if status, _ := manager.ResolveApproval("approval", "deny"); status != gateStatusOK {
		t.Fatal("resolve approval")
	}
	if manager.EnsureApprovalPending("approval", "write_file") {
		t.Fatal("terminal approval projection was reopened")
	}
	status, decision, exists := manager.ApprovalDisposition("approval")
	if !exists || status != "denied" || decision != "deny" {
		t.Fatalf("approval projection changed: status=%q decision=%q exists=%v", status, decision, exists)
	}
}

func TestUserInputManagerEnsurePendingNeverReopensTerminalProjection(t *testing.T) {
	manager := NewApprovalUserInputManager()
	if !manager.EnsureUserInputPending("input", "Continue?") || !manager.EnsureUserInputPending("input", "Continue?") {
		t.Fatal("exact pending user-input projection was not idempotent")
	}
	if manager.EnsureUserInputPending("input", "Different?") {
		t.Fatal("conflicting pending user-input projection was accepted")
	}
	if status, _ := manager.CancelUserInput("input"); status != gateStatusOK {
		t.Fatal("cancel user input")
	}
	if manager.EnsureUserInputPending("input", "Continue?") {
		t.Fatal("terminal user-input projection was reopened")
	}
	if status, exists := manager.UserInputDisposition("input"); !exists || status != "cancelled" {
		t.Fatalf("user-input projection changed: status=%q exists=%v", status, exists)
	}
}
