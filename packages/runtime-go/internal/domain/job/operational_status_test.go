package job

import "testing"

func TestOperationalStatusesUseClosedAllowlists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		valid    func(string) bool
		public   func(string) string
		accepted []string
		rejected string
	}{
		{name: "auto continue", valid: ValidAutoContinueStatusV1, public: PublicAutoContinueStatusV1, accepted: []string{"starting", "started", "skipped", "failed"}, rejected: "<think>private</think>"},
		{name: "completion delivery", valid: ValidCompletionDeliveryStatusV1, public: PublicCompletionDeliveryStatusV1, accepted: []string{"pending", "retry", "delivered", "skipped", "dead_letter"}, rejected: "PRIVATE_DELIVERY"},
		{name: "recovery", valid: ValidRecoveryStatusV1, public: PublicRecoveryStatusV1, accepted: []string{"recovering", "recovered", "dead_lettered"}, rejected: "PRIVATE_RECOVERY"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, value := range test.accepted {
				if !test.valid(value) || test.public(value) != value {
					t.Fatalf("expected %q to remain a valid closed status", value)
				}
			}
			if test.valid(test.rejected) || test.public(test.rejected) != PublicOperationalStatusUnknownV1 {
				t.Fatalf("untrusted status was accepted or reflected")
			}
		})
	}
}

func TestPersistedOperationalStatusesAdmitOnlyClosedValues(t *testing.T) {
	t.Parallel()
	if err := ValidatePersistedOperationalStatusesV1(Record{
		AutoContinueStatus:       "skipped",
		CompletionDeliveryStatus: "delivered",
		RecoveryStatus:           "recovered",
	}); err != nil {
		t.Fatalf("expected closed persisted statuses: %v", err)
	}
	if err := ValidatePersistedOperationalStatusesV1(Record{RecoveryStatus: "PRIVATE_RECOVERY"}); err == nil {
		t.Fatal("expected open persisted recovery status to fail closed")
	}
	if err := ValidatePersistedOperationalStatusesV1(Record{RecoveryStatus: PublicOperationalStatusUnknownV1}); err == nil {
		t.Fatal("public unknown projection must never become durable control state")
	}
}

func TestPauseAndSteerPublicStatusesNeverReflectOpenText(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"requested", "paused", "resumed", "rejected", "expired"} {
		if err := ValidatePauseRequestStatusV1(status); err != nil || PublicPauseStatusV1(status) != status {
			t.Fatalf("expected valid pause status %q: %v", status, err)
		}
	}
	if PublicPauseStatusV1("resume_requested") != "resume_requested" {
		t.Fatal("resume_requested event status was not preserved")
	}
	private := "paused<think>PRIVATE</think>"
	if ValidatePauseRequestStatusV1(private) == nil || PublicPauseStatusV1(private) != PublicOperationalStatusUnknownV1 || PublicSteerStatusV1(private) != PublicOperationalStatusUnknownV1 {
		t.Fatal("open pause/steer status was accepted or reflected")
	}
}

func TestOperationalStatusTransitionsCannotReopenSettledState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		before Record
		after  Record
		valid  bool
	}{
		{name: "auto starting settles", before: Record{Status: "completed", Background: true, AutoContinueParent: true, AutoContinueStatus: "starting"}, after: Record{Status: "completed", Background: true, AutoContinueParent: true, AutoContinueStatus: "started"}, valid: true},
		{name: "auto started cannot skip", before: Record{Status: "completed", Background: true, AutoContinueParent: true, AutoContinueStatus: "started"}, after: Record{Status: "completed", Background: true, AutoContinueParent: true, AutoContinueStatus: "skipped"}},
		{name: "delivery retries dead letter", before: Record{CompletionDeliveryStatus: "dead_letter", RecoveryStatus: "dead_lettered", DeadLetterReason: "parent_missing"}, after: Record{CompletionDeliveryStatus: "retry", RecoveryStatus: "recovering", DeadLetterReason: "parent_missing"}, valid: true},
		{name: "delivered cannot reopen", before: Record{CompletionDeliveryStatus: "delivered", RecoveryStatus: "recovered"}, after: Record{CompletionDeliveryStatus: "pending", RecoveryStatus: "recovered"}},
		{name: "recovered cannot dead letter", before: Record{RecoveryStatus: "recovered"}, after: Record{RecoveryStatus: "dead_lettered", DeadLetterReason: "parent_missing"}},
		{name: "dead letter delivery requires recovery", before: Record{}, after: Record{CompletionDeliveryStatus: "dead_letter", DeadLetterReason: "parent_missing"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateOperationalStatusTransitionV1(test.before, test.after)
			if test.valid && err != nil {
				t.Fatalf("expected valid transition: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected transition to fail closed")
			}
		})
	}
}
