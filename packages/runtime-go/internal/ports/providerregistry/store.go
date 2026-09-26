package providerregistry

import (
	"context"
	"errors"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

var (
	ErrInvalidRequest            = errors.New("provider registry: invalid request")
	ErrConflict                  = errors.New("provider registry: conflict")
	ErrNotFound                  = errors.New("provider registry: not found")
	ErrPersistence               = errors.New("provider registry: persistence failure")
	ErrCredentialUnavailable     = errors.New("provider registry: credential storage unavailable")
	ErrCredentialReentryRequired = errors.New("provider registry: credential re-entry required")
	ErrClosed                    = errors.New("provider registry: closed")
	ErrVerification              = errors.New("provider registry: verification failure")
)

type Store interface {
	WithExclusive(context.Context, func(Transaction) error) error
}

type Transaction interface {
	Load(context.Context) (domainregistry.Registry, error)
	// Implementations used for durable production Registry state must treat
	// Commit as the Registry linearization point: a successful return means the
	// implementation has journaled the candidate, performed exact readback, and
	// completed the prior-byte rollback-evidence protocol needed for failed
	// publication or restart recovery. Memory and test doubles may satisfy this
	// method for logical tests, but are not durable evidence. This remains a
	// logical all-or-none contract and does not claim physical filesystem
	// atomicity.
	Commit(context.Context, domainregistry.Registry) error
}
