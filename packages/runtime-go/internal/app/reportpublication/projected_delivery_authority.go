package reportpublication

import (
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
	"strings"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authoritystoreport "analytix.local/runtime-go/internal/ports/authorityadvance"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

var (
	ErrProjectedDeliveryUnavailable = publicationport.ErrProjectedDeliveryUnavailable
	ErrProjectedDeliveryInvalid     = publicationport.ErrProjectedDeliveryInvalid
	ErrProjectedDeliveryStale       = publicationport.ErrProjectedDeliveryStale
	ErrProjectedDeliveryNotFound    = publicationport.ErrProjectedDeliveryNotFound
	ErrProjectedDeliveryRejected    = publicationport.ErrProjectedDeliveryRejected
	ErrProjectedDeliveryIntegrity   = publicationport.ErrProjectedDeliveryIntegrity
	ErrProjectedDeliveryCancelled   = publicationport.ErrProjectedDeliveryCancelled
)

type ProjectedDeliveryCurrentContextValidatorV1 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) error

type ProjectedDeliveryPendingResolverV1 interface {
	ResolveTrustedCompletedReportStageV1(
		context.Context,
		string,
	) (pendingworkapp.TrustedCompletedReportStageV1, error)
}

// ProjectedDeliveryAuthorityConfigV1 contains only keyed readers. Controlled
// release must not perform an O(all reports) inventory scan or let unrelated
// historical corruption become a release-time denial-of-service primitive.
type ProjectedDeliveryAuthorityConfigV1 struct {
	InstallationID       string
	EnrollmentID         string
	WitnessKeyID         string
	WitnessKey           []byte
	Attempts             publicationport.AttemptResolver
	Receipts             publicationport.ReceiptResolver
	Indexes              publicationport.IndexResolver
	Selections           publicationport.CommitSelectionResolver
	Commits              publicationport.CommitReceiptResolver
	Decisions            publicationport.DeliveryDecisionResolver
	GrantSettlements     publicationport.ReportGrantSettlementResolver
	StageCompletions     publicationport.ReportStageCompletionResolver
	DeliveryOutcomes     publicationport.DeliveryOutcomeResolver
	Ledgers              publicationport.ClaimLedgerResolver
	Projections          publicationport.PIIProjectionResolver
	Inspections          publicationport.RenderInspectionResolver
	Intents              authoritystoreport.IntentResolver
	Settlements          authoritystoreport.SettlementResolver
	Bundles              evidenceauthorityport.BundleResolver
	Observations         evidenceauthorityport.ObservationResolver
	Artifacts            publicationport.ArtifactResolver
	ControlledMetadata   publicationport.ControlledArtifactMetadataResolver
	Pending              ProjectedDeliveryPendingResolverV1
	Evidence             registryport.WitnessedSnapshotReader
	PIIAuthority         publicationport.ControlledArtifactPIIAuthorizationAuthority
	Authority            finalauthorityport.Verifier
	ValidateCurrent      ProjectedDeliveryCurrentContextValidatorV1
	AcquireContextEffect RestartContextEffectAcquirerV1
}

type ProjectedDeliveryAuthorityV1 struct {
	config ProjectedDeliveryAuthorityConfigV1
}

var _ publicationport.ProjectedDeliveryAuthority = (*ProjectedDeliveryAuthorityV1)(nil)
var _ publicationport.LinearizedProjectedDeliveryAuthority = (*ProjectedDeliveryAuthorityV1)(nil)
var _ publicationport.HistoricalProjectedDeliveryAuthority = (*ProjectedDeliveryAuthorityV1)(nil)

func NewProjectedDeliveryAuthorityV1(
	config ProjectedDeliveryAuthorityConfigV1,
) (*ProjectedDeliveryAuthorityV1, error) {
	if !validHistoricalProjectedDeliveryConfigV1(config) || config.Evidence == nil || config.PIIAuthority == nil ||
		config.ValidateCurrent == nil || config.AcquireContextEffect == nil {
		return nil, ErrProjectedDeliveryUnavailable
	}
	config.WitnessKey = append([]byte(nil), config.WitnessKey...)
	return &ProjectedDeliveryAuthorityV1{config: config}, nil
}

// NewHistoricalProjectedDeliveryAuthorityV1 constructs only immutable audit
// authority. It intentionally accepts no current context, witnessed snapshot,
// PII-currentness, or effect-lease authority and therefore cannot authorize a
// live release through ResolveCurrentProjectedDelivery.
func NewHistoricalProjectedDeliveryAuthorityV1(
	config ProjectedDeliveryAuthorityConfigV1,
) (*ProjectedDeliveryAuthorityV1, error) {
	config.Evidence = nil
	config.PIIAuthority = nil
	config.ValidateCurrent = nil
	config.AcquireContextEffect = nil
	if !validHistoricalProjectedDeliveryConfigV1(config) {
		return nil, ErrProjectedDeliveryUnavailable
	}
	config.WitnessKey = append([]byte(nil), config.WitnessKey...)
	return &ProjectedDeliveryAuthorityV1{config: config}, nil
}

func validHistoricalProjectedDeliveryConfigV1(config ProjectedDeliveryAuthorityConfigV1) bool {
	return config.Attempts != nil && config.Receipts != nil && config.Indexes != nil && config.Selections != nil &&
		config.Commits != nil && config.Decisions != nil && config.GrantSettlements != nil &&
		config.StageCompletions != nil && config.DeliveryOutcomes != nil && config.Ledgers != nil &&
		config.Projections != nil && config.Inspections != nil && config.Intents != nil && config.Settlements != nil &&
		config.Bundles != nil && config.Observations != nil && config.Artifacts != nil && config.ControlledMetadata != nil &&
		config.Pending != nil && config.Authority != nil &&
		domainsecurity.IsSHA256Hex(strings.TrimSpace(config.InstallationID)) &&
		domainsecurity.IsSHA256Hex(strings.TrimSpace(config.EnrollmentID)) &&
		len(config.WitnessKey) == ed25519.PublicKeySize &&
		strings.TrimSpace(config.WitnessKeyID) == domainsecurity.SHA256Hex(config.WitnessKey) &&
		len(config.Authority.PublicKey()) == ed25519.PublicKeySize &&
		config.Authority.KeyID() == domainsecurity.SHA256Hex(config.Authority.PublicKey())
}

func (authority *ProjectedDeliveryAuthorityV1) ResolveCurrentProjectedDelivery(
	ctx context.Context,
	selector publicationport.ProjectedDeliverySelectorV1,
) (domainpublication.ReportDeliveryProjectionV1, error) {
	return authority.resolveCurrentProjectedDeliveryV1(ctx, selector, nil)
}

func (authority *ProjectedDeliveryAuthorityV1) WithCurrentProjectedDelivery(
	ctx context.Context,
	selector publicationport.ProjectedDeliverySelectorV1,
	use func(domainpublication.ReportDeliveryProjectionV1) error,
) error {
	if use == nil {
		return ErrProjectedDeliveryInvalid
	}
	_, err := authority.resolveCurrentProjectedDeliveryV1(ctx, selector, use)
	return err
}

func (authority *ProjectedDeliveryAuthorityV1) ResolveTrustedHistoricalProjectedDelivery(
	ctx context.Context,
	selector publicationport.HistoricalProjectedDeliverySelectorV1,
) (domainpublication.ReportDeliveryProjectionV1, error) {
	if authority == nil || ctx == nil || !validHistoricalProjectedDeliverySelectorV1(selector) {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryCancelled, err)
	}
	outcome, err := authority.config.DeliveryOutcomes.ResolveOutcome(ctx, selector.DeliveryID)
	if errors.Is(err, publicationport.ErrNotFound) {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryNotFound
	}
	if err != nil {
		return domainpublication.ReportDeliveryProjectionV1{}, normalizeProjectedDeliveryErrorV1(ctx, err)
	}
	if domainpublication.ValidateReportDeliveryOutcomeV1(outcome) != nil ||
		domainpublication.ReportDeliveryOutcomeID(outcome) != selector.DeliveryID ||
		domainpublication.ReportDeliveryOutcomeRecordDigest(outcome) != selector.OutcomeRecordDigest ||
		verifyTrustedDeliveryOutcomeV1(ctx, authority.config.Authority, outcome) != nil {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryIntegrity
	}
	if outcome.Kind == domainpublication.ReportDeliveryOutcomeRejectedV1 {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryRejected
	}
	if outcome.Kind != domainpublication.ReportDeliveryOutcomeProjectedV1 || outcome.Projection == nil {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryIntegrity
	}
	graph, err := authority.resolveExactProjectedGraphV1(
		ctx, exactProjectedDeliverySelectorFromHistoricalV1(selector), *outcome.Projection,
	)
	if err != nil {
		return domainpublication.ReportDeliveryProjectionV1{}, normalizeProjectedDeliveryErrorV1(ctx, err)
	}
	return graph.Projection, nil
}

// VerifyTrustedHistoricalDeliveryOutcomeV1 audits either durable terminal
// through its actual completion, Core grant/result, witness and materials.
// It grants no current release authority. Callers still own the complete
// inventory denominator and the physical Original/current-key read boundary.
func (authority *ProjectedDeliveryAuthorityV1) VerifyTrustedHistoricalDeliveryOutcomeV1(
	ctx context.Context,
	selector publicationport.HistoricalProjectedDeliverySelectorV1,
) error {
	if authority == nil || ctx == nil || !validHistoricalProjectedDeliverySelectorV1(selector) {
		return ErrProjectedDeliveryInvalid
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrProjectedDeliveryCancelled, err)
	}
	outcome, err := authority.config.DeliveryOutcomes.ResolveOutcome(ctx, selector.DeliveryID)
	if errors.Is(err, publicationport.ErrNotFound) {
		return normalizeProjectedDeliveryErrorV1(ctx, errors.Join(ErrProjectedDeliveryNotFound, err))
	}
	if err != nil {
		return normalizeProjectedDeliveryErrorV1(ctx, err)
	}
	if domainpublication.ValidateReportDeliveryOutcomeV1(outcome) != nil ||
		domainpublication.ReportDeliveryOutcomeID(outcome) != selector.DeliveryID ||
		domainpublication.ReportDeliveryOutcomeRecordDigest(outcome) != selector.OutcomeRecordDigest {
		return ErrProjectedDeliveryIntegrity
	}
	if err := verifyTrustedDeliveryOutcomeV1(ctx, authority.config.Authority, outcome); err != nil {
		return normalizeProjectedDeliveryErrorV1(ctx, err)
	}
	completionID := domainpublication.ReportDeliveryOutcomeCompletionID(outcome)
	completion, err := authority.config.StageCompletions.Resolve(ctx, completionID)
	if err != nil || completion.CompletionID != completionID {
		return normalizeProjectedDeliveryErrorV1(ctx, projectedDeliveryIntegrityV1(err))
	}
	graph, err := authority.resolveExactCompletedGraphV1(ctx, exactProjectedDeliverySelectorFromHistoricalV1(selector), completion)
	if err != nil {
		return normalizeProjectedDeliveryErrorV1(ctx, err)
	}
	if err := validateRestartDeliveryOutcomeGraphV1(
		outcome, graph.Decision, graph.GrantSettlement, graph.StageReceipt, graph.StageDisposition, graph.Completion,
		authority.config.Authority.KeyID(), authority.config.Authority.PublicKey(),
	); err != nil {
		return ErrProjectedDeliveryIntegrity
	}
	current, err := authority.config.DeliveryOutcomes.ResolveOutcome(ctx, selector.DeliveryID)
	if err != nil || !reflect.DeepEqual(current, outcome) {
		return normalizeProjectedDeliveryErrorV1(ctx, projectedDeliveryIntegrityV1(err))
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrProjectedDeliveryCancelled, err)
	}
	return nil
}

func (authority *ProjectedDeliveryAuthorityV1) resolveCurrentProjectedDeliveryV1(
	ctx context.Context,
	selector publicationport.ProjectedDeliverySelectorV1,
	use func(domainpublication.ReportDeliveryProjectionV1) error,
) (domainpublication.ReportDeliveryProjectionV1, error) {
	if authority == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(selector.SecurityContext) != nil ||
		!domainsecurity.IsSHA256Hex(selector.DeliveryID) ||
		!domainsecurity.IsSHA256Hex(selector.OutcomeRecordDigest) ||
		!domainsecurity.IsSHA256Hex(selector.PublicationCommitDigest) {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryInvalid
	}
	if authority.config.Evidence == nil || authority.config.PIIAuthority == nil ||
		authority.config.ValidateCurrent == nil || authority.config.AcquireContextEffect == nil {
		return domainpublication.ReportDeliveryProjectionV1{}, ErrProjectedDeliveryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryCancelled, err)
	}
	effectCtx, releaseEffect, err := authority.config.AcquireContextEffect(ctx, selector.SecurityContext)
	if err != nil || effectCtx == nil || releaseEffect == nil {
		if releaseEffect != nil {
			releaseEffect()
		}
		if ctx.Err() != nil {
			return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryCancelled, ctx.Err())
		}
		return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryStale, err)
	}
	defer releaseEffect()
	ctx = effectCtx
	if err := authority.config.ValidateCurrent(ctx, selector.SecurityContext); err != nil {
		if ctx.Err() != nil {
			return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryCancelled, ctx.Err())
		}
		return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryStale, err)
	}
	projection, err := authority.resolveCurrentProjectedWithinSnapshotV1(ctx, selector, use)
	if err != nil {
		var useErr *projectedDeliveryUseErrorV1
		if errors.As(err, &useErr) {
			return domainpublication.ReportDeliveryProjectionV1{}, useErr.cause
		}
		return domainpublication.ReportDeliveryProjectionV1{}, normalizeProjectedDeliveryErrorV1(ctx, err)
	}
	if err := authority.config.ValidateCurrent(ctx, selector.SecurityContext); err != nil {
		if ctx.Err() != nil {
			return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryCancelled, ctx.Err())
		}
		return domainpublication.ReportDeliveryProjectionV1{}, errors.Join(ErrProjectedDeliveryStale, err)
	}
	return projection, nil
}

type projectedDeliveryUseErrorV1 struct {
	cause error
}

func (failure *projectedDeliveryUseErrorV1) Error() string {
	if failure == nil || failure.cause == nil {
		return "projected delivery use failed"
	}
	return failure.cause.Error()
}

func (failure *projectedDeliveryUseErrorV1) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func normalizeProjectedDeliveryErrorV1(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return errors.Join(ErrProjectedDeliveryCancelled, ctx.Err(), context.Cause(ctx), projectedDeliveryUnderlyingCausesV1(err))
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(ErrProjectedDeliveryCancelled, projectedDeliveryUnderlyingCausesV1(err))
	}
	for _, classified := range []error{
		ErrProjectedDeliveryInvalid, ErrProjectedDeliveryStale, ErrProjectedDeliveryNotFound,
		ErrProjectedDeliveryRejected, ErrProjectedDeliveryIntegrity, ErrProjectedDeliveryUnavailable,
	} {
		if errors.Is(err, classified) {
			return err
		}
	}
	return errors.Join(ErrProjectedDeliveryUnavailable, err)
}

// Cancellation supersedes delivery classifications, but never erases the
// underlying physical/dependency causes joined with them.
func projectedDeliveryUnderlyingCausesV1(err error) error {
	classified := false
	for _, class := range []error{
		ErrProjectedDeliveryInvalid, ErrProjectedDeliveryStale, ErrProjectedDeliveryNotFound,
		ErrProjectedDeliveryRejected, ErrProjectedDeliveryIntegrity, ErrProjectedDeliveryUnavailable,
		ErrProjectedDeliveryCancelled,
	} {
		if err == class {
			return nil
		}
		classified = classified || errors.Is(err, class)
	}
	if !classified {
		return err
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		causes := []error{}
		for _, cause := range wrapped.Unwrap() {
			causes = append(causes, projectedDeliveryUnderlyingCausesV1(cause))
		}
		return errors.Join(causes...)
	case interface{ Unwrap() error }:
		return projectedDeliveryUnderlyingCausesV1(wrapped.Unwrap())
	default:
		return err
	}
}

func validHistoricalProjectedDeliverySelectorV1(selector publicationport.HistoricalProjectedDeliverySelectorV1) bool {
	for _, value := range []string{
		selector.ThreadID, selector.TurnID, selector.ContextDigest, selector.CaseBindingHash,
		selector.DatasetSnapshotID, selector.SourceManifestHash,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return false
		}
	}
	return domainsecurity.IsSHA256Hex(selector.DeliveryID) &&
		domainsecurity.IsSHA256Hex(selector.OutcomeRecordDigest) &&
		domainsecurity.IsSHA256Hex(selector.PublicationCommitDigest) &&
		domainsecurity.IsSHA256Hex(selector.ContextDigest) &&
		domainsecurity.IsSHA256Hex(selector.CaseBindingHash) && selector.ContextEpoch > 0 &&
		domainsecurity.IsDatasetSnapshotIDV2Syntax(selector.DatasetSnapshotID) &&
		domainsecurity.IsSHA256Hex(selector.SourceManifestHash)
}

func verifyTrustedDeliveryOutcomeV1(
	ctx context.Context,
	authority finalauthorityport.Verifier,
	outcome domainpublication.ReportDeliveryOutcomeV1,
) error {
	switch outcome.Kind {
	case domainpublication.ReportDeliveryOutcomeProjectedV1:
		projection := *outcome.Projection
		keyID, publicKey, signature, err := domainpublication.ReportDeliveryProjectionAuthorityMaterialV1(projection)
		if err != nil {
			return err
		}
		return authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.ReportDeliveryProjectionSigningBytesV1(projection), signature)
	case domainpublication.ReportDeliveryOutcomeRejectedV1:
		rejection := *outcome.Rejection
		keyID, publicKey, signature, err := domainpublication.ReportDeliveryRejectionAuthorityMaterialV1(rejection)
		if err != nil {
			return err
		}
		return authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.ReportDeliveryRejectionSigningBytesV1(rejection), signature)
	default:
		return ErrProjectedDeliveryIntegrity
	}
}
