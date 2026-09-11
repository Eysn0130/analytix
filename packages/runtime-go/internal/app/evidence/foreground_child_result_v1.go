package evidence

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"sort"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// ResolveCurrentCaseForegroundAnswerSlotsV1 creates closed delegated bindings
// from the existing current evidence registry. Claim RecordDigest stability is
// anchored to the owning EvidenceReceipt IssuedAt, so consume can reproduce
// the same exact refs without a second claim store or reverse map.
func ResolveCurrentCaseForegroundAnswerSlotsV1(
	ctx context.Context,
	registry registryport.Registry,
	parent domainsecurity.TurnSecurityContext,
	outputs []domainnative.AccountFlowProviderModelOutputV1,
) ([]domainjob.CaseDelegatedAnswerSlotBindingV1, error) {
	if ctx == nil || registry == nil || len(outputs) != 1 ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil {
		return nil, errors.New("foreground case answer-slot authority is unavailable")
	}
	bindings := make([]domainjob.CaseDelegatedAnswerSlotBindingV1, len(outputs))
	for index, output := range outputs {
		slot, err := domainnative.NewAccountFlowDelegatedAnswerSlotV1(parent.ContextDigest, output)
		if err != nil {
			return nil, errors.New("foreground case answer slot is invalid")
		}
		binding, ok := currentForegroundBindingForSlotV1(ctx, registry, parent, slot)
		if !ok {
			return nil, errors.New("foreground case answer slot has no unique current claim/evidence group")
		}
		bindings[index] = binding
	}
	return bindings, nil
}

// ValidateCurrentCaseForegroundAnswerSlotsV1 replays the host-created
// delegation bindings before submit admission and again before consumption.
func ValidateCurrentCaseForegroundAnswerSlotsV1(
	ctx context.Context,
	registry registryport.Registry,
	parent domainsecurity.TurnSecurityContext,
	bindings []domainjob.CaseDelegatedAnswerSlotBindingV1,
) error {
	if ctx == nil || registry == nil || len(bindings) != 1 {
		return errors.New("foreground case answer-slot binding is unavailable")
	}
	current, ok := currentForegroundBindingForSlotV1(ctx, registry, parent, bindings[0].AnswerSlot)
	if !ok || !reflect.DeepEqual(current, bindings[0]) {
		return errors.New("foreground case answer-slot binding is not current")
	}
	return nil
}

// ValidateCurrentCaseForegroundChildResultV1 re-resolves the selected tuple
// from the current parent registry. This is candidate validation only: the
// parent turn must still pass its normal Final Evidence Gate and accepted-final
// authority before any fact-bearing answer or local display is possible.
func ValidateCurrentCaseForegroundChildResultV1(
	ctx context.Context,
	registry registryport.Registry,
	parent domainsecurity.TurnSecurityContext,
	result domainjob.CaseForegroundChildResultV1,
) error {
	if ctx == nil || registry == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil ||
		domainjob.ValidateCaseForegroundChildResultShapeV1(result) != nil ||
		len(result.Claims) != 3 || len(result.Evidence) != 1 {
		return errors.New("foreground case result registry authority is unavailable")
	}
	binding, ok := currentForegroundBindingForSlotV1(ctx, registry, parent, result.AnswerSlots[0])
	if !ok || !reflect.DeepEqual(binding.Claims, result.Claims) || !reflect.DeepEqual(binding.Evidence, result.Evidence) {
		return errors.New("foreground case result answer slot or references are not current")
	}
	return nil
}

func currentForegroundBindingForSlotV1(
	ctx context.Context,
	registry registryport.Registry,
	parent domainsecurity.TurnSecurityContext,
	selected domainnative.AccountFlowDelegatedAnswerSlotV1,
) (domainjob.CaseDelegatedAnswerSlotBindingV1, bool) {
	snapshot, err := registry.Replay(ctx, parent)
	if err != nil {
		return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
	}
	var matched *domainjob.CaseDelegatedAnswerSlotBindingV1
	for _, entry := range snapshot.Entries {
		if entry.Operation != domainevidence.EvidenceRegistryIssue {
			continue
		}
		registered, resolveErr := registry.Resolve(ctx, registryport.MembershipQuery{Context: parent, ReceiptID: entry.ReceiptID})
		if resolveErr != nil || registered.Revoked || registered.Receipt.QueryHash != selected.QueryHash ||
			len(registered.Receipt.TransformationLineage) != 2 ||
			registered.Receipt.TransformationLineage[0].OutputHash != selected.ResultHash {
			continue
		}
		issuedAt, timeErr := time.Parse(time.RFC3339Nano, registered.Receipt.IssuedAt)
		if timeErr != nil {
			continue
		}
		candidates, projectionErr := VerifiedPublicationCandidatesFromRegistrySnapshot(ctx, registry, parent, issuedAt)
		if projectionErr != nil {
			return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
		}
		claims := make([]domainevidence.ClaimRecord, 0, 3)
		for _, claim := range candidates.Claims {
			if len(claim.EvidenceIDs) == 1 && claim.EvidenceIDs[0] == registered.Receipt.ReceiptID {
				claims = append(claims, claim)
			}
		}
		if currentForegroundAccountFlowSlotV1(ctx, registry, parent, selected, registered, claims) != nil {
			continue
		}
		binding, bindingErr := foregroundCaseAnswerSlotBindingV1(selected, registered.Receipt, claims)
		if bindingErr != nil || matched != nil {
			return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
		}
		matched = &binding
	}
	if matched == nil {
		return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
	}
	return *matched, true
}

func foregroundCaseAnswerSlotBindingV1(
	slot domainnative.AccountFlowDelegatedAnswerSlotV1,
	receipt domainevidence.EvidenceReceipt,
	claims []domainevidence.ClaimRecord,
) (domainjob.CaseDelegatedAnswerSlotBindingV1, error) {
	if len(claims) != 3 {
		return domainjob.CaseDelegatedAnswerSlotBindingV1{}, errors.New("foreground case claim group is incomplete")
	}
	references := make([]domainjob.CaseDelegatedClaimReferenceV1, 0, len(claims))
	for _, claim := range claims {
		investigationState := ""
		switch claim.SupportState {
		case domainevidence.ClaimVerified:
			investigationState = domaincaseentity.InvestigationConfirmedV1
		case domainevidence.ClaimPartial:
			investigationState = domaincaseentity.InvestigationOpenV1
		default:
			return domainjob.CaseDelegatedAnswerSlotBindingV1{}, errors.New("foreground case claim group has unsupported state")
		}
		references = append(references, domainjob.CaseDelegatedClaimReferenceV1{
			Digest: claim.RecordDigest, InvestigationState: investigationState,
		})
	}
	sort.Slice(references, func(left, right int) bool { return references[left].Digest < references[right].Digest })
	return domainjob.CaseDelegatedAnswerSlotBindingV1{
		AnswerSlot: slot,
		Claims:     references,
		Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{{
			Digest: receipt.ReceiptDigest, Currentness: domaincaseentity.SnapshotCurrentV1,
		}},
	}, nil
}

func currentForegroundAccountFlowSlotV1(
	ctx context.Context,
	registry registryport.Registry,
	parent domainsecurity.TurnSecurityContext,
	selected domainnative.AccountFlowDelegatedAnswerSlotV1,
	registered domainevidence.RegisteredEvidence,
	claims []domainevidence.ClaimRecord,
) error {
	receipt := registered.Receipt
	if len(claims) != 3 || receipt.ToolName != fundsAccountFlowCanonicalTool ||
		receipt.CaseID != parent.CaseID || receipt.CaseBindingHash != parent.CaseBindingHash ||
		receipt.ContextEpoch != parent.ContextEpoch || receipt.ContextDigest != parent.ContextDigest ||
		receipt.DatasetSnapshotID != parent.DatasetSnapshotID || len(receipt.QueryRange.SourceIDs) != 1 ||
		domainevidence.ValidateAccountFlowQueryScopeRefV1(receipt.QueryRange.SourceIDs[0]) != nil ||
		len(receipt.TransformationLineage) != 2 {
		return errors.New("current account-flow group is incomplete")
	}
	expectedSupport := domainevidence.ClaimPartial
	if receipt.PaginationCompleteness == domainevidence.PaginationComplete {
		expectedSupport = domainevidence.ClaimVerified
	} else if receipt.PaginationCompleteness != domainevidence.PaginationPartial {
		return errors.New("current account-flow group has unknown completeness")
	}
	var inflow, outflow string
	var count uint64
	gate := FinalEvidenceGate{Registry: registry}
	for _, claim := range claims {
		if claim.SupportState != expectedSupport || len(claim.EvidenceIDs) != 1 ||
			claim.EvidenceIDs[0] != receipt.ReceiptID || len(claim.CounterEvidenceIDs) != 0 {
			return errors.New("current account-flow claim group is not closed")
		}
		if _, verifyErr := gate.verifyClaimAuthority(ctx, parent, claim); verifyErr != nil {
			return errors.New("current account-flow claim group is not authoritative")
		}
		switch {
		case claim.ClaimType == domainevidence.ClaimAmount && claim.NormalizedPayload.Direction == "in":
			inflow = claim.NormalizedPayload.AmountMinor
		case claim.ClaimType == domainevidence.ClaimAmount && claim.NormalizedPayload.Direction == "out":
			outflow = claim.NormalizedPayload.AmountMinor
		case claim.ClaimType == domainevidence.ClaimCount:
			parsed, ok := new(big.Int).SetString(claim.NormalizedPayload.Count, 10)
			if !ok || !parsed.IsUint64() {
				return errors.New("current account-flow count is invalid")
			}
			count = parsed.Uint64()
		default:
			return errors.New("current account-flow claim group has an unknown member")
		}
	}
	inflowInt, inflowOK := new(big.Int).SetString(inflow, 10)
	outflowInt, outflowOK := new(big.Int).SetString(outflow, 10)
	if !inflowOK || !outflowOK {
		return errors.New("current account-flow amount is invalid")
	}
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
	if err != nil || material.SchemaVersion != domainevidence.CanonicalEvidenceVersionV3 ||
		len(material.AcceptedSlotSourceBindings) == 0 {
		return errors.New("current account-flow source binding is unavailable")
	}
	field := material.AcceptedSlotSourceBindings[0].Field
	for _, binding := range material.AcceptedSlotSourceBindings {
		if binding.Field != field {
			return errors.New("current account-flow source binding is ambiguous")
		}
	}
	resultHash := receipt.TransformationLineage[0].OutputHash
	sourceFieldReference, err := domainevidence.NewAccountFlowTypedSourceFieldReferenceV1(receipt.QueryHash, resultHash, field)
	if err != nil {
		return errors.New("current account-flow source binding is invalid")
	}
	selectedStart, startErr := canonicalAccountFlowEvidenceTime(selected.StartInclusive)
	selectedEnd, endErr := canonicalAccountFlowEvidenceTime(selected.EndInclusive)
	if selected.SubjectAlias == "" || startErr != nil || endErr != nil ||
		selectedStart != receipt.QueryRange.StartAt || selectedEnd != receipt.QueryRange.EndAt || selected.Timezone != receipt.Timezone ||
		selected.Currency != receipt.Currency || selected.InflowMinor != inflow || selected.OutflowMinor != outflow ||
		selected.NetMinor != new(big.Int).Sub(new(big.Int).Set(inflowInt), outflowInt).String() ||
		selected.TransactionCount != count || selected.QueryScopeRef != receipt.QueryRange.SourceIDs[0] ||
		selected.QueryHash != receipt.QueryHash || selected.ResultHash != resultHash ||
		selected.Outcome.SourceFieldReference != sourceFieldReference ||
		(receipt.PaginationCompleteness == domainevidence.PaginationComplete) !=
			(selected.AggregateComplete && selected.EvidenceRowsComplete) {
		return errors.New("current account-flow answer slot does not match its receipt")
	}
	return domainnative.ValidateAccountFlowDelegatedAnswerSlotV1(selected)
}
