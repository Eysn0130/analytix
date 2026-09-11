package acceptedfinalevent

import (
	"context"
	"errors"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

var ErrPublicationReserved = errors.New("accepted final event publication is reserved")

type ReservationWork func(context.Context) error

// Readback proves that accepted-final delivery events came from the host's
// durable event log. It does not sign, activate, or otherwise grant
// publication authority.
type Readback interface {
	// VerifyReservedTail requires an active same-thread/same-commit
	// publication reservation and an exact physical event-log tail.
	VerifyReservedTail(context.Context, []map[string]any) error

	// VerifyCommittedManifest accepts an exact, unique durable commit group
	// for an already-active replay; the group need not remain the log tail.
	VerifyCommittedManifest(context.Context, []map[string]any) error
}

// Delivery is the single side-effect authority for accepted-final event
// staging, durable readback, same-thread reservation, projection activation,
// and no-error live publication. Implementations must serialize these methods
// with the same owner lock used by every ordinary event writer.
type Delivery interface {
	Readback

	Stage(context.Context, []map[string]any) ([]map[string]any, error)
	WithReservation(context.Context, string, string, ReservationWork) error
	ActivateAndPublish(
		context.Context,
		[]map[string]any,
		domainevent.AcceptedFinalDeliverySealV1,
		func() error,
	) error
}
