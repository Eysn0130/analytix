package monotonichead

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrNotEnrolled      = errors.New("monotonic head witness is not enrolled")
	ErrCASConflict      = errors.New("monotonic head compare-and-swap conflict")
	ErrMutationConflict = errors.New("monotonic head mutation ID conflict")
	ErrUnavailable      = errors.New("monotonic head witness is unavailable")
	ErrInvalidReceipt   = errors.New("monotonic head witness returned an invalid receipt")
	// ErrIndeterminate means an Advance timed out or was cancelled after it
	// may have committed. Callers must issue a fresh Observe and reconcile the
	// mutation; they must not blindly retry with a new mutation ID.
	ErrIndeterminate = errors.New("monotonic head advance outcome is indeterminate")
	// ErrEquivocation means the enrolled witness produced incompatible signed
	// heads for the same namespace generation. It is a fail-closed integrity
	// incident, not an availability or ordinary receipt-validation error.
	ErrEquivocation = errors.New("monotonic head witness equivocated")
)

// Witness is an independent monotonic-head authority. Observe must sign the
// caller's fresh challenge together with the exact stable checkpoint; it must
// not mutate that checkpoint. Advance is an exact
// compare-and-swap operation over generation, checkpoint digest, state digest,
// and witness-issued fence nonce. Implementations must persist mutation ID -> canonical
// request/receipt atomically with the head:
//
//   - an identical retry returns the exact original receipt bytes;
//   - reuse of a mutation ID with different request bytes returns
//     ErrMutationConflict;
//   - a mismatched expected head returns ErrCASConflict and never advances;
//   - a successful call advances exactly one generation.
//   - timeout/cancellation after dispatch returns ErrIndeterminate when commit
//     status cannot be proven and requires Observe-based reconciliation;
//   - incompatible signed heads at one generation return ErrEquivocation.
//
// Domain validators in security/monotonic_head.go remain mandatory at both
// sides of this port. A cached or merely well-signed checkpoint is authentic,
// but it is not proof of the witness's current head.
type Witness interface {
	Observe(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error)
	Advance(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error)
}

// MutationRecoveryWitness adds the read-only, challenge-bound lookup required
// to reconcile an Advance that may have committed remotely before its local
// settlement became durable. ResolveMutation never mutates the witness head
// and an absent response never authorizes a retry, supersession, projection,
// or publication.
type MutationRecoveryWitness interface {
	Witness
	ResolveMutation(ctx context.Context, request domainsecurity.MonotonicHeadMutationResolveRequestV1) (domainsecurity.MonotonicHeadMutationResolutionV1, error)
}
