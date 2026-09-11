package security

import (
	"strings"
	"testing"
	"time"
)

func TestApprovedGrantTransitionIsDeterministicAndBindsDisposition(t *testing.T) {
	context := mustGeneralTurnSecurityContextV2(t, "thread-transition", "turn-transition", "/workspace")
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	pending := registryGrantForTest(context, "pending", now.Add(-time.Minute), now.Add(10*time.Minute))
	approved := registryGrantForTest(context, "approved", now, now.Add(10*time.Minute))
	input := ApprovalGrantTransitionInputV1{
		Context: context, ApprovalID: "appr_test", ApprovalItemID: "item_appr_test", ContinuationReceiptID: SHA256Hex([]byte("receipt")),
		ContinuationDispositionID: SHA256Hex([]byte("disposition")), PendingGrant: pending, ApprovedGrant: approved, TransitionedAt: now,
	}
	first, err := NewApprovalGrantTransitionV1(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewApprovalGrantTransitionV1(input)
	if err != nil || second != first {
		t.Fatalf("deterministic transition = %#v, want %#v, err=%v", second, first, err)
	}
	tampered := first
	tampered.ContinuationDispositionID = SHA256Hex([]byte("other-disposition"))
	if err := ValidateApprovalGrantTransitionV1(tampered, context, pending, approved); err == nil {
		t.Fatal("transition accepted a disposition identity change without a new digest")
	}
	wrongTime := approved
	wrongTime.IssuedAt = now.Add(time.Second).Format(time.RFC3339Nano)
	wrongTime.GrantID = executionGrantHash(wrongTime)
	if err := ValidateApprovalGrantTransitionV1(first, context, pending, wrongTime); err == nil {
		t.Fatal("transition accepted an approved grant not issued by the disposition timestamp")
	}
}

func TestApprovalGrantTransitionRejectsRawProviderIdentityWithoutEcho(t *testing.T) {
	context := mustGeneralTurnSecurityContextV2(t, "thread-transition-raw", "turn-transition-raw", "/workspace")
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	rawProviderID := "provider_call_6222020202020202020"
	build := func(state string, issuedAt time.Time) ExecutionGrant {
		return NewExecutionGrant(ExecutionGrantInput{
			Context: context, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: rawProviderID,
			ArgsHash: SHA256Hex([]byte("args")), SchemaHash: SHA256Hex([]byte("schema")), ScopeHash: SHA256Hex([]byte("scope")),
			ReadOnly: true, ApprovalState: state, IssuedAt: issuedAt, ExpiresAt: now.Add(10 * time.Minute),
		})
	}
	pending := build("pending", now.Add(-time.Minute))
	approved := build("approved", now)
	transition, err := NewApprovalGrantTransitionV1(ApprovalGrantTransitionInputV1{
		Context: context, ApprovalID: "appr_raw", ApprovalItemID: "item_appr_raw",
		ContinuationReceiptID: SHA256Hex([]byte("receipt-raw")), ContinuationDispositionID: SHA256Hex([]byte("disposition-raw")),
		PendingGrant: pending, ApprovedGrant: approved, TransitionedAt: now,
	})
	if err == nil || transition.TransitionID != "" || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider identity entered an approval transition: transition=%#v err=%v", transition, err)
	}
}
