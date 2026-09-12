package reportpublication

import (
	"errors"
	"time"

	piiauthorizationapp "analytix.local/runtime-go/internal/app/piiauthorization"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
)

// ValidateStoredControlledAccessOutcomeV2 checks cross-owner references to an
// actual signed projected outcome. It does not construct VerifiedArtifactDelivery
// or replace the live HistoricalAuthority's complete Core report-stage proof.
func ValidateStoredControlledAccessOutcomeV2(access domainpii.ControlledArtifactAccessReceiptV2, outcome domainpublication.ReportDeliveryOutcomeV1, materials piiauthorizationapp.StoredControlledPublicationMaterialsV1) error {
	if domainpii.ValidateControlledArtifactAccessReceiptForGrantV2(access, materials.Grant) != nil ||
		piiauthorizationapp.ValidateStoredControlledPublicationV1(materials) != nil ||
		domainpublication.ValidatePublicationCommitReceiptMaterialsV1(materials.Commit, materials.Receipt, materials.Index) != nil ||
		domainpublication.ValidateReportDeliveryOutcomeV1(outcome) != nil || outcome.Kind != domainpublication.ReportDeliveryOutcomeProjectedV1 {
		return errors.New("stored controlled access V2 lost its exact projected publication")
	}
	projection := outcome.Projection
	publicationIssuedAt, publicationErr := time.Parse(time.RFC3339Nano, materials.Receipt.IssuedAt)
	requestedAt, requestErr := time.Parse(time.RFC3339Nano, access.RequestedAt)
	if access.DeliveryID != domainpublication.ReportDeliveryOutcomeID(outcome) || access.DeliveryOutcomeRecordDigest != projection.RecordDigest ||
		access.PublicationCommitDigest != projection.CommitRecordDigest || access.PublicationCommitDigest != materials.Commit.RecordDigest ||
		access.PublicationReceiptDigest != materials.Receipt.RecordDigest ||
		access.PIIProjectionDigest != projection.PIIProjectionDigest || access.PIIProjectionDigest != materials.Projection.ProjectionDigest ||
		access.PIIAuthorizationDigest != projection.AuthorizationAuditDigest || access.PIIAuthorizationDigest != materials.Grant.RecordDigest ||
		access.ClaimLedgerDigest != projection.ClaimLedgerDigest || access.ClaimLedgerDigest != materials.Ledger.LedgerDigest ||
		access.TargetIdentityDigest != projection.TargetIdentityDigest || access.TargetIdentityDigest != materials.Receipt.TargetIdentityDigest ||
		access.ArtifactSHA256 != projection.ReportSHA256 || access.ArtifactSHA256 != materials.Receipt.ReportSHA256 ||
		access.ArtifactByteLength != projection.ReportByteLength || access.ArtifactByteLength != materials.Receipt.ReportByteLength ||
		access.MediaType != projection.MediaType || access.MediaType != materials.Receipt.MediaType ||
		projection.PIIProjectionClass != domainpublication.PIIProjectionControlledFull ||
		projection.ThreadID != access.Context.ThreadID || projection.TurnID != access.Context.TurnID || projection.ContextDigest != access.Context.ContextDigest ||
		projection.ContextEpoch != access.Context.ContextEpoch || projection.CaseBindingHash != access.Context.CaseBindingHash ||
		projection.DatasetSnapshotID != access.Context.DatasetSnapshotID || projection.SourceManifestHash != access.Context.SourceManifestHash ||
		publicationErr != nil || requestErr != nil || requestedAt.Before(publicationIssuedAt) {
		return errors.New("stored controlled access V2 does not match its projected publication")
	}
	return nil
}

// StoredControlledOutcomesV1 retains parsed historical outcomes inside their
// owner. It can validate stored references but cannot publish or grant access.
type StoredControlledOutcomesV1 struct {
	outcomes map[string]domainpublication.ReportDeliveryOutcomeV1
}

func (inventory *StoredControlledOutcomesV1) Add(digest string, body []byte) error {
	outcome, err := domainpublication.ParseReportDeliveryOutcomeV1(body)
	if err != nil || domainpublication.ReportDeliveryOutcomeID(outcome) != digest {
		return errors.New("publication prepared delivery outcome is invalid")
	}
	if inventory.outcomes == nil {
		inventory.outcomes = make(map[string]domainpublication.ReportDeliveryOutcomeV1)
	}
	if _, exists := inventory.outcomes[digest]; exists {
		return errors.New("publication prepared delivery outcome is duplicated")
	}
	inventory.outcomes[digest] = outcome
	return nil
}

func (inventory *StoredControlledOutcomesV1) ValidateAccess(access domainpii.ControlledArtifactAccessReceiptV2, materials piiauthorizationapp.StoredControlledPublicationMaterialsV1) error {
	return ValidateStoredControlledAccessOutcomeV2(access, inventory.outcomes[access.DeliveryID], materials)
}
