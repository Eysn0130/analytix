package evidenceregistry

import (
	"context"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

// AuthorityIndexStore is immutable and content addressed. It deliberately
// exposes neither List nor Current; only a fresh shared-witness head may select
// an index node.
type AuthorityIndexStore interface {
	PutIfAbsent(context.Context, domainevidence.EvidenceRegistryAuthorityIndexV2) error
	Resolve(context.Context, string) (domainevidence.EvidenceRegistryAuthorityIndexV2, error)
}

// AuthorityCapsuleStore contains complete signed registry snapshots selected
// by V2 index entries. A capsule's presence alone is not membership authority.
type AuthorityCapsuleStore interface {
	PutIfAbsent(context.Context, domainevidence.EvidenceRegistryAuthorityCapsule) error
	Resolve(context.Context, string) (domainevidence.EvidenceRegistryAuthorityCapsule, error)
}

// WitnessedSnapshot binds one context registry to the exact fresh shared
// bundle and global V2 registry root that selected it.
type WitnessedSnapshot struct {
	Head              evidenceauthorityport.FreshHead
	RootIndex         domainevidence.EvidenceRegistryAuthorityIndexV2
	HasSelection      bool
	SelectedIndex     domainevidence.EvidenceRegistryAuthorityIndexV2
	SelectedCapsule   domainevidence.EvidenceRegistryAuthorityCapsule
	RegistryIndexPath []domainevidence.EvidenceRegistryAuthorityIndexV2
	Context           domainsecurity.TurnSecurityContext
	Registry          domainevidence.EvidenceReceiptRegistry
}

type WitnessedSnapshotReader interface {
	WithWitnessedSnapshot(context.Context, domainsecurity.TurnSecurityContext, func(WitnessedSnapshot) error) error
}

// WitnessedSnapshotCapability proves that an exact snapshot came from the
// currently executing fresh-witness callback. UseExact must fail after the
// issuing callback returns, on cancellation, or for a copied/changed snapshot.
type WitnessedSnapshotCapability interface {
	UseExact(WitnessedSnapshot, func() error) error
}

type WitnessedSnapshotAuthority interface {
	WithWitnessedSnapshotAuthority(
		context.Context,
		domainsecurity.TurnSecurityContext,
		func(WitnessedSnapshot, WitnessedSnapshotCapability) error,
	) error
}

// FactFinalWitnessVerifier replays a signed fact-final admission against a
// newly challenged shared authority chain. Implementations must not infer
// freshness from the embedded admission or local registry files.
type FactFinalWitnessVerifier interface {
	VerifyFactFinalWitnessCurrent(context.Context, domainevidence.PrivateAcceptedFinalRecord) error
}

type FactFinalWitnessRequest struct {
	Context                domainsecurity.TurnSecurityContext
	Envelope               domainevidence.FinalAnswerEnvelope
	RenderedText           string
	PublicationProof       *domainevidence.PublicationSnapshotProof
	PublicationIntent      domainevidence.TerminalPublicationIntent
	BindingObservation     domainsecurity.CaseBindingObservationV1
	SourceProbes           []domainsecurity.VerifiedSourceProbe
	DatasetSelection       datasetsnapshotport.CurrentSelectionV2
	HostEvidenceCapability sourceprobeport.HostEvidenceCapability
}

// FactFinalWitnessCapability is a callback-scoped, non-serializable authority
// for exactly one admission-time registry snapshot. PrivateFinal and UseExact must
// fail after the issuing callback returns.
type FactFinalWitnessCapability interface {
	PrivateFinal() (domainevidence.PrivateAcceptedFinalRecord, error)
	UseExact(domainevidence.PrivateAcceptedFinalRecord, func() error) error
}

type FactFinalWitnessIssuer interface {
	WithFactFinalWitnessAuthority(context.Context, FactFinalWitnessRequest, func(FactFinalWitnessCapability) error) error
}

// RecoveredFactFinalWitnessIssuer re-admits an immutable committed original.
// It never reissues the fact or reconstructs process authority from JSON.
type RecoveredFactFinalWitnessIssuer interface {
	WithRecoveredFactFinalWitness(context.Context, domainevidence.PrivateAcceptedFinalRecord, func(FactFinalWitnessCapability) error) error
}
