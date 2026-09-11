package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	authorityadvanceapp "analytix.local/runtime-go/internal/app/authorityadvance"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	piiauthorizationapp "analytix.local/runtime-go/internal/app/piiauthorization"
	domainauthorityadvance "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainrestrictedevidence "analytix.local/runtime-go/internal/domain/restrictedevidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

const maxPublicationIndexDepthV1 = 100_000

const maxOrdinaryReportCanonicalTextBytesV1 = 64 << 20

var (
	ErrPublicationUnavailable  = errors.New("report publication authority is unavailable")
	ErrPublicationIntegrity    = errors.New("report publication authority integrity failure")
	ErrPublicationChanged      = errors.New("report publication evidence authority changed")
	ErrControlledLeaseRequired = errors.New("controlled report publication requires an authorized terminal lease")
)

type StageAuthority interface {
	BeginReportStage(context.Context, pendingworkapp.ReportStageRequest) (pendingworkapp.ReportStageLease, error)
	ResolveReportStageLease(context.Context, pendingworkapp.ReportStageLease, pendingworkapp.ReportStageRequest) (domainpendingwork.PendingWorkReceiptV1, error)
	VerifyReportStageRequest(context.Context, pendingworkapp.ReportStageLease, pendingworkapp.ReportStageRequest, time.Time) error
	CloseReportStageLease(context.Context, pendingworkapp.ReportStageLease, pendingworkapp.ReportStageRequest, string, time.Time) (domainpendingwork.PendingWorkDispositionV1, error)
}

type ExactPublicationCoordinator interface {
	CommitExact(context.Context, domainauthorityadvance.MonotonicAdvanceIntentV2) (authorityadvanceapp.CommitResultV2, error)
	RecoverExact(context.Context, string) (authorityadvanceapp.CommitResultV2, error)
}

type Config struct {
	InstallationID   string
	EnrollmentID     string
	Authority        finalauthorityport.Authority
	WitnessKeyID     string
	WitnessKey       []byte
	Stages           StageAuthority
	Evidence         registryport.WitnessedSnapshotReader
	HeadReader       evidenceauthorityport.FreshHeadReader
	Advances         ExactPublicationCoordinator
	Bundles          evidenceauthorityport.BundleStore
	Observations     evidenceauthorityport.ObservationStore
	Attempts         publicationport.AttemptStore
	Receipts         publicationport.ReceiptStore
	Commits          publicationport.CommitReceiptStore
	Selections       publicationport.CommitSelectionStore
	Decisions        publicationport.DeliveryDecisionStore
	Indexes          publicationport.IndexStore
	Ledgers          publicationport.ClaimLedgerStore
	PIIProjections   publicationport.PIIProjectionStore
	Inspections      publicationport.RenderInspectionStore
	Artifacts        publicationport.ArtifactStore
	SurfaceExtractor publicationport.CompleteReportSurfaceExtractor
	PIIAuthority     publicationport.PIIAuthorizationAuthority
	DeliveryOutcomes publicationport.DeliveryOutcomeStore
	Now              func() time.Time
}

type PublishInput struct {
	WriteReport          bool
	PendingToolCall      appmodel.PendingToolCall
	ReportVariant        string
	ClaimLedger          domainpublication.ClaimLedgerV1
	PIIProjection        domainpublication.PIIProjectionV1
	RenderInspection     domainpublication.RenderInspectionV1
	ReportBytes          []byte
	Publisher            string
	PublisherVersion     string
	TargetIdentityDigest string
	IssuedAt             time.Time
}

type PublishResult struct {
	Skipped   bool
	Candidate domainpublication.PublicationReceiptV1
	Index     domainpublication.PublicationIndexV1
	Commit    domainpublication.PublicationCommitReceiptV1
	Decision  domainpublication.ReportDeliveryDecisionV1
	Lease     pendingworkapp.ReportStageLease
}

func withWitnessedSnapshotAuthorityV2(
	ctx context.Context,
	reader registryport.WitnessedSnapshotReader,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(registryport.WitnessedSnapshot, registryport.WitnessedSnapshotCapability) error,
) error {
	authority, ok := reader.(registryport.WitnessedSnapshotAuthority)
	if ctx == nil || reader == nil || callback == nil || !ok {
		return ErrPublicationUnavailable
	}
	return authority.WithWitnessedSnapshotAuthority(ctx, securityContext, callback)
}

type Service struct {
	installationID   string
	enrollmentID     string
	authority        finalauthorityport.Authority
	keyID            string
	publicKey        []byte
	witnessKeyID     string
	witnessKey       []byte
	stages           StageAuthority
	evidence         registryport.WitnessedSnapshotReader
	headReader       evidenceauthorityport.FreshHeadReader
	advances         ExactPublicationCoordinator
	bundles          evidenceauthorityport.BundleStore
	observations     evidenceauthorityport.ObservationStore
	attempts         publicationport.AttemptStore
	receipts         publicationport.ReceiptStore
	commits          publicationport.CommitReceiptStore
	selections       publicationport.CommitSelectionStore
	decisions        publicationport.DeliveryDecisionStore
	indexes          publicationport.IndexStore
	ledgers          publicationport.ClaimLedgerStore
	piiProjections   publicationport.PIIProjectionStore
	inspections      publicationport.RenderInspectionStore
	artifacts        publicationport.ArtifactStore
	surfaceExtractor publicationport.CompleteReportSurfaceExtractor
	piiAuthority     publicationport.PIIAuthorizationAuthority
	deliveryOutcomes publicationport.DeliveryOutcomeStore
	now              func() time.Time

	mu sync.Mutex
}

func New(config Config) (*Service, error) {
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	publicKey := []byte(nil)
	keyID := ""
	if config.Authority != nil {
		publicKey = append([]byte(nil), config.Authority.PublicKey()...)
		keyID = strings.TrimSpace(config.Authority.KeyID())
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(config.InstallationID)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.EnrollmentID)) || config.Authority == nil ||
		len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		len(config.WitnessKey) != ed25519.PublicKeySize || strings.TrimSpace(config.WitnessKeyID) != domainsecurity.SHA256Hex(config.WitnessKey) || config.Stages == nil ||
		config.Evidence == nil || config.HeadReader == nil || config.Advances == nil || config.Bundles == nil || config.Attempts == nil ||
		config.Observations == nil ||
		config.Receipts == nil || config.Commits == nil || config.Selections == nil || config.Decisions == nil || config.Indexes == nil || config.Ledgers == nil ||
		config.PIIProjections == nil || config.Inspections == nil || config.Artifacts == nil || config.SurfaceExtractor == nil ||
		config.DeliveryOutcomes == nil || config.Now == nil {
		return nil, ErrPublicationUnavailable
	}
	return &Service{
		installationID: strings.TrimSpace(config.InstallationID), enrollmentID: strings.TrimSpace(config.EnrollmentID),
		authority: config.Authority, keyID: keyID, publicKey: publicKey, stages: config.Stages, evidence: config.Evidence,
		witnessKeyID: strings.TrimSpace(config.WitnessKeyID), witnessKey: append([]byte(nil), config.WitnessKey...),
		headReader: config.HeadReader, advances: config.Advances, bundles: config.Bundles, observations: config.Observations,
		attempts: config.Attempts,
		receipts: config.Receipts, commits: config.Commits, selections: config.Selections, decisions: config.Decisions,
		indexes: config.Indexes, ledgers: config.Ledgers,
		piiProjections: config.PIIProjections, inspections: config.Inspections, artifacts: config.Artifacts,
		surfaceExtractor: config.SurfaceExtractor,
		piiAuthority:     config.PIIAuthority, deliveryOutcomes: config.DeliveryOutcomes, now: config.Now,
	}, nil
}

func (service *Service) Publish(ctx context.Context, input PublishInput) (result PublishResult, err error) {
	// This check intentionally precedes dependency, context, grant, renderer,
	// filesystem, witness, and stage validation. write_report=false is a
	// mechanical zero-side-effect branch.
	if !input.WriteReport {
		return PublishResult{Skipped: true}, nil
	}
	if input.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull {
		return PublishResult{}, ErrControlledLeaseRequired
	}
	return service.publish(ctx, input, false)
}

// PublishControlled is the only controlled-full publication entry. Raw
// controlled bytes cannot be supplied in PublishInput; they are read exactly
// once from the concrete authorization-issued terminal lease.
func (service *Service) PublishControlled(
	ctx context.Context,
	input PublishInput,
	lease *piiauthorizationapp.ControlledPIITerminalLeaseV1,
) (result PublishResult, err error) {
	if !input.WriteReport {
		if lease != nil {
			_ = lease.Close()
		}
		return PublishResult{Skipped: true}, nil
	}
	if service == nil || ctx == nil || lease == nil || len(input.ReportBytes) != 0 ||
		input.PIIProjection.ProjectionClass != domainpublication.PIIProjectionControlledFull {
		if lease != nil {
			_ = lease.Close()
		}
		return PublishResult{}, ErrControlledLeaseRequired
	}
	defer lease.Close()
	metadata, metadataErr := lease.Metadata()
	securityContext := input.PendingToolCall.SecurityContext
	if metadataErr != nil || metadata.ContextDigest != securityContext.ContextDigest ||
		metadata.ContextEpoch != securityContext.ContextEpoch || metadata.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		metadata.ClaimLedgerDigest != input.ClaimLedger.LedgerDigest ||
		metadata.TargetIdentityDigest != input.TargetIdentityDigest ||
		metadata.ArtifactSHA256 != input.PIIProjection.ProjectedContentSHA256 ||
		metadata.ArtifactByteLength != input.RenderInspection.ReportByteLength ||
		metadata.MediaType != input.RenderInspection.MediaType ||
		metadata.PIIAuthorizationDigest != input.PIIProjection.AuthorizationAuditDigest ||
		metadata.PIIProjectionDigest != input.PIIProjection.ProjectionDigest ||
		service.validateControlledArtifactAuthorization(ctx, securityContext, input.PIIProjection, metadata.ArtifactMetadata) != nil {
		return PublishResult{}, ErrControlledLeaseRequired
	}
	var publicationErr error
	disposition, handoffErr := lease.Consume(ctx, func(
		consumeCtx context.Context,
		reader io.Reader,
		_ piiauthorizationapp.ControlledPIITerminalLeaseMetadataV1,
	) error {
		reportBytes, readErr := io.ReadAll(io.LimitReader(reader, int64(metadata.ArtifactByteLength)+1))
		defer clearControlledPublicationBytesV1(reportBytes)
		if readErr != nil || uint64(len(reportBytes)) != metadata.ArtifactByteLength ||
			domainsecurity.SHA256Hex(reportBytes) != metadata.ArtifactSHA256 {
			return ErrPublicationIntegrity
		}
		controlledInput := input
		controlledInput.ReportBytes = reportBytes
		result, publicationErr = service.publish(consumeCtx, controlledInput, true)
		return publicationErr
	})
	if handoffErr != nil || disposition.Status != piiauthorizationapp.ControlledPIITerminalHandoffConsumedV1 {
		return result, errors.Join(ErrPublicationIntegrity, handoffErr, publicationErr)
	}
	return result, publicationErr
}

func (service *Service) publish(ctx context.Context, input PublishInput, allowControlled bool) (result PublishResult, err error) {
	if service == nil || ctx == nil {
		return PublishResult{}, ErrPublicationUnavailable
	}
	isControlled := input.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull
	if isControlled != allowControlled {
		return PublishResult{}, ErrControlledLeaseRequired
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	input.ReportBytes = bytes.Clone(input.ReportBytes)
	if allowControlled {
		defer clearControlledPublicationBytesV1(input.ReportBytes)
	}
	if err := service.validateOrdinaryReportSurface(ctx, input); err != nil {
		return PublishResult{}, err
	}
	controlledMetadata, err := service.validateControlledReportBytes(input)
	if err != nil {
		return PublishResult{}, err
	}

	stageRequest, err := service.validateInput(input)
	if err != nil {
		return PublishResult{}, err
	}
	lease, err := service.stages.BeginReportStage(ctx, stageRequest)
	if err != nil {
		return PublishResult{}, err
	}
	result.Lease = lease
	succeeded := false
	publicationCASInvoked := false
	defer func() {
		// AdvancePublication is the first irreversible external boundary. Its
		// error may be an acknowledgement loss after a successful witness CAS,
		// so no later failure may be rewritten as a definitive failed effect.
		// Leaving the signed stage open makes restart classify it mechanically
		// as outcome-unknown until a durable outbox reconciler is available.
		if !succeeded && !publicationCASInvoked {
			_, _ = service.stages.CloseReportStageLease(context.WithoutCancel(ctx), lease, stageRequest, domainpendingwork.StatusFailed, service.now().UTC())
		}
	}()
	stageReceipt, err := service.stages.ResolveReportStageLease(ctx, lease, stageRequest)
	if err != nil {
		return result, err
	}
	attemptID := domainpublication.PublicationAttemptIDV1(service.installationID, service.enrollmentID, stageReceipt.WorkID)
	priorAttempt, err := service.attempts.Resolve(ctx, attemptID)
	if err == nil {
		// A prior reservation may already have crossed any later effect
		// boundary. Resolve it only through restart reconciliation, including
		// when the publication head has already selected its index.
		publicationCASInvoked = true
		stageReceiptBody, bodyErr := domainpendingwork.PendingWorkReceiptV1Bytes(stageReceipt)
		if bodyErr != nil ||
			domainpublication.ValidatePublicationAttemptForInstallationV1(
				priorAttempt, service.installationID, service.enrollmentID, service.keyID, service.publicKey,
			) != nil ||
			priorAttempt.AttemptID != attemptID || priorAttempt.ReportStageWorkID != stageReceipt.WorkID ||
			priorAttempt.ReportStageReceiptID != stageReceipt.ReceiptID ||
			priorAttempt.ReportStageReceiptSHA256 != domainsecurity.SHA256Hex(stageReceiptBody) {
			return result, ErrPublicationIntegrity
		}
		return result, errors.New("report publication attempt is already reserved for restart reconciliation")
	}
	if !errors.Is(err, publicationport.ErrNotFound) {
		// An indeterminate read cannot prove that no write-ahead reservation
		// exists, so it must not close the report stage as definitively failed.
		publicationCASInvoked = true
		return result, err
	}

	securityContext := input.PendingToolCall.SecurityContext
	if allowControlled {
		if service.validateControlledArtifactAuthorization(
			ctx, securityContext, input.PIIProjection, controlledMetadata,
		) != nil {
			return result, errors.New("controlled report PII authorization is unavailable")
		}
	}
	var inspected registryport.WitnessedSnapshot
	if err := service.evidence.WithWitnessedSnapshot(ctx, securityContext, func(snapshot registryport.WitnessedSnapshot) error {
		if err := service.validateLedgerSnapshot(ctx, snapshot, input.ReportVariant, input.ClaimLedger, allowControlled); err != nil {
			return err
		}
		inspected = snapshot
		return nil
	}); err != nil {
		return result, err
	}
	if err := service.stages.VerifyReportStageRequest(ctx, lease, stageRequest, service.now().UTC()); err != nil {
		return result, err
	}

	if allowControlled {
		if service.validateControlledArtifactAuthorization(
			ctx, securityContext, input.PIIProjection, controlledMetadata,
		) != nil {
			return result, errors.New("controlled report PII authorization changed before publication planning")
		}
	}

	var publishSnapshot registryport.WitnessedSnapshot
	if err := service.evidence.WithWitnessedSnapshot(ctx, securityContext, func(snapshot registryport.WitnessedSnapshot) error {
		if snapshot.Head.Bundle.RecordDigest != inspected.Head.Bundle.RecordDigest ||
			snapshot.Head.Bundle.DatasetSnapshotIndexDigest != inspected.Head.Bundle.DatasetSnapshotIndexDigest ||
			snapshot.Head.Bundle.EvidenceRegistryIndexDigest != inspected.Head.Bundle.EvidenceRegistryIndexDigest ||
			snapshot.Registry.Sequence != inspected.Registry.Sequence || snapshot.Registry.StateDigest != inspected.Registry.StateDigest {
			return ErrPublicationChanged
		}
		if err := service.validateLedgerSnapshot(ctx, snapshot, input.ReportVariant, input.ClaimLedger, allowControlled); err != nil {
			return err
		}
		publishSnapshot = snapshot
		return nil
	}); err != nil {
		return result, err
	}
	if err := service.stages.VerifyReportStageRequest(ctx, lease, stageRequest, service.now().UTC()); err != nil {
		return result, err
	}
	plan, err := service.preparePublicationPlan(ctx, input, stageRequest, stageReceipt, publishSnapshot)
	if err != nil {
		return result, err
	}
	created, err := service.attempts.CreateExclusive(ctx, plan.attempt)
	if err != nil {
		// The exclusive CAS may have installed the attempt before losing its
		// acknowledgement. Never downgrade that state to a definitive failure.
		publicationCASInvoked = true
		return result, err
	}
	if !created {
		// An exact prior reservation may already have crossed later effect
		// boundaries. Only restart reconciliation may resolve it.
		publicationCASInvoked = true
		return result, errors.New("report publication attempt is already reserved for restart reconciliation")
	}
	storedAttempt, err := service.attempts.Resolve(ctx, plan.attempt.AttemptID)
	if err != nil || !reflect.DeepEqual(storedAttempt, plan.attempt) {
		return result, ErrPublicationIntegrity
	}

	if err := service.artifacts.InstallNoReplace(ctx, input.TargetIdentityDigest, input.ReportBytes); err != nil {
		return result, err
	}
	artifact, err := service.artifacts.ResolveExact(ctx, input.TargetIdentityDigest)
	if err != nil || !bytes.Equal(artifact, input.ReportBytes) || domainsecurity.SHA256Hex(artifact) != input.RenderInspection.ReportSHA256 {
		return result, ErrPublicationIntegrity
	}
	revalidatedInput := input
	revalidatedInput.ReportBytes = bytes.Clone(artifact)
	if err := service.validateOrdinaryReportSurface(ctx, revalidatedInput); err != nil {
		return result, err
	}
	storedControlledMetadata, err := service.validateStoredControlledReportArtifact(
		ctx, revalidatedInput, controlledMetadata,
	)
	if err != nil {
		return result, err
	}
	if allowControlled {
		controlledMetadata = storedControlledMetadata
	}
	if err := service.persistMaterials(ctx, input); err != nil {
		return result, err
	}
	if allowControlled && service.validateControlledArtifactAuthorization(
		ctx, securityContext, input.PIIProjection, controlledMetadata,
	) != nil {
		return result, errors.New("controlled report PII authorization changed before durable publication intent")
	}
	if err := service.receipts.PutIfAbsent(ctx, plan.receipt); err != nil {
		return result, err
	}
	storedReceipt, err := service.receipts.Resolve(ctx, plan.receipt.RecordDigest)
	if err != nil || !reflect.DeepEqual(storedReceipt, plan.receipt) {
		return result, ErrPublicationIntegrity
	}
	if err := service.validatePublicationHead(ctx, publishSnapshot.Head, plan.receipt); err != nil {
		return result, err
	}
	if err := service.indexes.PutIfAbsent(ctx, plan.index); err != nil {
		return result, err
	}
	storedIndex, err := service.indexes.Resolve(ctx, plan.index.IndexDigest)
	if err != nil || !reflect.DeepEqual(storedIndex, plan.index) ||
		domainpublication.ValidatePublicationIndexReceiptV1(plan.index, plan.receipt) != nil {
		return result, ErrPublicationIntegrity
	}
	if err := service.bundles.PutIfAbsent(ctx, plan.nextBundle); err != nil {
		return result, err
	}
	storedBundle, err := service.bundles.Resolve(ctx, plan.nextBundle.RecordDigest)
	if err != nil || !reflect.DeepEqual(storedBundle, plan.nextBundle) {
		return result, ErrPublicationIntegrity
	}
	if err := service.stages.VerifyReportStageRequest(ctx, lease, stageRequest, service.now().UTC()); err != nil {
		return result, err
	}
	publicationCASInvoked = true
	committed, err := service.advances.CommitExact(ctx, plan.intent)
	if err != nil {
		return result, err
	}
	if !reflect.DeepEqual(committed.Intent, plan.intent) ||
		domainauthorityadvance.ValidateMonotonicAdvanceSettlementForIntentV2(committed.Settlement, plan.intent, nil) != nil ||
		committed.Settlement.Kind != domainauthorityadvance.MonotonicAdvanceSettlementCommittedV2 {
		return result, ErrPublicationIntegrity
	}
	advanced, err := service.headReader.ObserveFresh(ctx)
	if err != nil {
		return result, err
	}
	if err := service.validateCommittedPublication(ctx, publishSnapshot.Head, advanced, plan.index, plan.receipt, input); err != nil {
		return result, err
	}
	if advanced.Bundle.RecordDigest != plan.nextBundle.RecordDigest {
		return result, ErrPublicationIntegrity
	}
	if err := persistPublicationObservationExactV1(ctx, service.observations, advanced); err != nil {
		return result, err
	}
	if allowControlled && service.validateControlledArtifactAuthorization(
		ctx, securityContext, input.PIIProjection, controlledMetadata,
	) != nil {
		return result, errors.New("controlled report PII authorization changed after witness commit")
	}
	commit, _, err := selectAndSignPublicationCommitV1(
		ctx, plan.attempt, plan.receipt, plan.index, committed.Settlement.RecordDigest, &advanced,
		CommitSelectorConfigV1{
			InstallationID: service.installationID, EnrollmentID: service.enrollmentID, Authority: service.authority,
			WitnessKeyID: service.witnessKeyID, WitnessKey: service.witnessKey,
			HeadReader: service.headReader, Bundles: service.bundles, Observations: service.observations,
			Selections: service.selections, Commits: service.commits,
		},
	)
	if err != nil {
		return result, err
	}
	decision, err := service.admitReportDeliveryDecisionV1(
		ctx, input, lease, stageRequest, stageReceipt, plan, committed.Settlement.RecordDigest, commit,
	)
	if err != nil {
		return result, err
	}
	succeeded = true
	result.Candidate = plan.receipt
	result.Index = plan.index
	result.Commit = commit
	result.Decision = decision
	return result, nil
}

func (service *Service) validateOrdinaryReportSurface(ctx context.Context, input PublishInput) error {
	if input.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull {
		return nil
	}
	if input.PIIProjection.ProjectionClass != domainpublication.PIIProjectionOrdinaryMasked ||
		service == nil || service.surfaceExtractor == nil || ctx == nil {
		return errors.New("ordinary report privacy inspection is unavailable")
	}
	surface, err := service.surfaceExtractor.ExtractCompleteCanonicalSurface(
		ctx, input.RenderInspection.MediaType, bytes.Clone(input.ReportBytes),
	)
	canonicalText := string(surface.CanonicalText)
	filteredText, reasoningErr := domainevent.FilterPublicText(canonicalText)
	if err != nil || strings.TrimSpace(surface.ExtractorID) == "" || strings.TrimSpace(surface.ExtractorVersion) == "" ||
		len(surface.CanonicalText) == 0 || len(surface.CanonicalText) > maxOrdinaryReportCanonicalTextBytesV1 ||
		!utf8.Valid(surface.CanonicalText) || reasoningErr != nil || filteredText != canonicalText ||
		domainrestrictedevidence.ValidateCanonicalText(canonicalText) != nil ||
		domainprivacy.ValidateOrdinaryText(canonicalText) != nil {
		return errors.New("ordinary report contains unverified or restricted content")
	}
	return nil
}

func (service *Service) validateControlledReportBytes(
	input PublishInput,
) (domainpii.ControlledPIIArtifactMetadataV1, error) {
	if input.PIIProjection.ProjectionClass != domainpublication.PIIProjectionControlledFull {
		return domainpii.ControlledPIIArtifactMetadataV1{}, nil
	}
	if input.RenderInspection.MediaType != domainpii.ControlledPIIArtifactMediaTypeV1 {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("controlled report media type is invalid")
	}
	if input.PIIProjection.RestrictedFieldCount != input.PIIProjection.PreservedControlledFieldCount {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("controlled report field count is invalid")
	}
	metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(input.ReportBytes)
	if err != nil || domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
		metadata, input.PendingToolCall.SecurityContext, input.ClaimLedger.LedgerDigest,
		input.PIIProjection.RulesetHash, input.TargetIdentityDigest,
		input.PIIProjection.ProjectedContentSHA256, uint64(len(input.ReportBytes)),
		input.PIIProjection.PreservedControlledFieldCount,
	) != nil {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("controlled report artifact binding is invalid")
	}
	return metadata, nil
}

func (service *Service) validateStoredControlledReportArtifact(
	ctx context.Context,
	input PublishInput,
	expected domainpii.ControlledPIIArtifactMetadataV1,
) (domainpii.ControlledPIIArtifactMetadataV1, error) {
	if input.PIIProjection.ProjectionClass != domainpublication.PIIProjectionControlledFull {
		return domainpii.ControlledPIIArtifactMetadataV1{}, nil
	}
	store, ok := service.artifacts.(publicationport.ControlledArtifactMetadataStore)
	if !ok {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("controlled report artifact metadata authority is unavailable")
	}
	stored, err := store.ResolveControlledMetadata(ctx, input.TargetIdentityDigest)
	if err != nil || !reflect.DeepEqual(stored, expected) {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("controlled report artifact metadata readback is invalid")
	}
	if err := domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
		stored, input.PendingToolCall.SecurityContext, input.ClaimLedger.LedgerDigest,
		input.PIIProjection.RulesetHash, input.TargetIdentityDigest,
		input.PIIProjection.ProjectedContentSHA256, uint64(len(input.ReportBytes)),
		input.PIIProjection.PreservedControlledFieldCount,
	); err != nil {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("controlled report artifact metadata binding changed")
	}
	return stored, nil
}

func (service *Service) validateControlledArtifactAuthorization(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	metadata domainpii.ControlledPIIArtifactMetadataV1,
) error {
	authority, ok := service.piiAuthority.(publicationport.ControlledArtifactPIIAuthorizationAuthority)
	if !ok {
		return errors.New("controlled report PII authorization authority is incomplete")
	}
	return authority.ValidateControlledArtifactCurrent(ctx, securityContext, projection, metadata)
}

func clearControlledPublicationBytesV1(body []byte) {
	for index := range body {
		body[index] = 0
	}
	runtime.KeepAlive(body)
}

type publicationPlanV1 struct {
	receipt    domainpublication.PublicationReceiptV1
	index      domainpublication.PublicationIndexV1
	nextBundle domainevidence.EvidenceAuthorityBundleV1
	intent     domainauthorityadvance.MonotonicAdvanceIntentV2
	attempt    domainpublication.PublicationAttemptV1
}

func (service *Service) preparePublicationPlan(
	ctx context.Context,
	input PublishInput,
	stageRequest pendingworkapp.ReportStageRequest,
	stageReceipt domainpendingwork.PendingWorkReceiptV1,
	publishSnapshot registryport.WitnessedSnapshot,
) (publicationPlanV1, error) {
	if service == nil || ctx == nil || !publishSnapshot.Head.HasBundle ||
		domainpendingwork.ValidatePendingWorkReceiptV1(stageReceipt) != nil {
		return publicationPlanV1{}, ErrPublicationIntegrity
	}
	securityContext := input.PendingToolCall.SecurityContext
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID, Context: securityContext,
		ReportVariant: input.ReportVariant, EvidenceAuthorityBundleDigest: publishSnapshot.Head.Bundle.RecordDigest,
		EvidenceRegistryIndexDigest: publishSnapshot.Head.Bundle.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       publishSnapshot.Head.Bundle.EvidenceRegistryCount,
		EvidenceRegistrySequence:    publishSnapshot.Registry.Sequence, EvidenceRegistryStateDigest: publishSnapshot.Registry.StateDigest,
		ClaimLedger: input.ClaimLedger, ReportSHA256: input.RenderInspection.ReportSHA256,
		ReportByteLength: uint64(len(input.ReportBytes)), MediaType: input.RenderInspection.MediaType,
		PIIProjection: input.PIIProjection, RenderInspection: input.RenderInspection,
		Publisher: input.Publisher, PublisherVersion: input.PublisherVersion,
		TargetIdentityDigest: input.TargetIdentityDigest, IssuedAt: input.IssuedAt,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: service.publicKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return publicationPlanV1{}, err
	}
	if err := service.validatePublicationHead(ctx, publishSnapshot.Head, receipt); err != nil {
		return publicationPlanV1{}, err
	}
	attemptID := domainpublication.PublicationAttemptIDV1(service.installationID, service.enrollmentID, stageReceipt.WorkID)
	index, err := domainpublication.NewPublicationIndexV1(domainpublication.PublicationIndexInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		Generation:          publishSnapshot.Head.Bundle.PublicationCount + 1,
		PreviousIndexDigest: publishSnapshot.Head.Bundle.PublicationIndexDigest,
		MutationID:          domainpublication.PublicationIndexMutationIDForAttemptV1(attemptID),
		ReceiptID:           receipt.ReceiptID, ReceiptRecordDigest: receipt.RecordDigest, TargetIdentityDigest: receipt.TargetIdentityDigest,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: service.publicKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return publicationPlanV1{}, err
	}
	authorityMutationID := domainpublication.PublicationAuthorityMutationIDForAttemptV1(attemptID)
	previous := publishSnapshot.Head.Bundle
	nextBundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                 authorityMutationID,
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount,
		PublicationIndexDigest: index.IndexDigest, PublicationCount: previous.PublicationCount + 1,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: service.publicKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return publicationPlanV1{}, err
	}
	checkpoint := publishSnapshot.Head.Observation.Checkpoint
	if domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(previous, checkpoint) != nil {
		return publicationPlanV1{}, ErrPublicationIntegrity
	}
	advanceRequest, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		Namespace: checkpoint.Namespace, ExpectedGeneration: checkpoint.Generation,
		ExpectedCheckpointDigest: checkpoint.CheckpointDigest, ExpectedStateDigest: checkpoint.CurrentStateDigest,
		NextGeneration: checkpoint.Generation + 1, NextStateDigest: nextBundle.RecordDigest,
		ExpectedFenceNonce: checkpoint.FenceNonce, MutationID: authorityMutationID,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: service.publicKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return publicationPlanV1{}, err
	}
	root, transition, err := domainauthorityadvance.NewEvidenceTransitionBindingV2(previous, nextBundle)
	if err != nil || root != domainauthorityadvance.AdvanceRootPublicationV2 {
		return publicationPlanV1{}, ErrPublicationIntegrity
	}
	intent, err := domainauthorityadvance.NewMonotonicAdvanceIntentV2(domainauthorityadvance.MonotonicAdvanceIntentInputV2{
		Root: root, PreviousCheckpoint: checkpoint, AdvanceRequest: advanceRequest, Transition: transition,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: service.publicKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return publicationPlanV1{}, err
	}
	attempt, err := domainpublication.NewPublicationAttemptV1(domainpublication.PublicationAttemptInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		ReportStageReceipt: stageReceipt, ToolCallID: input.PendingToolCall.ExecutionGrant.ToolCallID,
		StageInputHash: stageRequest.StageInputHash, Candidate: receipt, Index: index,
		ExpectedEvidenceBundleDigest:   previous.RecordDigest,
		ExpectedPublicationIndexDigest: previous.PublicationIndexDigest, ExpectedPublicationCount: previous.PublicationCount,
		NextEvidenceBundleDigest: nextBundle.RecordDigest, AuthorityAdvanceIntentDigest: intent.RecordDigest,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: service.publicKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return publicationPlanV1{}, err
	}
	return publicationPlanV1{receipt: receipt, index: index, nextBundle: nextBundle, intent: intent, attempt: attempt}, nil
}

func (service *Service) validateInput(input PublishInput) (pendingworkapp.ReportStageRequest, error) {
	pending := input.PendingToolCall
	securityContext := pending.SecurityContext
	grant := pending.ExecutionGrant
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil ||
		!domainmodel.IsHostToolCallIDV1(pending.Call.ID) ||
		!domainmodel.IsHostToolCallIDV1(grant.ToolCallID) ||
		grant.ToolName != pendingworkapp.ReportStageToolName || grant.ToolCallID != strings.TrimSpace(pending.Call.ID) ||
		grant.ToolName != strings.TrimSpace(pending.Call.Name) || grant.ReadOnly || grant.ApprovalState != "approved" ||
		pending.ThreadID != securityContext.ThreadID || pending.TurnID != securityContext.TurnID ||
		domainpublication.ValidateClaimLedgerV1(input.ClaimLedger) != nil ||
		domainpublication.ValidatePIIProjectionV1(input.PIIProjection) != nil ||
		domainpublication.ValidateRenderInspectionV1(input.RenderInspection) != nil ||
		input.ClaimLedger.ContextDigest != securityContext.ContextDigest ||
		len(input.ReportBytes) == 0 || domainsecurity.SHA256Hex(input.ReportBytes) != input.RenderInspection.ReportSHA256 ||
		uint64(len(input.ReportBytes)) != input.RenderInspection.ReportByteLength ||
		input.PIIProjection.ProjectedContentSHA256 != input.RenderInspection.ReportSHA256 ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.TargetIdentityDigest)) || strings.TrimSpace(input.Publisher) == "" ||
		strings.TrimSpace(input.PublisherVersion) == "" || input.IssuedAt.UTC().IsZero() {
		return pendingworkapp.ReportStageRequest{}, errors.New("report publication input is invalid")
	}
	stageHash := publicationStageInputHash(input)
	return pendingworkapp.ReportStageRequest{PendingToolCall: pending, StageInputHash: stageHash, IssuedAt: input.IssuedAt.UTC()}, nil
}

func (service *Service) validateLedgerSnapshot(ctx context.Context, snapshot registryport.WitnessedSnapshot, variant string, ledger domainpublication.ClaimLedgerV1, allowControlled bool) error {
	return validateLedgerSnapshotV1(ctx, snapshot, variant, ledger, allowControlled)
}

func validateLedgerSnapshotV1(ctx context.Context, snapshot registryport.WitnessedSnapshot, variant string, ledger domainpublication.ClaimLedgerV1, allowControlled bool) error {
	if snapshot.Context.ContextDigest != ledger.ContextDigest || snapshot.Registry.ContextDigest != ledger.ContextDigest ||
		domainpublication.ValidateClaimLedgerV1(ledger) != nil {
		return ErrPublicationIntegrity
	}
	registry := snapshotRegistry{snapshot: snapshot.Registry}
	used := map[string]bool{}
	for _, claim := range ledger.Claims {
		if err := evidenceapp.VerifyClaimRecordForPublication(ctx, registry, snapshot.Context, claim, allowControlled); err != nil {
			return err
		}
		for _, receiptID := range claim.EvidenceIDs {
			used[receiptID] = true
		}
		for _, receiptID := range claim.CounterEvidenceIDs {
			used[receiptID] = true
		}
	}
	for _, receiptID := range ledger.EvidenceReceiptIDs {
		registered, err := registry.Resolve(ctx, registryport.MembershipQuery{Context: snapshot.Context, ReceiptID: receiptID})
		if err != nil || registered.Revoked {
			return errors.New("claim ledger evidence receipt is not current membership")
		}
		if variant == domainpublication.VerifiedNoHitReport {
			material, materialErr := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
			if materialErr != nil || registered.Receipt.PaginationCompleteness != domainevidence.PaginationComplete ||
				len(registered.Receipt.SourceRecordIDs) != 0 || len(material.Facts) != 0 {
				return errors.New("verified no-hit report receipt is incomplete")
			}
		}
	}
	if variant != domainpublication.VerifiedNoHitReport {
		if len(used) != len(ledger.EvidenceReceiptIDs) {
			return errors.New("claim ledger contains evidence not bound to a claim")
		}
		for _, receiptID := range ledger.EvidenceReceiptIDs {
			if !used[receiptID] {
				return errors.New("claim ledger evidence set exceeds claim authority")
			}
		}
	}
	return nil
}

func (service *Service) persistMaterials(ctx context.Context, input PublishInput) error {
	if err := service.ledgers.PutIfAbsent(ctx, input.ClaimLedger); err != nil {
		return err
	}
	if stored, err := service.ledgers.Resolve(ctx, input.ClaimLedger.LedgerDigest); err != nil || !reflect.DeepEqual(stored, input.ClaimLedger) {
		return ErrPublicationIntegrity
	}
	if err := service.piiProjections.PutIfAbsent(ctx, input.PIIProjection); err != nil {
		return err
	}
	if stored, err := service.piiProjections.Resolve(ctx, input.PIIProjection.ProjectionDigest); err != nil || !reflect.DeepEqual(stored, input.PIIProjection) {
		return ErrPublicationIntegrity
	}
	if err := service.inspections.PutIfAbsent(ctx, input.RenderInspection); err != nil {
		return err
	}
	if stored, err := service.inspections.Resolve(ctx, input.RenderInspection.InspectionDigest); err != nil || !reflect.DeepEqual(stored, input.RenderInspection) {
		return ErrPublicationIntegrity
	}
	return nil
}

func (service *Service) validatePublicationHead(ctx context.Context, head evidenceauthorityport.FreshHead, nextReceipt domainpublication.PublicationReceiptV1) error {
	if service.validateWitnessedHead(head) != nil {
		return ErrPublicationIntegrity
	}
	root, count := head.Bundle.PublicationIndexDigest, head.Bundle.PublicationCount
	if count == 0 {
		if root != domainpublication.PublicationIndexGenesisDigestV1() {
			return ErrPublicationIntegrity
		}
		return nil
	}
	if count > maxPublicationIndexDepthV1 || root == domainpublication.PublicationIndexGenesisDigestV1() {
		return ErrPublicationIntegrity
	}
	currentDigest := root
	var newer *domainpublication.PublicationIndexV1
	for generation := count; generation > 0; generation-- {
		index, err := service.indexes.Resolve(ctx, currentDigest)
		if err != nil || domainpublication.ValidatePublicationIndexForInstallationV1(
			index, service.installationID, service.enrollmentID, service.keyID, service.publicKey,
		) != nil || index.Generation != generation || index.IndexDigest != currentDigest {
			return ErrPublicationIntegrity
		}
		if generation == count && domainpublication.ValidatePublicationIndexWitnessRootV1(index, root, count) != nil {
			return ErrPublicationIntegrity
		}
		if newer != nil && domainpublication.ValidatePublicationIndexTransitionV1(index, *newer) != nil {
			return ErrPublicationIntegrity
		}
		if index.ReceiptID == nextReceipt.ReceiptID || index.ReceiptRecordDigest == nextReceipt.RecordDigest ||
			index.TargetIdentityDigest == nextReceipt.TargetIdentityDigest {
			return errors.New("publication receipt or target was already committed")
		}
		if generation == 1 {
			if index.PreviousIndexDigest != domainpublication.PublicationIndexGenesisDigestV1() {
				return ErrPublicationIntegrity
			}
			break
		}
		newer = &index
		currentDigest = index.PreviousIndexDigest
	}
	return nil
}

func (service *Service) validateCommittedPublication(
	ctx context.Context,
	previous, advanced evidenceauthorityport.FreshHead,
	index domainpublication.PublicationIndexV1,
	receipt domainpublication.PublicationReceiptV1,
	input PublishInput,
) error {
	if service.validateWitnessedHead(previous) != nil || service.validateWitnessedHead(advanced) != nil ||
		advanced.Bundle.PublicationIndexDigest != index.IndexDigest ||
		advanced.Bundle.PublicationCount != previous.Bundle.PublicationCount+1 ||
		advanced.Bundle.DatasetSnapshotIndexDigest != previous.Bundle.DatasetSnapshotIndexDigest ||
		advanced.Bundle.DatasetSnapshotCount != previous.Bundle.DatasetSnapshotCount ||
		advanced.Bundle.EvidenceRegistryIndexDigest != previous.Bundle.EvidenceRegistryIndexDigest ||
		advanced.Bundle.EvidenceRegistryCount != previous.Bundle.EvidenceRegistryCount {
		return ErrPublicationIntegrity
	}
	selected, err := service.indexes.Resolve(ctx, advanced.Bundle.PublicationIndexDigest)
	if err != nil || !reflect.DeepEqual(selected, index) ||
		domainpublication.ValidatePublicationIndexWitnessRootV1(selected, advanced.Bundle.PublicationIndexDigest, advanced.Bundle.PublicationCount) != nil ||
		domainpublication.ValidatePublicationIndexReceiptV1(selected, receipt) != nil {
		return ErrPublicationIntegrity
	}
	storedReceipt, err := service.receipts.Resolve(ctx, receipt.RecordDigest)
	if err != nil || !reflect.DeepEqual(storedReceipt, receipt) ||
		domainpublication.ValidatePublicationReceiptMaterialsV1(receipt, input.ClaimLedger, input.PIIProjection, input.RenderInspection) != nil {
		return ErrPublicationIntegrity
	}
	artifact, err := service.artifacts.ResolveExact(ctx, receipt.TargetIdentityDigest)
	if err != nil || uint64(len(artifact)) != receipt.ReportByteLength || domainsecurity.SHA256Hex(artifact) != receipt.ReportSHA256 {
		return ErrPublicationIntegrity
	}
	return nil
}

func (service *Service) validateWitnessedHead(head evidenceauthorityport.FreshHead) error {
	if !head.HasBundle {
		return ErrPublicationIntegrity
	}
	if _, err := domainevidence.NewEvidenceAuthorityWitnessBindingV1(
		head.Bundle, head.Request, head.Observation,
		service.installationID, service.enrollmentID, service.keyID, service.publicKey,
		service.witnessKeyID, service.witnessKey,
	); err != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func persistPublicationObservationExactV1(
	ctx context.Context,
	store evidenceauthorityport.ObservationStore,
	head evidenceauthorityport.FreshHead,
) error {
	if ctx == nil || store == nil || !head.HasBundle || !domainsecurity.IsSHA256Hex(head.Observation.ObservationDigest) {
		return ErrPublicationIntegrity
	}
	bundle := evidenceauthorityport.ObservationBundle{
		Bundle: head.Bundle, Request: head.Request, Observation: head.Observation,
	}
	if err := store.PutIfAbsent(ctx, bundle); err != nil {
		return err
	}
	stored, err := store.Resolve(ctx, head.Observation.ObservationDigest)
	if err != nil || !reflect.DeepEqual(stored, bundle) {
		return ErrPublicationIntegrity
	}
	return nil
}

func publicationStageInputHash(input PublishInput) string {
	body, _ := json.Marshal(struct {
		ContextDigest       string `json:"contextDigest"`
		GrantID             string `json:"grantId"`
		ReportVariant       string `json:"reportVariant"`
		ClaimLedgerDigest   string `json:"claimLedgerDigest"`
		PIIProjectionDigest string `json:"piiProjectionDigest"`
		InspectionDigest    string `json:"inspectionDigest"`
		ReportSHA256        string `json:"reportSha256"`
		TargetDigest        string `json:"targetDigest"`
	}{
		ContextDigest: input.PendingToolCall.SecurityContext.ContextDigest, GrantID: input.PendingToolCall.ExecutionGrant.GrantID,
		ReportVariant: input.ReportVariant, ClaimLedgerDigest: input.ClaimLedger.LedgerDigest,
		PIIProjectionDigest: input.PIIProjection.ProjectionDigest, InspectionDigest: input.RenderInspection.InspectionDigest,
		ReportSHA256: input.RenderInspection.ReportSHA256, TargetDigest: input.TargetIdentityDigest,
	})
	return domainsecurity.SHA256Hex(body)
}

type snapshotRegistry struct {
	snapshot domainevidence.EvidenceReceiptRegistry
}

func (registry snapshotRegistry) Resolve(_ context.Context, query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	return domainevidence.VerifyEvidenceReceiptMembership(registry.snapshot, query.Context, query.ReceiptID)
}
func (snapshotRegistry) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	return domainevidence.EvidenceReceipt{}, errors.New("publication snapshot registry is read-only")
}
func (snapshotRegistry) Revoke(context.Context, registryport.RevokeInput) error {
	return errors.New("publication snapshot registry is read-only")
}
func (registry snapshotRegistry) Replay(context.Context, domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	copy, err := domainevidence.ParseEvidenceReceiptRegistry(registry.snapshot)
	return copy, err
}

func sortedPublicationEvidenceIDs(claims []domainevidence.ClaimRecord) []string {
	seen := map[string]bool{}
	for _, claim := range claims {
		for _, id := range append(append([]string(nil), claim.EvidenceIDs...), claim.CounterEvidenceIDs...) {
			seen[id] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
