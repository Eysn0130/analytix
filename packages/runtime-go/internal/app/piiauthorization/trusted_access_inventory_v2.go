package piiauthorization

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	artifactdeliveryport "analytix.local/runtime-go/internal/ports/artifactdelivery"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

type ControlledAccessInventoryDependenciesV2 struct {
	Access     piiauthorizationport.AccessInventoryStoreV2
	Historical artifactdeliveryport.HistoricalAuthority
	Grants     piiauthorizationport.GrantResolver
	Artifacts  artifactdeliveryport.ControlledMetadataResolver
	Authority  finalauthorityport.Authority
}

// ControlledAccessRestartPlanV2 contains installation-trusted crash-open V2
// reservations only. It cannot be constructed outside this package.
type ControlledAccessRestartPlanV2 struct {
	verified     bool
	openReceipts []domainpii.ControlledArtifactAccessReceiptV2
}

func (plan ControlledAccessRestartPlanV2) OpenReceiptCount() int {
	if !plan.verified {
		return 0
	}
	return len(plan.openReceipts)
}

// VerifyControlledAccessInventoryV2 validates historical integrity without
// granting current release authority. Current context/evidence/PII checks
// remain mandatory on every live V2 side effect.
func VerifyControlledAccessInventoryV2(
	ctx context.Context,
	dependencies ControlledAccessInventoryDependenciesV2,
) (ControlledAccessRestartPlanV2, error) {
	if ctx == nil || dependencies.Access == nil || dependencies.Historical == nil || dependencies.Grants == nil ||
		dependencies.Artifacts == nil || dependencies.Authority == nil {
		return ControlledAccessRestartPlanV2{}, errors.New("controlled access V2 trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return ControlledAccessRestartPlanV2{}, err
	}
	receipts := make(map[string]domainpii.ControlledArtifactAccessReceiptV2)
	var ordered []domainpii.ControlledArtifactAccessReceiptV2
	if err := dependencies.Access.VisitAccessReceiptsV2(ctx, func(receipt domainpii.ControlledArtifactAccessReceiptV2) error {
		if _, duplicate := receipts[receipt.AccessID]; duplicate {
			return errors.New("controlled access V2 inventory repeats an access receipt")
		}

		receipts[receipt.AccessID] = receipt
		ordered = append(ordered, receipt)
		return nil
	}); err != nil {
		return ControlledAccessRestartPlanV2{}, fmt.Errorf("verify controlled access V2 receipt inventory: %w", err)
	}
	// Resolver calls must run after the inventory releases its CAS access guard.
	for _, receipt := range ordered {
		if err := ctx.Err(); err != nil {
			return ControlledAccessRestartPlanV2{}, fmt.Errorf("verify controlled access V2 receipt inventory: %w", err)
		}
		if err := verifyControlledAccessReceiptAuthorityV2(ctx, dependencies.Authority, receipt); err != nil {
			return ControlledAccessRestartPlanV2{}, fmt.Errorf("verify controlled access V2 receipt inventory: %w", err)
		}
		if err := verifyControlledAccessHistoricalMaterialsV2(ctx, dependencies, receipt); err != nil {
			return ControlledAccessRestartPlanV2{}, fmt.Errorf("verify controlled access V2 receipt inventory: %w", err)
		}
	}
	closed := make(map[string]struct{}, len(receipts))
	if err := dependencies.Access.VisitAccessDispositionsV2(ctx, func(disposition domainpii.ControlledArtifactAccessDispositionV2) error {
		receipt, found := receipts[disposition.AccessID]
		if !found {
			return errors.New("controlled access V2 inventory contains an orphan disposition")
		}
		if _, duplicate := closed[disposition.AccessID]; duplicate {
			return errors.New("controlled access V2 inventory repeats a terminal disposition")
		}
		if domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil ||
			verifyControlledAccessDispositionAuthorityV2(ctx, dependencies.Authority, disposition) != nil {
			return errors.New("controlled access V2 disposition does not close its exact trusted receipt")
		}
		closed[disposition.AccessID] = struct{}{}
		return nil
	}); err != nil {
		return ControlledAccessRestartPlanV2{}, fmt.Errorf("verify controlled access V2 disposition inventory: %w", err)
	}
	open := make([]domainpii.ControlledArtifactAccessReceiptV2, 0, len(receipts)-len(closed))
	for accessID, receipt := range receipts {
		if _, found := closed[accessID]; !found {
			open = append(open, receipt)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].AccessID < open[j].AccessID })
	return ControlledAccessRestartPlanV2{verified: true, openReceipts: open}, nil
}

func ApplyControlledAccessRestartPlanV2(
	ctx context.Context,
	plan ControlledAccessRestartPlanV2,
	store piiauthorizationport.RestartAccessStoreV2,
	authority finalauthorityport.Authority,
	disposedAt time.Time,
) error {
	if ctx == nil || !plan.verified || store == nil || authority == nil || disposedAt.IsZero() {
		return errors.New("controlled access V2 restart plan is invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(plan.openReceipts) != 0 && !store.IsSemanticStageControlledAccessStoreV2() {
		return errors.New("controlled access V2 restart writes require an isolated semantic stage")
	}
	disposedAt = disposedAt.UTC()
	dispositions := make([]domainpii.ControlledArtifactAccessDispositionV2, 0, len(plan.openReceipts))
	for _, planned := range plan.openReceipts {
		if err := verifyControlledAccessReceiptAuthorityV2(ctx, authority, planned); err != nil {
			return err
		}
		current, err := store.ResolveAccessReceiptV2(ctx, planned.AccessID)
		if err != nil || !reflect.DeepEqual(current, planned) {
			return errors.Join(errors.New("controlled access V2 restart receipt changed"), err)
		}
		if _, err := store.ResolveAccessDispositionV2(ctx, planned.AccessID); err == nil {
			return errors.New("controlled access V2 restart plan became stale")
		} else if !errors.Is(err, piiauthorizationport.ErrNotFound) {
			return err
		}
		requestedAt, err := time.Parse(time.RFC3339Nano, planned.RequestedAt)
		if err != nil || disposedAt.Before(requestedAt) {
			return errors.New("controlled access V2 restart time precedes the reservation")
		}
		disposition, err := domainpii.NewControlledArtifactAccessDispositionV2(
			planned,
			domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1,
			domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1,
			planned.ArtifactByteLength,
			disposedAt,
			authority.KeyID(),
			authority.PublicKey(),
			func(message []byte) ([]byte, error) { return authority.Sign(ctx, message) },
		)
		if err != nil || verifyControlledAccessDispositionAuthorityV2(ctx, authority, disposition) != nil {
			return errors.Join(errors.New("controlled access V2 restart terminal authority is invalid"), err)
		}
		dispositions = append(dispositions, disposition)
	}
	for _, disposition := range dispositions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := store.PutAccessDispositionIfAbsentV2(ctx, disposition); err != nil {
			return err
		}
		stored, err := store.ResolveAccessDispositionV2(ctx, disposition.AccessID)
		if err != nil || !reflect.DeepEqual(stored, disposition) ||
			verifyControlledAccessDispositionAuthorityV2(ctx, authority, stored) != nil {
			return errors.Join(errors.New("controlled access V2 restart disposition readback failed"), err)
		}
	}
	return nil
}

func verifyControlledAccessHistoricalMaterialsV2(
	ctx context.Context,
	dependencies ControlledAccessInventoryDependenciesV2,
	access domainpii.ControlledArtifactAccessReceiptV2,
) error {
	binding := artifactDeliveryContextBindingFromPIIV2(access.Context)
	if domainartifactdelivery.ValidateContextBindingV1(binding) != nil {
		return errors.New("controlled access V2 receipt contains an invalid context binding")
	}
	delivery, err := dependencies.Historical.ResolveTrustedHistorical(
		ctx,
		artifactdeliveryport.HistoricalSelectorV1{
			Context:    binding,
			DeliveryID: access.DeliveryID, OutcomeRecordDigest: access.DeliveryOutcomeRecordDigest,
			PublicationCommitDigest: access.PublicationCommitDigest,
		},
	)
	if err != nil || domainartifactdelivery.ValidateVerifiedArtifactDeliveryV1(delivery) != nil ||
		delivery.Context != binding || delivery.DeliveryID != access.DeliveryID ||
		delivery.OutcomeRecordDigest != access.DeliveryOutcomeRecordDigest ||
		delivery.PublicationCommitDigest != access.PublicationCommitDigest ||
		delivery.PublicationReceiptDigest != access.PublicationReceiptDigest ||
		delivery.ExposureClass != domainartifactdelivery.ExposureClassRestrictedExactV1 {
		return errors.Join(errors.New("controlled access V2 receipt lost its trusted historical delivery outcome"), err)
	}
	grant, err := dependencies.Grants.ResolveGrant(ctx, access.PIIAuthorizationDigest)
	if err != nil || verifyPIIGrantAuthorityV1(ctx, dependencies.Authority, grant) != nil ||
		domainpii.ValidateControlledArtifactAccessReceiptForGrantV2(access, grant) != nil {
		return errors.New("controlled access V2 receipt lost its trusted PII grant")
	}
	artifact, err := dependencies.Artifacts.ResolveControlledMetadata(ctx, access.TargetIdentityDigest)
	if err != nil {
		return errors.New("controlled access V2 receipt lost its protected artifact metadata")
	}
	publicationIssuedAt, publicationTimeErr := time.Parse(time.RFC3339Nano, delivery.PublicationIssuedAt)
	accessRequestedAt, accessTimeErr := time.Parse(time.RFC3339Nano, access.RequestedAt)
	artifactRenderedAt, artifactTimeErr := time.Parse(time.RFC3339Nano, artifact.RenderedAt)
	grantIssuedAt, grantTimeErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	if access.PIIProjectionDigest != delivery.ContentProjectionDigest ||
		access.PIIAuthorizationDigest != delivery.AuthorizationAuditDigest ||
		access.ClaimLedgerDigest != delivery.ClaimLedgerDigest || access.TargetIdentityDigest != delivery.TargetIdentityDigest ||
		access.ArtifactSHA256 != delivery.ArtifactSHA256 || access.ArtifactByteLength != delivery.ArtifactByteLength ||
		access.MediaType != delivery.MediaType || artifact.SHA256 != access.ArtifactSHA256 ||
		artifact.ByteLength != access.ArtifactByteLength || artifact.MediaType != access.MediaType ||
		artifact.Context != grant.Context || artifact.RequesterUserID != grant.RequesterUserID ||
		artifact.DisclosurePurpose != grant.DisclosurePurpose || artifact.DeliveryScope != grant.DeliveryScope ||
		artifact.ClaimLedgerDigest != grant.ClaimLedgerDigest || artifact.ProjectionRulesetHash != grant.ProjectionRulesetHash ||
		artifact.TargetIdentityDigest != grant.TargetIdentityDigest ||
		artifact.PreservedControlledFieldCount != grant.PreservedControlledFieldCount ||
		artifact.FieldBindingSetDigest != grant.FieldBindingSetDigest ||
		!reflect.DeepEqual(artifact.FieldBindings, grant.FieldBindings) ||
		publicationTimeErr != nil || accessTimeErr != nil || artifactTimeErr != nil || grantTimeErr != nil ||
		artifactRenderedAt.After(grantIssuedAt) || publicationIssuedAt.Before(artifactRenderedAt) ||
		accessRequestedAt.Before(publicationIssuedAt) {
		return errors.New("controlled access V2 receipt does not match its exact historical projected publication")
	}
	return nil
}

func artifactDeliveryContextBindingFromPIIV2(binding domainpii.ContextBindingV1) domainartifactdelivery.ContextBindingV1 {
	return domainartifactdelivery.ContextBindingV1{
		Version: binding.Version, ThreadID: binding.ThreadID, TurnID: binding.TurnID,
		WorkspaceRealPath: binding.WorkspaceRealPath, TenantID: binding.TenantID, UserID: binding.UserID,
		CaseID: binding.CaseID, CaseBindingHash: binding.CaseBindingHash,
		DatasetSnapshotID: binding.DatasetSnapshotID, SourceManifestHash: binding.SourceManifestHash,
		ContextEpoch: binding.ContextEpoch, IssuedAt: binding.ContextIssuedAt, ContextDigest: binding.ContextDigest,
	}
}
