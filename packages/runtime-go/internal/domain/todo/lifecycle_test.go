package todo

import "testing"

func TestValidateStatusReasonUsesClosedStatusSpecificEnums(t *testing.T) {
	for _, test := range []struct {
		status Status
		reason string
		valid  bool
	}{
		{StatusFailed, string(ReasonToolFailed), true},
		{StatusFailed, string(ReasonUserCanceled), false},
		{StatusCanceled, string(ReasonUserCanceled), true},
		{StatusCanceled, string(ReasonRuntimeFailed), false},
		{StatusPending, "", true},
		{StatusCompleted, string(ReasonExecutionFailed), false},
		{StatusFailed, "model_said_failed", false},
	} {
		if err := ValidateStatusReason(test.status, test.reason); (err == nil) != test.valid {
			t.Fatalf("status=%q reason=%q valid=%v err=%v", test.status, test.reason, test.valid, err)
		}
	}
}

func TestValidateOperationTransitionRequiresExplicitRetry(t *testing.T) {
	if err := ValidateOperationTransition("retry", StatusFailed, StatusInProgress); err != nil {
		t.Fatalf("explicit retry rejected: %v", err)
	}
	for _, operation := range []string{"start", "done"} {
		if err := ValidateOperationTransition(operation, StatusFailed, map[string]Status{"start": StatusInProgress, "done": StatusCompleted}[operation]); err == nil {
			t.Fatalf("%s bypassed failed todo retry", operation)
		}
	}
	if err := ValidateOperationTransition("retry", StatusCompleted, StatusInProgress); err == nil {
		t.Fatal("completed todo was reopened through retry")
	}
}
