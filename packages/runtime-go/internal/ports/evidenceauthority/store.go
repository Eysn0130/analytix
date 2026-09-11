package evidenceauthority

import (
	"context"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// BundleStore is immutable and content addressed by RecordDigest. It has no
// list/current operation: only a fresh independent witness observation may
// select the authoritative evidence bundle.
type BundleResolver interface {
	Resolve(context.Context, string) (domainevidence.EvidenceAuthorityBundleV1, error)
}

type BundleStore interface {
	BundleResolver
	PutIfAbsent(context.Context, domainevidence.EvidenceAuthorityBundleV1) error
}

// ObservationBundle preserves the exact fresh witness exchange that selected
// Bundle. It is addressed by Observation.ObservationDigest and is historical
// audit evidence, not a source of current-head authority.
type ObservationBundle struct {
	Bundle      domainevidence.EvidenceAuthorityBundleV1
	Request     domainsecurity.MonotonicHeadObserveRequestV1
	Observation domainsecurity.MonotonicHeadObservationV1
}

// ObservationStore is immutable and content addressed. Deliberately omitting
// List and Current prevents local files from becoming freshness authority.
type ObservationResolver interface {
	Resolve(context.Context, string) (ObservationBundle, error)
}

type ObservationStore interface {
	ObservationResolver
	PutIfAbsent(context.Context, ObservationBundle) error
}

// WitnessBindingChainResolver re-challenges the independently enrolled
// witness and resolves one historical binding only when its exact observed
// bundle remains on the newly selected authority chain. Local observation or
// bundle storage alone is never current-head authority.
type WitnessBindingChainResolver interface {
	ResolveWitnessBindingOnFreshChain(context.Context, domainevidence.EvidenceAuthorityWitnessBindingV1) (WitnessBindingChainResolution, error)
}

// WitnessBindingChainResolution keeps the freshly selected head distinct from
// the historical exchange named by the admission. Consumers must use Current
// to reject a later same-context receipt append or revocation; Historical is
// only the exact admission-time proof.
type WitnessBindingChainResolution struct {
	Current    FreshHead
	Historical ObservationBundle
}

// Projection accepts only an exact bundle selected by a fresh witness
// observation and fully verified by the application. It has no read method
// and therefore cannot select or repair an authority head.
type Projection interface {
	ValidateWitnessEmpty(context.Context) error
	ProjectWitnessSelected(context.Context, domainevidence.EvidenceAuthorityBundleV1) error
}

// FreshHead is returned only after a new random challenge to the enrolled
// witness. Generation zero is represented by HasBundle=false.
type FreshHead struct {
	HasBundle   bool
	Bundle      domainevidence.EvidenceAuthorityBundleV1
	Request     domainsecurity.MonotonicHeadObserveRequestV1
	Observation domainsecurity.MonotonicHeadObservationV1
}

type DatasetAdvanceInput struct {
	ExpectedBundleDigest string
	NextIndexDigest      string
}

type RegistryAdvanceInput struct {
	ExpectedBundleDigest string
	NextIndexDigest      string
}

type PublicationAdvanceInput struct {
	ExpectedBundleDigest string
	NextIndexDigest      string
}

// FreshHeadReader can only observe a newly challenged witness head. It is the
// least authority needed by report validation and restart reconciliation;
// holding it cannot mutate dataset, registry, or publication state.
type FreshHeadReader interface {
	ObserveFresh(context.Context) (FreshHead, error)
}

// DatasetCoordinator exposes one fixed child. Dataset code cannot advance the
// registry or publication roots, and callers cannot substitute a local head.
type DatasetCoordinator interface {
	ObserveFresh(context.Context) (FreshHead, error)
	AdvanceDatasetSnapshot(context.Context, DatasetAdvanceInput) (FreshHead, error)
}

// RegistryCoordinator exposes only the evidence-registry child. Registry
// code cannot mutate dataset or publication roots and cannot select a local
// candidate as current.
type RegistryCoordinator interface {
	ObserveFresh(context.Context) (FreshHead, error)
	AdvanceEvidenceRegistry(context.Context, RegistryAdvanceInput) (FreshHead, error)
}

// PublicationCoordinator exposes only the publication child. Report code
// cannot change the dataset or registry roots it validated.
type PublicationCoordinator interface {
	FreshHeadReader
	AdvancePublication(context.Context, PublicationAdvanceInput) (FreshHead, error)
}
