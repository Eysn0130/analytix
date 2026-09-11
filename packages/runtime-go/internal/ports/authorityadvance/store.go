package authorityadvance

import (
	"context"
	"errors"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
)

var (
	ErrNotFound    = errors.New("authority advance journal record is not found")
	ErrConflict    = errors.New("authority advance journal mutation conflicts")
	ErrCorrupt     = errors.New("authority advance journal record is corrupt")
	ErrUnavailable = errors.New("authority advance journal is unavailable")
)

// IntentStore is keyed only by MutationID. It has no current/latest/list
// operation, so local journal inventory cannot become witness authority.
type IntentResolver interface {
	ResolveIntent(context.Context, string) (domainauthority.MonotonicAdvanceIntentV2, error)
}

type IntentStore interface {
	IntentResolver
	PutIntentIfAbsent(context.Context, domainauthority.MonotonicAdvanceIntentV2) error
}

type IntentInventoryStore interface {
	VisitIntents(context.Context, func(domainauthority.MonotonicAdvanceIntentV2) error) error
}

// SettlementStore permits at most one immutable terminal settlement for a
// mutation ID. Live commit code resolves only an exact known mutation; startup
// inventory belongs to a separate read-only reconciliation port.
type SettlementResolver interface {
	ResolveSettlement(context.Context, string) (domainauthority.MonotonicAdvanceSettlementV2, error)
}

type SettlementStore interface {
	SettlementResolver
	PutSettlementIfAbsent(context.Context, domainauthority.MonotonicAdvanceSettlementV2) error
}

type SettlementInventoryStore interface {
	VisitSettlements(context.Context, func(domainauthority.MonotonicAdvanceSettlementV2) error) error
}
