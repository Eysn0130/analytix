package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	CaseForegroundChildResultSchemaVersionV1 = 1
	CaseForegroundChildResultPurposeV1       = "analytix.case-foreground-child-result/v1"
	CaseForegroundChildSelectionPurposeV1    = "analytix.case-foreground-child-selection/v1"
	maxCaseForegroundChildResultBytesV1      = 64 * 1024
)

// CaseForegroundChildSelectionV1 is the only provider-submittable case
// shape. It selects one host-issued commitment without carrying values,
// claims, evidence, gaps, prose, or provider output.
type CaseForegroundChildSelectionV1 struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Purpose          string `json:"purpose"`
	DelegationDigest string `json:"delegationDigest"`
	AnswerSlotDigest string `json:"answerSlotDigest"`
}

// CaseForegroundChildResultV1 is a closed candidate selection over one exact
// CaseDelegationContextV1. It is never evidence or fact authority and is never
// durable output. The host re-resolves every selected digest and answer slot
// from the current parent evidence registry before opening it to the current
// parent provider attempt.
type CaseForegroundChildResultV1 struct {
	SchemaVersion    int                                             `json:"schemaVersion"`
	Purpose          string                                          `json:"purpose"`
	DelegationDigest string                                          `json:"delegationDigest"`
	EntityAliases    []domaincaseentity.ModelEntityAliasV1           `json:"entityAliases"`
	Claims           []CaseDelegatedClaimReferenceV1                 `json:"claims"`
	Evidence         []CaseDelegatedEvidenceReferenceV1              `json:"evidence"`
	Gaps             []string                                        `json:"gaps"`
	AnswerSlots      []domainnative.AccountFlowDelegatedAnswerSlotV1 `json:"answerSlots"`
	Currentness      string                                          `json:"currentness"`
}

func ParseCaseForegroundChildResultV1(
	value any,
	delegation *CaseDelegationContextV1,
) (CaseForegroundChildResultV1, []byte, error) {
	body, err := json.Marshal(value)
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxCaseForegroundChildResultBytesV1,
		MaxDepth: 12, MaxTokens: 2048, MaxStringBytes: 4096,
	}) != nil {
		return CaseForegroundChildResultV1{}, nil, errors.New("case foreground child result is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var result CaseForegroundChildResultV1
	if decoder.Decode(&result) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		ValidateCaseForegroundChildResultV1(result, delegation) != nil {
		return CaseForegroundChildResultV1{}, nil, errors.New("case foreground child result is invalid")
	}
	canonical, err := json.Marshal(result)
	if err != nil || len(canonical) == 0 || len(canonical) > maxCaseForegroundChildResultBytesV1 {
		return CaseForegroundChildResultV1{}, nil, errors.New("case foreground child result is invalid")
	}
	return result, canonical, nil
}

func ParseCaseForegroundChildSelectionV1(
	value any,
	delegation *CaseDelegationContextV1,
) (CaseForegroundChildSelectionV1, []byte, error) {
	body, err := json.Marshal(value)
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4096, MaxDepth: 2, MaxTokens: 32, MaxStringBytes: 256,
	}) != nil {
		return CaseForegroundChildSelectionV1{}, nil, errors.New("case foreground child selection is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var providerSelection CaseForegroundChildSelectionV1
	if decoder.Decode(&providerSelection) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return CaseForegroundChildSelectionV1{}, nil, errors.New("case foreground child selection is invalid")
	}
	selection, err := normalizeCaseForegroundChildSelectionV1(providerSelection, delegation)
	if err != nil {
		return CaseForegroundChildSelectionV1{}, nil, errors.New("case foreground child selection is invalid")
	}
	canonical, err := json.Marshal(providerSelection)
	if err != nil {
		return CaseForegroundChildSelectionV1{}, nil, errors.New("case foreground child selection is invalid")
	}
	return selection, canonical, nil
}

func normalizeCaseForegroundChildSelectionV1(
	selection CaseForegroundChildSelectionV1,
	delegation *CaseDelegationContextV1,
) (CaseForegroundChildSelectionV1, error) {
	if delegation == nil || len(delegation.Semantic.AnswerSlots) != 1 {
		return CaseForegroundChildSelectionV1{}, errors.New("case foreground child selection is not delegated")
	}
	expectedDelegation := CaseDelegationProviderCommitmentTokenV1(delegation.DelegationDigest)
	expectedSlot := CaseDelegationProviderCommitmentTokenV1(delegation.Semantic.AnswerSlots[0].Digest)
	if ValidateCaseDelegationProviderCommitmentTokenV1(selection.DelegationDigest) != nil ||
		ValidateCaseDelegationProviderCommitmentTokenV1(selection.AnswerSlotDigest) != nil ||
		selection.DelegationDigest != expectedDelegation || selection.AnswerSlotDigest != expectedSlot {
		return CaseForegroundChildSelectionV1{}, errors.New("case foreground child selection is not delegated")
	}
	selection.DelegationDigest = delegation.DelegationDigest
	selection.AnswerSlotDigest = delegation.Semantic.AnswerSlots[0].Digest
	if ValidateNormalizedCaseForegroundChildSelectionV1(selection, delegation) != nil {
		return CaseForegroundChildSelectionV1{}, errors.New("case foreground child selection is not delegated")
	}
	return selection, nil
}

// ValidateNormalizedCaseForegroundChildSelectionV1 accepts only the
// host-private raw selection produced after the untrusted token parser has
// matched the exact current delegation. Provider input must enter through
// ParseCaseForegroundChildSelectionV1 instead.
func ValidateNormalizedCaseForegroundChildSelectionV1(
	selection CaseForegroundChildSelectionV1,
	delegation *CaseDelegationContextV1,
) error {
	if selection.SchemaVersion != CaseForegroundChildResultSchemaVersionV1 ||
		selection.Purpose != CaseForegroundChildSelectionPurposeV1 ||
		!domainsecurity.IsSHA256Hex(selection.DelegationDigest) ||
		!domainsecurity.IsSHA256Hex(selection.AnswerSlotDigest) || delegation == nil ||
		selection.DelegationDigest != delegation.DelegationDigest || len(delegation.Semantic.AnswerSlots) != 1 ||
		selection.AnswerSlotDigest != delegation.Semantic.AnswerSlots[0].Digest {
		return errors.New("case foreground child selection is not delegated")
	}
	return nil
}

func ParseCaseForegroundChildResultShapeV1(value any) (CaseForegroundChildResultV1, error) {
	body, err := json.Marshal(value)
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxCaseForegroundChildResultBytesV1,
		MaxDepth: 12, MaxTokens: 2048, MaxStringBytes: 4096,
	}) != nil {
		return CaseForegroundChildResultV1{}, errors.New("case foreground child result is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var result CaseForegroundChildResultV1
	if decoder.Decode(&result) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		ValidateCaseForegroundChildResultShapeV1(result) != nil {
		return CaseForegroundChildResultV1{}, errors.New("case foreground child result is invalid")
	}
	return result, nil
}

func ValidateCaseForegroundChildResultShapeV1(result CaseForegroundChildResultV1) error {
	if result.SchemaVersion != CaseForegroundChildResultSchemaVersionV1 ||
		result.Purpose != CaseForegroundChildResultPurposeV1 ||
		!domainsecurity.IsSHA256Hex(result.DelegationDigest) ||
		result.Currentness != domaincaseentity.SnapshotCurrentV1 ||
		len(result.EntityAliases) == 0 || len(result.EntityAliases) > MaxCaseDelegationEntitiesV1 ||
		len(result.Claims) != 3 || len(result.Evidence) != 1 ||
		result.Claims == nil || result.Evidence == nil || result.Gaps == nil ||
		len(result.AnswerSlots) != 1 {
		return errors.New("case foreground child result shape is incomplete")
	}
	for index, alias := range result.EntityAliases {
		if domaincaseentity.ValidateModelEntityAliasV1(string(alias)) != nil ||
			(index > 0 && result.EntityAliases[index-1] >= alias) {
			return errors.New("case foreground child result aliases are invalid")
		}
	}
	for index, claim := range result.Claims {
		if !domainsecurity.IsSHA256Hex(claim.Digest) ||
			(claim.InvestigationState != domaincaseentity.InvestigationConfirmedV1 &&
				claim.InvestigationState != domaincaseentity.InvestigationRejectedV1 &&
				claim.InvestigationState != domaincaseentity.InvestigationOpenV1) ||
			(index > 0 && result.Claims[index-1].Digest >= claim.Digest) {
			return errors.New("case foreground child result claims are invalid")
		}
	}
	for index, evidence := range result.Evidence {
		if !domainsecurity.IsSHA256Hex(evidence.Digest) || evidence.Currentness != domaincaseentity.SnapshotCurrentV1 ||
			(index > 0 && result.Evidence[index-1].Digest >= evidence.Digest) {
			return errors.New("case foreground child result evidence is invalid")
		}
	}
	if domainnative.ValidateAccountFlowDelegatedAnswerSlotV1(result.AnswerSlots[0]) != nil ||
		!reflect.DeepEqual(result.Gaps, result.AnswerSlots[0].Gaps) ||
		!containsCaseForegroundAliasV1(result.EntityAliases, result.AnswerSlots[0].SubjectAlias) {
		return errors.New("case foreground child result answer slot is invalid")
	}
	return nil
}

func ValidateCaseForegroundChildResultV1(
	result CaseForegroundChildResultV1,
	delegation *CaseDelegationContextV1,
) error {
	if ValidateCaseForegroundChildResultShapeV1(result) != nil ||
		delegation == nil || result.DelegationDigest != delegation.DelegationDigest ||
		result.Currentness != delegation.Semantic.Currentness || len(delegation.Semantic.AnswerSlots) != 1 {
		return errors.New("case foreground child result delegation is invalid")
	}
	for _, alias := range result.EntityAliases {
		if !containsCaseDelegatedEntityAliasV1(delegation.Semantic.Entities, alias) {
			return errors.New("case foreground child result contains an undelegated alias")
		}
	}
	selected := result.AnswerSlots[0]
	wantScope, err := domainevidence.NewAccountFlowQueryScopeRefV1(delegation.ParentContextDigest, selected.QueryHash)
	if err != nil || selected.QueryScopeRef != wantScope {
		return errors.New("case foreground child result query scope is invalid")
	}
	commitment, commitmentErr := NewCaseDelegatedAnswerSlotCommitmentV1(CaseDelegatedAnswerSlotBindingV1{
		AnswerSlot: selected, Claims: result.Claims, Evidence: result.Evidence,
	})
	if commitmentErr != nil || commitment != delegation.Semantic.AnswerSlots[0] {
		return errors.New("case foreground child result contains an undelegated answer slot")
	}
	return nil
}

func NewCaseForegroundChildResultV1(
	delegation *CaseDelegationContextV1,
	binding CaseDelegatedAnswerSlotBindingV1,
) (CaseForegroundChildResultV1, error) {
	commitment, err := NewCaseDelegatedAnswerSlotCommitmentV1(binding)
	if err != nil || delegation == nil || len(delegation.Semantic.AnswerSlots) != 1 ||
		commitment != delegation.Semantic.AnswerSlots[0] {
		return CaseForegroundChildResultV1{}, errors.New("case foreground child result binding is not delegated")
	}
	result := CaseForegroundChildResultV1{
		SchemaVersion: CaseForegroundChildResultSchemaVersionV1, Purpose: CaseForegroundChildResultPurposeV1,
		DelegationDigest: delegation.DelegationDigest,
		EntityAliases:    []domaincaseentity.ModelEntityAliasV1{domaincaseentity.ModelEntityAliasV1(binding.AnswerSlot.SubjectAlias)},
		Claims:           append([]CaseDelegatedClaimReferenceV1{}, binding.Claims...),
		Evidence:         append([]CaseDelegatedEvidenceReferenceV1{}, binding.Evidence...),
		Gaps:             append([]string{}, binding.AnswerSlot.Gaps...),
		AnswerSlots:      []domainnative.AccountFlowDelegatedAnswerSlotV1{binding.AnswerSlot},
		Currentness:      domaincaseentity.SnapshotCurrentV1,
	}
	result.AnswerSlots[0].Gaps = append([]string{}, binding.AnswerSlot.Gaps...)
	if ValidateCaseForegroundChildResultV1(result, delegation) != nil {
		return CaseForegroundChildResultV1{}, errors.New("case foreground child result is invalid")
	}
	return result, nil
}

func CanonicalCaseForegroundChildResultV1(result CaseForegroundChildResultV1) ([]byte, error) {
	if ValidateCaseForegroundChildResultShapeV1(result) != nil {
		return nil, errors.New("case foreground child result is invalid")
	}
	return json.Marshal(result)
}

func CloneCaseForegroundChildResultV1(result *CaseForegroundChildResultV1) *CaseForegroundChildResultV1 {
	if result == nil {
		return nil
	}
	clone := *result
	clone.EntityAliases = append([]domaincaseentity.ModelEntityAliasV1(nil), result.EntityAliases...)
	clone.Claims = append([]CaseDelegatedClaimReferenceV1{}, result.Claims...)
	clone.Evidence = append([]CaseDelegatedEvidenceReferenceV1{}, result.Evidence...)
	clone.Gaps = append([]string{}, result.Gaps...)
	clone.AnswerSlots = append([]domainnative.AccountFlowDelegatedAnswerSlotV1(nil), result.AnswerSlots...)
	for index := range clone.AnswerSlots {
		clone.AnswerSlots[index].Gaps = append([]string{}, result.AnswerSlots[index].Gaps...)
	}
	return &clone
}

func containsCaseForegroundAliasV1(values []domaincaseentity.ModelEntityAliasV1, expected string) bool {
	for _, value := range values {
		if string(value) == expected {
			return true
		}
	}
	return false
}

func containsCaseDelegatedEntityAliasV1(values []CaseDelegatedEntitySemanticV1, expected domaincaseentity.ModelEntityAliasV1) bool {
	for _, value := range values {
		if value.Alias == expected {
			return true
		}
	}
	return false
}
