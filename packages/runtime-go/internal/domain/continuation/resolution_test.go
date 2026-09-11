package continuation

import "testing"

func TestResolveProjectionRequiresExactKindStatusAndReason(t *testing.T) {
	tests := []struct {
		kind, status, reason, publicStatus, decision string
	}{
		{KindApproval, StatusAllowed, "approval_allowed", "allowed", "allow"},
		{KindApproval, StatusDenied, "approval_denied", "denied", "deny"},
		{KindUserInput, StatusSubmitted, "user_input_submitted", "submitted", ""},
		{KindUserInput, StatusCancelled, "user_input_cancelled", "cancelled", ""},
	}
	for _, test := range tests {
		projection, ok := ResolveProjection(test.kind, test.status, test.reason)
		if !ok || projection.PublicStatus != test.publicStatus || projection.Decision != test.decision {
			t.Fatalf("projection(%q, %q, %q) = %#v, %v", test.kind, test.status, test.reason, projection, ok)
		}
		if _, ok := ResolveProjection(test.kind, test.status, test.reason+"_wrong"); ok {
			t.Fatalf("mismatched reason was accepted for %q/%q", test.kind, test.status)
		}
	}
	if _, ok := ResolveProjection(KindUserInput, StatusAllowed, "approval_allowed"); ok {
		t.Fatal("cross-kind disposition was accepted")
	}
}
