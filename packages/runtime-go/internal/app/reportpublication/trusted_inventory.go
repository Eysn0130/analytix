package reportpublication

import (
	"context"
	"errors"
	"fmt"

	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// VerifyTrustedInventoryV1 pins every self-contained report publication
// signature to the current installation authority and rejects mixed
// installation/enrollment lineages. Recovery graph validation remains
// responsible for material and commit references.
func VerifyTrustedInventoryV1(
	ctx context.Context,
	attempts publicationport.AttemptInventoryStore,
	receipts publicationport.ReceiptInventoryStore,
	indexes publicationport.IndexInventoryStore,
	selections publicationport.CommitSelectionInventoryStore,
	commits publicationport.CommitReceiptInventoryStore,
	decisions publicationport.DeliveryDecisionInventoryStore,
	grantSettlements publicationport.ReportGrantSettlementInventoryStore,
	stageCompletions publicationport.ReportStageCompletionInventoryStore,
	deliveryOutcomes publicationport.DeliveryOutcomeInventoryStore,
	authority finalauthorityport.Authority,
) error {
	if ctx == nil || attempts == nil || receipts == nil || indexes == nil || selections == nil || commits == nil || decisions == nil || grantSettlements == nil || stageCompletions == nil || deliveryOutcomes == nil || authority == nil {
		return errors.New("report publication trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	type lineage struct{ installationID, enrollmentID string }
	var observed lineage
	validateLineage := func(installationID, enrollmentID string) error {
		candidate := lineage{installationID: installationID, enrollmentID: enrollmentID}
		if observed == (lineage{}) {
			observed = candidate
			return nil
		}
		if candidate != observed {
			return errors.New("report publication inventory mixes installation or enrollment lineages")
		}
		return nil
	}
	verify := func(keyID string, publicKey, signingBytes, signature []byte) error {
		if authority.VerifyTrusted(ctx, keyID, publicKey, signingBytes, signature) != nil {
			return errors.New("report publication record lacks trusted installation authority")
		}
		return nil
	}
	if err := attempts.VisitAttempts(ctx, func(attempt domainpublication.PublicationAttemptV1) error {
		keyID, publicKey, signature, err := domainpublication.PublicationAttemptAuthorityMaterialV1(attempt)
		if err != nil || verify(keyID, publicKey, domainpublication.PublicationAttemptSigningBytesV1(attempt), signature) != nil {
			return errors.New("publication attempt is untrusted")
		}
		return validateLineage(attempt.InstallationID, attempt.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify publication attempt inventory: %w", err)
	}
	if err := receipts.VisitReceipts(ctx, func(receipt domainpublication.PublicationReceiptV1) error {
		keyID, publicKey, signature, err := domainpublication.PublicationReceiptAuthorityMaterialV1(receipt)
		if err != nil || verify(keyID, publicKey, domainpublication.PublicationReceiptSigningBytesV1(receipt), signature) != nil {
			return errors.New("publication receipt is untrusted")
		}
		return validateLineage(receipt.InstallationID, receipt.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify publication receipt inventory: %w", err)
	}
	if err := indexes.VisitIndexes(ctx, func(index domainpublication.PublicationIndexV1) error {
		keyID, publicKey, signature, err := domainpublication.PublicationIndexAuthorityMaterialV1(index)
		if err != nil || verify(keyID, publicKey, domainpublication.PublicationIndexSigningBytesV1(index), signature) != nil {
			return errors.New("publication index is untrusted")
		}
		return validateLineage(index.InstallationID, index.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify publication index inventory: %w", err)
	}
	if err := selections.VisitCommitSelections(ctx, func(selection domainpublication.PublicationCommitSelectionV1) error {
		keyID, publicKey, signature, err := domainpublication.PublicationCommitSelectionAuthorityMaterialV1(selection)
		if err != nil || verify(keyID, publicKey, domainpublication.PublicationCommitSelectionSigningBytesV1(selection), signature) != nil {
			return errors.New("publication commit selection is untrusted")
		}
		return validateLineage(selection.InstallationID, selection.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify publication commit selection inventory: %w", err)
	}
	if err := commits.VisitCommitReceipts(ctx, func(commit domainpublication.PublicationCommitReceiptV1) error {
		keyID, publicKey, signature, err := domainpublication.PublicationCommitReceiptAuthorityMaterialV1(commit)
		if err != nil || verify(keyID, publicKey, domainpublication.PublicationCommitReceiptSigningBytesV1(commit), signature) != nil {
			return errors.New("publication commit receipt is untrusted")
		}
		return validateLineage(commit.InstallationID, commit.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify publication commit inventory: %w", err)
	}
	if err := decisions.VisitDeliveryDecisions(ctx, func(decision domainpublication.ReportDeliveryDecisionV1) error {
		keyID, publicKey, signature, err := domainpublication.ReportDeliveryDecisionAuthorityMaterialV1(decision)
		if err != nil || verify(keyID, publicKey, domainpublication.ReportDeliveryDecisionSigningBytesV1(decision), signature) != nil {
			return errors.New("report delivery decision is untrusted")
		}
		return validateLineage(decision.InstallationID, decision.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify report delivery decision inventory: %w", err)
	}
	if err := grantSettlements.VisitGrantSettlements(ctx, func(settlement domainpublication.ReportGrantSettlementV1) error {
		keyID, publicKey, signature, err := domainpublication.ReportGrantSettlementAuthorityMaterialV1(settlement)
		if err != nil || verify(keyID, publicKey, domainpublication.ReportGrantSettlementSigningBytesV1(settlement), signature) != nil {
			return errors.New("report grant settlement is untrusted")
		}
		return validateLineage(settlement.InstallationID, settlement.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify report grant settlement inventory: %w", err)
	}
	if err := stageCompletions.VisitStageCompletions(ctx, func(completion domainpublication.ReportStageCompletionV1) error {
		keyID, publicKey, signature, err := domainpublication.ReportStageCompletionAuthorityMaterialV1(completion)
		if err != nil || verify(keyID, publicKey, domainpublication.ReportStageCompletionSigningBytesV1(completion), signature) != nil {
			return errors.New("report stage completion is untrusted")
		}
		return validateLineage(completion.InstallationID, completion.EnrollmentID)
	}); err != nil {
		return fmt.Errorf("verify report stage completion inventory: %w", err)
	}
	if err := deliveryOutcomes.VisitDeliveryOutcomes(ctx, func(outcome domainpublication.ReportDeliveryOutcomeV1) error {
		if domainpublication.ValidateReportDeliveryOutcomeV1(outcome) != nil {
			return errors.New("report delivery outcome is malformed or untrusted")
		}
		switch outcome.Kind {
		case domainpublication.ReportDeliveryOutcomeProjectedV1:
			projection := *outcome.Projection
			keyID, publicKey, signature, err := domainpublication.ReportDeliveryProjectionAuthorityMaterialV1(projection)
			if err != nil || verify(keyID, publicKey, domainpublication.ReportDeliveryProjectionSigningBytesV1(projection), signature) != nil {
				return errors.New("projected report delivery outcome is untrusted")
			}
			return validateLineage(projection.InstallationID, projection.EnrollmentID)
		case domainpublication.ReportDeliveryOutcomeRejectedV1:
			rejection := *outcome.Rejection
			keyID, publicKey, signature, err := domainpublication.ReportDeliveryRejectionAuthorityMaterialV1(rejection)
			if err != nil || verify(keyID, publicKey, domainpublication.ReportDeliveryRejectionSigningBytesV1(rejection), signature) != nil {
				return errors.New("rejected report delivery outcome is untrusted")
			}
			return validateLineage(rejection.InstallationID, rejection.EnrollmentID)
		default:
			return errors.New("report delivery outcome kind is untrusted")
		}
	}); err != nil {
		return fmt.Errorf("verify report delivery outcome inventory: %w", err)
	}
	return nil
}
