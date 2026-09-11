package job

import "testing"

func TestNormalizeFailureCodeIsClosedAndStatusDerived(t *testing.T) {
	if got := NormalizeFailureCode("SOL_PRIVATE_TRACE_7C", string(StatusFailed), true); got != FailureChildExecutionFailed {
		t.Fatalf("arbitrary failure code escaped: %q", got)
	}
	if got := NormalizeFailureCode("", string(StatusKilled), false); got != FailureChildKilled {
		t.Fatalf("killed status was not classified: %q", got)
	}
	if got := NormalizeFailureCode("", string(StatusCanceled), false); got != FailureChildCanceled {
		t.Fatalf("canceled status was not classified: %q", got)
	}
	if got := NormalizeFailureCode("", string(StatusTimeout), false); got != FailureChildTimeout {
		t.Fatalf("timeout status was not classified: %q", got)
	}
	if got := NormalizeFailureCode("", string(StatusRunning), false); got != "" {
		t.Fatalf("running job received a failure code: %q", got)
	}
	if ValidFailureCode("SOL_PRIVATE_TRACE_7C") {
		t.Fatal("unknown failure code entered the closed set")
	}
}
