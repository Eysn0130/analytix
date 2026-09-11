package evidence

import (
	"context"
	"errors"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type CaseBoundaryInput struct {
	Context            domainsecurity.TurnSecurityContext
	TerminalReason     TerminalReason
	OrdinaryResult     *domainordinaryresult.ResultSlotV1
	CaseSlotIntent     CaseSlotPublicationIntentV1
	SourceUnavailable  bool
	ReportRequested    bool
	PublicationBlocker string
	IssuedAt           time.Time
}

type CaseSlotPublicationIntentV1 string

const (
	CaseSlotRequestedV1    CaseSlotPublicationIntentV1 = "requested"
	CaseSlotNotRequestedV1 CaseSlotPublicationIntentV1 = "not_requested"
)

func CaseSlotPublicationIntentForTurnV1(ordinaryCandidate, fundsActive, reportRequested bool) CaseSlotPublicationIntentV1 {
	if ordinaryCandidate && !fundsActive && !reportRequested {
		return CaseSlotNotRequestedV1
	}
	return CaseSlotRequestedV1
}

type CaseBoundaryResult struct {
	Envelope domainevidence.FinalAnswerEnvelope
	Text     string
}

// FinalizeCaseBoundary projects claims only from the locked host registry.
// A separately typed ordinary result is non-evidentiary; the Final Gate owns
// whether it can accompany the exact host-derived case result.
func FinalizeCaseBoundary(ctx context.Context, registry registryport.Registry, input CaseBoundaryInput) (CaseBoundaryResult, error) {
	if input.OrdinaryResult != nil {
		switch input.CaseSlotIntent {
		case CaseSlotNotRequestedV1:
			envelope, err := (FinalEvidenceGate{}).Finalize(ctx, FinalGateInput{
				Context: input.Context, TerminalReason: input.TerminalReason, OrdinaryResult: input.OrdinaryResult,
				GeneralGuidance: []string{domainevidence.OrdinaryResultOnlyGuidanceCodeV1}, IssuedAt: input.IssuedAt,
			})
			if err != nil {
				return CaseBoundaryResult{}, err
			}
			text, err := domainevidence.RenderFinalAnswer(envelope)
			if err != nil || text == "" {
				if err == nil {
					err = errors.New("ordinary-only renderer returned empty text")
				}
				return CaseBoundaryResult{}, err
			}
			return CaseBoundaryResult{Envelope: envelope, Text: text}, nil
		case CaseSlotRequestedV1:
		default:
			return CaseBoundaryResult{}, errors.New("case-slot publication intent is unavailable")
		}
	}
	gateInput := FinalGateInput{
		Context: input.Context, TerminalReason: input.TerminalReason, OrdinaryResult: input.OrdinaryResult,
		SourceUnavailable: input.SourceUnavailable, IssuedAt: input.IssuedAt,
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_same_context_evidence"},
	}
	if input.SourceUnavailable {
		gateInput.Blocker = "current_case_source_unavailable"
		gateInput.AcquisitionSteps = []string{"reconnect_and_verify_current_case_source"}
	}
	if input.ReportRequested && !input.SourceUnavailable && input.PublicationBlocker == "" {
		gateInput.MissingScope = []string{"publication_receipt"}
		gateInput.AcquisitionSteps = []string{"validate_claims_and_issue_publication_receipt"}
	}
	if input.PublicationBlocker != "" && !input.SourceUnavailable {
		if input.PublicationBlocker == domainsecurity.PublicationBlockerRiskAuthorityUnavailable {
			gateInput.MissingScope = nil
			gateInput.AcquisitionSteps = nil
			gateInput.GeneralGuidance = []string{"explain_agent_safety_authority"}
		} else {
			gateInput.Blocker = input.PublicationBlocker
			gateInput.AcquisitionSteps = []string{"refresh_and_verify_current_case_snapshot"}
		}
	}
	if !input.SourceUnavailable && !input.ReportRequested && input.PublicationBlocker == "" {
		candidates, projectionErr := VerifiedPublicationCandidatesFromRegistrySnapshot(ctx, registry, input.Context, input.IssuedAt)
		if projectionErr != nil {
			gateInput.Blocker = "host_claim_projection_failed"
		} else {
			gateInput.Claims = candidates.Claims
			gateInput.NoHitReceiptIDs = candidates.NoHitReceiptIDs
		}
	}
	envelope, err := (FinalEvidenceGate{Registry: registry}).Finalize(ctx, gateInput)
	if err != nil {
		return CaseBoundaryResult{}, err
	}
	text, err := RenderFinalAnswer(envelope)
	if err != nil || text == "" {
		if err == nil {
			err = errors.New("case boundary renderer returned empty text")
		}
		return CaseBoundaryResult{}, err
	}
	return CaseBoundaryResult{Envelope: envelope, Text: text}, nil
}
