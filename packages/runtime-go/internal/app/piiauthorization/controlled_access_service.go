package piiauthorization

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

const controlledAccessSettlementTimeoutV1 = 5 * time.Second

var (
	ErrControlledAccessUnavailable   = errors.New("controlled PII artifact access is unavailable")
	ErrControlledAccessInvalid       = errors.New("controlled PII artifact access request is invalid")
	ErrControlledAccessStale         = errors.New("controlled PII artifact access context is stale")
	ErrControlledAccessRejected      = errors.New("controlled PII artifact access is rejected")
	ErrControlledAccessDuplicate     = errors.New("controlled PII artifact access slot is already used")
	ErrControlledAccessCancelled     = errors.New("controlled PII artifact access is cancelled")
	ErrControlledAccessIndeterminate = errors.New("controlled PII artifact release is indeterminate")
	ErrControlledAccessIntegrity     = errors.New("controlled PII artifact access integrity failure")
)

type ControlledAccessAcquireEffectV1 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) (context.Context, func(), error)

type controlledAccessJournalV1 interface {
	piiauthorizationport.AccessStore
	piiauthorizationport.AccessInventoryStore
}

type ControlledAccessConfigV1 struct {
	Authority        finalauthorityport.Authority
	Access           controlledAccessJournalV1
	Admissions       piiauthorizationport.ControlledAccessAdmissionAuthority
	Sink             piiauthorizationport.ControlledReleaseSink
	Grants           piiauthorizationport.Store
	Receipts         publicationport.ReceiptStore
	Commits          publicationport.CommitReceiptStore
	Indexes          publicationport.IndexStore
	Ledgers          publicationport.ClaimLedgerStore
	Projections      publicationport.PIIProjectionStore
	Inspections      publicationport.RenderInspectionStore
	Artifacts        publicationport.ArtifactStore
	ArtifactMetadata publicationport.ControlledArtifactMetadataStore
	PIIAuthority     publicationport.PIIAuthorizationAuthority
	ValidateCurrent  CurrentContextValidator
	AcquireEffect    ControlledAccessAcquireEffectV1
	Now              func() time.Time
}

type ControlledAccessServiceV1 struct {
	config ControlledAccessConfigV1
}

// ControlledAccessInputV1 contains ephemeral opaque tokens only. The app
// hashes them after a host authority validates the exact current desktop
// principal and never places the raw tokens in a receipt or result.
type ControlledAccessInputV1 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	AccessAction       string
	ControlledHandle   string
	UseSlot            string
	RendererPrincipal  string
	RendererGeneration uint64
	BackendGeneration  uint64
}

// ControlledAccessResultV1 is safe audit metadata. Complete account/card
// values exist only in the protected artifact and inside the trusted sink call.
type ControlledAccessResultV1 struct {
	AccessID            string
	AccessReceiptDigest string
	DispositionDigest   string
	Status              string
	ReleasedByteLength  uint64
	ArtifactSHA256      string
	ArtifactByteLength  uint64
	MediaType           string
}

type controlledAccessMaterialsV1 struct {
	grant      domainpii.PIIProjectionGrantV1
	receipt    domainpublication.PublicationReceiptV1
	commit     domainpublication.PublicationCommitReceiptV1
	projection domainpublication.PIIProjectionV1
	body       []byte
}

func NewControlledAccessServiceV1(config ControlledAccessConfigV1) (*ControlledAccessServiceV1, error) {
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.Authority == nil || config.Access == nil || config.Admissions == nil || config.Sink == nil ||
		config.Grants == nil || config.Receipts == nil || config.Commits == nil || config.Indexes == nil ||
		config.Ledgers == nil || config.Projections == nil || config.Inspections == nil || config.Artifacts == nil ||
		config.ArtifactMetadata == nil || config.PIIAuthority == nil || config.ValidateCurrent == nil ||
		config.AcquireEffect == nil || config.Now == nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.Authority.KeyID())) || len(config.Authority.PublicKey()) == 0 {
		return nil, ErrControlledAccessUnavailable
	}
	return &ControlledAccessServiceV1{config: config}, nil
}

// Release validates the current case, renderer principal, publication commit,
// PII grant, artifact bytes, and one-use slot under one effect lease. It
// reserves and reads back a signed access receipt before the trusted sink sees
// any bytes, then durably terminalizes every attempted release.
func (service *ControlledAccessServiceV1) Release(
	ctx context.Context,
	input ControlledAccessInputV1,
) (ControlledAccessResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessInvalid
	}
	if err := service.config.ValidateCurrent(ctx, input.SecurityContext); err != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessStale
	}
	before, err := service.resolveAdmission(ctx, input, service.config.Now().UTC())
	if err != nil {
		return ControlledAccessResultV1{}, err
	}
	effectCtx, releaseEffect, err := service.config.AcquireEffect(ctx, input.SecurityContext)
	if err != nil || effectCtx == nil || releaseEffect == nil {
		if releaseEffect != nil {
			releaseEffect()
		}
		return ControlledAccessResultV1{}, ErrControlledAccessStale
	}
	defer releaseEffect()

	requestedAt := service.config.Now().UTC()
	if requestedAt.IsZero() || !requestedAt.Before(before.AuthorizedUntil) ||
		service.config.ValidateCurrent(effectCtx, input.SecurityContext) != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessStale
	}
	current, err := service.resolveAdmission(effectCtx, input, requestedAt)
	if err != nil || !sameControlledAccessAdmissionV1(before, current) {
		return ControlledAccessResultV1{}, ErrControlledAccessStale
	}

	materials, err := service.resolveMaterials(effectCtx, input.SecurityContext, current)
	if err != nil {
		return ControlledAccessResultV1{}, err
	}
	defer clearControlledArtifactBytesV1(materials.body)
	receipt, err := service.newAccessReceipt(effectCtx, input.SecurityContext, current, materials, requestedAt)
	if err != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessRejected
	}
	if err := verifyControlledAccessPublicationGraphV1(effectCtx, service.inventoryDependencies(), receipt); err != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessRejected
	}
	if err := service.config.Access.ReserveAccessReceipt(effectCtx, receipt); err != nil {
		switch {
		case errors.Is(err, piiauthorizationport.ErrAlreadyReserved), errors.Is(err, piiauthorizationport.ErrConflict):
			return ControlledAccessResultV1{}, ErrControlledAccessDuplicate
		default:
			return ControlledAccessResultV1{}, ErrControlledAccessIntegrity
		}
	}
	readbackCtx, cancelReadback := context.WithTimeout(context.WithoutCancel(effectCtx), controlledAccessSettlementTimeoutV1)
	stored, err := service.config.Access.ResolveAccessReceipt(readbackCtx, receipt.AccessID)
	if err != nil || !reflect.DeepEqual(stored, receipt) || verifyControlledAccessReceiptAuthorityV1(readbackCtx, service.config.Authority, stored) != nil {
		cancelReadback()
		return ControlledAccessResultV1{}, ErrControlledAccessIntegrity
	}
	if _, err := service.config.Access.ResolveAccessDisposition(readbackCtx, receipt.AccessID); err == nil {
		cancelReadback()
		return ControlledAccessResultV1{}, ErrControlledAccessDuplicate
	} else if !errors.Is(err, piiauthorizationport.ErrNotFound) {
		cancelReadback()
		return ControlledAccessResultV1{}, ErrControlledAccessIntegrity
	}
	cancelReadback()

	if effectCtx.Err() != nil {
		return service.closeBeforeRelease(effectCtx, stored,
			domainpii.ControlledArtifactAccessDispositionCancelledV1,
			domainpii.ControlledArtifactAccessReasonAccessCancelledV1,
			ErrControlledAccessCancelled,
		)
	}
	if err := service.revalidateBeforeRelease(effectCtx, input, current, stored, materials); err != nil {
		status := domainpii.ControlledArtifactAccessDispositionRejectedV1
		reason := domainpii.ControlledArtifactAccessReasonAccessRejectedV1
		failure := ErrControlledAccessRejected
		if errors.Is(err, ErrControlledAccessStale) {
			status = domainpii.ControlledArtifactAccessDispositionStaleContextV1
			reason = domainpii.ControlledArtifactAccessReasonStaleContextV1
			failure = ErrControlledAccessStale
		}
		return service.closeBeforeRelease(effectCtx, stored, status, reason, failure)
	}

	releaseCtx, cancel := context.WithDeadline(effectCtx, current.AuthorizedUntil)
	releaseResult, releaseErr := service.config.Sink.Release(releaseCtx, piiauthorizationport.ControlledReleaseRequestV1{
		Receipt: stored, Body: bytes.NewReader(materials.body), ArtifactByteLength: uint64(len(materials.body)),
	})
	cancel()

	settlementCtx, settlementCancel := context.WithTimeout(context.WithoutCancel(effectCtx), controlledAccessSettlementTimeoutV1)
	defer settlementCancel()
	postNow := service.config.Now().UTC()
	postValid := service.revalidateAfterRelease(settlementCtx, input, current, stored, materials) == nil
	committed := releaseErr == nil && releaseResult.Committed &&
		releaseResult.ReleasedByteLength == stored.ArtifactByteLength && postValid &&
		!postNow.IsZero() && postNow.Before(current.AuthorizedUntil)
	status := domainpii.ControlledArtifactAccessDispositionHostReleaseCommittedV1
	reason := domainpii.ControlledArtifactAccessReasonHostReleaseCommittedV1
	released := stored.ArtifactByteLength
	resultErr := error(nil)
	if !committed {
		status = domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1
		reason = domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1
		// Once the sink was called, even a reported zero-byte or failed result
		// cannot prove that no complete PII escaped. Record the full artifact as
		// the conservative exposure upper bound.
		released = stored.ArtifactByteLength
		resultErr = ErrControlledAccessIndeterminate
	}
	disposition, err := service.persistDisposition(settlementCtx, stored, status, reason, released, postNow)
	if err != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessIntegrity
	}
	result := controlledAccessResultV1(stored, disposition)
	return result, resultErr
}

func (service *ControlledAccessServiceV1) resolveAdmission(
	ctx context.Context,
	input ControlledAccessInputV1,
	now time.Time,
) (piiauthorizationport.ControlledAccessAdmissionV1, error) {
	handleDigest, handleErr := domainpii.ControlledAccessHandleDigestV1(input.ControlledHandle)
	useSlotDigest, slotErr := domainpii.ControlledAccessUseSlotDigestV1(input.UseSlot)
	principalDigest, principalErr := domainpii.ControlledAccessRendererPrincipalDigestV1(input.RendererPrincipal)
	if handleErr != nil || slotErr != nil || principalErr != nil || input.RendererGeneration == 0 || input.BackendGeneration == 0 ||
		(input.AccessAction != domainpii.ControlledArtifactAccessActionDisplayV1 &&
			input.AccessAction != domainpii.ControlledArtifactAccessActionExportV1) {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrControlledAccessInvalid
	}
	request := piiauthorizationport.ControlledAccessAdmissionRequestV1{
		SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
		ControlledHandle: input.ControlledHandle, UseSlot: input.UseSlot, RendererPrincipal: input.RendererPrincipal,
		RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
	}
	admission, err := service.config.Admissions.ResolveCurrent(ctx, request)
	admission.AuthorizedUntil = admission.AuthorizedUntil.UTC()
	if err != nil || admission.SecurityContext != input.SecurityContext || admission.AccessAction != input.AccessAction ||
		admission.ControlledHandleDigest != handleDigest || admission.UseSlotDigest != useSlotDigest ||
		admission.RendererPrincipalDigest != principalDigest || admission.RendererGeneration != input.RendererGeneration ||
		admission.BackendGeneration != input.BackendGeneration ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(admission.PublicationCommitDigest)) || now.IsZero() ||
		admission.AuthorizedUntil.IsZero() || !now.Before(admission.AuthorizedUntil) ||
		admission.AuthorizedUntil.Sub(now) > domainpii.PIIProjectionGrantMaxTTL {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrControlledAccessStale
	}
	return admission, nil
}

func (service *ControlledAccessServiceV1) resolveMaterials(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	admission piiauthorizationport.ControlledAccessAdmissionV1,
) (controlledAccessMaterialsV1, error) {
	commit, err := service.config.Commits.Resolve(ctx, admission.PublicationCommitDigest)
	if err != nil || commit.RecordDigest != admission.PublicationCommitDigest ||
		verifyPublicationCommitAuthorityV1(ctx, service.config.Authority, commit) != nil {
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	receipt, err := service.config.Receipts.Resolve(ctx, commit.CandidateRecordDigest)
	if err != nil || verifyPublicationReceiptAuthorityV1(ctx, service.config.Authority, receipt) != nil {
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	index, err := service.config.Indexes.Resolve(ctx, commit.PublicationIndexDigest)
	if err != nil || verifyPublicationIndexAuthorityV1(ctx, service.config.Authority, index) != nil ||
		domainpublication.ValidatePublicationCommitReceiptMaterialsV1(commit, receipt, index) != nil {
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	projection, err := service.config.Projections.Resolve(ctx, receipt.PIIProjectionDigest)
	if err != nil || projection.ProjectionClass != domainpublication.PIIProjectionControlledFull ||
		projection.AuthorizationAuditDigest != receipt.AuthorizationAuditDigest {
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	grant, err := service.config.Grants.ResolveGrant(ctx, receipt.AuthorizationAuditDigest)
	if err != nil || verifyPIIGrantAuthorityV1(ctx, service.config.Authority, grant) != nil {
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	body, err := service.config.Artifacts.ResolveExact(ctx, receipt.TargetIdentityDigest)
	if err != nil || len(body) == 0 || uint64(len(body)) != receipt.ReportByteLength ||
		domainsecurity.SHA256Hex(body) != receipt.ReportSHA256 || receipt.MediaType != domainpii.ControlledPIIArtifactMediaTypeV1 {
		clearControlledArtifactBytesV1(body)
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	parsedMetadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
	storedMetadata, storedErr := service.config.ArtifactMetadata.ResolveControlledMetadata(ctx, receipt.TargetIdentityDigest)
	if err != nil || storedErr != nil || !reflect.DeepEqual(parsedMetadata, storedMetadata) ||
		parsedMetadata.Context.ContextDigest != securityContext.ContextDigest ||
		parsedMetadata.TargetIdentityDigest != receipt.TargetIdentityDigest ||
		parsedMetadata.SHA256 != receipt.ReportSHA256 || parsedMetadata.ByteLength != receipt.ReportByteLength {
		clearControlledArtifactBytesV1(body)
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	if err := service.config.PIIAuthority.ValidateCurrent(ctx, securityContext, projection, receipt.TargetIdentityDigest); err != nil {
		clearControlledArtifactBytesV1(body)
		return controlledAccessMaterialsV1{}, ErrControlledAccessRejected
	}
	return controlledAccessMaterialsV1{grant: grant, receipt: receipt, commit: commit, projection: projection, body: body}, nil
}

func (service *ControlledAccessServiceV1) newAccessReceipt(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	admission piiauthorizationport.ControlledAccessAdmissionV1,
	materials controlledAccessMaterialsV1,
	requestedAt time.Time,
) (domainpii.ControlledArtifactAccessReceiptV1, error) {
	return domainpii.NewControlledArtifactAccessReceiptV1(domainpii.ControlledArtifactAccessReceiptInputV1{
		SecurityContext: securityContext, AccessAction: admission.AccessAction,
		ControlledHandleDigest: admission.ControlledHandleDigest, UseSlotDigest: admission.UseSlotDigest,
		RendererPrincipalDigest: admission.RendererPrincipalDigest, RendererGeneration: admission.RendererGeneration,
		BackendGeneration: admission.BackendGeneration, AccessPolicyDigest: materials.grant.AccessPolicyDigest,
		RetentionPolicyDigest:   materials.grant.RetentionPolicyDigest,
		PublicationCommitDigest: materials.commit.RecordDigest, PublicationReceiptDigest: materials.receipt.RecordDigest,
		PIIProjectionDigest: materials.projection.ProjectionDigest, PIIAuthorizationDigest: materials.grant.RecordDigest,
		ClaimLedgerDigest: materials.receipt.ClaimLedgerDigest, TargetIdentityDigest: materials.receipt.TargetIdentityDigest,
		ArtifactSHA256: materials.receipt.ReportSHA256, ArtifactByteLength: materials.receipt.ReportByteLength,
		MediaType: materials.receipt.MediaType, RequestedAt: requestedAt, AuthorizedUntil: admission.AuthorizedUntil,
		AuthorityKeyID: service.config.Authority.KeyID(), AuthorityPublicKey: service.config.Authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.config.Authority.Sign(ctx, message) })
}

func (service *ControlledAccessServiceV1) revalidateBeforeRelease(
	ctx context.Context,
	input ControlledAccessInputV1,
	admission piiauthorizationport.ControlledAccessAdmissionV1,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
	materials controlledAccessMaterialsV1,
) error {
	if ctx.Err() != nil || service.config.ValidateCurrent(ctx, input.SecurityContext) != nil {
		return ErrControlledAccessStale
	}
	current, err := service.resolveAdmission(ctx, input, service.config.Now().UTC())
	if err != nil || !sameControlledAccessAdmissionV1(admission, current) {
		return ErrControlledAccessStale
	}
	if verifyControlledAccessPublicationGraphV1(ctx, service.inventoryDependencies(), receipt) != nil ||
		service.config.PIIAuthority.ValidateCurrent(ctx, input.SecurityContext, materials.projection, materials.receipt.TargetIdentityDigest) != nil {
		return ErrControlledAccessRejected
	}
	body, err := service.config.Artifacts.ResolveExact(ctx, materials.receipt.TargetIdentityDigest)
	if err != nil || !bytes.Equal(body, materials.body) || domainsecurity.SHA256Hex(body) != receipt.ArtifactSHA256 {
		clearControlledArtifactBytesV1(body)
		return ErrControlledAccessRejected
	}
	clearControlledArtifactBytesV1(body)
	return nil
}

func (service *ControlledAccessServiceV1) revalidateAfterRelease(
	ctx context.Context,
	input ControlledAccessInputV1,
	admission piiauthorizationport.ControlledAccessAdmissionV1,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
	materials controlledAccessMaterialsV1,
) error {
	if service.config.ValidateCurrent(ctx, input.SecurityContext) != nil {
		return ErrControlledAccessStale
	}
	current, err := service.resolveAdmission(ctx, input, service.config.Now().UTC())
	if err != nil || !sameControlledAccessAdmissionV1(admission, current) {
		return ErrControlledAccessStale
	}
	if verifyControlledAccessPublicationGraphV1(ctx, service.inventoryDependencies(), receipt) != nil ||
		service.config.PIIAuthority.ValidateCurrent(ctx, input.SecurityContext, materials.projection, materials.receipt.TargetIdentityDigest) != nil {
		return ErrControlledAccessRejected
	}
	return nil
}

func (service *ControlledAccessServiceV1) closeBeforeRelease(
	ctx context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
	status string,
	reason string,
	failure error,
) (ControlledAccessResultV1, error) {
	settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlledAccessSettlementTimeoutV1)
	defer cancel()
	disposition, err := service.persistDisposition(settlementCtx, receipt, status, reason, 0, service.config.Now().UTC())
	if err != nil {
		return ControlledAccessResultV1{}, ErrControlledAccessIntegrity
	}
	return controlledAccessResultV1(receipt, disposition), failure
}

func (service *ControlledAccessServiceV1) persistDisposition(
	ctx context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
	status string,
	reason string,
	released uint64,
	disposedAt time.Time,
) (domainpii.ControlledArtifactAccessDispositionV1, error) {
	if disposedAt.IsZero() {
		return domainpii.ControlledArtifactAccessDispositionV1{}, ErrControlledAccessIntegrity
	}
	disposition, err := domainpii.NewControlledArtifactAccessDispositionV1(
		receipt, status, reason, released, disposedAt,
		service.config.Authority.KeyID(), service.config.Authority.PublicKey(),
		func(message []byte) ([]byte, error) { return service.config.Authority.Sign(ctx, message) },
	)
	if err != nil || verifyControlledAccessDispositionAuthorityV1(ctx, service.config.Authority, disposition) != nil {
		return domainpii.ControlledArtifactAccessDispositionV1{}, ErrControlledAccessIntegrity
	}
	if err := service.config.Access.PutAccessDispositionIfAbsent(ctx, disposition); err != nil {
		return domainpii.ControlledArtifactAccessDispositionV1{}, ErrControlledAccessIntegrity
	}
	stored, err := service.config.Access.ResolveAccessDisposition(ctx, receipt.AccessID)
	if err != nil || !reflect.DeepEqual(stored, disposition) ||
		verifyControlledAccessDispositionAuthorityV1(ctx, service.config.Authority, stored) != nil {
		return domainpii.ControlledArtifactAccessDispositionV1{}, ErrControlledAccessIntegrity
	}
	return stored, nil
}

func (service *ControlledAccessServiceV1) inventoryDependencies() ControlledAccessInventoryDependenciesV1 {
	return ControlledAccessInventoryDependenciesV1{
		Access: service.config.Access, Grants: service.config.Grants, Receipts: service.config.Receipts,
		Commits: service.config.Commits, Indexes: service.config.Indexes, Ledgers: service.config.Ledgers,
		Projections: service.config.Projections, Inspections: service.config.Inspections,
		Artifacts: service.config.ArtifactMetadata, Authority: service.config.Authority,
	}
}

func sameControlledAccessAdmissionV1(
	left piiauthorizationport.ControlledAccessAdmissionV1,
	right piiauthorizationport.ControlledAccessAdmissionV1,
) bool {
	return left.SecurityContext == right.SecurityContext && left.AccessAction == right.AccessAction &&
		left.ControlledHandleDigest == right.ControlledHandleDigest && left.UseSlotDigest == right.UseSlotDigest &&
		left.RendererPrincipalDigest == right.RendererPrincipalDigest && left.RendererGeneration == right.RendererGeneration &&
		left.BackendGeneration == right.BackendGeneration && left.PublicationCommitDigest == right.PublicationCommitDigest &&
		left.AuthorizedUntil.Equal(right.AuthorizedUntil)
}

func controlledAccessResultV1(
	receipt domainpii.ControlledArtifactAccessReceiptV1,
	disposition domainpii.ControlledArtifactAccessDispositionV1,
) ControlledAccessResultV1 {
	return ControlledAccessResultV1{
		AccessID: receipt.AccessID, AccessReceiptDigest: receipt.RecordDigest, DispositionDigest: disposition.RecordDigest,
		Status: disposition.Status, ReleasedByteLength: disposition.ReleasedByteLength,
		ArtifactSHA256: receipt.ArtifactSHA256, ArtifactByteLength: receipt.ArtifactByteLength, MediaType: receipt.MediaType,
	}
}

func clearControlledArtifactBytesV1(body []byte) {
	for index := range body {
		body[index] = 0
	}
	runtime.KeepAlive(body)
}
