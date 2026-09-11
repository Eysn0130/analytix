package subagent

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const maxCaseDelegationRawPromptBytesV1 = 32 * 1024

// CaseBoundProviderChildCallRequiresCaseAuthorityV1 classifies the existing
// subagent tool family from host-owned TSC identity and tool name only. Raw
// provider arguments never upgrade an ordinary task to case authority.
func CaseBoundProviderChildCallRequiresCaseAuthorityV1(
	securityContext domainsecurity.TurnSecurityContext,
	toolName string,
) bool {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return false
	}
	switch strings.TrimSpace(toolName) {
	case "task", "delegate_task", "parallel_tasks":
		return true
	default:
		return false
	}
}

type BindCaseDelegationInputV1 struct {
	SecurityContext        domainsecurity.TurnSecurityContext
	SecurityBinding        *domainjob.SecurityBinding
	CaseEntities           *caseentityapp.Service
	Request                TaskRequest
	SourceThreadID         string
	Expected               *domainjob.CaseDelegationContextV1
	AccountFlowSemantics   []domainnative.AccountFlowProviderModelOutputV1
	AccountFlowAnswerSlots []domainjob.CaseDelegatedAnswerSlotBindingV1
}

type BoundCaseDelegationV1 struct {
	Request   TaskRequest
	Context   *domainjob.CaseDelegationContextV1
	selection *verifiedCaseDelegationSelectionV1
}

type verifiedCaseDelegationSelectionV1 struct {
	securityContext domainsecurity.TurnSecurityContext
	record          caseentityapp.PrivateRecordReferenceV1
	references      []domaincaseentity.ReferenceV1
	aliases         []domaincaseentity.ModelEntityAliasV1
}

// UseHostSelectionV1 exposes the exact current private references only to the
// synchronous host callback that constructs the existing process-private
// HostCaseEntitySelectionV1. No reference or reverse map is durable or public.
func (bound BoundCaseDelegationV1) UseHostSelectionV1(
	securityContext domainsecurity.TurnSecurityContext,
	use func(
		caseentityapp.PrivateRecordReferenceV1,
		[]domaincaseentity.ReferenceV1,
		[]domaincaseentity.ModelEntityAliasV1,
	) error,
) error {
	selection := bound.selection
	if selection == nil || use == nil || selection.securityContext != securityContext ||
		!domainsecurity.IsSHA256Hex(selection.record.RecordID) ||
		!domainsecurity.IsSHA256Hex(selection.record.RecordDigest) ||
		len(selection.references) == 0 || len(selection.references) != len(selection.aliases) {
		return errors.New("case delegation host selection is unavailable")
	}
	return use(
		selection.record,
		append([]domaincaseentity.ReferenceV1(nil), selection.references...),
		append([]domaincaseentity.ModelEntityAliasV1(nil), selection.aliases...),
	)
}

// BindCaseDelegationV1 replaces a case-bound provider task with a host-built
// typed projection. A model-looking alias has no authority on its own: every
// alias must resolve through the exact current private case index before this
// function returns any executable request.
func BindCaseDelegationV1(ctx context.Context, input BindCaseDelegationInputV1) (BoundCaseDelegationV1, error) {
	return bindCaseDelegationWithAliasesV1(ctx, input, input.CaseEntities.UseCaseLongitudinalAliasesV1)
}

func bindCaseDelegationWithAliasesV1(ctx context.Context, input BindCaseDelegationInputV1, useAliases func(context.Context, caseentityapp.UseCaseLongitudinalAliasesInputV1, func(caseentityapp.CaseLongitudinalAliasSelectionV1) error) error) (BoundCaseDelegationV1, error) {
	request := input.Request
	if (len(input.AccountFlowSemantics) != 0 || len(input.AccountFlowAnswerSlots) != 0) &&
		(!request.ForegroundHandoff || request.ForegroundHandoffKind != ForegroundHandoffCaseTypedV1 || input.Expected != nil) {
		return BoundCaseDelegationV1{}, errors.New("account-flow delegation semantics are attached outside a new case-typed foreground child")
	}
	if request.ForegroundHandoff && request.ForegroundHandoffKind == ForegroundHandoffCaseTypedV1 &&
		input.Expected == nil && (len(input.AccountFlowSemantics) != 1 || len(input.AccountFlowAnswerSlots) != 1) {
		return BoundCaseDelegationV1{}, errors.New("case-typed foreground child requires one account-flow answer slot")
	}
	caseBound := input.SecurityBinding != nil && input.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID
	if !caseBound {
		if input.Expected != nil || request.CaseDelegation != nil {
			return BoundCaseDelegationV1{}, errors.New("case delegation cannot be attached to an unbound child")
		}
		return BoundCaseDelegationV1{Request: request}, nil
	}
	if ctx == nil || input.CaseEntities == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		domainjob.ValidateSecurityBinding(input.SecurityBinding) != nil {
		return BoundCaseDelegationV1{}, errors.New("case delegation authority is unavailable")
	}
	if input.Expected == nil {
		if !domainjob.SecurityBindingMatchesContext(input.SecurityBinding, input.SecurityContext) {
			return BoundCaseDelegationV1{}, errors.New("case delegation parent authority does not match")
		}
	} else {
		exactParentReplay := domainjob.SecurityBindingMatchesContext(input.SecurityBinding, input.SecurityContext) &&
			strings.TrimSpace(input.SourceThreadID) == ""
		descendantReplay := domainjob.SecurityBindingMatchesCaseEpochScope(input.SecurityBinding, input.SecurityContext) &&
			strings.TrimSpace(input.SourceThreadID) == input.SecurityBinding.ParentThreadID &&
			input.SecurityContext.ThreadID != input.SecurityBinding.ParentThreadID
		if domainjob.ValidateCaseDelegationContextV1(input.Expected, input.SecurityBinding) != nil ||
			(!exactParentReplay && !descendantReplay) {
			return BoundCaseDelegationV1{}, errors.New("case delegation replay authority does not match")
		}
	}

	aliases, err := caseDelegationAliasesFromPromptV1(request.Prompt)
	if input.Expected != nil {
		aliases = domainjob.CaseDelegationAliasesV1(input.Expected)
		if err != nil || request.Prompt != mustCaseDelegationProviderPromptV1(input.Expected, input.SecurityBinding) {
			return BoundCaseDelegationV1{}, errors.New("case delegation durable prompt is invalid")
		}
	}
	if err != nil || len(aliases) == 0 {
		return BoundCaseDelegationV1{}, errors.New("case-bound subagent requires a valid delegated alias subset")
	}

	providerText := "Host-delegated aliases: " + joinCaseDelegationAliasesV1(aliases)
	var (
		semantic  domainjob.CaseDelegationSemanticContextV1
		selection *verifiedCaseDelegationSelectionV1
		useCalls  int
	)
	err = useAliases(ctx, caseentityapp.UseCaseLongitudinalAliasesInputV1{
		SecurityContext: input.SecurityContext,
		Aliases:         aliases,
		ProviderText:    providerText,
		Relation: func() string {
			if strings.TrimSpace(input.SourceThreadID) != "" {
				return "primary"
			}
			return ""
		}(),
		SourceThreadID: strings.TrimSpace(input.SourceThreadID),
	}, func(current caseentityapp.CaseLongitudinalAliasSelectionV1) error {
		useCalls++
		if useCalls != 1 || len(current.Aliases) != len(aliases) || len(current.References) != len(aliases) {
			return caseentityapp.ErrPrivateStateIntegrity
		}
		entities, longitudinal, consumeErr := consumeCaseDelegationProjectionV1(current)
		if consumeErr != nil {
			return consumeErr
		}
		observedSemantic := domainjob.CaseDelegationSemanticContextV1{
			TaskKind: domainjob.CaseDelegationTaskKindV1,
			Entities: entities, Currentness: longitudinal.Currentness,
			Claims:        make([]domainjob.CaseDelegatedClaimReferenceV1, len(longitudinal.Claims)),
			Evidence:      make([]domainjob.CaseDelegatedEvidenceReferenceV1, len(longitudinal.Evidence)),
			Continuations: make([]domainjob.CaseDelegatedContinuationReferenceV1, len(longitudinal.Continuations)),
		}
		for index, claim := range longitudinal.Claims {
			observedSemantic.Claims[index] = domainjob.CaseDelegatedClaimReferenceV1{Digest: claim.Digest, InvestigationState: claim.InvestigationState}
		}
		for index, evidence := range longitudinal.Evidence {
			observedSemantic.Evidence[index] = domainjob.CaseDelegatedEvidenceReferenceV1{Digest: evidence.Digest, Currentness: evidence.Currentness}
		}
		for index, continuation := range longitudinal.Continuations {
			observedSemantic.Continuations[index] = domainjob.CaseDelegatedContinuationReferenceV1{Digest: continuation.Digest, Currentness: continuation.Currentness}
		}
		if input.Expected == nil {
			if request.ForegroundHandoff && request.ForegroundHandoffKind == ForegroundHandoffCaseTypedV1 {
				observedSemantic.Claims = []domainjob.CaseDelegatedClaimReferenceV1{}
				observedSemantic.Evidence = []domainjob.CaseDelegatedEvidenceReferenceV1{}
				observedSemantic.Continuations = []domainjob.CaseDelegatedContinuationReferenceV1{}
			}
			observedSemantic.AnswerSlots = make([]domainjob.CaseDelegatedAnswerSlotCommitmentV1, len(input.AccountFlowSemantics))
			for index, providerSemantic := range input.AccountFlowSemantics {
				slot, slotErr := domainnative.NewAccountFlowDelegatedAnswerSlotV1(
					input.SecurityContext.ContextDigest, providerSemantic,
				)
				binding := input.AccountFlowAnswerSlots[index]
				commitment, commitmentErr := domainjob.NewCaseDelegatedAnswerSlotCommitmentV1(binding)
				if slotErr != nil || commitmentErr != nil || !reflect.DeepEqual(binding.AnswerSlot, slot) {
					return caseentityapp.ErrPrivateStateIntegrity
				}
				observedSemantic.AnswerSlots[index] = commitment
			}
		}
		semantic = observedSemantic
		if input.Expected != nil {
			if !caseDelegationExpectedSemanticIsCurrentV1(input.Expected.Semantic, observedSemantic) {
				return caseentityapp.ErrPrivateStateIntegrity
			}
			semantic = cloneCaseDelegationSemanticV1(input.Expected.Semantic)
		}
		selection = &verifiedCaseDelegationSelectionV1{
			securityContext: input.SecurityContext,
			record:          current.Record,
			references:      append([]domaincaseentity.ReferenceV1(nil), current.References...),
			aliases:         append([]domaincaseentity.ModelEntityAliasV1(nil), current.Aliases...),
		}
		return nil
	})
	if err != nil || useCalls != 1 || selection == nil {
		return BoundCaseDelegationV1{}, errors.New("case delegation alias resolution failed closed")
	}
	observed, err := domainjob.NewCaseDelegationContextV1(input.SecurityBinding, semantic)
	expected := input.Expected
	if expected == nil {
		expected = request.CaseDelegation
	}
	if err != nil || expected != nil && !domainjob.CaseDelegationContextsEqualV1(observed, expected) {
		return BoundCaseDelegationV1{}, errors.New("case delegation context changed")
	}
	providerPrompt, err := domainjob.CaseDelegationProviderPromptV1(observed, input.SecurityBinding)
	if err != nil {
		return BoundCaseDelegationV1{}, errors.New("case delegation provider prompt is unavailable")
	}
	request.Prompt = providerPrompt
	request.Name = domainjob.CaseDelegationNameV1
	request.Label = domainjob.CaseDelegationLabelV1
	// These flags now describe host authority, not the discarded provider
	// arguments. Keeping them explicit prevents continue/fork source labels
	// from rewriting the executable typed identity after binding.
	request.NameExplicit = true
	request.LabelExplicit = true
	request.CaseDelegation = domainjob.CloneCaseDelegationContextV1(observed)
	if input.Expected == nil && len(input.AccountFlowAnswerSlots) == 1 {
		binding := input.AccountFlowAnswerSlots[0]
		binding.AnswerSlot.Gaps = append([]string{}, binding.AnswerSlot.Gaps...)
		binding.Claims = append([]domainjob.CaseDelegatedClaimReferenceV1{}, binding.Claims...)
		binding.Evidence = append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, binding.Evidence...)
		request.caseAnswerSlotBinding = &binding
	}
	return BoundCaseDelegationV1{Request: request, Context: observed, selection: selection}, nil
}

func caseDelegationExpectedSemanticIsCurrentV1(
	expected domainjob.CaseDelegationSemanticContextV1,
	observed domainjob.CaseDelegationSemanticContextV1,
) bool {
	if expected.TaskKind != observed.TaskKind || expected.Currentness != observed.Currentness ||
		len(expected.Entities) != len(observed.Entities) {
		return false
	}
	for index := range expected.Entities {
		if expected.Entities[index] != observed.Entities[index] {
			return false
		}
	}
	for _, claim := range expected.Claims {
		if !containsDelegatedClaimV1(observed.Claims, claim) {
			return false
		}
	}
	for _, evidence := range expected.Evidence {
		if !containsDelegatedEvidenceV1(observed.Evidence, evidence) {
			return false
		}
	}
	for _, continuation := range expected.Continuations {
		if !containsDelegatedContinuationV1(observed.Continuations, continuation) {
			return false
		}
	}
	return true
}

func containsDelegatedClaimV1(values []domainjob.CaseDelegatedClaimReferenceV1, expected domainjob.CaseDelegatedClaimReferenceV1) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsDelegatedEvidenceV1(values []domainjob.CaseDelegatedEvidenceReferenceV1, expected domainjob.CaseDelegatedEvidenceReferenceV1) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsDelegatedContinuationV1(values []domainjob.CaseDelegatedContinuationReferenceV1, expected domainjob.CaseDelegatedContinuationReferenceV1) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func cloneCaseDelegationSemanticV1(value domainjob.CaseDelegationSemanticContextV1) domainjob.CaseDelegationSemanticContextV1 {
	value.Entities = append([]domainjob.CaseDelegatedEntitySemanticV1(nil), value.Entities...)
	value.Claims = append([]domainjob.CaseDelegatedClaimReferenceV1{}, value.Claims...)
	value.Evidence = append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, value.Evidence...)
	value.Continuations = append([]domainjob.CaseDelegatedContinuationReferenceV1{}, value.Continuations...)
	value.AnswerSlots = append([]domainjob.CaseDelegatedAnswerSlotCommitmentV1(nil), value.AnswerSlots...)
	return value
}

func consumeCaseDelegationProjectionV1(
	selection caseentityapp.CaseLongitudinalAliasSelectionV1,
) ([]domainjob.CaseDelegatedEntitySemanticV1, caseentityapp.ProviderIngressLongitudinalStateV1, error) {
	entities := []domainjob.CaseDelegatedEntitySemanticV1{}
	var state caseentityapp.ProviderIngressLongitudinalStateV1
	useCalls := 0
	err := selection.ProviderProjection.UseExactWithDescriptorsAndStateV1(func(
		_ string,
		count uint32,
		useDescriptors caseentityapp.ProviderIngressDescriptorsUseV1,
		longitudinal caseentityapp.ProviderIngressLongitudinalStateV1,
	) error {
		useCalls++
		state = longitudinal
		if count != uint32(len(selection.Aliases)) || useDescriptors == nil {
			return caseentityapp.ErrPrivateStateIntegrity
		}
		return useDescriptors(func(
			ordinal uint32,
			alias domaincaseentity.ModelEntityAliasV1,
			entityType string,
			financialAccountType string,
			_ string,
			_ string,
		) error {
			if ordinal >= uint32(len(selection.Aliases)) || selection.Aliases[ordinal] != alias || selection.References[ordinal] == "" {
				return caseentityapp.ErrPrivateStateIntegrity
			}
			entities = append(entities, domainjob.CaseDelegatedEntitySemanticV1{
				Alias: alias, EntityType: entityType, FinancialAccountType: financialAccountType,
				ResolutionDigest: domainsecurity.SHA256Hex([]byte(strings.Join([]string{
					"analytix.case-delegation-resolution/v1",
					string(alias),
					string(selection.References[ordinal]),
				}, "\x00"))),
			})
			return nil
		})
	})
	if err != nil || useCalls != 1 || len(entities) != len(selection.Aliases) {
		return nil, caseentityapp.ProviderIngressLongitudinalStateV1{}, caseentityapp.ErrPrivateStateIntegrity
	}
	return entities, state, nil
}

func caseDelegationAliasesFromPromptV1(prompt string) ([]domaincaseentity.ModelEntityAliasV1, error) {
	if len(prompt) == 0 || len(prompt) > maxCaseDelegationRawPromptBytesV1 {
		return nil, errors.New("case delegation prompt is empty or unbounded")
	}
	tokens := strings.FieldsFunc(prompt, func(current rune) bool {
		return !((current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') || current == ':')
	})
	aliases := make([]domaincaseentity.ModelEntityAliasV1, 0, 4)
	seen := make(map[domaincaseentity.ModelEntityAliasV1]struct{})
	for _, token := range tokens {
		separator := strings.IndexByte(token, ':')
		if separator <= 0 || separator == len(token)-1 ||
			!caseDelegationAliasPrefixCandidateV1(token[:separator]) ||
			!caseDelegationAliasOrdinalCandidateV1(token[separator+1:]) {
			continue
		}
		if domaincaseentity.ValidateModelEntityAliasV1(token) != nil {
			return nil, errors.New("case delegation alias is malformed or unsupported")
		}
		alias := domaincaseentity.ModelEntityAliasV1(token)
		if _, duplicate := seen[alias]; duplicate {
			return nil, errors.New("case delegation alias is duplicated")
		}
		seen[alias] = struct{}{}
		aliases = append(aliases, alias)
		if len(aliases) > domainjob.MaxCaseDelegationEntitiesV1 {
			return nil, errors.New("case delegation alias set is unbounded")
		}
	}
	if len(aliases) == 0 {
		return nil, errors.New("case delegation alias set is empty")
	}
	sort.Slice(aliases, func(left, right int) bool { return aliases[left] < aliases[right] })
	return aliases, nil
}

func caseDelegationAliasPrefixCandidateV1(value string) bool {
	for _, current := range value {
		if !((current >= 'A' && current <= 'Z') || (current >= 'a' && current <= 'z')) {
			return false
		}
	}
	return value != ""
}

func caseDelegationAliasOrdinalCandidateV1(value string) bool {
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return value != ""
}

func joinCaseDelegationAliasesV1(aliases []domaincaseentity.ModelEntityAliasV1) string {
	values := make([]string, len(aliases))
	for index, alias := range aliases {
		values[index] = string(alias)
	}
	return strings.Join(values, ", ")
}

func mustCaseDelegationProviderPromptV1(context *domainjob.CaseDelegationContextV1, binding *domainjob.SecurityBinding) string {
	prompt, _ := domainjob.CaseDelegationProviderPromptV1(context, binding)
	return prompt
}
