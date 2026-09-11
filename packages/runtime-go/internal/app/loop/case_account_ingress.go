package loop

import (
	"context"
	"encoding/json"
	"strings"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// InitialCaseAccountIngressInputV1 keeps the source-exact prompt inside the
// case-entity compiler input. The loop package retains only the safe public
// prompt and a boolean describing whether projection removed private data.
type InitialCaseAccountIngressInputV1 struct {
	policy               *CaseFundAnalysisPolicy
	securityContext      domainsecurity.TurnSecurityContext
	publicPrompt         string
	rawDiffersFromPublic bool
	subagent             bool
	relation             string
	sourceThreadID       string
	service              *caseentityapp.Service
	compileInput         caseentityapp.CompileAccountIngressInputV1
}

func NewInitialCaseAccountIngressInputV1(
	policy *CaseFundAnalysisPolicy,
	securityContext domainsecurity.TurnSecurityContext,
	publicPrompt, rawPrompt string,
	subagent bool,
	relation, sourceThreadID string,
	service *caseentityapp.Service,
	resolve caseentityapp.ResolveAccountIngressCandidatesV1,
) InitialCaseAccountIngressInputV1 {
	return InitialCaseAccountIngressInputV1{
		policy: policy, securityContext: securityContext, publicPrompt: publicPrompt,
		rawDiffersFromPublic: rawPrompt != publicPrompt, subagent: subagent,
		relation: strings.TrimSpace(relation), sourceThreadID: strings.TrimSpace(sourceThreadID), service: service,
		compileInput: caseentityapp.NewCompileAccountIngressInputV1(
			securityContext, domaincaseentity.CaseIngressKindTurnV1, 0, rawPrompt, resolve,
		).WithCaseContinuityV1(relation, sourceThreadID),
	}
}

type InitialCaseAccountIngressResultV1 struct {
	Policy              *CaseFundAnalysisPolicy
	SourceUnavailable   bool
	ProviderProjection  *caseentityapp.ProviderIngressProjectionV1
	HostEntitySelection HostCaseEntitySelectionV1
}

// PrepareInitialCaseAccountIngressV1 compiles a provider-only stable-alias
// projection while leaving the durable prompt in its ordinary [ACCOUNT]
// representation. Any private-source failure closes only the case-funds lane.
func PrepareInitialCaseAccountIngressV1(
	ctx context.Context,
	input InitialCaseAccountIngressInputV1,
) InitialCaseAccountIngressResultV1 {
	result := InitialCaseAccountIngressResultV1{Policy: input.policy}
	if input.policy == nil || !input.policy.Active || input.policy.SourceUnavailable ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.securityContext) != nil {
		return result
	}
	unavailable := func() InitialCaseAccountIngressResultV1 {
		return InitialCaseAccountIngressUnavailableV1(input.policy)
	}
	if input.subagent {
		if input.rawDiffersFromPublic {
			return unavailable()
		}
		return result
	}
	if input.service == nil {
		if input.rawDiffersFromPublic {
			return unavailable()
		}
		return result
	}
	compiled, err := input.service.CompileAccountIngressV1(ctx, input.compileInput)
	if err != nil || privacyprojectionapp.ProjectOrdinaryText(compiled.PublicText) != input.publicPrompt {
		return unavailable()
	}
	if compiled.IsNoop() {
		aliases, aliasesErr := explicitCaseModelAliasesV1(input.publicPrompt)
		if aliasesErr != nil {
			return unavailable()
		}
		entityTypes := taskRelevantFinancialEntityTypesV1(input.publicPrompt)
		if len(aliases) == 0 && len(entityTypes) == 0 {
			return result
		}
		continuityCalls := 0
		consume := func(selection caseentityapp.CaseLongitudinalAliasSelectionV1) error {
			continuityCalls++
			if len(selection.Aliases) == 0 || len(selection.Aliases) != len(selection.References) {
				return caseentityapp.ErrPrivateStateIntegrity
			}
			var selectionErr error
			result.HostEntitySelection, selectionErr = NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(
				input.securityContext,
				selection.Record,
				selection.References,
				selection.Aliases,
			)
			projection := selection.ProviderProjection
			result.ProviderProjection = &projection
			return selectionErr
		}
		if len(aliases) != 0 {
			err = input.service.UseCaseLongitudinalAliasesV1(ctx, caseentityapp.UseCaseLongitudinalAliasesInputV1{
				SecurityContext: input.securityContext, Aliases: aliases, ProviderText: input.publicPrompt,
				Relation: input.relation, SourceThreadID: input.sourceThreadID,
			}, consume)
		} else {
			err = input.service.UseCaseLongitudinalEntityTypesV1(ctx, caseentityapp.UseCaseLongitudinalEntityTypesInputV1{
				SecurityContext: input.securityContext, EntityTypes: entityTypes, ProviderText: input.publicPrompt,
				Relation: input.relation, SourceThreadID: input.sourceThreadID,
			}, consume)
		}
		if err != nil || continuityCalls != 1 || !result.HostEntitySelection.AvailableV1() || result.ProviderProjection == nil {
			return unavailable()
		}
		return result
	}
	if !compiled.IsPersisted() {
		return unavailable()
	}
	var hostSelection HostCaseEntitySelectionV1
	selectionCalls := 0
	err = compiled.UsePrivateIngressEntitySelectionV1(func(
		record caseentityapp.PrivateRecordReferenceV1,
		references []domaincaseentity.ReferenceV1,
		aliases []domaincaseentity.ModelEntityAliasV1,
	) error {
		selectionCalls++
		if selectionCalls != 1 {
			return caseentityapp.ErrPrivateStateIntegrity
		}
		var selectionErr error
		if appendErr := input.service.AppendCaseLongitudinalIngressV1(ctx, caseentityapp.AppendCaseLongitudinalIngressInputV1{
			SecurityContext: input.securityContext, References: references,
			Relation: input.relation, SourceThreadID: input.sourceThreadID,
		}); appendErr != nil {
			return appendErr
		}
		hostSelection, selectionErr = NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(
			input.securityContext, record, references, aliases,
		)
		return selectionErr
	})
	if err != nil || selectionCalls != 1 || !hostSelection.AvailableV1() {
		return unavailable()
	}
	providerCalls := 0
	var projection caseentityapp.ProviderIngressProjectionV1
	err = compiled.UseProviderProjectionV1(
		func(current caseentityapp.ProviderIngressProjectionV1) error {
			providerCalls++
			projection = current
			return nil
		},
	)
	if err != nil || providerCalls != 1 {
		return unavailable()
	}
	result.ProviderProjection = &projection
	result.HostEntitySelection = hostSelection
	return result
}

func taskRelevantFinancialEntityTypesV1(text string) []string {
	english := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(current rune) bool {
		return !((current >= 'a' && current <= 'z') || (current >= '0' && current <= '9'))
	}) {
		english[token] = true
	}
	entityTypes := make([]string, 0, 2)
	if english["account"] || english["accounts"] || english["acct"] ||
		strings.Contains(text, "账户") || strings.Contains(text, "账号") {
		entityTypes = append(entityTypes, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1)
	}
	if english["card"] || english["cards"] || strings.Contains(text, "银行卡") || strings.Contains(text, "卡号") {
		entityTypes = append(entityTypes, domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1)
	}
	return entityTypes
}

func explicitCaseModelAliasesV1(text string) ([]domaincaseentity.ModelEntityAliasV1, error) {
	tokens := strings.FieldsFunc(text, func(current rune) bool {
		return !((current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') || current == ':')
	})
	seen := make(map[domaincaseentity.ModelEntityAliasV1]struct{})
	aliases := make([]domaincaseentity.ModelEntityAliasV1, 0, 4)
	for _, token := range tokens {
		lower := strings.ToLower(token)
		candidate := strings.HasPrefix(lower, "acct:") || strings.HasPrefix(lower, "card:") ||
			strings.HasPrefix(lower, "person:") || strings.HasPrefix(lower, "org:") ||
			strings.HasPrefix(lower, "phone:") || strings.HasPrefix(lower, "device:") ||
			strings.HasPrefix(lower, "merchant:")
		if !candidate {
			continue
		}
		if domaincaseentity.ValidateModelEntityAliasV1(token) != nil {
			return nil, caseentityapp.ErrPrivateStateIntegrity
		}
		alias := domaincaseentity.ModelEntityAliasV1(token)
		if _, duplicate := seen[alias]; duplicate {
			return nil, caseentityapp.ErrPrivateStateIntegrity
		}
		seen[alias] = struct{}{}
		aliases = append(aliases, alias)
	}
	return aliases, nil
}

func InitialCaseAccountIngressUnavailableV1(
	policy *CaseFundAnalysisPolicy,
) InitialCaseAccountIngressResultV1 {
	if policy == nil || !policy.Active {
		return InitialCaseAccountIngressResultV1{Policy: policy}
	}
	ordinaryPrompt := policy.OrdinaryPrompt
	unavailable := CaseFundSourceUnavailablePolicyV1(policy.OrdinaryWorkRequested)
	unavailable.OrdinaryPrompt = ordinaryPrompt
	unavailable.OrdinaryWorkRequested = strings.TrimSpace(ordinaryPrompt) != ""
	return InitialCaseAccountIngressResultV1{Policy: &unavailable, SourceUnavailable: true}
}

func ConsumeInitialProviderIngressV1(
	projection *caseentityapp.ProviderIngressProjectionV1,
	fallback string,
) (string, error) {
	consumed, err := ConsumeInitialProviderIngressWithAliasesV1(projection, fallback)
	return consumed.Text, err
}

type InitialProviderIngressConsumptionV1 struct {
	Text    string
	Aliases []domaincaseentity.ModelEntityAliasV1
}

type ResolveInitialProviderIngressInputV1 struct {
	Projection          *caseentityapp.ProviderIngressProjectionV1
	Fallback            string
	Policy              *CaseFundAnalysisPolicy
	SourceUnavailable   bool
	HostEntitySelection HostCaseEntitySelectionV1
}

type ResolvedInitialProviderIngressV1 struct {
	Consumption         InitialProviderIngressConsumptionV1
	Policy              *CaseFundAnalysisPolicy
	SourceUnavailable   bool
	HostEntitySelection HostCaseEntitySelectionV1
}

func ResolveInitialProviderIngressV1(
	input ResolveInitialProviderIngressInputV1,
) ResolvedInitialProviderIngressV1 {
	consumed, err := ConsumeInitialProviderIngressWithAliasesV1(input.Projection, input.Fallback)
	result := ResolvedInitialProviderIngressV1{
		Consumption: consumed, Policy: input.Policy, SourceUnavailable: input.SourceUnavailable,
		HostEntitySelection: input.HostEntitySelection,
	}
	if err == nil && (input.Projection == nil || input.HostEntitySelection.AvailableV1()) {
		return result
	}
	unavailable := InitialCaseAccountIngressUnavailableV1(input.Policy)
	result.Consumption = InitialProviderIngressConsumptionV1{Text: input.Fallback}
	result.Policy = unavailable.Policy
	result.SourceUnavailable = unavailable.SourceUnavailable
	result.HostEntitySelection = HostCaseEntitySelectionV1{}
	return result
}

func ConsumeInitialProviderIngressWithAliasesV1(
	projection *caseentityapp.ProviderIngressProjectionV1,
	fallback string,
) (InitialProviderIngressConsumptionV1, error) {
	if projection == nil {
		return InitialProviderIngressConsumptionV1{Text: fallback}, nil
	}
	providerText := ""
	providerAliases := []domaincaseentity.ModelEntityAliasV1{}
	useCalls := 0
	err := projection.UseExactWithDescriptorsAndStateV1(func(
		current string,
		descriptorCount uint32,
		useDescriptors caseentityapp.ProviderIngressDescriptorsUseV1,
		longitudinal caseentityapp.ProviderIngressLongitudinalStateV1,
	) error {
		useCalls++
		if descriptorCount == 0 || useDescriptors == nil {
			return caseentityapp.ErrPrivateStateUnavailable
		}
		descriptors := make([]providerIngressEntitySemanticV1, 0, descriptorCount)
		if err := useDescriptors(func(
			ordinal uint32,
			alias domaincaseentity.ModelEntityAliasV1,
			entityType string,
			financialAccountType string,
			bankInstitution string,
			accountType string,
		) error {
			if ordinal != uint32(len(descriptors)) {
				return caseentityapp.ErrPrivateStateIntegrity
			}
			descriptors = append(descriptors, providerIngressEntitySemanticV1{
				Alias:                string(alias),
				EntityType:           entityType,
				FinancialAccountType: financialAccountType,
				BankInstitution:      bankInstitution,
				AccountType:          accountType,
			})
			providerAliases = append(providerAliases, alias)
			return nil
		}); err != nil || len(descriptors) != int(descriptorCount) {
			return caseentityapp.ErrPrivateStateUnavailable
		}
		semantic := providerIngressSemanticContextV1{Entities: descriptors}
		if longitudinal.SchemaVersion != 0 {
			state := providerIngressLongitudinalSemanticV1{
				SchemaVersion:      longitudinal.SchemaVersion,
				ScopeBindingDigest: longitudinal.ScopeBindingDigest,
				SelectionBudget:    longitudinal.SelectionBudget,
				Items:              make([]providerIngressLongitudinalItemSemanticV1, len(longitudinal.Items)),
				OmittedTotal:       longitudinal.OmittedTotal,
				OmittedCoverage:    make([]providerIngressLongitudinalOmittedCoverageSemanticV1, len(longitudinal.OmittedCoverage)),
			}
			for index, item := range longitudinal.Items {
				state.Items[index] = providerIngressLongitudinalItemSemanticV1{
					Kind: item.Kind, ReferenceKind: item.ReferenceKind,
					ReferenceDigest: item.ReferenceDigest, Digest: item.Digest,
					Currentness: item.Currentness, InvestigationState: item.InvestigationState,
					ClaimType: item.ClaimType, EvidenceBindingDigest: item.EvidenceBindingDigest,
					EvidenceReferenceCount:        item.EvidenceReferenceCount,
					CounterEvidenceBindingDigest:  item.CounterEvidenceBindingDigest,
					CounterEvidenceReferenceCount: item.CounterEvidenceReferenceCount,
					SnapshotBindingDigest:         item.SnapshotBindingDigest,
					ComparedSnapshotBindingDigest: item.ComparedSnapshotBindingDigest,
				}
			}
			for index, coverage := range longitudinal.OmittedCoverage {
				state.OmittedCoverage[index] = providerIngressLongitudinalOmittedCoverageSemanticV1{
					Kind: coverage.Kind, Count: coverage.Count,
				}
			}
			semantic.Longitudinal = &state
		}
		semanticJSON, err := json.Marshal(semantic)
		if err != nil {
			return caseentityapp.ErrPrivateStateUnavailable
		}
		providerText = current + "\n\n" + providerIngressSemanticContextStartV1 + "\n" +
			string(semanticJSON) + "\n" + providerIngressSemanticContextEndV1
		return nil
	})
	if err != nil || useCalls != 1 || strings.TrimSpace(providerText) == "" {
		return InitialProviderIngressConsumptionV1{}, caseentityapp.ErrPrivateStateUnavailable
	}
	return InitialProviderIngressConsumptionV1{
		Text: providerText, Aliases: append([]domaincaseentity.ModelEntityAliasV1(nil), providerAliases...),
	}, nil
}

const (
	providerIngressSemanticContextStartV1 = "<analytix_host_verified_case_entity_semantics>"
	providerIngressSemanticContextEndV1   = "</analytix_host_verified_case_entity_semantics>"
)

// providerIngressSemanticContextV1 is provider-only, process-local context.
// It is assembled from the current snapshot's trusted resolver output and is
// never persisted in the ordinary thread, SSE, log, or public projection.
type providerIngressSemanticContextV1 struct {
	Entities     []providerIngressEntitySemanticV1      `json:"entities"`
	Longitudinal *providerIngressLongitudinalSemanticV1 `json:"longitudinal,omitempty"`
}

type providerIngressLongitudinalSemanticV1 struct {
	SchemaVersion      int                                                    `json:"schemaVersion"`
	ScopeBindingDigest string                                                 `json:"scopeBindingDigest"`
	SelectionBudget    uint32                                                 `json:"selectionBudget"`
	Items              []providerIngressLongitudinalItemSemanticV1            `json:"items"`
	OmittedTotal       uint32                                                 `json:"omittedTotal"`
	OmittedCoverage    []providerIngressLongitudinalOmittedCoverageSemanticV1 `json:"omittedCoverage"`
}

type providerIngressLongitudinalItemSemanticV1 struct {
	Kind                          string `json:"kind"`
	ReferenceKind                 string `json:"referenceKind"`
	ReferenceDigest               string `json:"referenceDigest,omitempty"`
	Digest                        string `json:"digest,omitempty"`
	Currentness                   string `json:"currentness,omitempty"`
	InvestigationState            string `json:"investigationState,omitempty"`
	ClaimType                     string `json:"claimType,omitempty"`
	EvidenceBindingDigest         string `json:"evidenceBindingDigest,omitempty"`
	EvidenceReferenceCount        uint32 `json:"evidenceReferenceCount,omitempty"`
	CounterEvidenceBindingDigest  string `json:"counterEvidenceBindingDigest,omitempty"`
	CounterEvidenceReferenceCount uint32 `json:"counterEvidenceReferenceCount,omitempty"`
	SnapshotBindingDigest         string `json:"snapshotBindingDigest,omitempty"`
	ComparedSnapshotBindingDigest string `json:"comparedSnapshotBindingDigest,omitempty"`
}

type providerIngressLongitudinalOmittedCoverageSemanticV1 struct {
	Kind  string `json:"kind"`
	Count uint32 `json:"count"`
}

type providerIngressEntitySemanticV1 struct {
	Alias                string `json:"alias"`
	EntityType           string `json:"entityType"`
	FinancialAccountType string `json:"financialAccountType"`
	BankInstitution      string `json:"bankInstitution,omitempty"`
	AccountType          string `json:"accountType,omitempty"`
}
