package evidence

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type TerminalReason = domaincachetelemetry.ProviderTurnTerminalReasonV1

const (
	TerminalSuccess              = domaincachetelemetry.ProviderTurnTerminalSuccessV1
	TerminalSourceUnavailable    = domaincachetelemetry.ProviderTurnTerminalSourceUnavailableV1
	TerminalSemanticFailure      = domaincachetelemetry.ProviderTurnTerminalSemanticFailureV1
	TerminalProviderFailure      = domaincachetelemetry.ProviderTurnTerminalProviderFailureV1
	TerminalCancel               = domaincachetelemetry.ProviderTurnTerminalCancelV1
	TerminalTimeout              = domaincachetelemetry.ProviderTurnTerminalTimeoutV1
	TerminalStreamAbort          = domaincachetelemetry.ProviderTurnTerminalStreamAbortV1
	TerminalRecovery             = domaincachetelemetry.ProviderTurnTerminalRecoveryV1
	TerminalApproval             = domaincachetelemetry.ProviderTurnTerminalApprovalV1
	TerminalUserInput            = domaincachetelemetry.ProviderTurnTerminalUserInputV1
	TerminalResume               = domaincachetelemetry.ProviderTurnTerminalResumeV1
	TerminalRestart              = domaincachetelemetry.ProviderTurnTerminalRestartV1
	TerminalReportFallback       = domaincachetelemetry.ProviderTurnTerminalReportFallbackV1
	TerminalStepLimit            = domaincachetelemetry.ProviderTurnTerminalStepLimitV1
	TerminalBackgroundCompletion = domaincachetelemetry.ProviderTurnTerminalBackgroundCompletionV1
	TerminalToolFailure          = domaincachetelemetry.ProviderTurnTerminalToolFailureV1
	TerminalApprovalDenied       = domaincachetelemetry.ProviderTurnTerminalApprovalDeniedV1
	TerminalInputCancelled       = domaincachetelemetry.ProviderTurnTerminalInputCancelledV1
)

type FinalGateInput struct {
	Context           domainsecurity.TurnSecurityContext
	TerminalReason    TerminalReason
	OrdinaryResult    *domainordinaryresult.ResultSlotV1
	Claims            []domainevidence.ClaimRecord
	NoHitReceiptIDs   []string
	SourceUnavailable bool
	Blocker           string
	CheckedScope      *domainevidence.EvidenceQueryRange
	RequestedScope    *domainevidence.EvidenceQueryRange
	MissingScope      []string
	AcquisitionSteps  []string
	GeneralGuidance   []string
	IssuedAt          time.Time
}

type FinalEvidenceGate struct {
	Registry registryport.Registry
}

func AllTerminalReasons() []TerminalReason {
	return domaincachetelemetry.AllProviderTurnTerminalReasonsV1()
}

func (gate FinalEvidenceGate) Finalize(ctx context.Context, input FinalGateInput) (domainevidence.FinalAnswerEnvelope, error) {
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(input.Context) != nil || !validTerminalReason(input.TerminalReason) {
		return domainevidence.FinalAnswerEnvelope{}, errors.New("final evidence gate input is invalid")
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	if len(input.GeneralGuidance) > 0 && !input.SourceUnavailable && len(input.Claims) == 0 && len(input.NoHitReceiptIDs) == 0 {
		if !validGeneralGuidanceCodes(input.GeneralGuidance) {
			return domainevidence.FinalAnswerEnvelope{}, errors.New("general guidance contains an unknown host code")
		}
		return domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
			Variant: domainevidence.GeneralGuidanceAnswer, Context: input.Context, TerminalReason: string(input.TerminalReason),
			OrdinaryResult: input.OrdinaryResult, Guidance: input.GeneralGuidance, IssuedAt: issuedAt,
		})
	}
	if input.SourceUnavailable || input.TerminalReason == TerminalSourceUnavailable {
		blocker := strings.TrimSpace(input.Blocker)
		if blocker == "" {
			blocker = "current_case_source_unavailable"
		}
		steps := nonEmptyOrDefault(input.AcquisitionSteps, []string{"reconnect_and_verify_current_case_source"})
		return domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
			Variant: domainevidence.SourceUnavailableAnswer, Context: input.Context, TerminalReason: string(input.TerminalReason),
			OrdinaryResult: input.OrdinaryResult, CheckedScope: input.CheckedScope, Blocker: blocker, AcquisitionSteps: steps, IssuedAt: issuedAt,
		})
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
		return gate.needsEvidence(input, issuedAt, "current_case_fact_publication_authority_required")
	}
	if !terminalAllowsEvidence(input.TerminalReason) {
		return gate.needsEvidence(input, issuedAt, "terminal_path_cannot_publish_case_facts")
	}
	if len(input.NoHitReceiptIDs) > 0 && len(input.Claims) > 0 {
		return gate.needsEvidence(input, issuedAt, "conflicting_fact_and_no_hit_candidates")
	}
	if len(input.NoHitReceiptIDs) > 0 {
		requestedScope := input.RequestedScope
		if requestedScope == nil {
			requestedScope = input.CheckedScope
		}
		receiptIDs, scope, err := gate.verifyNoHit(ctx, input.Context, input.NoHitReceiptIDs, requestedScope)
		if err == nil {
			return domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.VerifiedNoHitAnswer, Context: input.Context, TerminalReason: string(input.TerminalReason),
				OrdinaryResult: input.OrdinaryResult, EvidenceReceiptIDs: receiptIDs, CheckedScope: scope, NoHitWording: domainevidence.VerifiedNoHitWording, IssuedAt: issuedAt,
			})
		}
		return gate.needsEvidence(input, issuedAt, "complete_current_scope_receipt_required")
	}
	acceptedClaims := []domainevidence.ClaimRecord{}
	receiptIDs := []string{}
	hasRejectedOrPartialClaim := false
	hasDifferentAcceptedScope := false
	var acceptedScope *domainevidence.EvidenceQueryRange
	for _, claim := range input.Claims {
		if claim.SupportState != domainevidence.ClaimVerified && claim.SupportState != domainevidence.ClaimPartial {
			hasRejectedOrPartialClaim = true
			continue
		}
		derivedScope, err := gate.verifyClaimAuthority(ctx, input.Context, claim)
		if err != nil {
			hasRejectedOrPartialClaim = true
			continue
		}
		acceptedClaims = append(acceptedClaims, claim)
		receiptIDs = append(receiptIDs, claim.EvidenceIDs...)
		hasRejectedOrPartialClaim = hasRejectedOrPartialClaim || claim.SupportState == domainevidence.ClaimPartial
		if acceptedScope == nil {
			acceptedScope = &derivedScope
		} else if !reflect.DeepEqual(*acceptedScope, derivedScope) {
			hasDifferentAcceptedScope = true
		}
	}
	if len(acceptedClaims) > 0 {
		receiptIDs = sortedUnique(receiptIDs)
		scope := acceptedScope
		hasBoundaryScopeMismatch := false
		if input.CheckedScope != nil && (scope == nil || !reflect.DeepEqual(*input.CheckedScope, *scope)) {
			hasBoundaryScopeMismatch = true
		}
		if input.RequestedScope != nil && (scope == nil || !reflect.DeepEqual(*input.RequestedScope, *scope)) {
			hasBoundaryScopeMismatch = true
		}
		hasPartialOrRejected := hasRejectedOrPartialClaim || hasDifferentAcceptedScope || hasBoundaryScopeMismatch
		completeAccountFlowGroups := !hasRejectedOrPartialClaim && !hasBoundaryScopeMismatch &&
			gate.hasOnlyCompleteCurrentAccountFlowClaimGroups(ctx, input.Context, acceptedClaims)
		var accountFlowOutcome *domainevidence.AccountFlowTypedAnswerOutcomeV1
		if completeAccountFlowGroups && !hasPartialOrRejected {
			if outcome, ok := gate.completeCurrentAccountFlowTypedOutcome(ctx, input.Context, acceptedClaims); ok {
				accountFlowOutcome = &outcome
			}
		}
		if !hasPartialOrRejected {
			ordinaryResult := input.OrdinaryResult
			if completeAccountFlowGroups {
				// There is no typed model-explanation contract for exact account-flow
				// conclusions. Once every accepted claim belongs to a complete, current
				// oracle group, do not concatenate provider prose with host exact results.
				ordinaryResult = nil
			}
			return domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.EvidenceBackedAnswer, Context: input.Context, TerminalReason: string(input.TerminalReason),
				OrdinaryResult: ordinaryResult, AccountFlowOutcome: accountFlowOutcome,
				Claims: acceptedClaims, EvidenceReceiptIDs: receiptIDs, CheckedScope: scope, IssuedAt: issuedAt,
			})
		}
		ordinaryResult := input.OrdinaryResult
		if completeAccountFlowGroups && hasDifferentAcceptedScope {
			// Multiple independently settled query scopes retain the partial envelope
			// but remain separate host-rendered groups. Untyped provider prose cannot
			// supply or combine arithmetic across those exact groups.
			ordinaryResult = nil
		}
		missing := nonEmptyOrDefault(input.MissingScope, []string{"additional_case_scope"})
		return domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
			Variant: domainevidence.PartialEvidenceAnswer, Context: input.Context, TerminalReason: string(input.TerminalReason),
			OrdinaryResult: ordinaryResult, Claims: acceptedClaims, EvidenceReceiptIDs: receiptIDs, CheckedScope: scope, MissingScope: missing,
			AcquisitionSteps: nonEmptyOrDefault(input.AcquisitionSteps, []string{"collect_missing_scope_evidence"}), IssuedAt: issuedAt,
		})
	}
	return gate.needsEvidence(input, issuedAt, "current_exact_evidence_receipt_required")
}

func (gate FinalEvidenceGate) hasOnlyCompleteCurrentAccountFlowClaimGroups(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	claims []domainevidence.ClaimRecord,
) bool {
	if gate.Registry == nil || len(claims) < 3 || len(claims)%3 != 0 {
		return false
	}
	groups := make(map[string][]domainevidence.ClaimRecord, len(claims)/3)
	for _, claim := range claims {
		if claim.SupportState != domainevidence.ClaimVerified || len(claim.EvidenceIDs) != 1 || len(claim.CounterEvidenceIDs) != 0 {
			return false
		}
		receiptID := claim.EvidenceIDs[0]
		groups[receiptID] = append(groups[receiptID], claim)
	}
	receiptIDs := make([]string, 0, len(groups))
	for receiptID := range groups {
		receiptIDs = append(receiptIDs, receiptID)
	}
	sort.Strings(receiptIDs)
	for _, receiptID := range receiptIDs {
		if !gate.isCompleteCurrentAccountFlowClaimGroup(ctx, securityContext, groups[receiptID]) {
			return false
		}
	}
	return len(receiptIDs) > 0
}

func (gate FinalEvidenceGate) isCompleteCurrentAccountFlowClaimGroup(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	claims []domainevidence.ClaimRecord,
) bool {
	if gate.Registry == nil || len(claims) != 3 {
		return false
	}
	receiptID := ""
	var inflow, outflow, count *domainevidence.ClaimRecord
	for index := range claims {
		claim := &claims[index]
		if claim.SupportState != domainevidence.ClaimVerified || len(claim.EvidenceIDs) != 1 || len(claim.CounterEvidenceIDs) != 0 ||
			claim.SupportedScope == nil || claim.NormalizedPayload.Granularity != accountFlowReceiptGranularity {
			return false
		}
		if receiptID == "" {
			receiptID = claim.EvidenceIDs[0]
		} else if claim.EvidenceIDs[0] != receiptID {
			return false
		}
		switch {
		case claim.ClaimType == domainevidence.ClaimAmount && claim.NormalizedPayload.Direction == "in" && inflow == nil:
			inflow = claim
		case claim.ClaimType == domainevidence.ClaimAmount && claim.NormalizedPayload.Direction == "out" && outflow == nil:
			outflow = claim
		case claim.ClaimType == domainevidence.ClaimCount && claim.NormalizedPayload.Direction == "" && count == nil:
			count = claim
		default:
			return false
		}
	}
	if inflow == nil || outflow == nil || count == nil ||
		inflow.NormalizedPayload.SubjectID != outflow.NormalizedPayload.SubjectID ||
		inflow.NormalizedPayload.SubjectID != count.NormalizedPayload.SubjectID ||
		inflow.NormalizedPayload.EntityID != outflow.NormalizedPayload.EntityID ||
		inflow.NormalizedPayload.EntityID != count.NormalizedPayload.EntityID ||
		inflow.NormalizedPayload.AccountID == "" || inflow.NormalizedPayload.AccountID != outflow.NormalizedPayload.AccountID ||
		count.NormalizedPayload.AccountID != "" || inflow.NormalizedPayload.Currency == "" ||
		inflow.NormalizedPayload.Currency != outflow.NormalizedPayload.Currency || count.NormalizedPayload.Currency != "" ||
		!reflect.DeepEqual(*inflow.SupportedScope, *outflow.SupportedScope) ||
		!reflect.DeepEqual(*inflow.SupportedScope, *count.SupportedScope) ||
		!sameAccountFlowClaimRange(inflow.NormalizedPayload, outflow.NormalizedPayload) ||
		!sameAccountFlowClaimRange(inflow.NormalizedPayload, count.NormalizedPayload) {
		return false
	}
	registered, err := gate.Registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
	if err != nil || registered.Revoked || registered.Receipt.ToolName != fundsAccountFlowCanonicalTool ||
		registered.Receipt.SourceType != "transactions" ||
		registered.Receipt.PaginationCompleteness != domainevidence.PaginationComplete ||
		registered.Receipt.Granularity != accountFlowReceiptGranularity ||
		registered.Receipt.PIIClassification != domainevidence.PIINone ||
		registered.Receipt.Currency != inflow.NormalizedPayload.Currency ||
		!reflect.DeepEqual(registered.Receipt.QueryRange, *inflow.SupportedScope) ||
		registered.Receipt.QueryHash == "" || registered.Receipt.ResultHash == "" || registered.Receipt.RawSHA256 == "" ||
		len(registered.Receipt.TransformationLineage) != 2 {
		return false
	}
	lineage := registered.Receipt.TransformationLineage
	if lineage[0].StepID != accountFlowNativeBindStepIDV1 || lineage[0].Transformer != accountFlowNativeBinderV1 ||
		lineage[0].TransformerVersion != "1" || lineage[0].InputHash != registered.Receipt.RawSHA256 ||
		!domainsecurity.IsSHA256Hex(lineage[0].OutputHash) || lineage[1].StepID != accountFlowNormalizeStepIDV1 ||
		lineage[1].Transformer != accountFlowNormalizerV1 || lineage[1].TransformerVersion != "1" ||
		lineage[1].InputHash != lineage[0].OutputHash || lineage[1].OutputHash != registered.Receipt.ResultHash {
		return false
	}
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
	if err != nil || len(material.Facts) != len(claims) {
		return false
	}
	matched := make([]bool, len(claims))
	for _, fact := range material.Facts {
		if !strings.HasPrefix(fact.FactID, accountFlowAggregateFactPrefix) {
			return false
		}
		factMatched := false
		for index, claim := range claims {
			if !matched[index] && fact.ClaimType == claim.ClaimType &&
				domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, claim.NormalizedPayload) {
				matched[index] = true
				factMatched = true
				break
			}
		}
		if !factMatched {
			return false
		}
	}
	return true
}

func (gate FinalEvidenceGate) completeCurrentAccountFlowTypedOutcome(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	claims []domainevidence.ClaimRecord,
) (domainevidence.AccountFlowTypedAnswerOutcomeV1, bool) {
	if gate.Registry == nil || len(claims) < 3 || len(claims)%3 != 0 {
		return domainevidence.AccountFlowTypedAnswerOutcomeV1{}, false
	}
	groups := make(map[string][]domainevidence.ClaimRecord, len(claims)/3)
	for _, claim := range claims {
		if len(claim.EvidenceIDs) != 1 {
			return domainevidence.AccountFlowTypedAnswerOutcomeV1{}, false
		}
		groups[claim.EvidenceIDs[0]] = append(groups[claim.EvidenceIDs[0]], claim)
	}
	receiptIDs := make([]string, 0, len(groups))
	for receiptID := range groups {
		receiptIDs = append(receiptIDs, receiptID)
	}
	sort.Strings(receiptIDs)
	typedGroups := make([]domainevidence.AccountFlowTypedAnswerGroupV1, 0, len(receiptIDs))
	for _, receiptID := range receiptIDs {
		group, ok := gate.completeCurrentAccountFlowTypedGroup(ctx, securityContext, groups[receiptID])
		if !ok {
			return domainevidence.AccountFlowTypedAnswerOutcomeV1{}, false
		}
		typedGroups = append(typedGroups, group)
	}
	outcome, err := domainevidence.NewAccountFlowTypedAnswerOutcomeV1(typedGroups)
	return outcome, err == nil
}

func (gate FinalEvidenceGate) completeCurrentAccountFlowTypedGroup(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	claims []domainevidence.ClaimRecord,
) (domainevidence.AccountFlowTypedAnswerGroupV1, bool) {
	if !gate.isCompleteCurrentAccountFlowClaimGroup(ctx, securityContext, claims) {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false
	}
	receiptID := claims[0].EvidenceIDs[0]
	registered, err := gate.Registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
	if err != nil || registered.Revoked ||
		registered.Receipt.CaseID != securityContext.CaseID ||
		registered.Receipt.CaseBindingHash != securityContext.CaseBindingHash ||
		registered.Receipt.ContextEpoch != securityContext.ContextEpoch ||
		registered.Receipt.ContextDigest != securityContext.ContextDigest ||
		registered.Receipt.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		len(registered.Receipt.QueryRange.SourceIDs) != 1 ||
		domainevidence.ValidateAccountFlowQueryScopeRefV1(registered.Receipt.QueryRange.SourceIDs[0]) != nil ||
		len(registered.Receipt.TransformationLineage) != 2 {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false
	}
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
	if err != nil || material.SchemaVersion != domainevidence.CanonicalEvidenceVersionV3 ||
		len(material.AcceptedSlotSourceBindings) == 0 {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false
	}
	field := material.AcceptedSlotSourceBindings[0].Field
	for _, binding := range material.AcceptedSlotSourceBindings {
		if binding.Field != field {
			return domainevidence.AccountFlowTypedAnswerGroupV1{}, false
		}
	}
	nativeResultHash := registered.Receipt.TransformationLineage[0].OutputHash
	sourceFieldReference, err := domainevidence.NewAccountFlowTypedSourceFieldReferenceV1(
		registered.Receipt.QueryHash,
		nativeResultHash,
		field,
	)
	if err != nil {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false
	}
	claimIDs := make([]string, len(claims))
	for index, claim := range claims {
		claimIDs[index] = claim.ClaimID
	}
	sort.Strings(claimIDs)
	return domainevidence.AccountFlowTypedAnswerGroupV1{
		EvidenceReceiptID:    receiptID,
		ClaimIDs:             claimIDs,
		QueryScopeRef:        registered.Receipt.QueryRange.SourceIDs[0],
		QueryHash:            registered.Receipt.QueryHash,
		ResultHash:           nativeResultHash,
		SourceFieldReference: sourceFieldReference,
	}, true
}

func sameAccountFlowClaimRange(left, right domainevidence.NormalizedClaimPayload) bool {
	return left.StartAt != "" && left.StartAt == right.StartAt && left.EndAt != "" && left.EndAt == right.EndAt
}

func terminalAllowsEvidence(reason TerminalReason) bool {
	return domainterminal.CandidateAllowedV1(string(reason))
}

func (gate FinalEvidenceGate) verifyClaimAuthority(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, claim domainevidence.ClaimRecord) (domainevidence.EvidenceQueryRange, error) {
	if gate.Registry == nil || domainevidence.ValidateClaimRecord(claim) != nil || len(claim.EvidenceIDs) == 0 || len(claim.CounterEvidenceIDs) != 0 {
		return domainevidence.EvidenceQueryRange{}, errors.New("claim authority is unavailable")
	}
	proposal := domainevidence.ClaimProposal{ClaimType: claim.ClaimType, NormalizedPayload: claim.NormalizedPayload}
	hasPartial := false
	var derivedScope *domainevidence.EvidenceQueryRange
	for _, receiptID := range claim.EvidenceIDs {
		registered, err := gate.Registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil || registered.Revoked || !domainevidence.SourceTypeSupportsClaim(registered.Receipt.SourceType, claim.ClaimType) {
			return domainevidence.EvidenceQueryRange{}, errors.New("claim receipt is invalid")
		}
		if !receiptMetadataSupportsClaim(registered.Receipt, proposal) || !ordinaryChatPIIAllowed(registered.Receipt.PIIClassification) {
			return domainevidence.EvidenceQueryRange{}, errors.New("claim receipt metadata is not publishable")
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil || !canonicalMaterialSupportsClaim(material, claim) || !claimScopeSupportsPayload(registered.Receipt.QueryRange, claim.NormalizedPayload) {
			return domainevidence.EvidenceQueryRange{}, errors.New("claim receipt does not support normalized payload")
		}
		scope := registered.Receipt.QueryRange
		if derivedScope == nil {
			derivedScope = &scope
		} else if !reflect.DeepEqual(*derivedScope, scope) {
			return domainevidence.EvidenceQueryRange{}, errors.New("claim receipts do not share one exact supported scope")
		}
		hasPartial = hasPartial || registered.Receipt.PaginationCompleteness != domainevidence.PaginationComplete
	}
	if derivedScope == nil || claim.SupportedScope == nil || !reflect.DeepEqual(*claim.SupportedScope, *derivedScope) ||
		(claim.SupportState == domainevidence.ClaimPartial) != hasPartial {
		return domainevidence.EvidenceQueryRange{}, errors.New("claim support state or scope does not match receipt authority")
	}
	return *derivedScope, nil
}

func ordinaryChatPIIAllowed(classification domainevidence.PIIClassification) bool {
	return classification == domainevidence.PIINone || classification == domainevidence.PIIMasked
}

func (gate FinalEvidenceGate) verifyNoHit(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, receiptIDs []string, requestedScope *domainevidence.EvidenceQueryRange) ([]string, *domainevidence.EvidenceQueryRange, error) {
	if gate.Registry == nil || len(receiptIDs) == 0 {
		return nil, nil, errors.New("no-hit registry authority is unavailable")
	}
	ids := sortedUnique(receiptIDs)
	if len(ids) != len(receiptIDs) {
		return nil, nil, errors.New("no-hit receipt ids are invalid")
	}
	var checkedScope *domainevidence.EvidenceQueryRange
	for _, receiptID := range ids {
		registered, err := gate.Registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil || registered.Revoked || registered.Receipt.PaginationCompleteness != domainevidence.PaginationComplete || len(registered.Receipt.SourceRecordIDs) != 0 {
			return nil, nil, errors.New("no-hit receipt is incomplete")
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil || len(material.Facts) != 0 {
			return nil, nil, errors.New("no-hit receipt contains facts or invalid material")
		}
		scope := registered.Receipt.QueryRange
		if checkedScope == nil {
			checkedScope = &scope
		} else if !reflect.DeepEqual(*checkedScope, scope) {
			return nil, nil, errors.New("no-hit receipts cover different scopes")
		}
	}
	if checkedScope == nil || (requestedScope != nil && !reflect.DeepEqual(*checkedScope, *requestedScope)) {
		return nil, nil, errors.New("no-hit checked scope is mismatched")
	}
	return ids, checkedScope, nil
}

func (gate FinalEvidenceGate) needsEvidence(input FinalGateInput, issuedAt time.Time, reason string) (domainevidence.FinalAnswerEnvelope, error) {
	missing := nonEmptyOrDefault(input.MissingScope, []string{"current_case_facts"})
	steps := nonEmptyOrDefault(input.AcquisitionSteps, []string{"collect_and_verify_current_case_evidence"})
	blocker := strings.TrimSpace(input.Blocker)
	if blocker == "" {
		blocker = reason
	}
	return domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: input.Context, TerminalReason: string(input.TerminalReason),
		OrdinaryResult: input.OrdinaryResult, CheckedScope: input.CheckedScope, MissingScope: missing, Blocker: blocker, AcquisitionSteps: steps, IssuedAt: issuedAt,
	})
}

func canonicalMaterialSupportsClaim(material domainevidence.CanonicalEvidenceMaterial, claim domainevidence.ClaimRecord) bool {
	for _, fact := range material.Facts {
		if fact.ClaimType == claim.ClaimType && domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, claim.NormalizedPayload) {
			return true
		}
	}
	return false
}

func validTerminalReason(reason TerminalReason) bool {
	return domaincachetelemetry.ValidateProviderTurnTerminalReasonV1(reason) == nil
}

func validGeneralGuidanceCodes(codes []string) bool {
	allowed := map[string]bool{
		"explain_evidence_requirements":                 true,
		"explain_checked_scope":                         true,
		"explain_source_connection":                     true,
		"explain_privacy_controls":                      true,
		"explain_agent_safety_authority":                true,
		domainevidence.OrdinaryResultOnlyGuidanceCodeV1: true,
	}
	for _, code := range codes {
		if !allowed[strings.TrimSpace(code)] {
			return false
		}
	}
	return len(codes) > 0
}

func nonEmptyOrDefault(values []string, fallback []string) []string {
	values = sortedUnique(values)
	if len(values) > 0 {
		return values
	}
	return append([]string(nil), fallback...)
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
