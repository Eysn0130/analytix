package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type RegistryPublicationCandidates struct {
	Claims          []domainevidence.ClaimRecord
	NoHitReceiptIDs []string
}

// VerifiedClaimsFromRegistrySnapshot deterministically converts only current,
// exact canonical facts into claim records. It never consumes provider prose,
// candidate citations, or self-reported support flags.
func VerifiedClaimsFromRegistrySnapshot(ctx context.Context, registry registryport.Registry, securityContext domainsecurity.TurnSecurityContext, verifiedAt time.Time) ([]domainevidence.ClaimRecord, error) {
	candidates, err := VerifiedPublicationCandidatesFromRegistrySnapshot(ctx, registry, securityContext, verifiedAt)
	return candidates.Claims, err
}

// VerifiedPublicationCandidatesFromRegistrySnapshot derives both factual
// claims and scoped no-hit candidates exclusively from the locked host
// registry. An empty canonical material is eligible only when the receipt
// proves complete pagination and zero source-record lineage.
func VerifiedPublicationCandidatesFromRegistrySnapshot(ctx context.Context, registry registryport.Registry, securityContext domainsecurity.TurnSecurityContext, verifiedAt time.Time) (RegistryPublicationCandidates, error) {
	if registry == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil || verifiedAt.UTC().IsZero() {
		return RegistryPublicationCandidates{}, errors.New("registry claim projection authority is unavailable")
	}
	snapshot, err := registry.Replay(ctx, securityContext)
	if err != nil {
		return RegistryPublicationCandidates{}, err
	}
	claims := []domainevidence.ClaimRecord{}
	noHitReceiptIDs := []string{}
	seenFacts := map[string]bool{}
	seenNoHit := map[string]bool{}
	for _, entry := range snapshot.Entries {
		if entry.Operation != domainevidence.EvidenceRegistryIssue {
			continue
		}
		registered, err := registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: entry.ReceiptID})
		if err != nil || registered.Revoked {
			continue
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil {
			return RegistryPublicationCandidates{}, err
		}
		if len(material.Facts) == 0 && registered.Receipt.PaginationCompleteness == domainevidence.PaginationComplete &&
			len(registered.Receipt.SourceRecordIDs) == 0 && ordinaryChatPIIAllowed(registered.Receipt.PIIClassification) &&
			!seenNoHit[registered.Receipt.ReceiptID] {
			seenNoHit[registered.Receipt.ReceiptID] = true
			noHitReceiptIDs = append(noHitReceiptIDs, registered.Receipt.ReceiptID)
		}
		for _, fact := range material.Facts {
			if fact.ClaimType == domainevidence.ClaimLegalCharacterization {
				continue
			}
			if registered.Receipt.ToolName == fundsAccountFlowCanonicalTool {
				if (fact.ClaimType != domainevidence.ClaimAmount && fact.ClaimType != domainevidence.ClaimCount) ||
					fact.NormalizedPayload.Granularity != accountFlowReceiptGranularity {
					continue
				}
			}
			factBody, _ := json.Marshal(struct {
				ClaimType domainevidence.ClaimType              `json:"claimType"`
				Payload   domainevidence.NormalizedClaimPayload `json:"payload"`
			}{ClaimType: fact.ClaimType, Payload: fact.NormalizedPayload})
			factKey := domainsecurity.CanonicalJSONHash(factBody)
			if factKey == "" || seenFacts[factKey] {
				continue
			}
			seenFacts[factKey] = true
			proposal := domainevidence.ClaimProposal{
				SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "prp_" + factKey,
				ClaimType: fact.ClaimType, NormalizedPayload: fact.NormalizedPayload,
				EvidenceIDs: []string{registered.Receipt.ReceiptID}, CounterEvidenceIDs: []string{},
			}
			claimID := "clm_" + domainsecurity.SHA256Hex([]byte(strings.Join([]string{
				securityContext.ContextDigest, registered.Receipt.ReceiptID, fact.FactID, factKey,
			}, "\x00")))
			verifier := ClaimVerifier{
				Registry: registry, Now: func() time.Time { return verifiedAt.UTC() },
				IDGenerator: func() (string, error) { return claimID, nil },
			}
			claim, err := verifier.Verify(ctx, securityContext, proposal)
			if err != nil {
				return RegistryPublicationCandidates{}, err
			}
			if claim.SupportState == domainevidence.ClaimVerified || claim.SupportState == domainevidence.ClaimPartial {
				claims = append(claims, claim)
			}
		}
	}
	return RegistryPublicationCandidates{Claims: claims, NoHitReceiptIDs: noHitReceiptIDs}, nil
}
