package sourceprobe

import (
	"context"
	"encoding/json"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

type Input struct {
	ServerID          string
	Context           domainsecurity.TurnSecurityContext
	Binding           domainsecurity.CaseBindingObservationV1
	WorkspaceRealPath string
	ThreadID          string
	TurnID            string
	CaseID            string
	CaseBindingHash   string
	DatasetSnapshotID string
	ContextEpoch      uint64
	ContextDigest     string
}

type Prober interface {
	ProbeCaseSource(context.Context, Input) (domainsecurity.VerifiedSourceProbe, error)
}

// HostEvidenceCapability is a callback-scoped, non-serializable authority for
// one exact frozen context, current native source probe, and independently
// witnessed dataset selection. DatasetSelection and UseExact must fail after
// the issuing callback returns or its context is cancelled. Selection bytes,
// source booleans, and probe digests are evidence material, never authority.
type HostEvidenceCapability interface {
	DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error)
	UseExact(
		domainsecurity.TurnSecurityContext,
		domainsecurity.VerifiedSourceProbe,
		datasetsnapshotport.CurrentSelectionV2,
		func(context.Context) error,
	) error
}

// HostEvidenceRegistryCommitCapabilityV1 binds the one prepared registry
// effect to this active source-probe callback. It grants no general write.
type HostEvidenceRegistryCommitCapabilityV1 interface {
	UseExactRegistryCommit(domainsecurity.TurnSecurityContext, domainsecurity.VerifiedSourceProbe,
		datasetsnapshotport.CurrentSelectionV2, domainevidence.PreparedEvidenceSettlement,
		domainevidence.HostEvidenceSettlementMarker, func(context.Context) error) error
}

type CurrentInput struct {
	ServerID        string
	Context         domainsecurity.TurnSecurityContext
	Binding         domainsecurity.CaseBindingObservationV1
	ConnectionEpoch uint64
}

// LockedCurrent is a compatibility surface. Production implementations must
// fail closed without I/O; positive authority is available only through
// LockedCurrentAuthority.
type LockedCurrent interface {
	WithCurrentProbe(context.Context, CurrentInput, func(domainsecurity.VerifiedSourceProbe) error) error
}

// LockedCurrentAuthority is the only positive current-probe surface. The old
// method remains a deterministic zero-I/O compatibility boundary.
type LockedCurrentAuthority interface {
	WithCurrentProbeAuthority(
		context.Context,
		CurrentInput,
		func(domainsecurity.VerifiedSourceProbe, HostEvidenceCapability) error,
	) error
}

// CurrentValidator checks that the exact native probe captured for a frozen
// turn is still bound to the current live connection, verified identity,
// catalog provenance, case, epoch, context digest, and dataset. It performs no
// local "latest" inference and returns no provider-controlled authority.
type CurrentValidator interface {
	ValidateCurrentProbe(context.Context, string, domainsecurity.TurnSecurityContext) error
}

type EvidenceReadInput struct {
	Context   domainsecurity.TurnSecurityContext
	Binding   domainsecurity.CaseBindingObservationV1
	Grant     domainsecurity.ExecutionGrant
	Arguments json.RawMessage
}

// LockedEvidenceReader is a compatibility surface. Production implementations
// must fail closed without I/O; positive evidence reads are available only
// through LockedEvidenceAuthorityReader.
type LockedEvidenceReader interface {
	WithCurrentEvidenceRead(context.Context, EvidenceReadInput, func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error) error
}

// LockedEvidenceAuthorityReader is the only evidence-read surface that can
// enter the host evidence capability.
type LockedEvidenceAuthorityReader interface {
	WithCurrentEvidenceReadAuthority(
		context.Context,
		EvidenceReadInput,
		func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult, HostEvidenceCapability) error,
	) error
}

type PublicationSourceRequirement struct {
	ReceiptID         string
	ServerID          string
	ServerIdentity    string
	ServerVersion     string
	ConnectionEpoch   uint64
	ToolName          string
	DatasetSnapshotID string
}

type PublicationInput struct {
	Context      domainsecurity.TurnSecurityContext
	Binding      domainsecurity.CaseBindingObservationV1
	Requirements []PublicationSourceRequirement
}

// LockedPublicationSnapshot is a compatibility surface. Production
// implementations must fail closed without I/O; positive publication
// authority is available only through LockedPublicationSnapshotAuthority.
type LockedPublicationSnapshot interface {
	WithFreshPublicationSnapshot(context.Context, PublicationInput, func([]domainsecurity.VerifiedSourceProbe) error) error
}

// LockedPublicationSnapshotAuthority is the only fresh-publication surface
// that can carry exact host evidence authority.
type LockedPublicationSnapshotAuthority interface {
	WithFreshPublicationSnapshotAuthority(
		context.Context,
		PublicationInput,
		func([]domainsecurity.VerifiedSourceProbe, HostEvidenceCapability) error,
	) error
}
