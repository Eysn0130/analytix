package reportpublication

import (
	"context"
	"errors"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

var ErrNotFound = errors.New("report publication record not found")

// AttemptStore reserves one immutable write-ahead plan under the stable
// report-stage AttemptID. created=false is allowed only for a byte-exact
// replay; reuse of the same AttemptID with any changed binding must fail.
type AttemptResolver interface {
	Resolve(context.Context, string) (domainpublication.PublicationAttemptV1, error)
}

type AttemptStore interface {
	AttemptResolver
	CreateExclusive(context.Context, domainpublication.PublicationAttemptV1) (created bool, err error)
}

type AttemptInventoryStore interface {
	VisitAttempts(context.Context, func(domainpublication.PublicationAttemptV1) error) error
}

type ReceiptResolver interface {
	Resolve(context.Context, string) (domainpublication.PublicationReceiptV1, error)
}

type ReceiptStore interface {
	ReceiptResolver
	PutIfAbsent(context.Context, domainpublication.PublicationReceiptV1) error
}

type ReceiptInventoryStore interface {
	VisitReceipts(context.Context, func(domainpublication.PublicationReceiptV1) error) error
}

type CommitReceiptResolver interface {
	Resolve(context.Context, string) (domainpublication.PublicationCommitReceiptV1, error)
}

type CommitReceiptStore interface {
	CommitReceiptResolver
	PutIfAbsent(context.Context, domainpublication.PublicationCommitReceiptV1) error
}

type CommitReceiptInventoryStore interface {
	VisitCommitReceipts(context.Context, func(domainpublication.PublicationCommitReceiptV1) error) error
}

// CommitSelectionStore is a stable-key no-replace CAS. Competing fresh
// witness observations for one attempt may propose different selections; the
// first durable SelectionID is the only commit message that may be signed.
type CommitSelectionResolver interface {
	Resolve(context.Context, string) (domainpublication.PublicationCommitSelectionV1, error)
}

type CommitSelectionStore interface {
	CommitSelectionResolver
	CreateExclusive(context.Context, domainpublication.PublicationCommitSelectionV1) (created bool, err error)
}

type CommitSelectionInventoryStore interface {
	VisitCommitSelections(context.Context, func(domainpublication.PublicationCommitSelectionV1) error) error
}

// DeliveryDecisionStore is the stable-key, no-replace authority for formal
// report exposure. A commit receipt alone has no delivery capability.
type DeliveryDecisionResolver interface {
	Resolve(context.Context, string) (domainpublication.ReportDeliveryDecisionV1, error)
}

type DeliveryDecisionStore interface {
	DeliveryDecisionResolver
	CreateExclusive(context.Context, domainpublication.ReportDeliveryDecisionV1) (created bool, err error)
}

type DeliveryDecisionInventoryStore interface {
	VisitDeliveryDecisions(context.Context, func(domainpublication.ReportDeliveryDecisionV1) error) error
}

// ReportGrantSettlementStore is the stable-key authority proving that one
// admitted report decision's exact execution grant and tool result are
// durably settled. It still does not authorize public projection by itself.
type ReportGrantSettlementResolver interface {
	Resolve(context.Context, string) (domainpublication.ReportGrantSettlementV1, error)
}

type ReportGrantSettlementStore interface {
	ReportGrantSettlementResolver
	CreateExclusive(context.Context, domainpublication.ReportGrantSettlementV1) (created bool, err error)
}

type ReportGrantSettlementInventoryStore interface {
	VisitGrantSettlements(context.Context, func(domainpublication.ReportGrantSettlementV1) error) error
}

// ReportStageCompletionStore is the stable-key authority joining one exact
// admitted decision and grant settlement to the exact signed completed
// report-stage disposition. Only this joined record can enter final delivery.
type ReportStageCompletionResolver interface {
	Resolve(context.Context, string) (domainpublication.ReportStageCompletionV1, error)
}

type ReportStageCompletionStore interface {
	ReportStageCompletionResolver
	CreateExclusive(context.Context, domainpublication.ReportStageCompletionV1) (created bool, err error)
}

type ReportStageCompletionInventoryStore interface {
	VisitStageCompletions(context.Context, func(domainpublication.ReportStageCompletionV1) error) error
}

type IndexResolver interface {
	Resolve(context.Context, string) (domainpublication.PublicationIndexV1, error)
}

type IndexStore interface {
	IndexResolver
	PutIfAbsent(context.Context, domainpublication.PublicationIndexV1) error
}

type IndexInventoryStore interface {
	VisitIndexes(context.Context, func(domainpublication.PublicationIndexV1) error) error
}

type ClaimLedgerResolver interface {
	Resolve(context.Context, string) (domainpublication.ClaimLedgerV1, error)
}

type ClaimLedgerStore interface {
	ClaimLedgerResolver
	PutIfAbsent(context.Context, domainpublication.ClaimLedgerV1) error
}

type PIIProjectionResolver interface {
	Resolve(context.Context, string) (domainpublication.PIIProjectionV1, error)
}

type PIIProjectionStore interface {
	PIIProjectionResolver
	PutIfAbsent(context.Context, domainpublication.PIIProjectionV1) error
}

type RenderInspectionResolver interface {
	Resolve(context.Context, string) (domainpublication.RenderInspectionV1, error)
}

type RenderInspectionStore interface {
	RenderInspectionResolver
	PutIfAbsent(context.Context, domainpublication.RenderInspectionV1) error
}

type ArtifactResolver interface {
	// ResolveExact returns caller-owned bytes. Implementations must never expose
	// an internal mutable backing slice.
	ResolveExact(context.Context, string) ([]byte, error)
}

// ArtifactStore installs content under a host-derived opaque target identity.
// It must not expose directory listing or a user-visible alias.
type ArtifactStore interface {
	ArtifactResolver
	InstallNoReplace(context.Context, string, []byte) error
}

// ExtractedReportSurfaceV1 contains the complete canonical text surface of a
// staged ordinary report. The text is ephemeral and must never be persisted,
// logged, or returned to a provider.
type ExtractedReportSurfaceV1 struct {
	ExtractorID      string
	ExtractorVersion string
	CanonicalText    []byte
}

// CompleteReportSurfaceExtractor either returns every inspectable text
// surface for the media type or fails closed. It never returns a caller-owned
// pass/fail decision; the report use case applies the privacy policy itself.
type CompleteReportSurfaceExtractor interface {
	ExtractCompleteCanonicalSurface(context.Context, string, []byte) (ExtractedReportSurfaceV1, error)
}

// ControlledArtifactMetadataStore parses protected controlled-PII bytes inside
// the outbound adapter and returns only a raw-value-free binding projection.
type ControlledArtifactMetadataResolver interface {
	ResolveControlledMetadata(context.Context, string) (domainpii.ControlledPIIArtifactMetadataV1, error)
}

type ControlledArtifactMetadataStore interface {
	ControlledArtifactMetadataResolver
}

// PIIAuthorizationAuthority revalidates a controlled-artifact authorization
// at both inspection and publication-CAS boundaries. Ordinary masked reports
// do not call it.
type PIIAuthorizationAuthority interface {
	ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext, domainpublication.PIIProjectionV1, string) error
}

// ControlledArtifactPIIAuthorizationAuthority binds the signed authorization
// grant to the canonical controlled artifact metadata, including its exact
// claim ledger and field-binding set. Report publication must require this
// stronger interface; ValidateCurrent alone cannot prove artifact shape.
type ControlledArtifactPIIAuthorizationAuthority interface {
	ValidateControlledArtifactCurrent(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainpublication.PIIProjectionV1,
		domainpii.ControlledPIIArtifactMetadataV1,
	) error
	ValidateControlledArtifactCurrentWithinSnapshot(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainpublication.PIIProjectionV1,
		domainpii.ControlledPIIArtifactMetadataV1,
		registryport.WitnessedSnapshot,
		registryport.WitnessedSnapshotCapability,
	) error
}

// DeliveryOutcomeStore is the single stable-key authority for both projected
// and rejected completion-bound terminal outcomes. A decision, commit, grant
// settlement, or generic disposition is intentionally not accepted. The
// returned winner, rather than a caller's candidate, is authoritative when
// concurrent opposite outcomes race or a write acknowledgement is lost.
type DeliveryOutcomeResolver interface {
	ResolveOutcome(context.Context, string) (domainpublication.ReportDeliveryOutcomeV1, error)
}

type DeliveryOutcomeStore interface {
	DeliveryOutcomeResolver
	CreateOutcomeExclusive(
		context.Context,
		domainpublication.ReportStageCompletionV1,
		domainpublication.ReportDeliveryOutcomeV1,
	) (domainpublication.ReportDeliveryOutcomeV1, bool, error)
}

type DeliveryOutcomeInventoryStore interface {
	VisitDeliveryOutcomes(context.Context, func(domainpublication.ReportDeliveryOutcomeV1) error) error
}
