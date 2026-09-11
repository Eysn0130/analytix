package identity

import (
	"context"
	"errors"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
)

var (
	ErrUnavailable = errors.New("host identity authority is unavailable")
	ErrMismatch    = errors.New("host identity authority mismatch")
)

type Authority interface {
	ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error)
	ValidateCurrent(context.Context, domainidentity.PrincipalV1) error
}
