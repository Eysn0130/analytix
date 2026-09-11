package evidenceauthority

import (
	"errors"

	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

// ErrWitnessObserveUnavailableV1 identifies failure at the remote Observe
// call, before any response validation, checkpoint projection or local write.
// It is not the broader ErrUnavailable. An outer operation may have performed
// effects before calling Observe (for example, Advance reconciliation); this
// marker alone does not prove that the outer operation is safe to replace.
var ErrWitnessObserveUnavailableV1 = errors.New("evidence witness Observe is unavailable")

// IsWitnessObserveUnavailableV1 accepts only the producer marker plus pure
// remote unavailability. A joined integrity, cancellation, indeterminate or
// unknown failure is not an availability boundary, even if errors.Is matches.
func IsWitnessObserveUnavailableV1(err error) bool {
	budget := 64
	unavailable, marker, valid := witnessObservationErrorLeavesV1(err, &budget)
	return valid && unavailable && marker
}

func witnessObservationErrorLeavesV1(err error, budget *int) (unavailable, marker, valid bool) {
	if err == nil || *budget <= 0 {
		return false, false, false
	}
	*budget--
	if err == monotonicheadport.ErrUnavailable {
		return true, false, true
	}
	if err == ErrWitnessObserveUnavailableV1 {
		return false, true, true
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		children := wrapped.Unwrap()
		if len(children) == 0 || len(children) > 32 {
			return false, false, false
		}
		for _, child := range children {
			u, m, ok := witnessObservationErrorLeavesV1(child, budget)
			if !ok {
				return false, false, false
			}
			unavailable, marker = unavailable || u, marker || m
		}
		return unavailable, marker, true
	case interface{ Unwrap() error }:
		return witnessObservationErrorLeavesV1(wrapped.Unwrap(), budget)
	default:
		return false, false, false
	}
}
