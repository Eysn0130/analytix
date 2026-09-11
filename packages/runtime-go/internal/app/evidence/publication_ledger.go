package evidence

import (
	"context"
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// VerifyClaimLedgerForPublication replays every claim against one immutable
// registry snapshot and requires the ledger evidence set to equal the union of
// concrete supporting and counter-evidence references. A valid claim record or
// nonempty citation id alone is never publication authority.
func VerifyClaimLedgerForPublication(
	ctx context.Context,
	registry domainevidence.EvidenceReceiptRegistry,
	securityContext domainsecurity.TurnSecurityContext,
	ledger domainpublication.ClaimLedgerV1,
	allowControlledPII bool,
) error {
	if ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainpublication.ValidateClaimLedgerV1(ledger) != nil || !domainevidence.EvidenceReceiptRegistryMatchesContext(registry, securityContext) ||
		ledger.ThreadID != securityContext.ThreadID || ledger.TurnID != securityContext.TurnID ||
		ledger.ContextDigest != securityContext.ContextDigest || ledger.CaseID != securityContext.CaseID ||
		ledger.CaseBindingHash != securityContext.CaseBindingHash || ledger.ContextEpoch != securityContext.ContextEpoch ||
		ledger.DatasetSnapshotID != securityContext.DatasetSnapshotID || ledger.SourceManifestHash != securityContext.SourceManifestHash {
		return errors.New("claim ledger publication context is invalid")
	}
	snapshot := newSnapshotRegistry(registry)
	used := make(map[string]bool, len(ledger.EvidenceReceiptIDs))
	for _, claim := range ledger.Claims {
		if err := VerifyClaimRecordForPublication(ctx, snapshot, securityContext, claim, allowControlledPII); err != nil {
			return err
		}
		for _, receiptID := range claim.EvidenceIDs {
			used[receiptID] = true
		}
		for _, receiptID := range claim.CounterEvidenceIDs {
			used[receiptID] = true
		}
	}
	if len(used) != len(ledger.EvidenceReceiptIDs) {
		return errors.New("claim ledger evidence set is not exactly claim-bound")
	}
	for _, receiptID := range ledger.EvidenceReceiptIDs {
		registered, err := snapshot.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil || registered.Revoked || !used[receiptID] {
			return errors.New("claim ledger evidence receipt is not current exact membership")
		}
	}
	return nil
}
