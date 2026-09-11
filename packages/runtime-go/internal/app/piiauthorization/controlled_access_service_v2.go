package piiauthorization

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	artifactdeliveryport "analytix.local/runtime-go/internal/ports/artifactdelivery"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

const controlledAccessSettlementTimeoutV2 = 5 * time.Second

type ControlledAccessAcquireEffectV2 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) (context.Context, func(), error)

type ControlledAccessConfigV2 struct {
	Authority       finalauthorityport.Authority
	Access          piiauthorizationport.AccessStoreV2
	Admissions      piiauthorizationport.ControlledAccessAdmissionAuthorityV2
	Sink            piiauthorizationport.ControlledReleaseSinkV2
	Delivery        artifactdeliveryport.LinearizedCurrentAuthority
	Grants          piiauthorizationport.GrantResolver
	Artifacts       artifactdeliveryport.ArtifactResolver
	ValidateCurrent CurrentContextValidator
	AcquireEffect   ControlledAccessAcquireEffectV2
	Now             func() time.Time
}

type ControlledAccessServiceV2 struct {
	config ControlledAccessConfigV2
}

// ControlledAccessInputV2 contains bearer tokens only in ephemeral memory.
// The service persists V2 domain-separated digests, never the raw tokens.
type ControlledAccessInputV2 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	AccessAction       string
	ControlledHandle   string
	UseSlot            string
	RendererPrincipal  string
	RendererGeneration uint64
	BackendGeneration  uint64
}

type ControlledAccessResultV2 struct {
	AccessID                    string
	AccessReceiptDigest         string
	DispositionDigest           string
	DeliveryID                  string
	DeliveryOutcomeRecordDigest string
	Status                      string
	ReleasedByteLength          uint64
	ArtifactSHA256              string
	ArtifactByteLength          uint64
	MediaType                   string
}

type controlledAccessMaterialsV2 struct {
	delivery domainartifactdelivery.VerifiedArtifactDeliveryV1
	grant    domainpii.PIIProjectionGrantV1
	body     []byte
}

func NewControlledAccessServiceV2(config ControlledAccessConfigV2) (*ControlledAccessServiceV2, error) {
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.Authority == nil || config.Access == nil || config.Admissions == nil || config.Sink == nil ||
		config.Delivery == nil || config.Grants == nil ||
		config.Artifacts == nil || config.ValidateCurrent == nil || config.AcquireEffect == nil || config.Now == nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.Authority.KeyID())) || len(config.Authority.PublicKey()) == 0 {
		return nil, ErrControlledAccessUnavailable
	}
	return &ControlledAccessServiceV2{config: config}, nil
}

// ReleaseV2 is the only V2 raw-byte release path. It validates one exact
// projected delivery before reservation, again while the witnessed snapshot
// remains held across the synchronous sink call, and once more after the sink
// returns. Once the sink has been invoked, every non-exact outcome is recorded
// as full-length indeterminate exposure.
func (service *ControlledAccessServiceV2) ReleaseV2(
	ctx context.Context,
	input ControlledAccessInputV2,
) (ControlledAccessResultV2, error) {
	if service == nil || ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil {
		return ControlledAccessResultV2{}, ErrControlledAccessInvalid
	}
	if err := service.config.ValidateCurrent(ctx, input.SecurityContext); err != nil {
		return ControlledAccessResultV2{}, ErrControlledAccessStale
	}
	admissionObservedAt := service.config.Now().UTC()
	if admissionObservedAt.IsZero() {
		return ControlledAccessResultV2{}, ErrControlledAccessUnavailable
	}
	before, err := service.resolveAdmissionV2(ctx, input, admissionObservedAt)
	if err != nil {
		return ControlledAccessResultV2{}, err
	}
	effectCtx, releaseEffect, err := service.config.AcquireEffect(ctx, input.SecurityContext)
	if err != nil || effectCtx == nil || releaseEffect == nil {
		if releaseEffect != nil {
			releaseEffect()
		}
		if ctx.Err() != nil {
			return ControlledAccessResultV2{}, ErrControlledAccessCancelled
		}
		return ControlledAccessResultV2{}, ErrControlledAccessStale
	}
	defer releaseEffect()

	requestedAt := service.config.Now().UTC()
	if requestedAt.IsZero() || requestedAt.Before(admissionObservedAt) ||
		!requestedAt.Before(before.AuthorizedUntil) ||
		service.config.ValidateCurrent(effectCtx, input.SecurityContext) != nil {
		return ControlledAccessResultV2{}, ErrControlledAccessStale
	}
	current, err := service.resolveAdmissionV2(effectCtx, input, requestedAt)
	if err != nil || !sameControlledAccessAdmissionV2(before, current) {
		return ControlledAccessResultV2{}, ErrControlledAccessStale
	}
	selector := artifactDeliverySelectorV2(current)
	delivery, err := service.config.Delivery.ResolveCurrent(effectCtx, selector)
	if err != nil {
		return ControlledAccessResultV2{}, controlledAccessFromArtifactDeliveryErrorV2(err)
	}
	if err := validateControlledAccessDeliveryV2(input.SecurityContext, current, delivery); err != nil {
		return ControlledAccessResultV2{}, err
	}
	materials, err := service.resolveMaterialsV2(effectCtx, input.SecurityContext, delivery, requestedAt)
	if err != nil {
		return ControlledAccessResultV2{}, err
	}
	defer clearControlledArtifactBytesV1(materials.body)

	authorizedUntil, err := controlledAccessAuthorizedUntilV2(current.AuthorizedUntil, materials.grant.ExpiresAt)
	if err != nil || !requestedAt.Before(authorizedUntil) {
		return ControlledAccessResultV2{}, ErrControlledAccessRejected
	}
	receipt, err := service.newAccessReceiptV2(effectCtx, input.SecurityContext, current, materials, requestedAt, authorizedUntil)
	if err != nil || validateControlledAccessReceiptMaterialsV2(receipt, materials) != nil {
		return ControlledAccessResultV2{}, ErrControlledAccessIntegrity
	}
	if err := service.config.Access.ReserveAccessReceiptV2(effectCtx, receipt); err != nil {
		switch {
		case errors.Is(err, piiauthorizationport.ErrAlreadyReserved), errors.Is(err, piiauthorizationport.ErrConflict):
			return ControlledAccessResultV2{}, ErrControlledAccessDuplicate
		default:
			return ControlledAccessResultV2{}, ErrControlledAccessIntegrity
		}
	}
	stored, err := service.readBackOpenReceiptV2(effectCtx, receipt)
	if err != nil {
		return ControlledAccessResultV2{}, err
	}
	if effectCtx.Err() != nil {
		return service.closeBeforeReleaseV2(effectCtx, stored,
			domainpii.ControlledArtifactAccessDispositionCancelledV1,
			domainpii.ControlledArtifactAccessReasonAccessCancelledV1,
			ErrControlledAccessCancelled,
		)
	}

	var releaseResult piiauthorizationport.ControlledReleaseResultV2
	var releaseErr error
	sinkCalled := false
	linearizedErr := service.config.Delivery.WithCurrent(effectCtx, selector,
		func(live domainartifactdelivery.VerifiedArtifactDeliveryV1) error {
			if !reflect.DeepEqual(live, materials.delivery) {
				return ErrControlledAccessIntegrity
			}
			if service.config.ValidateCurrent(effectCtx, input.SecurityContext) != nil {
				return ErrControlledAccessStale
			}
			liveObservedAt := service.config.Now().UTC()
			if liveObservedAt.IsZero() || liveObservedAt.Before(requestedAt) {
				return ErrControlledAccessStale
			}
			liveAdmission, err := service.resolveAdmissionV2(effectCtx, input, liveObservedAt)
			if err != nil || !sameControlledAccessAdmissionV2(current, liveAdmission) {
				return ErrControlledAccessStale
			}
			liveBody, err := service.config.Artifacts.ResolveExact(effectCtx, live.TargetIdentityDigest)
			if err != nil {
				clearControlledArtifactBytesV1(liveBody)
				return classifyControlledAccessDependencyV2(effectCtx, err)
			}
			defer clearControlledArtifactBytesV1(liveBody)
			if !bytes.Equal(liveBody, materials.body) || uint64(len(liveBody)) != stored.ArtifactByteLength ||
				domainsecurity.SHA256Hex(liveBody) != stored.ArtifactSHA256 {
				return ErrControlledAccessIntegrity
			}
			releaseCtx, cancel := context.WithDeadline(effectCtx, current.AuthorizedUntil)
			defer cancel()
			sinkCalled = true
			releaseResult, releaseErr = service.config.Sink.ReleaseV2(releaseCtx, piiauthorizationport.ControlledReleaseRequestV2{
				Receipt: stored, Body: bytes.NewReader(materials.body), ArtifactByteLength: uint64(len(materials.body)),
			})
			return releaseErr
		})

	if !sinkCalled {
		status, reason, failure := controlledAccessPreSinkTerminalV2(linearizedErr)
		return service.closeBeforeReleaseV2(effectCtx, stored, status, reason, failure)
	}

	settlementCtx, settlementCancel := context.WithTimeout(context.WithoutCancel(effectCtx), controlledAccessSettlementTimeoutV2)
	defer settlementCancel()
	postDelivery, postErr := service.config.Delivery.ResolveCurrent(settlementCtx, selector)
	postNow := service.config.Now().UTC()
	postAdmission, postAdmissionErr := service.resolveAdmissionV2(settlementCtx, input, postNow)
	committed := linearizedErr == nil && releaseErr == nil && releaseResult.Committed &&
		releaseResult.ReleasedByteLength == stored.ArtifactByteLength && postErr == nil &&
		reflect.DeepEqual(postDelivery, materials.delivery) && postAdmissionErr == nil &&
		sameControlledAccessAdmissionV2(current, postAdmission) && !postNow.IsZero() &&
		!postNow.Before(requestedAt) && postNow.Before(storedAuthorizedUntilV2(stored))
	status := domainpii.ControlledArtifactAccessDispositionHostReleaseCommittedV1
	reason := domainpii.ControlledArtifactAccessReasonHostReleaseCommittedV1
	resultErr := error(nil)
	if !committed {
		status = domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1
		reason = domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1
		resultErr = ErrControlledAccessIndeterminate
	}
	disposition, err := service.persistDispositionV2(
		settlementCtx, stored, status, reason, stored.ArtifactByteLength,
		controlledAccessDispositionTimeV2(stored, postNow),
	)
	if err != nil {
		return ControlledAccessResultV2{}, ErrControlledAccessIntegrity
	}
	return controlledAccessResultV2(stored, disposition), resultErr
}

func (service *ControlledAccessServiceV2) resolveAdmissionV2(
	ctx context.Context,
	input ControlledAccessInputV2,
	now time.Time,
) (piiauthorizationport.ControlledAccessAdmissionV2, error) {
	handleDigest, handleErr := domainpii.ControlledAccessHandleDigestV2(input.ControlledHandle)
	useSlotDigest, slotErr := domainpii.ControlledAccessUseSlotDigestV2(input.UseSlot)
	principalDigest, principalErr := domainpii.ControlledAccessRendererPrincipalDigestV2(input.RendererPrincipal)
	if handleErr != nil || slotErr != nil || principalErr != nil || input.RendererGeneration == 0 || input.BackendGeneration == 0 ||
		(input.AccessAction != domainpii.ControlledArtifactAccessActionDisplayV1 &&
			input.AccessAction != domainpii.ControlledArtifactAccessActionExportV1) {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrControlledAccessInvalid
	}
	request := piiauthorizationport.ControlledAccessAdmissionRequestV2{
		SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
		ControlledHandle: input.ControlledHandle, UseSlot: input.UseSlot, RendererPrincipal: input.RendererPrincipal,
		RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
	}
	admission, err := service.config.Admissions.ResolveCurrentV2(ctx, request)
	admission.AuthorizedUntil = admission.AuthorizedUntil.UTC()
	if err != nil || admission.SecurityContext != input.SecurityContext || admission.AccessAction != input.AccessAction ||
		admission.ControlledHandleDigest != handleDigest || admission.UseSlotDigest != useSlotDigest ||
		admission.RendererPrincipalDigest != principalDigest || admission.RendererGeneration != input.RendererGeneration ||
		admission.BackendGeneration != input.BackendGeneration || !domainsecurity.IsSHA256Hex(admission.DeliveryID) ||
		!domainsecurity.IsSHA256Hex(admission.DeliveryOutcomeRecordDigest) ||
		!domainsecurity.IsSHA256Hex(admission.PublicationCommitDigest) ||
		!domainsecurity.IsSHA256Hex(admission.ReleaseTargetIdentityDigest) || now.IsZero() ||
		admission.AuthorizedUntil.IsZero() || !now.Before(admission.AuthorizedUntil) ||
		admission.AuthorizedUntil.Sub(now) > domainpii.PIIProjectionGrantMaxTTL {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrControlledAccessStale
	}
	return admission, nil
}

func (service *ControlledAccessServiceV2) resolveMaterialsV2(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	delivery domainartifactdelivery.VerifiedArtifactDeliveryV1,
	requestedAt time.Time,
) (controlledAccessMaterialsV2, error) {
	binding, bindErr := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(securityContext)
	if bindErr != nil || domainartifactdelivery.ValidateVerifiedArtifactDeliveryV1(delivery) != nil ||
		delivery.Context != binding {
		return controlledAccessMaterialsV2{}, ErrControlledAccessIntegrity
	}
	grant, err := service.config.Grants.ResolveGrant(ctx, delivery.AuthorizationAuditDigest)
	if err != nil {
		return controlledAccessMaterialsV2{}, classifyControlledAccessDependencyV2(ctx, err)
	}
	if domainpii.ValidatePIIProjectionGrantV1(grant) != nil || grant.RecordDigest != delivery.AuthorizationAuditDigest ||
		verifyPIIGrantAuthorityV1(ctx, service.config.Authority, grant) != nil ||
		grant.ClaimLedgerDigest != delivery.ClaimLedgerDigest || grant.TargetIdentityDigest != delivery.TargetIdentityDigest ||
		grant.ProjectedContentSHA256 != delivery.ArtifactSHA256 {
		return controlledAccessMaterialsV2{}, ErrControlledAccessIntegrity
	}
	receiptIssuedAt, receiptTimeErr := time.Parse(time.RFC3339Nano, delivery.PublicationIssuedAt)
	if receiptTimeErr != nil || requestedAt.Before(receiptIssuedAt) {
		return controlledAccessMaterialsV2{}, ErrControlledAccessIntegrity
	}
	body, err := service.config.Artifacts.ResolveExact(ctx, delivery.TargetIdentityDigest)
	if err != nil {
		clearControlledArtifactBytesV1(body)
		return controlledAccessMaterialsV2{}, classifyControlledAccessDependencyV2(ctx, err)
	}
	metadata, metadataErr := domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
	if metadataErr != nil || uint64(len(body)) != delivery.ArtifactByteLength ||
		domainsecurity.SHA256Hex(body) != delivery.ArtifactSHA256 ||
		domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
			metadata, securityContext, delivery.ClaimLedgerDigest, grant.ProjectionRulesetHash,
			delivery.TargetIdentityDigest, delivery.ArtifactSHA256, delivery.ArtifactByteLength,
			grant.PreservedControlledFieldCount,
		) != nil {
		clearControlledArtifactBytesV1(body)
		return controlledAccessMaterialsV2{}, ErrControlledAccessIntegrity
	}
	return controlledAccessMaterialsV2{delivery: delivery, grant: grant, body: body}, nil
}

func (service *ControlledAccessServiceV2) newAccessReceiptV2(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	admission piiauthorizationport.ControlledAccessAdmissionV2,
	materials controlledAccessMaterialsV2,
	requestedAt time.Time,
	authorizedUntil time.Time,
) (domainpii.ControlledArtifactAccessReceiptV2, error) {
	delivery := materials.delivery
	return domainpii.NewControlledArtifactAccessReceiptV2(domainpii.ControlledArtifactAccessReceiptInputV2{
		SecurityContext: securityContext, AccessAction: admission.AccessAction,
		ControlledHandleDigest: admission.ControlledHandleDigest, UseSlotDigest: admission.UseSlotDigest,
		RendererPrincipalDigest: admission.RendererPrincipalDigest, RendererGeneration: admission.RendererGeneration,
		BackendGeneration: admission.BackendGeneration, AccessPolicyDigest: materials.grant.AccessPolicyDigest,
		RetentionPolicyDigest: materials.grant.RetentionPolicyDigest,
		DeliveryID:            delivery.DeliveryID, DeliveryOutcomeRecordDigest: delivery.OutcomeRecordDigest,
		PublicationCommitDigest:  delivery.PublicationCommitDigest,
		PublicationReceiptDigest: delivery.PublicationReceiptDigest,
		PIIProjectionDigest:      delivery.ContentProjectionDigest,
		PIIAuthorizationDigest:   delivery.AuthorizationAuditDigest,
		ClaimLedgerDigest:        delivery.ClaimLedgerDigest, TargetIdentityDigest: delivery.TargetIdentityDigest,
		ReleaseTargetIdentityDigest: admission.ReleaseTargetIdentityDigest,
		ArtifactSHA256:              delivery.ArtifactSHA256, ArtifactByteLength: delivery.ArtifactByteLength,
		MediaType: delivery.MediaType, RequestedAt: requestedAt, AuthorizedUntil: authorizedUntil,
		AuthorityKeyID: service.config.Authority.KeyID(), AuthorityPublicKey: service.config.Authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.config.Authority.Sign(ctx, message) })
}

func (service *ControlledAccessServiceV2) readBackOpenReceiptV2(
	ctx context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
) (domainpii.ControlledArtifactAccessReceiptV2, error) {
	readbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlledAccessSettlementTimeoutV2)
	defer cancel()
	stored, err := service.config.Access.ResolveAccessReceiptV2(readbackCtx, receipt.AccessID)
	if err != nil || !reflect.DeepEqual(stored, receipt) ||
		verifyControlledAccessReceiptAuthorityV2(readbackCtx, service.config.Authority, stored) != nil {
		return domainpii.ControlledArtifactAccessReceiptV2{}, ErrControlledAccessIntegrity
	}
	if _, err := service.config.Access.ResolveAccessDispositionV2(readbackCtx, receipt.AccessID); err == nil {
		return domainpii.ControlledArtifactAccessReceiptV2{}, ErrControlledAccessDuplicate
	} else if !errors.Is(err, piiauthorizationport.ErrNotFound) {
		return domainpii.ControlledArtifactAccessReceiptV2{}, ErrControlledAccessIntegrity
	}
	return stored, nil
}

func (service *ControlledAccessServiceV2) closeBeforeReleaseV2(
	ctx context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
	status string,
	reason string,
	failure error,
) (ControlledAccessResultV2, error) {
	settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlledAccessSettlementTimeoutV2)
	defer cancel()
	disposition, err := service.persistDispositionV2(
		settlementCtx, receipt, status, reason, 0,
		controlledAccessDispositionTimeV2(receipt, service.config.Now().UTC()),
	)
	if err != nil {
		return ControlledAccessResultV2{}, ErrControlledAccessIntegrity
	}
	return controlledAccessResultV2(receipt, disposition), failure
}

func (service *ControlledAccessServiceV2) persistDispositionV2(
	ctx context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
	status string,
	reason string,
	released uint64,
	disposedAt time.Time,
) (domainpii.ControlledArtifactAccessDispositionV2, error) {
	if disposedAt.IsZero() {
		return domainpii.ControlledArtifactAccessDispositionV2{}, ErrControlledAccessIntegrity
	}
	disposition, err := domainpii.NewControlledArtifactAccessDispositionV2(
		receipt, status, reason, released, disposedAt,
		service.config.Authority.KeyID(), service.config.Authority.PublicKey(),
		func(message []byte) ([]byte, error) { return service.config.Authority.Sign(ctx, message) },
	)
	if err != nil || verifyControlledAccessDispositionAuthorityV2(ctx, service.config.Authority, disposition) != nil {
		return domainpii.ControlledArtifactAccessDispositionV2{}, ErrControlledAccessIntegrity
	}
	if err := service.config.Access.PutAccessDispositionIfAbsentV2(ctx, disposition); err != nil {
		return domainpii.ControlledArtifactAccessDispositionV2{}, ErrControlledAccessIntegrity
	}
	stored, err := service.config.Access.ResolveAccessDispositionV2(ctx, receipt.AccessID)
	if err != nil || !reflect.DeepEqual(stored, disposition) ||
		verifyControlledAccessDispositionAuthorityV2(ctx, service.config.Authority, stored) != nil {
		return domainpii.ControlledArtifactAccessDispositionV2{}, ErrControlledAccessIntegrity
	}
	return stored, nil
}

func validateControlledAccessDeliveryV2(
	securityContext domainsecurity.TurnSecurityContext,
	admission piiauthorizationport.ControlledAccessAdmissionV2,
	delivery domainartifactdelivery.VerifiedArtifactDeliveryV1,
) error {
	binding, bindErr := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(securityContext)
	if bindErr != nil || domainartifactdelivery.ValidateVerifiedArtifactDeliveryV1(delivery) != nil ||
		delivery.Context != binding || delivery.DeliveryID != admission.DeliveryID ||
		delivery.OutcomeRecordDigest != admission.DeliveryOutcomeRecordDigest ||
		delivery.PublicationCommitDigest != admission.PublicationCommitDigest ||
		admission.ReleaseTargetIdentityDigest == delivery.TargetIdentityDigest ||
		delivery.ExposureClass != domainartifactdelivery.ExposureClassRestrictedExactV1 {
		return ErrControlledAccessRejected
	}
	return nil
}

func validateControlledAccessReceiptMaterialsV2(
	receipt domainpii.ControlledArtifactAccessReceiptV2,
	materials controlledAccessMaterialsV2,
) error {
	delivery := materials.delivery
	if domainpii.ValidateControlledArtifactAccessReceiptForGrantV2(receipt, materials.grant) != nil ||
		receipt.DeliveryID != delivery.DeliveryID || receipt.DeliveryOutcomeRecordDigest != delivery.OutcomeRecordDigest ||
		receipt.PublicationCommitDigest != delivery.PublicationCommitDigest ||
		receipt.PublicationReceiptDigest != delivery.PublicationReceiptDigest ||
		receipt.PIIProjectionDigest != delivery.ContentProjectionDigest ||
		receipt.PIIAuthorizationDigest != delivery.AuthorizationAuditDigest ||
		receipt.ClaimLedgerDigest != delivery.ClaimLedgerDigest ||
		receipt.TargetIdentityDigest != delivery.TargetIdentityDigest ||
		receipt.ArtifactSHA256 != delivery.ArtifactSHA256 || receipt.ArtifactByteLength != delivery.ArtifactByteLength ||
		receipt.MediaType != delivery.MediaType {
		return ErrControlledAccessIntegrity
	}
	return nil
}

func verifyControlledAccessReceiptAuthorityV2(
	ctx context.Context,
	authority finalauthorityport.Authority,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
) error {
	keyID, publicKey, signature, err := domainpii.ControlledArtifactAccessReceiptAuthorityMaterialV2(receipt)
	if err != nil || authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpii.ControlledArtifactAccessReceiptSigningBytesV2(receipt), signature,
	) != nil {
		return ErrControlledAccessIntegrity
	}
	return nil
}

func verifyControlledAccessDispositionAuthorityV2(
	ctx context.Context,
	authority finalauthorityport.Authority,
	disposition domainpii.ControlledArtifactAccessDispositionV2,
) error {
	keyID, publicKey, signature, err := domainpii.ControlledArtifactAccessDispositionAuthorityMaterialV2(disposition)
	if err != nil || authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpii.ControlledArtifactAccessDispositionSigningBytesV2(disposition), signature,
	) != nil {
		return ErrControlledAccessIntegrity
	}
	return nil
}

func artifactDeliverySelectorV2(
	admission piiauthorizationport.ControlledAccessAdmissionV2,
) artifactdeliveryport.SelectorV1 {
	return artifactdeliveryport.SelectorV1{
		SecurityContext: admission.SecurityContext, DeliveryID: admission.DeliveryID,
		OutcomeRecordDigest:     admission.DeliveryOutcomeRecordDigest,
		PublicationCommitDigest: admission.PublicationCommitDigest,
	}
}

func sameControlledAccessAdmissionV2(
	left piiauthorizationport.ControlledAccessAdmissionV2,
	right piiauthorizationport.ControlledAccessAdmissionV2,
) bool {
	return left.SecurityContext == right.SecurityContext && left.AccessAction == right.AccessAction &&
		left.ControlledHandleDigest == right.ControlledHandleDigest && left.UseSlotDigest == right.UseSlotDigest &&
		left.RendererPrincipalDigest == right.RendererPrincipalDigest && left.RendererGeneration == right.RendererGeneration &&
		left.BackendGeneration == right.BackendGeneration && left.DeliveryID == right.DeliveryID &&
		left.DeliveryOutcomeRecordDigest == right.DeliveryOutcomeRecordDigest &&
		left.PublicationCommitDigest == right.PublicationCommitDigest &&
		left.ReleaseTargetIdentityDigest == right.ReleaseTargetIdentityDigest &&
		left.AuthorizedUntil.Equal(right.AuthorizedUntil)
}

func controlledAccessAuthorizedUntilV2(admissionUntil time.Time, grantExpiresAt string) (time.Time, error) {
	grantUntil, err := time.Parse(time.RFC3339Nano, grantExpiresAt)
	if err != nil || admissionUntil.IsZero() || grantUntil.IsZero() {
		return time.Time{}, ErrControlledAccessIntegrity
	}
	admissionUntil = admissionUntil.UTC()
	if grantUntil.Before(admissionUntil) {
		return grantUntil, nil
	}
	return admissionUntil, nil
}

func storedAuthorizedUntilV2(receipt domainpii.ControlledArtifactAccessReceiptV2) time.Time {
	value, _ := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	return value
}

func controlledAccessDispositionTimeV2(
	receipt domainpii.ControlledArtifactAccessReceiptV2,
	observed time.Time,
) time.Time {
	requestedAt, err := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	if err != nil || requestedAt.IsZero() {
		return observed.UTC()
	}
	observed = observed.UTC()
	if observed.IsZero() || observed.Before(requestedAt) {
		return requestedAt
	}
	return observed
}

func controlledAccessFromArtifactDeliveryErrorV2(err error) error {
	switch {
	case errors.Is(err, artifactdeliveryport.ErrCancelled), errors.Is(err, ErrControlledAccessCancelled),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ErrControlledAccessCancelled
	case errors.Is(err, artifactdeliveryport.ErrStale), errors.Is(err, ErrControlledAccessStale):
		return ErrControlledAccessStale
	case errors.Is(err, artifactdeliveryport.ErrRejected),
		errors.Is(err, artifactdeliveryport.ErrNotFound), errors.Is(err, ErrControlledAccessRejected):
		return ErrControlledAccessRejected
	case errors.Is(err, artifactdeliveryport.ErrIntegrity),
		errors.Is(err, artifactdeliveryport.ErrInvalid), errors.Is(err, ErrControlledAccessIntegrity):
		return ErrControlledAccessIntegrity
	case errors.Is(err, artifactdeliveryport.ErrUnavailable), errors.Is(err, ErrControlledAccessUnavailable):
		return ErrControlledAccessUnavailable
	default:
		return ErrControlledAccessUnavailable
	}
}

func classifyControlledAccessDependencyV2(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrControlledAccessCancelled
	}
	if errors.Is(err, artifactdeliveryport.ErrNotFound) || errors.Is(err, piiauthorizationport.ErrNotFound) {
		return ErrControlledAccessIntegrity
	}
	return ErrControlledAccessUnavailable
}

func controlledAccessPreSinkTerminalV2(err error) (string, string, error) {
	failure := controlledAccessFromArtifactDeliveryErrorV2(err)
	switch {
	case errors.Is(failure, ErrControlledAccessCancelled):
		return domainpii.ControlledArtifactAccessDispositionCancelledV1,
			domainpii.ControlledArtifactAccessReasonAccessCancelledV1, ErrControlledAccessCancelled
	case errors.Is(failure, ErrControlledAccessStale):
		return domainpii.ControlledArtifactAccessDispositionStaleContextV1,
			domainpii.ControlledArtifactAccessReasonStaleContextV1, ErrControlledAccessStale
	case errors.Is(failure, ErrControlledAccessRejected):
		return domainpii.ControlledArtifactAccessDispositionRejectedV1,
			domainpii.ControlledArtifactAccessReasonAccessRejectedV1, ErrControlledAccessRejected
	case errors.Is(failure, ErrControlledAccessUnavailable):
		return domainpii.ControlledArtifactAccessDispositionFailedV1,
			domainpii.ControlledArtifactAccessReasonAuthorityUnavailableV2, ErrControlledAccessUnavailable
	default:
		return domainpii.ControlledArtifactAccessDispositionFailedV1,
			domainpii.ControlledArtifactAccessReasonAuthorityIntegrityV2, ErrControlledAccessIntegrity
	}
}

func controlledAccessResultV2(
	receipt domainpii.ControlledArtifactAccessReceiptV2,
	disposition domainpii.ControlledArtifactAccessDispositionV2,
) ControlledAccessResultV2 {
	return ControlledAccessResultV2{
		AccessID: receipt.AccessID, AccessReceiptDigest: receipt.RecordDigest,
		DispositionDigest: disposition.RecordDigest, DeliveryID: receipt.DeliveryID,
		DeliveryOutcomeRecordDigest: receipt.DeliveryOutcomeRecordDigest, Status: disposition.Status,
		ReleasedByteLength: disposition.ReleasedByteLength, ArtifactSHA256: receipt.ArtifactSHA256,
		ArtifactByteLength: receipt.ArtifactByteLength, MediaType: receipt.MediaType,
	}
}
