package job

import "testing"

func TestStatusV1IsClosedAndPublicProjectionNeverReflectsUnknownText(t *testing.T) {
	valid := []Status{
		StatusQueued, StatusRunning, StatusPauseRequested, StatusPaused,
		StatusResumeRequested, StatusResuming, StatusCompleted, StatusFailed,
		StatusAborted, StatusInterrupted, StatusKilled, StatusCanceled, StatusTimeout,
	}
	for _, status := range valid {
		if err := ValidateStatusV1(string(status)); err != nil || PublicStatusV1(string(status)) != string(status) {
			t.Fatalf("valid status %q: public=%q err=%v", status, PublicStatusV1(string(status)), err)
		}
	}
	for _, value := range []string{"", "running<think>PRIVATE</think>", "private draft", "completed_extra"} {
		if err := ValidateStatusV1(value); err == nil || PublicStatusV1(value) != "unknown" {
			t.Fatalf("unknown status %q: public=%q err=%v", value, PublicStatusV1(value), err)
		}
	}
}

func TestTerminalStatusV1UsesTheSameClosedStatusSet(t *testing.T) {
	for _, status := range []Status{StatusCompleted, StatusFailed, StatusAborted, StatusInterrupted, StatusKilled, StatusCanceled, StatusTimeout} {
		if !TerminalStatusV1(string(status)) {
			t.Fatalf("terminal status %q was not terminal", status)
		}
	}
	for _, status := range []Status{StatusQueued, StatusRunning, StatusPauseRequested, StatusPaused, StatusResumeRequested, StatusResuming} {
		if TerminalStatusV1(string(status)) {
			t.Fatalf("active status %q was terminal", status)
		}
	}
	if TerminalStatusV1("failed: private draft") || ExternallyStoppedStatusV1("killed: private draft") {
		t.Fatal("unknown status text was interpreted as a lifecycle state")
	}
}
