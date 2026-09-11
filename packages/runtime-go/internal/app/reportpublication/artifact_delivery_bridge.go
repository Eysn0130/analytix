package reportpublication

import (
	"context"
	"errors"
	"reflect"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	artifactdeliveryport "analytix.local/runtime-go/internal/ports/artifactdelivery"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

type artifactDeliveryBridgeMaterialsV1 struct {
	commits   publicationport.CommitReceiptResolver
	receipts  publicationport.ReceiptResolver
	authority finalauthorityport.Verifier
}

type CurrentArtifactDeliveryBridgeV1 struct {
	projected publicationport.LinearizedProjectedDeliveryAuthority
	materials artifactDeliveryBridgeMaterialsV1
}

type HistoricalArtifactDeliveryBridgeV1 struct {
	projected publicationport.HistoricalProjectedDeliveryAuthority
	materials artifactDeliveryBridgeMaterialsV1
}

var _ artifactdeliveryport.LinearizedCurrentAuthority = (*CurrentArtifactDeliveryBridgeV1)(nil)
var _ artifactdeliveryport.HistoricalAuthority = (*HistoricalArtifactDeliveryBridgeV1)(nil)

func NewCurrentArtifactDeliveryBridgeV1(
	projected publicationport.LinearizedProjectedDeliveryAuthority,
	commits publicationport.CommitReceiptResolver,
	receipts publicationport.ReceiptResolver,
	authority finalauthorityport.Verifier,
) (*CurrentArtifactDeliveryBridgeV1, error) {
	materials := artifactDeliveryBridgeMaterialsV1{commits: commits, receipts: receipts, authority: authority}
	if projected == nil || !validArtifactDeliveryBridgeMaterialsV1(materials) {
		return nil, artifactdeliveryport.ErrUnavailable
	}
	return &CurrentArtifactDeliveryBridgeV1{projected: projected, materials: materials}, nil
}

func NewHistoricalArtifactDeliveryBridgeV1(
	projected publicationport.HistoricalProjectedDeliveryAuthority,
	commits publicationport.CommitReceiptResolver,
	receipts publicationport.ReceiptResolver,
	authority finalauthorityport.Verifier,
) (*HistoricalArtifactDeliveryBridgeV1, error) {
	materials := artifactDeliveryBridgeMaterialsV1{commits: commits, receipts: receipts, authority: authority}
	if projected == nil || !validArtifactDeliveryBridgeMaterialsV1(materials) {
		return nil, artifactdeliveryport.ErrUnavailable
	}
	return &HistoricalArtifactDeliveryBridgeV1{projected: projected, materials: materials}, nil
}

func (bridge *CurrentArtifactDeliveryBridgeV1) ResolveCurrent(
	ctx context.Context,
	selector artifactdeliveryport.SelectorV1,
) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error) {
	if bridge == nil || bridge.projected == nil || ctx == nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrUnavailable
	}
	projection, err := bridge.projected.ResolveCurrentProjectedDelivery(ctx, projectedSelectorFromArtifactDeliveryV1(selector))
	if err != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactDeliveryBridgeErrorV1(err)
	}
	binding, bindErr := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(selector.SecurityContext)
	if bindErr != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrInvalid
	}
	delivery, err := bridge.materials.resolve(ctx, binding, projection)
	if err != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactDeliveryBridgeErrorV1(err)
	}
	return delivery, nil
}

func (bridge *CurrentArtifactDeliveryBridgeV1) WithCurrent(
	ctx context.Context,
	selector artifactdeliveryport.SelectorV1,
	use func(domainartifactdelivery.VerifiedArtifactDeliveryV1) error,
) error {
	if bridge == nil || bridge.projected == nil || ctx == nil || use == nil {
		return artifactdeliveryport.ErrInvalid
	}
	return artifactDeliveryBridgeErrorV1(bridge.projected.WithCurrentProjectedDelivery(
		ctx,
		projectedSelectorFromArtifactDeliveryV1(selector),
		func(projection domainpublication.ReportDeliveryProjectionV1) error {
			binding, bindErr := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(selector.SecurityContext)
			if bindErr != nil {
				return artifactdeliveryport.ErrInvalid
			}
			delivery, err := bridge.materials.resolve(ctx, binding, projection)
			if err != nil {
				return err
			}
			return use(delivery)
		},
	))
}

func (bridge *HistoricalArtifactDeliveryBridgeV1) ResolveTrustedHistorical(
	ctx context.Context,
	selector artifactdeliveryport.HistoricalSelectorV1,
) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error) {
	if bridge == nil || bridge.projected == nil || ctx == nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrUnavailable
	}
	projection, err := bridge.projected.ResolveTrustedHistoricalProjectedDelivery(
		ctx,
		publicationport.HistoricalProjectedDeliverySelectorV1{
			DeliveryID: selector.DeliveryID, OutcomeRecordDigest: selector.OutcomeRecordDigest,
			PublicationCommitDigest: selector.PublicationCommitDigest,
			ThreadID:                selector.Context.ThreadID, TurnID: selector.Context.TurnID,
			ContextDigest:      selector.Context.ContextDigest,
			CaseBindingHash:    selector.Context.CaseBindingHash,
			ContextEpoch:       selector.Context.ContextEpoch,
			DatasetSnapshotID:  selector.Context.DatasetSnapshotID,
			SourceManifestHash: selector.Context.SourceManifestHash,
		},
	)
	if err != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactDeliveryBridgeErrorV1(err)
	}
	delivery, err := bridge.materials.resolve(ctx, selector.Context, projection)
	if err != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactDeliveryBridgeErrorV1(err)
	}
	return delivery, nil
}

func (materials artifactDeliveryBridgeMaterialsV1) resolve(
	ctx context.Context,
	binding domainartifactdelivery.ContextBindingV1,
	projection domainpublication.ReportDeliveryProjectionV1,
) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error) {
	if ctx == nil || domainartifactdelivery.ValidateContextBindingV1(binding) != nil ||
		domainpublication.ValidateReportDeliveryProjectionV1(projection) != nil ||
		projection.PIIProjectionClass != domainpublication.PIIProjectionControlledFull ||
		projection.ThreadID != binding.ThreadID || projection.TurnID != binding.TurnID ||
		projection.ContextDigest != binding.ContextDigest ||
		projection.CaseBindingHash != binding.CaseBindingHash ||
		projection.ContextEpoch != binding.ContextEpoch ||
		projection.DatasetSnapshotID != binding.DatasetSnapshotID ||
		projection.SourceManifestHash != binding.SourceManifestHash {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrIntegrity
	}
	commit, err := materials.commits.Resolve(ctx, projection.CommitRecordDigest)
	if err != nil || domainpublication.ValidatePublicationCommitReceiptV1(commit) != nil ||
		verifyArtifactDeliveryCommitAuthorityV1(ctx, materials.authority, commit) != nil ||
		commit.RecordDigest != projection.CommitRecordDigest ||
		commit.ThreadID != binding.ThreadID || commit.TurnID != binding.TurnID ||
		commit.ContextDigest != binding.ContextDigest || commit.CaseID != binding.CaseID ||
		commit.CaseBindingHash != binding.CaseBindingHash || commit.ContextEpoch != binding.ContextEpoch ||
		commit.DatasetSnapshotID != binding.DatasetSnapshotID ||
		commit.SourceManifestHash != binding.SourceManifestHash ||
		commit.ClaimLedgerDigest != projection.ClaimLedgerDigest ||
		commit.PIIProjectionDigest != projection.PIIProjectionDigest ||
		commit.AuthorizationAuditDigest != projection.AuthorizationAuditDigest ||
		commit.TargetIdentityDigest != projection.TargetIdentityDigest || commit.ReportSHA256 != projection.ReportSHA256 ||
		commit.ReportByteLength != projection.ReportByteLength || commit.MediaType != projection.MediaType {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, errors.Join(artifactdeliveryport.ErrIntegrity, err)
	}
	receipt, err := materials.receipts.Resolve(ctx, commit.CandidateRecordDigest)
	if err != nil || domainpublication.ValidatePublicationReceiptV1(receipt) != nil ||
		verifyArtifactDeliveryReceiptAuthorityV1(ctx, materials.authority, receipt) != nil ||
		receipt.RecordDigest != commit.CandidateRecordDigest || receipt.ReceiptID != commit.CandidateReceiptID ||
		receipt.ThreadID != binding.ThreadID || receipt.TurnID != binding.TurnID ||
		receipt.ContextDigest != binding.ContextDigest || receipt.CaseID != binding.CaseID ||
		receipt.CaseBindingHash != binding.CaseBindingHash || receipt.ContextEpoch != binding.ContextEpoch ||
		receipt.DatasetSnapshotID != binding.DatasetSnapshotID ||
		receipt.SourceManifestHash != binding.SourceManifestHash ||
		receipt.ClaimLedgerDigest != projection.ClaimLedgerDigest ||
		receipt.PIIProjectionDigest != projection.PIIProjectionDigest ||
		receipt.AuthorizationAuditDigest != projection.AuthorizationAuditDigest ||
		receipt.TargetIdentityDigest != projection.TargetIdentityDigest || receipt.ReportSHA256 != projection.ReportSHA256 ||
		receipt.ReportByteLength != projection.ReportByteLength || receipt.MediaType != projection.MediaType {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, errors.Join(artifactdeliveryport.ErrIntegrity, err)
	}
	delivery := domainartifactdelivery.VerifiedArtifactDeliveryV1{
		Context: binding, DeliveryID: projection.DeliveryID,
		OutcomeRecordDigest: projection.RecordDigest, PublicationCommitDigest: commit.RecordDigest,
		PublicationReceiptDigest: receipt.RecordDigest, ClaimLedgerDigest: projection.ClaimLedgerDigest,
		ContentProjectionDigest:  projection.PIIProjectionDigest,
		AuthorizationAuditDigest: projection.AuthorizationAuditDigest,
		TargetIdentityDigest:     projection.TargetIdentityDigest, ArtifactSHA256: projection.ReportSHA256,
		ArtifactByteLength: projection.ReportByteLength, MediaType: projection.MediaType,
		ExposureClass:       domainartifactdelivery.ExposureClassRestrictedExactV1,
		PublicationIssuedAt: receipt.IssuedAt,
	}
	if domainartifactdelivery.ValidateVerifiedArtifactDeliveryV1(delivery) != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrIntegrity
	}
	return delivery, nil
}

func validArtifactDeliveryBridgeMaterialsV1(materials artifactDeliveryBridgeMaterialsV1) bool {
	return materials.commits != nil && materials.receipts != nil && materials.authority != nil &&
		domainsecurity.IsSHA256Hex(materials.authority.KeyID()) && len(materials.authority.PublicKey()) != 0
}

func projectedSelectorFromArtifactDeliveryV1(
	selector artifactdeliveryport.SelectorV1,
) publicationport.ProjectedDeliverySelectorV1 {
	return publicationport.ProjectedDeliverySelectorV1{
		SecurityContext: selector.SecurityContext, DeliveryID: selector.DeliveryID,
		OutcomeRecordDigest:     selector.OutcomeRecordDigest,
		PublicationCommitDigest: selector.PublicationCommitDigest,
	}
}

func verifyArtifactDeliveryCommitAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Verifier,
	commit domainpublication.PublicationCommitReceiptV1,
) error {
	keyID, publicKey, signature, err := domainpublication.PublicationCommitReceiptAuthorityMaterialV1(commit)
	if err != nil || keyID != authority.KeyID() || !reflect.DeepEqual(publicKey, authority.PublicKey()) {
		return artifactdeliveryport.ErrIntegrity
	}
	return authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.PublicationCommitReceiptSigningBytesV1(commit), signature)
}

func verifyArtifactDeliveryReceiptAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Verifier,
	receipt domainpublication.PublicationReceiptV1,
) error {
	keyID, publicKey, signature, err := domainpublication.PublicationReceiptAuthorityMaterialV1(receipt)
	if err != nil || keyID != authority.KeyID() || !reflect.DeepEqual(publicKey, authority.PublicKey()) {
		return artifactdeliveryport.ErrIntegrity
	}
	return authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.PublicationReceiptSigningBytesV1(receipt), signature)
}

func artifactDeliveryBridgeErrorV1(err error) error {
	if err == nil {
		return nil
	}
	for _, mapping := range []struct{ from, to error }{
		{publicationport.ErrProjectedDeliveryInvalid, artifactdeliveryport.ErrInvalid},
		{publicationport.ErrProjectedDeliveryStale, artifactdeliveryport.ErrStale},
		{publicationport.ErrProjectedDeliveryNotFound, artifactdeliveryport.ErrNotFound},
		{publicationport.ErrProjectedDeliveryRejected, artifactdeliveryport.ErrRejected},
		{publicationport.ErrProjectedDeliveryIntegrity, artifactdeliveryport.ErrIntegrity},
		{publicationport.ErrProjectedDeliveryCancelled, artifactdeliveryport.ErrCancelled},
		{publicationport.ErrProjectedDeliveryUnavailable, artifactdeliveryport.ErrUnavailable},
	} {
		if errors.Is(err, mapping.from) {
			return errors.Join(mapping.to, err)
		}
	}
	if errors.Is(err, artifactdeliveryport.ErrInvalid) || errors.Is(err, artifactdeliveryport.ErrStale) ||
		errors.Is(err, artifactdeliveryport.ErrNotFound) || errors.Is(err, artifactdeliveryport.ErrRejected) ||
		errors.Is(err, artifactdeliveryport.ErrIntegrity) || errors.Is(err, artifactdeliveryport.ErrCancelled) ||
		errors.Is(err, artifactdeliveryport.ErrUnavailable) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(artifactdeliveryport.ErrCancelled, err)
	}
	return errors.Join(artifactdeliveryport.ErrUnavailable, err)
}
