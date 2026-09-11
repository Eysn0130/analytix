package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	piiauthorizationapp "analytix.local/runtime-go/internal/app/piiauthorization"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
)

func runtimePublicationDomainOwner(name string) bool {
	switch name {
	case "pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2":
		return true
	default:
		return false
	}
}

// isolateRuntimePublicationGraphFailures runs before recovery effects and
// before opening optional stores. It checks complete prepared cross-owner
// publication inventories, never resolving another CAS from a visitor. A domain
// rejection freezes the entire publication dependency closure; it cannot make
// surviving records authoritative or change any of the rejected owner's bytes.
func isolateRuntimePublicationGraphFailures(
	ctx context.Context,
	owners []runtimePrivateCASOwnerRecovery,
	prepared []runtimePreparedPrivateCASOwnerRecovery,
	installation *finalauthority.AnchoredFileAuthority,
) error {
	if ctx == nil || len(owners) != len(prepared) {
		return errors.New("publication prepared inventory is unavailable")
	}
	indexes := make(map[string]int)
	unavailable := false
	for index, owner := range owners {
		if !runtimePublicationDomainOwner(owner.name) {
			continue
		}
		if _, duplicate := indexes[owner.name]; duplicate || prepared[index] == nil {
			return errors.New("publication prepared inventory is ambiguous")
		}
		indexes[owner.name] = index
		if _, rejected := prepared[index].(*runtimeOptionalDomainBoundary); rejected {
			unavailable = true
		}
	}
	if len(indexes) == 0 {
		return nil
	}
	if len(indexes) != 4 {
		return errors.New("publication prepared dependency closure is incomplete")
	}
	revalidate := func() error {
		var err error
		if installation != nil {
			err = installation.ValidateCurrentInstallation(ctx)
		}
		for index, owner := range owners {
			if runtimePublicationDomainOwner(owner.name) {
				err = errors.Join(err, prepared[index].Revalidate(ctx))
			}
		}
		return errors.Join(err, ctx.Err())
	}
	if err := revalidate(); err != nil {
		return err
	}
	if !unavailable {
		leafPlan := func(ownerName, leaf string) (*finalauthority.PreparedSecurePrivateCASRecoveryV1, error) {
			var selected *finalauthority.PreparedSecurePrivateCASRecoveryV1
			for _, plan := range prepared[indexes[ownerName]].SecurePrivateCASRecoveryPlansV2() {
				if plan != nil && filepath.Base(plan.RootPath()) == leaf {
					if selected != nil {
						return nil, errors.New("publication prepared leaf is ambiguous")
					}
					selected = plan
				}
			}
			if selected == nil {
				return nil, errors.New("publication prepared leaf is unavailable")
			}
			return selected, nil
		}
		visit := func(ownerName, leaf string, collect func(finalauthority.SecurePrivateCASFile)) error {
			selected, err := leafPlan(ownerName, leaf)
			if err != nil {
				return err
			}
			return selected.VisitCommittedFiles(ctx, func(file finalauthority.SecurePrivateCASFile) error {
				collect(file)
				return nil
			})
		}
		visitArtifacts := func(required map[string]bool, read func(finalauthority.SecurePrivateCASFile) error) error {
			selected, err := leafPlan("report-publication", "artifacts")
			if err != nil {
				return err
			}
			return selected.VisitSelectedCommittedFiles(ctx, func(material finalauthority.SecurePrivateCASPreparedMaterialV1) bool {
				return required[material.Digest]
			}, read)
		}
		domainErr, storageErr := validateRuntimePublicationGraphV1(visit, visitArtifacts)
		// Pure graph errors cannot suppress a later physical or shared-key fault.
		if err := errors.Join(storageErr, revalidate()); err != nil {
			return err
		}
		unavailable = domainErr != nil
	}
	if !unavailable {
		return nil
	}
	if installation == nil {
		return errRuntimeOptionalDomainInstallationRequired
	}
	for index, owner := range owners {
		if !runtimePublicationDomainOwner(owner.name) {
			continue
		}
		boundary, err := newRuntimeOptionalDomainBoundary(owner, prepared[index], installation)
		if err != nil {
			return err
		}
		prepared[index] = boundary
	}
	return revalidate()
}

// validateRuntimePublicationGraphV1 checks the same complete dependency graph
// for native prepared inventories and authenticated immutable endpoints.
// Storage/control failures are returned separately from pure domain failures.
func validateRuntimePublicationGraphV1(visit func(string, string, func(finalauthority.SecurePrivateCASFile)) error, visitArtifacts func(map[string]bool, func(finalauthority.SecurePrivateCASFile) error) error) (error, error) {
	grants := make(map[string]domainpii.PIIProjectionGrantV1)
	ledgers := make(map[string]domainpublication.ClaimLedgerV1)
	var accessReceipts []domainpii.ControlledArtifactAccessReceiptV1
	var accessReceiptsV2 []domainpii.ControlledArtifactAccessReceiptV2
	receipts := make(map[string]domainpublication.PublicationReceiptV1)
	commits := make(map[string]domainpublication.PublicationCommitReceiptV1)
	publicationIndexes := make(map[string]domainpublication.PublicationIndexV1)
	projections := make(map[string]domainpublication.PIIProjectionV1)
	inspections := make(map[string]domainpublication.RenderInspectionV1)
	outcomes := make(map[string]domainpublication.ReportDeliveryOutcomeV1)
	artifacts := make(map[string]domainpii.ControlledPIIArtifactMetadataV1)
	var domainErr error
	storageErr := visit("pii-authorization", "grants", func(file finalauthority.SecurePrivateCASFile) {
		grant, err := domainpii.ParsePIIProjectionGrantV1(file.Body)
		if err != nil || grant.RecordDigest != file.Digest {
			domainErr = errors.New("publication prepared grant is invalid")
			return
		}
		grants[grant.RecordDigest] = grant
	})
	storageErr = errors.Join(storageErr, visit("report-publication", "claim-ledgers", func(file finalauthority.SecurePrivateCASFile) {
		ledger, err := domainpublication.ParseClaimLedgerV1(file.Body)
		if err != nil || ledger.LedgerDigest != file.Digest {
			domainErr = errors.New("publication prepared claim ledger is invalid")
			return
		}
		ledgers[ledger.LedgerDigest] = ledger
	}))
	storageErr = errors.Join(storageErr, visit("controlled-artifact-access", "access-receipts", func(file finalauthority.SecurePrivateCASFile) {
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV1(file.Body)
		if err != nil || receipt.AccessID != file.Digest {
			domainErr = errors.New("publication prepared access receipt is invalid")
			return
		}
		accessReceipts = append(accessReceipts, receipt)
	}))
	storageErr = errors.Join(storageErr, visit("controlled-artifact-access-v2", "access-receipts", func(file finalauthority.SecurePrivateCASFile) {
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV2(file.Body)
		if err != nil || receipt.AccessID != file.Digest {
			domainErr = errors.New("publication prepared access V2 receipt is invalid")
			return
		}
		accessReceiptsV2 = append(accessReceiptsV2, receipt)
	}))
	storageErr = errors.Join(storageErr, visit("report-publication", "receipts", func(file finalauthority.SecurePrivateCASFile) {
		record, err := domainpublication.ParsePublicationReceiptV1(file.Body)
		if err != nil || record.RecordDigest != file.Digest {
			domainErr = errors.New("publication prepared receipts record is invalid")
			return
		}
		receipts[file.Digest] = record
	}))
	storageErr = errors.Join(storageErr, visit("report-publication", "commit-receipts", func(file finalauthority.SecurePrivateCASFile) {
		record, err := domainpublication.ParsePublicationCommitReceiptV1(file.Body)
		if err != nil || record.RecordDigest != file.Digest {
			domainErr = errors.New("publication prepared commit-receipts record is invalid")
			return
		}
		commits[file.Digest] = record
	}))
	storageErr = errors.Join(storageErr, visit("report-publication", "indexes", func(file finalauthority.SecurePrivateCASFile) {
		record, err := domainpublication.ParsePublicationIndexV1(file.Body)
		if err != nil || record.IndexDigest != file.Digest {
			domainErr = errors.New("publication prepared indexes record is invalid")
			return
		}
		publicationIndexes[file.Digest] = record
	}))
	storageErr = errors.Join(storageErr, visit("report-publication", "pii-projections", func(file finalauthority.SecurePrivateCASFile) {
		record, err := domainpublication.ParsePIIProjectionV1(file.Body)
		if err != nil || record.ProjectionDigest != file.Digest {
			domainErr = errors.New("publication prepared pii-projections record is invalid")
			return
		}
		projections[file.Digest] = record
	}))
	storageErr = errors.Join(storageErr, visit("report-publication", "render-inspections", func(file finalauthority.SecurePrivateCASFile) {
		record, err := domainpublication.ParseRenderInspectionV1(file.Body)
		if err != nil || record.InspectionDigest != file.Digest {
			domainErr = errors.New("publication prepared render-inspections record is invalid")
			return
		}
		inspections[file.Digest] = record
	}))
	storageErr = errors.Join(storageErr, visit("report-publication", "delivery-projections", func(file finalauthority.SecurePrivateCASFile) {
		outcome, err := domainpublication.ParseReportDeliveryOutcomeV1(file.Body)
		if err != nil || domainpublication.ReportDeliveryOutcomeID(outcome) != file.Digest {
			domainErr = errors.New("publication prepared delivery outcome is invalid")
			return
		}
		outcomes[file.Digest] = outcome
	}))
	requiredArtifacts := make(map[string]bool)
	for _, receipt := range receipts {
		if receipt.PIIProjectionClass == domainpublication.PIIProjectionControlledFull {
			requiredArtifacts[receipt.TargetIdentityDigest] = true
		}
	}
	for _, access := range accessReceipts {
		requiredArtifacts[access.TargetIdentityDigest] = true
	}
	for _, access := range accessReceiptsV2 {
		requiredArtifacts[access.TargetIdentityDigest] = true
	}
	storageErr = errors.Join(storageErr, visitArtifacts(requiredArtifacts, func(file finalauthority.SecurePrivateCASFile) error {
		metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(file.Body)
		if err != nil || metadata.TargetIdentityDigest != file.Digest {
			domainErr = errors.New("publication prepared controlled artifact metadata is invalid")
			return nil
		}
		artifacts[file.Digest] = metadata
		return nil
	}))
	materials := func(receipt domainpublication.PublicationReceiptV1, grant domainpii.PIIProjectionGrantV1, commit domainpublication.PublicationCommitReceiptV1) piiauthorizationapp.StoredControlledPublicationMaterialsV1 {
		return piiauthorizationapp.StoredControlledPublicationMaterialsV1{
			Grant: grant, Receipt: receipt, Commit: commit, Index: publicationIndexes[commit.PublicationIndexDigest],
			Ledger: ledgers[grant.ClaimLedgerDigest], Projection: projections[receipt.PIIProjectionDigest],
			Inspection: inspections[receipt.RenderInspectionDigest], Artifact: artifacts[receipt.TargetIdentityDigest],
		}
	}
	for _, receipt := range receipts {
		if receipt.PIIProjectionClass == domainpublication.PIIProjectionControlledFull &&
			piiauthorizationapp.ValidateStoredControlledPublicationV1(materials(receipt, grants[receipt.AuthorizationAuditDigest], domainpublication.PublicationCommitReceiptV1{})) != nil {
			domainErr = errors.New("publication prepared controlled candidate lost its grant or artifact")
		}
	}
	for _, grant := range grants {
		ledger, found := ledgers[grant.ClaimLedgerDigest]
		if !found || piiauthorizationapp.ValidateStoredGrantClaimLedgerV1(grant, ledger) != nil {
			domainErr = errors.New("publication prepared grant lost its exact claim ledger")
		}
	}
	for _, receipt := range accessReceipts {
		grant, found := grants[receipt.PIIAuthorizationDigest]
		if !found || piiauthorizationapp.ValidateStoredControlledAccessPublicationV1(receipt, materials(receipts[receipt.PublicationReceiptDigest], grant, commits[receipt.PublicationCommitDigest])) != nil {
			domainErr = errors.New("publication prepared access receipt lost its exact publication references")
		}
	}
	for _, receipt := range accessReceiptsV2 {
		grant, found := grants[receipt.PIIAuthorizationDigest]
		if !found || piiauthorizationapp.ValidateStoredControlledAccessOutcomeV2(receipt, outcomes[receipt.DeliveryID], materials(receipts[receipt.PublicationReceiptDigest], grant, commits[receipt.PublicationCommitDigest])) != nil {
			domainErr = errors.New("publication prepared access V2 receipt lost its exact outcome references")
		}
	}

	return domainErr, storageErr
}
