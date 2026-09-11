package privacyprojection

import (
	"encoding/json"
	"sort"
	"strings"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// providerCaseReferenceAllowsetV1 remains an internal parameter type for the
// projection walkers, but the Provider boundary intentionally never
// populates it. AuthorityEntityRef values are host-private in every effect.
type providerCaseReferenceAllowsetV1 map[domaincaseentity.ReferenceV1]struct{}

type boundProviderCaseAliasesV1 struct {
	context         domainsecurity.TurnSecurityContext
	messageSHA256   string
	projectedSHA256 string
	aliases         []domaincaseentity.ModelEntityAliasV1
}

const (
	providerCaseSemanticStartV1                      = "<analytix_host_verified_case_entity_semantics>"
	providerCaseSemanticEndV1                        = "</analytix_host_verified_case_entity_semantics>"
	providerCaseLongitudinalSelectionDigestPurposeV1 = "analytix.case-longitudinal-selection/v1"
)

type providerCaseSemanticEnvelopeV1 struct {
	Entities     []providerCaseSemanticEntityV1      `json:"entities"`
	Longitudinal *providerCaseLongitudinalSemanticV1 `json:"longitudinal,omitempty"`
}

type providerCaseSemanticEntityV1 struct {
	Alias                string `json:"alias"`
	EntityType           string `json:"entityType"`
	FinancialAccountType string `json:"financialAccountType"`
	BankInstitution      string `json:"bankInstitution,omitempty"`
	AccountType          string `json:"accountType,omitempty"`
}

type providerCaseLongitudinalSemanticV1 struct {
	SchemaVersion      int                                                 `json:"schemaVersion"`
	ScopeBindingDigest string                                              `json:"scopeBindingDigest"`
	SelectionBudget    uint32                                              `json:"selectionBudget"`
	Items              []providerCaseLongitudinalItemSemanticV1            `json:"items"`
	OmittedTotal       uint32                                              `json:"omittedTotal"`
	OmittedCoverage    []providerCaseLongitudinalOmittedCoverageSemanticV1 `json:"omittedCoverage"`
}

type providerCaseLongitudinalItemSemanticV1 struct {
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

type providerCaseLongitudinalOmittedCoverageSemanticV1 struct {
	Kind  string `json:"kind"`
	Count uint32 `json:"count"`
}

var providerCaseLongitudinalPriorityV1 = []string{
	"current_verified_fact",
	"historical_comparison_fact",
	"key_relationship",
	"counterevidence_refuted_finding",
	"data_gap",
	"evidence_claim_reference",
	"snapshot_difference",
}

// BindProviderCaseAliasesToMessageV1 marks only the exact host-compiled
// provider message that contains current case aliases. It grants no authority
// to a long reference and does not make an unvalidated user alias meaningful.
func BindProviderCaseAliasesToMessageV1(
	context domainsecurity.TurnSecurityContext,
	aliases []domaincaseentity.ModelEntityAliasV1,
	message *domainmodel.Message,
) error {
	if message == nil || message.PrivateProviderSemanticBinding != nil ||
		message.PrivateProviderReferenceBinding != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := canonicalProviderCaseAliasesV1(aliases)
	if err != nil {
		return err
	}
	for _, alias := range canonical {
		if !strings.Contains(message.Content, string(alias)) {
			return ErrProviderPrivacyAuthorityUnavailable
		}
	}
	messageSHA256, err := providerReferenceMessageSHA256V1(*message)
	if err != nil {
		return err
	}
	projected, err := projectBoundProviderCaseAliasesV1(context, message.Content, canonical)
	if err != nil {
		return err
	}
	message.PrivateProviderSemanticBinding = boundProviderCaseAliasesV1{
		context: context, messageSHA256: messageSHA256,
		projectedSHA256: domainsecurity.SHA256Hex([]byte(projected)), aliases: canonical,
	}
	return nil
}

func collectProviderCaseReferenceProvenanceV1(
	context domainsecurity.TurnSecurityContext,
	request domainmodel.Request,
	preserveCaseEntityReferences bool,
) (providerCaseReferenceAllowsetV1, map[int]validatedAccountFlowProviderSemanticV1, error) {
	allowed := providerCaseReferenceAllowsetV1{}
	validatedSemantics := make(map[int]validatedAccountFlowProviderSemanticV1)
	if request.PrivateProviderReferenceBinding != nil {
		return nil, nil, ErrProviderPrivacyAuthorityUnavailable
	}
	for index, message := range request.Messages {
		if message.PrivateProviderReferenceBinding != nil {
			return nil, nil, ErrProviderPrivacyAuthorityUnavailable
		}
		if message.PrivateProviderSemanticBinding == nil {
			continue
		}
		if !preserveCaseEntityReferences ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
			return nil, nil, ErrProviderPrivacyAuthorityUnavailable
		}
		var validated validatedAccountFlowProviderSemanticV1
		var err error
		switch message.PrivateProviderSemanticBinding.(type) {
		case boundAccountFlowProviderSemanticV1:
			validated, err = validateBoundAccountFlowProviderSemanticV1(context, message, true)
		case boundAccountFlowSafeHistoryV1:
			validated, err = validateBoundAccountFlowSafeHistoryV1(context, message)
		case boundProviderCaseAliasesV1:
			validated, err = validateBoundProviderCaseAliasesV1(context, message)
		case boundCaseForegroundProviderSemanticV1:
			validated, err = validateBoundCaseForegroundProviderSemanticV1(context, message)
		case boundHostChildCasePromptV1:
			validated, err = validateBoundHostChildCasePromptV1(context, message)
		default:
			err = ErrProviderPrivacyAuthorityUnavailable
		}
		if err != nil || len(validated.references) != 0 {
			return nil, nil, ErrProviderPrivacyAuthorityUnavailable
		}
		validatedSemantics[index] = validated
	}
	return allowed, validatedSemantics, nil
}

func validateBoundProviderCaseAliasesV1(
	context domainsecurity.TurnSecurityContext,
	message domainmodel.Message,
) (validatedAccountFlowProviderSemanticV1, error) {
	binding, ok := message.PrivateProviderSemanticBinding.(boundProviderCaseAliasesV1)
	if !ok || binding.context != context ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := canonicalProviderCaseAliasesV1(binding.aliases)
	messageSHA256, digestErr := providerReferenceMessageSHA256V1(message)
	if err != nil || digestErr != nil || messageSHA256 != binding.messageSHA256 ||
		!sameProviderCaseAliasesV1(canonical, binding.aliases) {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	projected, err := projectBoundProviderCaseAliasesV1(context, message.Content, canonical)
	if err != nil {
		return validatedAccountFlowProviderSemanticV1{}, err
	}
	if !domainsecurity.IsSHA256Hex(binding.projectedSHA256) ||
		domainsecurity.SHA256Hex([]byte(projected)) != binding.projectedSHA256 {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	return validatedAccountFlowProviderSemanticV1{
		canonical: []byte(projected), digest: domainsecurity.SHA256Hex([]byte(projected)),
		references: []domaincaseentity.ReferenceV1{}, kind: validatedProviderSemanticSafeHistoryV1,
	}, nil
}

func projectBoundProviderCaseAliasesV1(
	context domainsecurity.TurnSecurityContext,
	text string,
	aliases []domaincaseentity.ModelEntityAliasV1,
) (string, error) {
	startCount := strings.Count(text, providerCaseSemanticStartV1)
	endCount := strings.Count(text, providerCaseSemanticEndV1)
	if startCount == 0 && endCount == 0 {
		return projectProviderTextWithModelAliasesV1(text, aliases)
	}
	if startCount != 1 || endCount != 1 {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	start := strings.Index(text, providerCaseSemanticStartV1)
	semanticStart := start + len(providerCaseSemanticStartV1)
	endOffset := strings.Index(text[semanticStart:], providerCaseSemanticEndV1)
	if start < 0 || endOffset < 0 {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	end := semanticStart + endOffset
	if strings.TrimSpace(text[end+len(providerCaseSemanticEndV1):]) != "" {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	canonicalSemantic, err := canonicalProviderCaseSemanticV1(
		context, strings.TrimSpace(text[semanticStart:end]), aliases,
	)
	if err != nil {
		return "", err
	}
	projectedPrefix, err := projectProviderTextWithModelAliasesV1(text[:start], aliases)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(projectedPrefix, " \t\r\n") + "\n\n" +
		providerCaseSemanticStartV1 + "\n" + string(canonicalSemantic) + "\n" + providerCaseSemanticEndV1, nil
}

func canonicalProviderCaseSemanticV1(
	context domainsecurity.TurnSecurityContext,
	raw string,
	aliases []domaincaseentity.ModelEntityAliasV1,
) ([]byte, error) {
	canonicalAliases, err := canonicalProviderCaseAliasesV1(aliases)
	if err != nil || raw == "" {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	var semantic providerCaseSemanticEnvelopeV1
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&semantic) != nil || !jsonDecoderAtEOFV1(decoder) ||
		len(semantic.Entities) != len(canonicalAliases) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	semanticAliases := make([]domaincaseentity.ModelEntityAliasV1, len(semantic.Entities))
	for index, entity := range semantic.Entities {
		alias := domaincaseentity.ModelEntityAliasV1(entity.Alias)
		entityType, ordinal, parseErr := domaincaseentity.ParseModelEntityAliasV1(entity.Alias)
		label, labelErr := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
			EntityType: entity.EntityType, StableOrdinal: ordinal,
			Institution: entity.BankInstitution, AccountType: entity.AccountType,
		})
		if parseErr != nil || labelErr != nil || entityType != entity.EntityType ||
			entity.FinancialAccountType != entity.EntityType || label.Institution != entity.BankInstitution ||
			label.AccountType != entity.AccountType {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		semanticAliases[index] = alias
	}
	if !sameProviderCaseAliasesV1(semanticAliases, canonicalAliases) ||
		validateProviderCaseLongitudinalSemanticV1(context, semantic.Longitudinal) != nil {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical, err := json.Marshal(semantic)
	if err != nil || string(canonical) != raw || domaincaseentity.ContainsReferenceCandidateV1(string(canonical)) {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	return canonical, nil
}

func validateProviderCaseLongitudinalSemanticV1(
	context domainsecurity.TurnSecurityContext,
	value *providerCaseLongitudinalSemanticV1,
) error {
	if value == nil {
		return nil
	}
	if value.SchemaVersion != 1 ||
		value.ScopeBindingDigest != providerCaseLongitudinalScopeBindingDigestV1(context.CaseBindingHash) ||
		value.SelectionBudget != 32 || len(value.Items) > 32 ||
		len(value.OmittedCoverage) != len(providerCaseLongitudinalPriorityV1) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	omittedTotal := uint32(0)
	for index, coverage := range value.OmittedCoverage {
		if coverage.Kind != providerCaseLongitudinalPriorityV1[index] ||
			^uint32(0)-omittedTotal < coverage.Count {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		omittedTotal += coverage.Count
	}
	if omittedTotal != value.OmittedTotal {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	seen := make(map[string]bool, len(value.Items))
	previousPriority := -1
	previousKey := ""
	for _, item := range value.Items {
		if validateProviderCaseLongitudinalItemV1(item) != nil {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		priority := providerCaseLongitudinalPriorityIndexV1(item.Kind)
		body, _ := json.Marshal(item)
		key := string(body)
		if seen[key] || priority < previousPriority || priority == previousPriority && key < previousKey {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		seen[key] = true
		previousPriority = priority
		previousKey = key
	}
	return nil
}

func providerCaseLongitudinalScopeBindingDigestV1(caseBindingHash string) string {
	if !domainsecurity.IsSHA256Hex(caseBindingHash) {
		return ""
	}
	body, _ := json.Marshal(struct {
		Purpose         string `json:"purpose"`
		CaseBindingHash string `json:"caseBindingHash"`
		BindingKind     string `json:"bindingKind"`
	}{
		Purpose:         providerCaseLongitudinalSelectionDigestPurposeV1,
		CaseBindingHash: caseBindingHash,
		BindingKind:     "scope",
	})
	return domainsecurity.SHA256Hex(body)
}

func validateProviderCaseLongitudinalItemV1(item providerCaseLongitudinalItemSemanticV1) error {
	if providerCaseLongitudinalPriorityIndexV1(item.Kind) < 0 ||
		(item.Digest != "" && !domainsecurity.IsSHA256Hex(item.Digest)) ||
		(item.EvidenceReferenceCount == 0) != (item.EvidenceBindingDigest == "") ||
		(item.EvidenceBindingDigest != "" && !domainsecurity.IsSHA256Hex(item.EvidenceBindingDigest)) ||
		(item.CounterEvidenceReferenceCount == 0) != (item.CounterEvidenceBindingDigest == "") ||
		(item.CounterEvidenceBindingDigest != "" && !domainsecurity.IsSHA256Hex(item.CounterEvidenceBindingDigest)) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	switch item.ReferenceKind {
	case "claim":
		if !domainsecurity.IsSHA256Hex(item.ReferenceDigest) ||
			!validProviderCaseCurrentnessV1(item.Currentness) ||
			!validProviderCaseInvestigationV1(item.InvestigationState) ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) ||
			item.ComparedSnapshotBindingDigest != "" {
			return ErrProviderPrivacyAuthorityUnavailable
		}
		if item.ClaimType != "" {
			if _, err := domaincaseentity.NewCaseClaimTypedStateV1(item.ClaimType); err != nil {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		}
		switch item.Kind {
		case "current_verified_fact":
			if item.Currentness != domaincaseentity.SnapshotCurrentV1 ||
				item.InvestigationState != domaincaseentity.InvestigationConfirmedV1 ||
				item.EvidenceReferenceCount == 0 || item.CounterEvidenceReferenceCount != 0 ||
				item.ClaimType == "relationship" {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		case "historical_comparison_fact":
			if item.Currentness == domaincaseentity.SnapshotCurrentV1 ||
				item.InvestigationState != domaincaseentity.InvestigationConfirmedV1 ||
				item.EvidenceReferenceCount == 0 || item.CounterEvidenceReferenceCount != 0 ||
				item.ClaimType == "relationship" {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		case "key_relationship":
			if item.ClaimType != "relationship" ||
				item.InvestigationState == domaincaseentity.InvestigationRejectedV1 ||
				item.CounterEvidenceReferenceCount != 0 ||
				item.InvestigationState == domaincaseentity.InvestigationConfirmedV1 && item.EvidenceReferenceCount == 0 {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		case "counterevidence_refuted_finding":
			if item.CounterEvidenceReferenceCount == 0 {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		case "evidence_claim_reference":
			if item.InvestigationState != domaincaseentity.InvestigationOpenV1 {
				return ErrProviderPrivacyAuthorityUnavailable
			}
		default:
			return ErrProviderPrivacyAuthorityUnavailable
		}
	case "evidence":
		if item.Kind != "evidence_claim_reference" ||
			!domainsecurity.IsSHA256Hex(item.ReferenceDigest) ||
			!validProviderCaseCurrentnessV1(item.Currentness) ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) ||
			item.InvestigationState != "" || item.ClaimType != "" ||
			item.EvidenceReferenceCount != 0 || item.CounterEvidenceReferenceCount != 0 ||
			item.ComparedSnapshotBindingDigest != "" {
			return ErrProviderPrivacyAuthorityUnavailable
		}
	case "continuation":
		if item.Kind != "evidence_claim_reference" || item.ReferenceDigest != "" ||
			!domainsecurity.IsSHA256Hex(item.Digest) || !validProviderCaseCurrentnessV1(item.Currentness) ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) || item.InvestigationState != "" ||
			item.ClaimType != "" || item.EvidenceReferenceCount != 0 ||
			item.CounterEvidenceReferenceCount != 0 || item.ComparedSnapshotBindingDigest != "" {
			return ErrProviderPrivacyAuthorityUnavailable
		}
	case "data_gap", "open_question":
		if item.Kind != "data_gap" || !domainsecurity.IsSHA256Hex(item.ReferenceDigest) ||
			item.Digest != "" || item.Currentness != "" ||
			item.InvestigationState != domaincaseentity.InvestigationOpenV1 || item.ClaimType != "" ||
			item.EvidenceReferenceCount != 0 || item.CounterEvidenceReferenceCount != 0 ||
			item.SnapshotBindingDigest != "" || item.ComparedSnapshotBindingDigest != "" {
			return ErrProviderPrivacyAuthorityUnavailable
		}
	case "snapshot":
		if item.Kind != "snapshot_difference" || item.ReferenceDigest != "" || item.Digest != "" ||
			item.Currentness == domaincaseentity.SnapshotCurrentV1 || !validProviderCaseCurrentnessV1(item.Currentness) ||
			item.InvestigationState != "" || item.ClaimType != "" ||
			item.EvidenceReferenceCount != 0 || item.CounterEvidenceReferenceCount != 0 ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) ||
			!domainsecurity.IsSHA256Hex(item.ComparedSnapshotBindingDigest) ||
			item.SnapshotBindingDigest == item.ComparedSnapshotBindingDigest {
			return ErrProviderPrivacyAuthorityUnavailable
		}
	default:
		return ErrProviderPrivacyAuthorityUnavailable
	}
	return nil
}

func providerCaseLongitudinalPriorityIndexV1(kind string) int {
	for index, current := range providerCaseLongitudinalPriorityV1 {
		if kind == current {
			return index
		}
	}
	return -1
}

func validProviderCaseCurrentnessV1(value string) bool {
	return value == domaincaseentity.SnapshotCurrentV1 || value == domaincaseentity.SnapshotHistoricalV1 ||
		value == domaincaseentity.SnapshotStaleV1 || value == domaincaseentity.SnapshotSupersededV1
}

func validProviderCaseInvestigationV1(value string) bool {
	return value == domaincaseentity.InvestigationConfirmedV1 ||
		value == domaincaseentity.InvestigationRejectedV1 || value == domaincaseentity.InvestigationOpenV1
}

func canonicalProviderCaseAliasesV1(
	aliases []domaincaseentity.ModelEntityAliasV1,
) ([]domaincaseentity.ModelEntityAliasV1, error) {
	if len(aliases) == 0 || len(aliases) > 64 {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical := append([]domaincaseentity.ModelEntityAliasV1(nil), aliases...)
	sort.Slice(canonical, func(left, right int) bool { return canonical[left] < canonical[right] })
	for index, alias := range canonical {
		if domaincaseentity.ValidateModelEntityAliasV1(string(alias)) != nil ||
			(index > 0 && alias == canonical[index-1]) {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
	}
	return canonical, nil
}

func sameProviderCaseAliasesV1(left, right []domaincaseentity.ModelEntityAliasV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func providerReferenceMessageSHA256V1(message domainmodel.Message) (string, error) {
	body, err := json.Marshal(message)
	if err != nil || len(body) == 0 {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	digest := domainsecurity.SHA256Hex(append(
		[]byte("analytix.provider-case-reference-message/v1\x00"),
		body...,
	))
	if !domainsecurity.IsSHA256Hex(digest) {
		return "", ErrProviderPrivacyAuthorityUnavailable
	}
	return digest, nil
}

func accountFlowProviderReferencesV1(
	canonical []byte,
) ([]domaincaseentity.ReferenceV1, error) {
	var envelope domainnative.AccountFlowProviderModelOutputV1
	if err := json.Unmarshal(canonical, &envelope); err != nil ||
		domainnative.ValidateAccountFlowProviderSemanticResultV1(
			envelope.Data,
			envelope.Data.EvidenceRowLimit,
			envelope.SemanticStatus,
		) != nil {
		return nil, ErrProviderPrivacyAuthorityUnavailable
	}
	// V2 provider semantics contain ModelEntityAliasV1 values only.
	return []domaincaseentity.ReferenceV1{}, nil
}
