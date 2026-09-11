package evidence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type ClaimIDGenerator func() (string, error)

type ClaimVerifier struct {
	Registry    registryport.Registry
	IDGenerator ClaimIDGenerator
	Now         func() time.Time
}

func (verifier ClaimVerifier) Verify(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, proposal domainevidence.ClaimProposal) (domainevidence.ClaimRecord, error) {
	proposal, err := domainevidence.NormalizeClaimProposal(proposal)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return domainevidence.ClaimRecord{}, errors.New("claim verification input is invalid")
	}
	claimID, err := verifier.claimID()
	if err != nil {
		return domainevidence.ClaimRecord{}, err
	}
	verifiedAt := time.Now().UTC()
	if verifier.Now != nil {
		verifiedAt = verifier.Now().UTC()
	}
	if proposal.ClaimType == domainevidence.ClaimLegalCharacterization {
		return verifier.unresolvedRecord(claimID, proposal, true, "legal_characterization_requires_evidence_combination_and_human_review", verifiedAt)
	}
	if verifier.Registry == nil || len(proposal.EvidenceIDs) == 0 {
		return verifier.unresolvedRecord(claimID, proposal, false, "current_host_evidence_receipt_required", verifiedAt)
	}
	supporting := verifier.matchingEvidence(ctx, securityContext, proposal, proposal.EvidenceIDs, true)
	counter := verifier.matchingEvidence(ctx, securityContext, proposal, proposal.CounterEvidenceIDs, false)
	if len(counter) > 0 {
		counterIDs := registeredReceiptIDs(counter)
		verifierReceiptID := domainevidence.VerifierReceiptDigest(claimID, proposal.ClaimType, proposal.NormalizedPayload, nil, counterIDs, domainevidence.ClaimRefuted)
		return domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
			ClaimID: claimID, Proposal: proposal, SupportState: domainevidence.ClaimRefuted,
			EvidenceIDs: []string{}, CounterEvidenceIDs: counterIDs, AllowedWording: []string{},
			ProhibitedUpgrades: []string{"publish_as_verified", "ignore_counterevidence"}, RequiresHumanReview: false,
			VerifierReceiptID: verifierReceiptID, VerificationReason: "current_counterevidence_refutes_claim", VerifiedAt: verifiedAt,
		})
	}
	if len(supporting) == 0 {
		return verifier.unresolvedRecord(claimID, proposal, false, "citation_missing_mismatched_or_source_unauthorized", verifiedAt)
	}
	for _, evidence := range supporting[1:] {
		if !reflect.DeepEqual(evidence.Receipt.QueryRange, supporting[0].Receipt.QueryRange) {
			return verifier.unresolvedRecord(claimID, proposal, false, "evidence_receipts_do_not_share_one_exact_scope", verifiedAt)
		}
	}
	supportState := domainevidence.ClaimVerified
	allowedWording := []string{"exact_verified_fact"}
	reason := "exact_current_evidence_support"
	for _, evidence := range supporting {
		if evidence.Receipt.PaginationCompleteness != domainevidence.PaginationComplete {
			supportState = domainevidence.ClaimPartial
			allowedWording = []string{"within_checked_scope_only"}
			reason = "exact_evidence_with_partial_coverage"
			break
		}
	}
	evidenceIDs := registeredReceiptIDs(supporting)
	verifierReceiptID := domainevidence.VerifierReceiptDigest(claimID, proposal.ClaimType, proposal.NormalizedPayload, evidenceIDs, nil, supportState)
	scope := supporting[0].Receipt.QueryRange
	prohibited := []string{"legal_characterization_without_review", "zero_or_nonexistence_upgrade"}
	if supportState == domainevidence.ClaimPartial {
		prohibited = append(prohibited, "whole_case_conclusion", "scope_broadening")
	}
	return domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: claimID, Proposal: proposal, SupportState: supportState, EvidenceIDs: evidenceIDs,
		CounterEvidenceIDs: []string{}, SupportedScope: &scope, AllowedWording: allowedWording,
		ProhibitedUpgrades: prohibited, RequiresHumanReview: false, VerifierReceiptID: verifierReceiptID,
		VerificationReason: reason, VerifiedAt: verifiedAt,
	})
}

func (verifier ClaimVerifier) matchingEvidence(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, proposal domainevidence.ClaimProposal, receiptIDs []string, supporting bool) []domainevidence.RegisteredEvidence {
	matches := []domainevidence.RegisteredEvidence{}
	for _, receiptID := range receiptIDs {
		registered, err := verifier.Registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil || registered.Revoked || !domainevidence.SourceTypeSupportsClaim(registered.Receipt.SourceType, proposal.ClaimType) ||
			!receiptMetadataSupportsClaim(registered.Receipt, proposal) || !ordinaryChatPIIAllowed(registered.Receipt.PIIClassification) {
			continue
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil {
			continue
		}
		matched := false
		for _, fact := range material.Facts {
			if fact.ClaimType != proposal.ClaimType || !claimScopeSupportsPayload(registered.Receipt.QueryRange, proposal.NormalizedPayload) {
				continue
			}
			if supporting && domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, proposal.NormalizedPayload) {
				matched = true
				break
			}
			if !supporting && sameClaimSubject(fact.NormalizedPayload, proposal.NormalizedPayload) &&
				!domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, proposal.NormalizedPayload) {
				matched = true
				break
			}
		}
		if matched {
			matches = append(matches, registered)
		}
	}
	return matches
}

func (verifier ClaimVerifier) unresolvedRecord(claimID string, proposal domainevidence.ClaimProposal, requiresHumanReview bool, reason string, verifiedAt time.Time) (domainevidence.ClaimRecord, error) {
	return domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: claimID, Proposal: proposal, SupportState: domainevidence.ClaimUnresolved,
		EvidenceIDs: []string{}, CounterEvidenceIDs: []string{}, AllowedWording: []string{},
		ProhibitedUpgrades:  []string{"publish_as_fact", "whole_case_conclusion", "zero_or_nonexistence_upgrade", "legal_characterization"},
		RequiresHumanReview: requiresHumanReview, VerificationReason: reason, VerifiedAt: verifiedAt,
	})
}

func (verifier ClaimVerifier) claimID() (string, error) {
	if verifier.IDGenerator != nil {
		claimID, err := verifier.IDGenerator()
		if err != nil || strings.TrimSpace(claimID) == "" {
			if err == nil {
				err = errors.New("claim id generator returned an empty id")
			}
			return "", err
		}
		return strings.TrimSpace(claimID), nil
	}
	body := make([]byte, 16)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return "clm_" + hex.EncodeToString(body), nil
}

func registeredReceiptIDs(evidence []domainevidence.RegisteredEvidence) []string {
	ids := make([]string, 0, len(evidence))
	for _, registered := range evidence {
		ids = append(ids, registered.Receipt.ReceiptID)
	}
	return ids
}

func sameClaimSubject(left domainevidence.NormalizedClaimPayload, right domainevidence.NormalizedClaimPayload) bool {
	return left.SubjectID == right.SubjectID && left.EntityID == right.EntityID && left.AccountID == right.AccountID &&
		left.CounterpartyID == right.CounterpartyID
}

func claimScopeSupportsPayload(scope domainevidence.EvidenceQueryRange, payload domainevidence.NormalizedClaimPayload) bool {
	return domainevidence.EvidenceQueryRangeSupportsClaimPayload(scope, payload)
}

func receiptMetadataSupportsClaim(receipt domainevidence.EvidenceReceipt, proposal domainevidence.ClaimProposal) bool {
	return domainevidence.EvidenceReceiptMetadataSupportsClaim(receipt, proposal.ClaimType, proposal.NormalizedPayload)
}
