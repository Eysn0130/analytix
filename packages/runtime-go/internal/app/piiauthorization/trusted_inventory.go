package piiauthorization

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// VerifyTrustedInventoryV1 upgrades canonical self-signed grants to
// installation-trusted authority and verifies their cross-owner claim-ledger
// references. Historical expiry is intentionally not a startup error; every
// attempted use still revalidates expiry, approval, context, and evidence.
func VerifyTrustedInventoryV1(
	ctx context.Context,
	inventory piiauthorizationport.InventoryStore,
	ledgers publicationport.ClaimLedgerStore,
	authority finalauthorityport.Authority,
) error {
	if ctx == nil || inventory == nil || ledgers == nil || authority == nil {
		return errors.New("PII authorization trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Finish the CAS visit before resolving records owned by another store.
	var snapshot []domainpii.PIIProjectionGrantV1
	if err := inventory.VisitGrants(ctx, func(grant domainpii.PIIProjectionGrantV1) error {
		snapshot = append(snapshot, grant)
		return nil
	}); err != nil {
		return fmt.Errorf("verify PII authorization trusted inventory: %w", err)
	}
	validate := func(grant domainpii.PIIProjectionGrantV1) error {
		keyID, publicKey, signature, err := domainpii.PIIProjectionGrantAuthorityMaterialV1(grant)
		if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainpii.PIIProjectionGrantSigningBytesV1(grant), signature) != nil {
			return errors.New("PII projection grant lacks trusted installation authority")
		}
		ledger, err := ledgers.Resolve(ctx, grant.ClaimLedgerDigest)
		if err != nil {
			return errors.New("PII projection grant claim ledger is missing")
		}
		return ValidateStoredGrantClaimLedgerV1(grant, ledger)
	}
	for _, grant := range snapshot {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validate(grant); err != nil {
			return fmt.Errorf("verify PII authorization trusted inventory: %w", err)
		}
	}
	return nil
}

// VerifyControlledPublicationInventoryV1 closes the cross-owner graph from a
// controlled PublicationReceipt through its PIIProjection and exact signed
// grant to the claim ledger. It validates historical publication time against
// the grant window without making that historical grant reusable now.
func VerifyControlledPublicationInventoryV1(
	ctx context.Context,
	receipts publicationport.ReceiptInventoryStore,
	projections publicationport.PIIProjectionStore,
	ledgers publicationport.ClaimLedgerStore,
	grants piiauthorizationport.Store,
	authority finalauthorityport.Authority,
) error {
	if ctx == nil || receipts == nil || projections == nil || ledgers == nil || grants == nil || authority == nil {
		return errors.New("controlled publication trusted inventory dependencies are required")
	}
	// Finish the CAS visit before resolving records owned by another store.
	var snapshot []domainpublication.PublicationReceiptV1
	if err := receipts.VisitReceipts(ctx, func(receipt domainpublication.PublicationReceiptV1) error {
		snapshot = append(snapshot, receipt)
		return nil
	}); err != nil {
		return fmt.Errorf("verify controlled publication inventory: %w", err)
	}
	validate := func(receipt domainpublication.PublicationReceiptV1) error {
		if receipt.PIIProjectionClass != domainpublication.PIIProjectionControlledFull {
			return nil
		}
		projection, err := projections.Resolve(ctx, receipt.PIIProjectionDigest)
		if err != nil || domainpublication.ValidatePIIProjectionV1(projection) != nil {
			return errors.New("controlled publication PII projection is missing")
		}
		grant, err := grants.ResolveGrant(ctx, receipt.AuthorizationAuditDigest)
		if err != nil {
			return errors.New("controlled publication PII grant is missing")
		}
		keyID, publicKey, signature, err := domainpii.PIIProjectionGrantAuthorityMaterialV1(grant)
		if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainpii.PIIProjectionGrantSigningBytesV1(grant), signature) != nil {
			return errors.New("controlled publication PII grant is untrusted")
		}
		ledger, err := ledgers.Resolve(ctx, receipt.ClaimLedgerDigest)
		if err != nil || ValidateStoredGrantClaimLedgerV1(grant, ledger) != nil ||
			!controlledPublicationReceiptMatchesGrantV1(receipt, projection, grant) {
			return errors.New("controlled publication lost its exact grant or claim ledger binding")
		}
		return nil
	}
	for _, receipt := range snapshot {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validate(receipt); err != nil {
			return fmt.Errorf("verify controlled publication inventory: %w", err)
		}
	}
	return nil
}

func controlledPublicationReceiptMatchesGrantV1(
	receipt domainpublication.PublicationReceiptV1,
	projection domainpublication.PIIProjectionV1,
	grant domainpii.PIIProjectionGrantV1,
) bool {
	receiptIssuedAt, receiptErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	grantIssuedAt, grantIssueErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	grantExpiresAt, grantExpiryErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	return domainpublication.ValidatePublicationReceiptV1(receipt) == nil &&
		domainpublication.ValidatePIIProjectionV1(projection) == nil && domainpii.ValidatePIIProjectionGrantV1(grant) == nil &&
		receipt.AuthorizationAuditDigest == grant.RecordDigest && projection.AuthorizationAuditDigest == grant.RecordDigest &&
		receipt.PIIProjectionDigest == projection.ProjectionDigest && receipt.PIIProjectionClass == projection.ProjectionClass &&
		receipt.ClaimLedgerDigest == grant.ClaimLedgerDigest && receipt.ReportSHA256 == grant.ProjectedContentSHA256 &&
		projection.ProjectedContentSHA256 == grant.ProjectedContentSHA256 && projection.RulesetHash == grant.ProjectionRulesetHash &&
		projection.RestrictedFieldCount == grant.PreservedControlledFieldCount &&
		projection.PreservedControlledFieldCount == grant.PreservedControlledFieldCount &&
		receipt.TargetIdentityDigest == grant.TargetIdentityDigest && receipt.ThreadID == grant.Context.ThreadID &&
		receipt.TurnID == grant.Context.TurnID && receipt.ContextDigest == grant.Context.ContextDigest &&
		receipt.CaseID == grant.Context.CaseID && receipt.CaseBindingHash == grant.Context.CaseBindingHash &&
		receipt.ContextEpoch == grant.Context.ContextEpoch && receipt.DatasetSnapshotID == grant.Context.DatasetSnapshotID &&
		receipt.SourceManifestHash == grant.Context.SourceManifestHash && receiptErr == nil && grantIssueErr == nil && grantExpiryErr == nil &&
		!receiptIssuedAt.Before(grantIssuedAt) && receiptIssuedAt.Before(grantExpiresAt)
}

// ValidateStoredGrantClaimLedgerV1 is a historical integrity check. It does
// not make an expired grant current and does not replace live approval,
// evidence-registry, or context validation at publication time.
func ValidateStoredGrantClaimLedgerV1(
	grant domainpii.PIIProjectionGrantV1,
	ledger domainpublication.ClaimLedgerV1,
) error {
	if domainpii.ValidatePIIProjectionGrantV1(grant) != nil || domainpublication.ValidateClaimLedgerV1(ledger) != nil ||
		grant.ClaimLedgerDigest != ledger.LedgerDigest || grant.Context.ThreadID != ledger.ThreadID ||
		grant.Context.TurnID != ledger.TurnID || grant.Context.ContextDigest != ledger.ContextDigest ||
		grant.Context.CaseID != ledger.CaseID || grant.Context.CaseBindingHash != ledger.CaseBindingHash ||
		grant.Context.ContextEpoch != ledger.ContextEpoch || grant.Context.DatasetSnapshotID != ledger.DatasetSnapshotID ||
		grant.Context.SourceManifestHash != ledger.SourceManifestHash {
		return ErrIntegrity
	}
	if err := validateLedgerFieldBindingsV1(ledger, grant.FieldBindings); err != nil {
		return errors.Join(ErrIntegrity, err)
	}
	return nil
}
