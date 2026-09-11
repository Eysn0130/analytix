package reportpublication

import (
	"bytes"
	"context"
	"errors"
	"reflect"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// admitReportDeliveryDecisionV1 is the final host admission boundary before a
// report may become eligible for grant settlement. It creates no public
// projection. Delivery remains impossible until a later use case proves the
// decision-bound tool settlement and completed report-stage disposition.
func (service *Service) admitReportDeliveryDecisionV1(
	ctx context.Context,
	input PublishInput,
	lease pendingworkapp.ReportStageLease,
	stageRequest pendingworkapp.ReportStageRequest,
	stageReceipt domainpendingwork.PendingWorkReceiptV1,
	plan publicationPlanV1,
	settlementDigest string,
	commit domainpublication.PublicationCommitReceiptV1,
) (domainpublication.ReportDeliveryDecisionV1, error) {
	if service == nil || ctx == nil || service.decisions == nil {
		return domainpublication.ReportDeliveryDecisionV1{}, ErrPublicationUnavailable
	}
	selectionID := domainpublication.PublicationCommitSelectionIDV1(
		service.installationID, service.enrollmentID, plan.attempt.AttemptID,
	)
	selection, err := service.selections.Resolve(ctx, selectionID)
	if err != nil {
		return domainpublication.ReportDeliveryDecisionV1{}, commitSelectorResolveErrorV1(err)
	}
	selectorConfig := CommitSelectorConfigV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID, Authority: service.authority,
		WitnessKeyID: service.witnessKeyID, WitnessKey: service.witnessKey,
		HeadReader: service.headReader, Bundles: service.bundles, Observations: service.observations,
		Selections: service.selections, Commits: service.commits,
	}
	commitInput, err := resolvePublicationCommitSelectionInputV1(
		ctx, selection, plan.attempt, plan.receipt, plan.index, settlementDigest, selectorConfig,
	)
	if err != nil || domainpublication.ValidatePublicationCommitReceiptExactV1(commit, commitInput) != nil {
		return domainpublication.ReportDeliveryDecisionV1{}, ErrPublicationIntegrity
	}

	securityContext := input.PendingToolCall.SecurityContext
	allowControlled := input.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull
	decision := domainpublication.ReportDeliveryDecisionV1{}
	err = withWitnessedSnapshotAuthorityV2(ctx, service.evidence, securityContext, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		if snapshot.Context != securityContext || !snapshot.Head.HasBundle ||
			snapshot.Head.Bundle.RecordDigest != commit.CommittedEvidenceBundleDigest ||
			snapshot.Head.Bundle.EvidenceRegistryIndexDigest != commit.EvidenceRegistryIndexDigest ||
			snapshot.Head.Bundle.EvidenceRegistryCount != commit.EvidenceRegistryCount ||
			snapshot.Registry.ContextDigest != securityContext.ContextDigest ||
			snapshot.Registry.Sequence != plan.receipt.EvidenceRegistrySequence ||
			snapshot.Registry.StateDigest != plan.receipt.EvidenceRegistryStateDigest {
			return ErrPublicationChanged
		}
		if err := service.validateLedgerSnapshot(ctx, snapshot, input.ReportVariant, input.ClaimLedger, allowControlled); err != nil {
			return err
		}
		if err := service.stages.VerifyReportStageRequest(ctx, lease, stageRequest, service.now().UTC()); err != nil {
			return err
		}
		artifact, err := service.artifacts.ResolveExact(ctx, input.TargetIdentityDigest)
		if err != nil || !bytes.Equal(artifact, input.ReportBytes) || domainsecurity.SHA256Hex(artifact) != commit.ReportSHA256 {
			return ErrPublicationIntegrity
		}
		revalidated := input
		revalidated.ReportBytes = bytes.Clone(artifact)
		if err := service.validateOrdinaryReportSurface(ctx, revalidated); err != nil {
			return err
		}
		controlledMetadata, err := service.validateControlledReportBytes(revalidated)
		if err != nil {
			return err
		}
		controlledMetadata, err = service.validateStoredControlledReportArtifact(ctx, revalidated, controlledMetadata)
		if err != nil {
			return err
		}
		if allowControlled {
			authority, ok := service.piiAuthority.(publicationport.ControlledArtifactPIIAuthorizationAuthority)
			if !ok || authority.ValidateControlledArtifactCurrentWithinSnapshot(
				ctx, securityContext, input.PIIProjection, controlledMetadata, snapshot, capability,
			) != nil {
				return errors.New("controlled report PII authorization is not current within final evidence snapshot")
			}
		}
		candidate, err := domainpublication.NewReportDeliveryDecisionV1(
			domainpublication.ReportDeliveryDecisionInputV1{
				Context: securityContext, StageReceipt: stageReceipt, Attempt: plan.attempt,
				Candidate: plan.receipt, Index: plan.index, Selection: selection, Commit: commit,
				Ledger: input.ClaimLedger, Projection: input.PIIProjection, Inspection: input.RenderInspection,
				CommitInput: commitInput, AuthorityAdvanceSettlementDigest: settlementDigest,
				WitnessedEvidenceAuthorityBundleDigest: snapshot.Head.Bundle.RecordDigest,
				WitnessedEvidenceRegistryIndexDigest:   snapshot.Head.Bundle.EvidenceRegistryIndexDigest,
				EvidenceRegistryCount:                  snapshot.Head.Bundle.EvidenceRegistryCount,
				EvidenceRegistrySequence:               snapshot.Registry.Sequence,
				EvidenceRegistryStateDigest:            snapshot.Registry.StateDigest,
			},
			func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) },
		)
		if err != nil {
			return err
		}
		_, createErr := service.decisions.CreateExclusive(ctx, candidate)
		readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
		defer cancel()
		stored, resolveErr := service.decisions.Resolve(readbackCtx, candidate.DecisionID)
		if resolveErr != nil {
			return errors.Join(ErrPublicationRestartUnresolved, createErr, resolveErr)
		}
		if !reflect.DeepEqual(stored, candidate) {
			return ErrPublicationIntegrity
		}
		decision = stored
		return nil
	})
	if err != nil {
		return domainpublication.ReportDeliveryDecisionV1{}, err
	}
	return decision, nil
}
