package evidence

import (
	"context"
	"errors"
	"reflect"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// VerifyClaimRecordForPublication replays one already normalized claim
// against current registry membership. Controlled PII may be admitted only
// after the report-publication use case independently validates its current
// controlled-artifact authorization.
func VerifyClaimRecordForPublication(
	ctx context.Context,
	registry registryport.Registry,
	securityContext domainsecurity.TurnSecurityContext,
	claim domainevidence.ClaimRecord,
	allowControlledPII bool,
) error {
	if ctx == nil || registry == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainevidence.ValidateClaimRecord(claim) != nil {
		return errors.New("publication claim authority is invalid")
	}
	switch claim.SupportState {
	case domainevidence.ClaimUnresolved:
		return nil
	case domainevidence.ClaimVerified, domainevidence.ClaimPartial:
		return verifySupportedPublicationClaim(ctx, registry, securityContext, claim, allowControlledPII)
	case domainevidence.ClaimRefuted:
		return verifyRefutedPublicationClaim(ctx, registry, securityContext, claim, allowControlledPII)
	default:
		return errors.New("publication claim support state is invalid")
	}
}

func verifySupportedPublicationClaim(ctx context.Context, registry registryport.Registry, securityContext domainsecurity.TurnSecurityContext, claim domainevidence.ClaimRecord, allowControlledPII bool) error {
	if len(claim.EvidenceIDs) == 0 || len(claim.CounterEvidenceIDs) != 0 {
		return errors.New("supported publication claim evidence set is invalid")
	}
	proposal := domainevidence.ClaimProposal{ClaimType: claim.ClaimType, NormalizedPayload: claim.NormalizedPayload}
	hasPartial := false
	var derivedScope *domainevidence.EvidenceQueryRange
	for _, receiptID := range claim.EvidenceIDs {
		registered, err := registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil || registered.Revoked || !domainevidence.SourceTypeSupportsClaim(registered.Receipt.SourceType, claim.ClaimType) ||
			!receiptMetadataSupportsClaim(registered.Receipt, proposal) || !publicationPIIAllowed(registered.Receipt.PIIClassification, allowControlledPII) {
			return errors.New("supported publication claim receipt is invalid")
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil || !canonicalMaterialSupportsClaim(material, claim) || !claimScopeSupportsPayload(registered.Receipt.QueryRange, claim.NormalizedPayload) {
			return errors.New("supported publication claim payload is not evidenced")
		}
		scope := registered.Receipt.QueryRange
		if derivedScope == nil {
			derivedScope = &scope
		} else if !reflect.DeepEqual(*derivedScope, scope) {
			return errors.New("supported publication claim receipts have different scopes")
		}
		hasPartial = hasPartial || registered.Receipt.PaginationCompleteness != domainevidence.PaginationComplete
	}
	if derivedScope == nil || claim.SupportedScope == nil || !reflect.DeepEqual(*claim.SupportedScope, *derivedScope) ||
		(claim.SupportState == domainevidence.ClaimPartial) != hasPartial {
		return errors.New("supported publication claim state does not match coverage")
	}
	return nil
}

func verifyRefutedPublicationClaim(ctx context.Context, registry registryport.Registry, securityContext domainsecurity.TurnSecurityContext, claim domainevidence.ClaimRecord, allowControlledPII bool) error {
	if len(claim.EvidenceIDs) != 0 || len(claim.CounterEvidenceIDs) == 0 {
		return errors.New("refuted publication claim counterevidence set is invalid")
	}
	proposal := domainevidence.ClaimProposal{ClaimType: claim.ClaimType, NormalizedPayload: claim.NormalizedPayload}
	for _, receiptID := range claim.CounterEvidenceIDs {
		registered, err := registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil || registered.Revoked || !domainevidence.SourceTypeSupportsClaim(registered.Receipt.SourceType, claim.ClaimType) ||
			!receiptMetadataSupportsClaim(registered.Receipt, proposal) || !publicationPIIAllowed(registered.Receipt.PIIClassification, allowControlledPII) {
			return errors.New("refuted publication claim counterevidence is invalid")
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil {
			return err
		}
		matched := false
		for _, fact := range material.Facts {
			if fact.ClaimType == claim.ClaimType && sameClaimSubject(fact.NormalizedPayload, claim.NormalizedPayload) &&
				!domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, claim.NormalizedPayload) &&
				claimScopeSupportsPayload(registered.Receipt.QueryRange, fact.NormalizedPayload) {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("refuted publication claim lacks exact counterevidence")
		}
	}
	return nil
}

func publicationPIIAllowed(classification domainevidence.PIIClassification, allowControlled bool) bool {
	if ordinaryChatPIIAllowed(classification) {
		return true
	}
	return allowControlled && (classification == domainevidence.PIIRestricted || classification == domainevidence.PIIControlled)
}
