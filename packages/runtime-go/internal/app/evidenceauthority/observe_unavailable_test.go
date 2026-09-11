package evidenceauthority

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

type failingObserveWitnessV1 struct {
	monotonicheadport.Witness
	err    error
	cancel context.CancelFunc
	calls  int
}

func (witness *failingObserveWitnessV1) Observe(context.Context, domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error) {
	witness.calls++
	if witness.cancel != nil {
		witness.cancel()
	}
	return domainsecurity.MonotonicHeadObservationV1{}, witness.err
}

func TestFreshObservationMarksOnlyRemoteUnavailable(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		want   bool
		cancel bool
	}{
		{"remote", monotonicheadport.ErrUnavailable, true, false},
		{"wrapped", fmt.Errorf("transport: %w", monotonicheadport.ErrUnavailable), true, false},
		{"mixed-integrity", errors.Join(monotonicheadport.ErrUnavailable, ErrIntegrity), false, false},
		{"mixed-unknown", errors.Join(monotonicheadport.ErrUnavailable, errors.New("local failure")), false, false},
		{"invalid-receipt", monotonicheadport.ErrInvalidReceipt, false, false},
		{"not-enrolled", monotonicheadport.ErrNotEnrolled, false, false},
		{"equivocation", monotonicheadport.ErrEquivocation, false, false},
		{"indeterminate", monotonicheadport.ErrIndeterminate, false, false},
		{"deadline", errors.Join(monotonicheadport.ErrUnavailable, context.DeadlineExceeded), false, false},
		{"cancel-during-observe", monotonicheadport.ErrUnavailable, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			witness := &failingObserveWitnessV1{Witness: fixture.witness, err: test.err}
			if test.cancel {
				witness.cancel = cancel
			}
			fixture.authority.witness = witness
			_, err := fixture.authority.ObserveFresh(ctx)
			if err == nil || IsWitnessObserveUnavailableV1(err) != test.want || witness.calls != 1 {
				t.Fatalf("Observe classification: calls=%d err=%v", witness.calls, err)
			}
			if test.want && !errors.Is(err, monotonicheadport.ErrUnavailable) {
				t.Fatal("remote cause was lost")
			}
			if test.cancel && !errors.Is(err, context.Canceled) {
				t.Fatal("Observe cancellation was lost")
			}
			if fixture.checkpointFloor.projectCalls.Load() != 0 || fixture.observations.putCalls.Load() != 0 || fixture.witness.advanceCalls.Load() != 0 {
				t.Fatal("failed Observe projected or wrote authority")
			}
		})
	}
}

func TestObserveUnavailableMarkerRejectsMixedAndUnmarkedFailures(t *testing.T) {
	marked := errors.Join(ErrWitnessObserveUnavailableV1, monotonicheadport.ErrUnavailable)
	if !IsWitnessObserveUnavailableV1(fmt.Errorf("registry: %w", marked)) {
		t.Fatal("producer provenance did not survive wrapping")
	}
	for _, err := range []error{ErrWitnessObserveUnavailableV1, monotonicheadport.ErrUnavailable, ErrUnavailable,
		errors.Join(marked, ErrIntegrity), errors.Join(marked, context.Canceled), errors.Join(marked, monotonicheadport.ErrCheckpointFloorUnavailable), errors.Join(marked, errors.New("physical failure"))} {
		if IsWitnessObserveUnavailableV1(err) {
			t.Fatalf("nonavailability cause accepted: %v", err)
		}
	}
}
