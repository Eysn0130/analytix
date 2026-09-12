package piiauthorization

import (
	"errors"
	"reflect"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
)

// StoredControlledPublicationMaterialsV1 is an untrusted in-memory inventory
// snapshot. Validating its historical bindings grants no current publication,
// controlled-access, approval, or replay authority.
type StoredControlledPublicationMaterialsV1 struct {
	Grant      domainpii.PIIProjectionGrantV1
	Receipt    domainpublication.PublicationReceiptV1
	Commit     domainpublication.PublicationCommitReceiptV1
	Index      domainpublication.PublicationIndexV1
	Ledger     domainpublication.ClaimLedgerV1
	Projection domainpublication.PIIProjectionV1
	Inspection domainpublication.RenderInspectionV1
	Artifact   domainpii.ControlledPIIArtifactMetadataV1
}

// ValidateStoredControlledPublicationV1 validates a controlled candidate and
// its exact grant, ledger and protected artifact, including historical time.
// A candidate need not yet have a committed publication or access reservation.
func ValidateStoredControlledPublicationV1(materials StoredControlledPublicationMaterialsV1) error {
	grant, receipt, artifact := materials.Grant, materials.Receipt, materials.Artifact
	if receipt.PIIProjectionClass != domainpublication.PIIProjectionControlledFull ||
		ValidateStoredGrantClaimLedgerV1(grant, materials.Ledger) != nil ||
		!controlledPublicationReceiptMatchesGrantV1(receipt, materials.Projection, grant) ||
		domainpublication.ValidatePublicationReceiptMaterialsV1(receipt, materials.Ledger, materials.Projection, materials.Inspection) != nil {
		return errors.New("stored controlled publication lost its exact grant or report materials")
	}
	publicationIssuedAt, publicationTimeErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	artifactRenderedAt, artifactTimeErr := time.Parse(time.RFC3339Nano, artifact.RenderedAt)
	grantIssuedAt, grantTimeErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	if artifact.SHA256 != receipt.ReportSHA256 || artifact.ByteLength != receipt.ReportByteLength || artifact.MediaType != receipt.MediaType ||
		artifact.Context != grant.Context || artifact.RequesterUserID != grant.RequesterUserID ||
		artifact.DisclosurePurpose != grant.DisclosurePurpose || artifact.DeliveryScope != grant.DeliveryScope ||
		artifact.ClaimLedgerDigest != grant.ClaimLedgerDigest || artifact.ProjectionRulesetHash != grant.ProjectionRulesetHash ||
		artifact.TargetIdentityDigest != grant.TargetIdentityDigest ||
		artifact.PreservedControlledFieldCount != grant.PreservedControlledFieldCount || artifact.FieldBindingSetDigest != grant.FieldBindingSetDigest ||
		!reflect.DeepEqual(artifact.FieldBindings, grant.FieldBindings) ||
		publicationTimeErr != nil || artifactTimeErr != nil || grantTimeErr != nil ||
		artifactRenderedAt.After(grantIssuedAt) || publicationIssuedAt.Before(artifactRenderedAt) {
		return errors.New("stored controlled publication lost its protected artifact binding")
	}
	return nil
}

func ValidateStoredControlledAccessPublicationV1(access domainpii.ControlledArtifactAccessReceiptV1, materials StoredControlledPublicationMaterialsV1) error {
	if domainpii.ValidateControlledArtifactAccessReceiptForGrantV1(access, materials.Grant) != nil ||
		ValidateStoredControlledPublicationV1(materials) != nil ||
		domainpublication.ValidatePublicationCommitReceiptMaterialsV1(materials.Commit, materials.Receipt, materials.Index) != nil {
		return errors.New("stored controlled access lost its exact committed publication")
	}
	publicationIssuedAt, publicationErr := time.Parse(time.RFC3339Nano, materials.Receipt.IssuedAt)
	requestedAt, requestErr := time.Parse(time.RFC3339Nano, access.RequestedAt)
	if access.PublicationCommitDigest != materials.Commit.RecordDigest || access.PublicationReceiptDigest != materials.Receipt.RecordDigest ||
		access.PIIProjectionDigest != materials.Projection.ProjectionDigest || access.PIIAuthorizationDigest != materials.Grant.RecordDigest ||
		access.ClaimLedgerDigest != materials.Ledger.LedgerDigest || access.TargetIdentityDigest != materials.Receipt.TargetIdentityDigest ||
		access.ArtifactSHA256 != materials.Receipt.ReportSHA256 || access.ArtifactByteLength != materials.Receipt.ReportByteLength ||
		access.MediaType != materials.Receipt.MediaType || publicationErr != nil || requestErr != nil || requestedAt.Before(publicationIssuedAt) {
		return errors.New("stored controlled access does not match its committed publication")
	}
	return nil
}
