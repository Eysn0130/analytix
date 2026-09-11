package caseentity

import (
	"context"
	"errors"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrNotFound  = errors.New("case entity private state is not found")
	ErrConflict  = errors.New("case entity private state conflicts with immutable storage")
	ErrIntegrity = errors.New("case entity private state integrity validation failed")
)

// Store is the private, non-enumerable semantic owner layered on the existing
// SecurePrivateCAS primitive. It preserves canonical bytes but does not possess
// the installation-keyed digester and therefore cannot authenticate the
// reference-to-value relationship by itself. The app service must re-derive
// that relationship for the current case before every persist/use boundary.
// Every method must execute while the caller keeps the current DSV2 exact-use
// lease live; this interface is not dataset authority and cannot mint or extend
// that lease.
type Store interface {
	EnsureBinding(context.Context, domaincaseentity.CaseEntityBindingRecordInputV1) (domaincaseentity.CaseEntityBindingRecord, error)
	ResolveBinding(context.Context, string) (domaincaseentity.CaseEntityBindingRecord, error)
	ResolveBindingByStableOrdinal(context.Context, domainsecurity.TurnSecurityContext, string, uint32) (domaincaseentity.CaseEntityBindingRecord, error)

	PutIngressIfAbsent(context.Context, domaincaseentity.CaseIngressRecord) error
	ResolveIngress(context.Context, string) (domaincaseentity.CaseIngressRecord, error)

	PutThreadContextIfAbsent(context.Context, domaincaseentity.ThreadCaseContextRecord) error
	ResolveThreadContext(context.Context, string) (domaincaseentity.ThreadCaseContextRecord, error)
	ResolveLatestCaseLongitudinalContext(context.Context, domainsecurity.TurnSecurityContext) (domaincaseentity.ThreadCaseContextRecord, error)
	ResolveLatestThreadContextForScope(context.Context, domainsecurity.TurnSecurityContext, string) (domaincaseentity.ThreadCaseContextRecord, error)
}
