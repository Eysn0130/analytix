package threadriskauthority

import (
	"context"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// IndexStore is an immutable, content-addressed store. IndexDigest is the
// only lookup key; implementations must not expose a list or a locally
// inferred "current" head.
type IndexStore interface {
	PutIfAbsent(context.Context, domainsecurity.ThreadRiskAuthorityIndexV1) error
	Resolve(context.Context, string) (domainsecurity.ThreadRiskAuthorityIndexV1, error)
}

// ObservationBundle is the exact triplet that justified one witnessed risk
// binding. Its content address is Observation.ObservationDigest. A bundle is
// historical issuance evidence, not proof that its index is still current.
type ObservationBundle struct {
	Index       domainsecurity.ThreadRiskAuthorityIndexV1
	Request     domainsecurity.MonotonicHeadObserveRequestV1
	Observation domainsecurity.MonotonicHeadObservationV1
}

// ObservationStore is immutable and content addressed. It deliberately has
// no list/current operation: a compact digest in a TSC becomes meaningful
// only after exact resolution and full semantic verification.
type ObservationStore interface {
	PutIfAbsent(context.Context, ObservationBundle) error
	Resolve(context.Context, string) (ObservationBundle, error)
}

// Projection receives only a current index already selected by a fresh,
// independently witnessed observation and fully verified by the app. It is a
// disposable local optimization and has no read method, so it cannot become
// authority or repair a missing immutable record.
type Projection interface {
	ValidateWitnessEmpty(context.Context) error
	ProjectWitnessSelected(context.Context, domainsecurity.ThreadRiskAuthorityIndexV1) error
}
