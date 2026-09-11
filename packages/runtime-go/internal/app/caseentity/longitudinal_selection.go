package caseentity

import (
	"encoding/json"
	"reflect"
	"sort"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const caseLongitudinalSelectionDigestPurposeV1 = "analytix.case-longitudinal-selection/v1"

var providerIngressLongitudinalPriorityV1 = []string{
	ProviderIngressLongitudinalCurrentVerifiedFactV1,
	ProviderIngressLongitudinalHistoricalComparisonFactV1,
	ProviderIngressLongitudinalKeyRelationshipV1,
	ProviderIngressLongitudinalCounterevidenceRefutedV1,
	ProviderIngressLongitudinalDataGapV1,
	ProviderIngressLongitudinalEvidenceClaimReferenceV1,
	ProviderIngressLongitudinalSnapshotDifferenceV1,
}

type providerIngressLongitudinalCandidateV1 struct {
	priority int
	key      string
	item     ProviderIngressLongitudinalItemV1
}

type providerIngressEvidenceBindingV1 struct {
	ReferenceDigest       string `json:"referenceDigest"`
	Digest                string `json:"digest,omitempty"`
	Currentness           string `json:"currentness"`
	SnapshotBindingDigest string `json:"snapshotBindingDigest"`
}

func providerIngressLongitudinalStateFromIndexV1(
	index domaincaseentity.ThreadCaseContextRecord,
) (ProviderIngressLongitudinalStateV1, error) {
	if !domaincaseentity.IsCaseLongitudinalIndexRecordV1(index) {
		return ProviderIngressLongitudinalStateV1{}, ErrPrivateStateIntegrity
	}
	snapshots := make(map[string]domaincaseentity.CaseSnapshotStateV1, len(index.Snapshots))
	snapshotBindings := make(map[string]string, len(index.Snapshots))
	currentSnapshotBinding := ""
	for _, snapshot := range index.Snapshots {
		snapshots[snapshot.DatasetSnapshotID] = snapshot
		binding := providerIngressSnapshotBindingDigestV1(index.CaseBindingHash, snapshot)
		snapshotBindings[snapshot.DatasetSnapshotID] = binding
		if snapshot.Currentness == domaincaseentity.SnapshotCurrentV1 {
			currentSnapshotBinding = binding
		}
	}
	if currentSnapshotBinding == "" {
		return ProviderIngressLongitudinalStateV1{}, ErrPrivateStateIntegrity
	}
	evidenceByIdentity := make(map[string]domaincaseentity.CaseEvidenceStateV1, len(index.Evidence))
	for _, evidence := range index.Evidence {
		evidenceByIdentity[evidence.EvidenceReference+"\x00"+evidence.DatasetSnapshotID] = evidence
	}

	candidates := make([]providerIngressLongitudinalCandidateV1, 0,
		len(index.Claims)+len(index.Evidence)+len(index.Continuations)+
			len(index.OpenQuestionReferences)+len(index.DataGapReferences)+len(index.Snapshots)-1)
	appendCandidate := func(item ProviderIngressLongitudinalItemV1) {
		priority := providerIngressLongitudinalPriorityIndexV1(item.Kind)
		body, _ := json.Marshal(item)
		candidates = append(candidates, providerIngressLongitudinalCandidateV1{
			priority: priority, key: string(body), item: item,
		})
	}
	for _, claim := range index.Claims {
		snapshot, found := snapshots[claim.DatasetSnapshotID]
		if !found {
			return ProviderIngressLongitudinalStateV1{}, ErrPrivateStateIntegrity
		}
		evidenceBinding, evidenceCount, err := providerIngressClaimEvidenceBindingDigestV1(
			index.CaseBindingHash, claim.EvidenceReferences, claim.DatasetSnapshotID,
			evidenceByIdentity, snapshotBindings,
		)
		if err != nil {
			return ProviderIngressLongitudinalStateV1{}, err
		}
		counterBinding, counterCount, err := providerIngressClaimEvidenceBindingDigestV1(
			index.CaseBindingHash, claim.CounterEvidenceReferences, claim.DatasetSnapshotID,
			evidenceByIdentity, snapshotBindings,
		)
		if err != nil {
			return ProviderIngressLongitudinalStateV1{}, err
		}
		claimType := ""
		if claim.TypedState != nil {
			claimType = claim.TypedState.ClaimType
		}
		kind := ProviderIngressLongitudinalEvidenceClaimReferenceV1
		switch {
		case claim.InvestigationState == domaincaseentity.InvestigationRejectedV1 || counterCount > 0:
			kind = ProviderIngressLongitudinalCounterevidenceRefutedV1
		case claimType == "relationship":
			kind = ProviderIngressLongitudinalKeyRelationshipV1
		case claim.InvestigationState == domaincaseentity.InvestigationConfirmedV1 &&
			claim.Currentness == domaincaseentity.SnapshotCurrentV1:
			kind = ProviderIngressLongitudinalCurrentVerifiedFactV1
		case claim.InvestigationState == domaincaseentity.InvestigationConfirmedV1:
			kind = ProviderIngressLongitudinalHistoricalComparisonFactV1
		}
		appendCandidate(ProviderIngressLongitudinalItemV1{
			Kind: kind, ReferenceKind: ProviderIngressLongitudinalReferenceClaimV1,
			ReferenceDigest: providerIngressReferenceDigestV1(
				index.CaseBindingHash, ProviderIngressLongitudinalReferenceClaimV1, claim.ClaimReference,
			),
			Digest:      claim.ClaimDigest,
			Currentness: claim.Currentness, InvestigationState: claim.InvestigationState,
			ClaimType: claimType, EvidenceBindingDigest: evidenceBinding,
			EvidenceReferenceCount: evidenceCount, CounterEvidenceBindingDigest: counterBinding,
			CounterEvidenceReferenceCount: counterCount,
			SnapshotBindingDigest:         snapshotBindings[snapshot.DatasetSnapshotID],
		})
	}
	for _, reference := range index.DataGapReferences {
		appendCandidate(ProviderIngressLongitudinalItemV1{
			Kind:          ProviderIngressLongitudinalDataGapV1,
			ReferenceKind: ProviderIngressLongitudinalReferenceDataGapV1,
			ReferenceDigest: providerIngressReferenceDigestV1(
				index.CaseBindingHash, ProviderIngressLongitudinalReferenceDataGapV1, reference,
			),
			InvestigationState: domaincaseentity.InvestigationOpenV1,
		})
	}
	for _, reference := range index.OpenQuestionReferences {
		appendCandidate(ProviderIngressLongitudinalItemV1{
			Kind:          ProviderIngressLongitudinalDataGapV1,
			ReferenceKind: ProviderIngressLongitudinalReferenceOpenQuestionV1,
			ReferenceDigest: providerIngressReferenceDigestV1(
				index.CaseBindingHash, ProviderIngressLongitudinalReferenceOpenQuestionV1, reference,
			),
			InvestigationState: domaincaseentity.InvestigationOpenV1,
		})
	}
	for _, evidence := range index.Evidence {
		appendCandidate(ProviderIngressLongitudinalItemV1{
			Kind:          ProviderIngressLongitudinalEvidenceClaimReferenceV1,
			ReferenceKind: ProviderIngressLongitudinalReferenceEvidenceV1,
			ReferenceDigest: providerIngressReferenceDigestV1(
				index.CaseBindingHash, ProviderIngressLongitudinalReferenceEvidenceV1, evidence.EvidenceReference,
			),
			Digest:                evidence.EvidenceDigest,
			Currentness:           evidence.Currentness,
			SnapshotBindingDigest: snapshotBindings[evidence.DatasetSnapshotID],
		})
	}
	for _, continuation := range index.Continuations {
		appendCandidate(ProviderIngressLongitudinalItemV1{
			Kind:          ProviderIngressLongitudinalEvidenceClaimReferenceV1,
			ReferenceKind: ProviderIngressLongitudinalReferenceContinuationV1,
			Digest:        continuation.ContinuationDigest, Currentness: continuation.Currentness,
			SnapshotBindingDigest: snapshotBindings[continuation.DatasetSnapshotID],
		})
	}
	for _, snapshot := range index.Snapshots {
		if snapshot.Currentness == domaincaseentity.SnapshotCurrentV1 {
			continue
		}
		appendCandidate(ProviderIngressLongitudinalItemV1{
			Kind:                          ProviderIngressLongitudinalSnapshotDifferenceV1,
			ReferenceKind:                 ProviderIngressLongitudinalReferenceSnapshotV1,
			Currentness:                   snapshot.Currentness,
			SnapshotBindingDigest:         snapshotBindings[snapshot.DatasetSnapshotID],
			ComparedSnapshotBindingDigest: currentSnapshotBinding,
		})
	}

	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].priority != candidates[right].priority {
			return candidates[left].priority < candidates[right].priority
		}
		return candidates[left].key < candidates[right].key
	})
	selectedCount := len(candidates)
	if selectedCount > ProviderIngressLongitudinalSelectionBudgetV1 {
		selectedCount = ProviderIngressLongitudinalSelectionBudgetV1
	}
	state := ProviderIngressLongitudinalStateV1{
		SchemaVersion:      ProviderIngressLongitudinalSchemaVersionV1,
		ScopeBindingDigest: providerIngressLongitudinalScopeBindingDigestV1(index.CaseBindingHash),
		SelectionBudget:    ProviderIngressLongitudinalSelectionBudgetV1,
		Items:              make([]ProviderIngressLongitudinalItemV1, selectedCount),
		OmittedCoverage:    make([]ProviderIngressLongitudinalOmittedCoverageV1, len(providerIngressLongitudinalPriorityV1)),
		Currentness:        domaincaseentity.SnapshotCurrentV1,
		Claims:             []ProviderIngressClaimDigestV1{}, Evidence: []ProviderIngressEvidenceDigestV1{},
		Continuations: []ProviderIngressContinuationDigestV1{},
	}
	for index, kind := range providerIngressLongitudinalPriorityV1 {
		state.OmittedCoverage[index].Kind = kind
	}
	for index, candidate := range candidates {
		if index < selectedCount {
			state.Items[index] = candidate.item
			continue
		}
		coverageIndex := providerIngressLongitudinalPriorityIndexV1(candidate.item.Kind)
		state.OmittedCoverage[coverageIndex].Count++
		state.OmittedTotal++
	}
	deriveProviderIngressLongitudinalDelegationStateV1(&state)
	if validateProviderIngressLongitudinalStateV1(state) != nil {
		return ProviderIngressLongitudinalStateV1{}, ErrPrivateStateIntegrity
	}
	return state, nil
}

// providerIngressLongitudinalScopeBindingDigestV1 derives a value-free scope
// marker used only to prevent a canonical provider selection from being rebound
// to another case. It is not case authority and cannot resolve an owner ID.
func providerIngressLongitudinalScopeBindingDigestV1(caseBindingHash string) string {
	if !domainsecurity.IsSHA256Hex(caseBindingHash) {
		return ""
	}
	body, _ := json.Marshal(struct {
		Purpose         string `json:"purpose"`
		CaseBindingHash string `json:"caseBindingHash"`
		BindingKind     string `json:"bindingKind"`
	}{
		Purpose:         caseLongitudinalSelectionDigestPurposeV1,
		CaseBindingHash: caseBindingHash,
		BindingKind:     "scope",
	})
	return domainsecurity.SHA256Hex(body)
}

func providerIngressSnapshotBindingDigestV1(
	caseBindingHash string,
	snapshot domaincaseentity.CaseSnapshotStateV1,
) string {
	body, _ := json.Marshal(struct {
		Purpose           string `json:"purpose"`
		CaseBindingHash   string `json:"caseBindingHash"`
		DatasetSnapshotID string `json:"datasetSnapshotId"`
		ContextEpoch      uint64 `json:"contextEpoch"`
		Currentness       string `json:"currentness"`
	}{
		Purpose:           caseLongitudinalSelectionDigestPurposeV1,
		CaseBindingHash:   caseBindingHash,
		DatasetSnapshotID: snapshot.DatasetSnapshotID,
		ContextEpoch:      snapshot.ContextEpoch,
		Currentness:       snapshot.Currentness,
	})
	return domainsecurity.SHA256Hex(body)
}

func providerIngressClaimEvidenceBindingDigestV1(
	caseBindingHash string,
	references []string,
	datasetSnapshotID string,
	evidenceByIdentity map[string]domaincaseentity.CaseEvidenceStateV1,
	snapshotBindings map[string]string,
) (string, uint32, error) {
	if len(references) == 0 {
		return "", 0, nil
	}
	bindings := make([]providerIngressEvidenceBindingV1, len(references))
	for index, reference := range references {
		evidence, found := evidenceByIdentity[reference+"\x00"+datasetSnapshotID]
		snapshotBinding := snapshotBindings[evidence.DatasetSnapshotID]
		if !found || snapshotBinding == "" {
			return "", 0, ErrPrivateStateIntegrity
		}
		bindings[index] = providerIngressEvidenceBindingV1{
			ReferenceDigest: providerIngressReferenceDigestV1(
				caseBindingHash, ProviderIngressLongitudinalReferenceEvidenceV1, reference,
			),
			Digest:      evidence.EvidenceDigest,
			Currentness: evidence.Currentness, SnapshotBindingDigest: snapshotBinding,
		}
	}
	body, err := json.Marshal(struct {
		Purpose         string                             `json:"purpose"`
		CaseBindingHash string                             `json:"caseBindingHash"`
		Bindings        []providerIngressEvidenceBindingV1 `json:"bindings"`
	}{
		Purpose:         caseLongitudinalSelectionDigestPurposeV1,
		CaseBindingHash: caseBindingHash,
		Bindings:        bindings,
	})
	if err != nil {
		return "", 0, ErrPrivateStateIntegrity
	}
	return domainsecurity.SHA256Hex(body), uint32(len(bindings)), nil
}

func providerIngressReferenceDigestV1(caseBindingHash, referenceKind, reference string) string {
	body, _ := json.Marshal(struct {
		Purpose         string `json:"purpose"`
		CaseBindingHash string `json:"caseBindingHash"`
		ReferenceKind   string `json:"referenceKind"`
		Reference       string `json:"reference"`
	}{
		Purpose:         caseLongitudinalSelectionDigestPurposeV1,
		CaseBindingHash: caseBindingHash,
		ReferenceKind:   referenceKind,
		Reference:       reference,
	})
	return domainsecurity.SHA256Hex(body)
}

func deriveProviderIngressLongitudinalDelegationStateV1(state *ProviderIngressLongitudinalStateV1) {
	if state == nil {
		return
	}
	seenClaims := map[string]bool{}
	seenEvidence := map[string]bool{}
	seenContinuations := map[string]bool{}
	for _, item := range state.Items {
		if item.Currentness != domaincaseentity.SnapshotCurrentV1 || item.Digest == "" {
			continue
		}
		switch item.ReferenceKind {
		case ProviderIngressLongitudinalReferenceClaimV1:
			if !seenClaims[item.Digest] {
				seenClaims[item.Digest] = true
				state.Claims = append(state.Claims, ProviderIngressClaimDigestV1{
					Digest: item.Digest, InvestigationState: item.InvestigationState,
				})
			}
		case ProviderIngressLongitudinalReferenceEvidenceV1:
			if !seenEvidence[item.Digest] {
				seenEvidence[item.Digest] = true
				state.Evidence = append(state.Evidence, ProviderIngressEvidenceDigestV1{
					Digest: item.Digest, Currentness: item.Currentness,
				})
			}
		case ProviderIngressLongitudinalReferenceContinuationV1:
			if !seenContinuations[item.Digest] {
				seenContinuations[item.Digest] = true
				state.Continuations = append(state.Continuations, ProviderIngressContinuationDigestV1{
					Digest: item.Digest, Currentness: item.Currentness,
				})
			}
		}
	}
}

func validateProviderIngressLongitudinalStateV1(state ProviderIngressLongitudinalStateV1) error {
	if state.SchemaVersion == 0 && state.ScopeBindingDigest == "" && state.SelectionBudget == 0 && state.Items == nil &&
		state.OmittedCoverage == nil && state.OmittedTotal == 0 && state.Currentness == "" &&
		state.Claims == nil && state.Evidence == nil && state.Continuations == nil {
		return nil
	}
	if state.SchemaVersion != ProviderIngressLongitudinalSchemaVersionV1 ||
		!domainsecurity.IsSHA256Hex(state.ScopeBindingDigest) ||
		state.SelectionBudget != ProviderIngressLongitudinalSelectionBudgetV1 ||
		len(state.Items) > ProviderIngressLongitudinalSelectionBudgetV1 ||
		len(state.OmittedCoverage) != len(providerIngressLongitudinalPriorityV1) ||
		state.Currentness != domaincaseentity.SnapshotCurrentV1 {
		return ErrPrivateStateIntegrity
	}
	omittedTotal := uint32(0)
	for index, coverage := range state.OmittedCoverage {
		if coverage.Kind != providerIngressLongitudinalPriorityV1[index] ||
			^uint32(0)-omittedTotal < coverage.Count {
			return ErrPrivateStateIntegrity
		}
		omittedTotal += coverage.Count
	}
	if omittedTotal != state.OmittedTotal {
		return ErrPrivateStateIntegrity
	}
	seen := make(map[string]bool, len(state.Items))
	previous := providerIngressLongitudinalCandidateV1{priority: -1}
	for index, item := range state.Items {
		if validateProviderIngressLongitudinalItemV1(item) != nil {
			return ErrPrivateStateIntegrity
		}
		body, _ := json.Marshal(item)
		candidate := providerIngressLongitudinalCandidateV1{
			priority: providerIngressLongitudinalPriorityIndexV1(item.Kind), key: string(body), item: item,
		}
		if seen[candidate.key] || index > 0 &&
			(candidate.priority < previous.priority || candidate.priority == previous.priority && candidate.key < previous.key) {
			return ErrPrivateStateIntegrity
		}
		seen[candidate.key] = true
		previous = candidate
	}
	derived := ProviderIngressLongitudinalStateV1{
		Items:  append([]ProviderIngressLongitudinalItemV1(nil), state.Items...),
		Claims: []ProviderIngressClaimDigestV1{}, Evidence: []ProviderIngressEvidenceDigestV1{},
		Continuations: []ProviderIngressContinuationDigestV1{},
	}
	deriveProviderIngressLongitudinalDelegationStateV1(&derived)
	if !reflect.DeepEqual(derived.Claims, state.Claims) || !reflect.DeepEqual(derived.Evidence, state.Evidence) ||
		!reflect.DeepEqual(derived.Continuations, state.Continuations) {
		return ErrPrivateStateIntegrity
	}
	return nil
}

func validateProviderIngressLongitudinalItemV1(item ProviderIngressLongitudinalItemV1) error {
	if providerIngressLongitudinalPriorityIndexV1(item.Kind) < 0 ||
		(item.Digest != "" && !domainsecurity.IsSHA256Hex(item.Digest)) ||
		(item.EvidenceReferenceCount == 0) != (item.EvidenceBindingDigest == "") ||
		(item.EvidenceBindingDigest != "" && !domainsecurity.IsSHA256Hex(item.EvidenceBindingDigest)) ||
		(item.CounterEvidenceReferenceCount == 0) != (item.CounterEvidenceBindingDigest == "") ||
		(item.CounterEvidenceBindingDigest != "" && !domainsecurity.IsSHA256Hex(item.CounterEvidenceBindingDigest)) {
		return ErrPrivateStateIntegrity
	}
	switch item.ReferenceKind {
	case ProviderIngressLongitudinalReferenceClaimV1:
		if !domainsecurity.IsSHA256Hex(item.ReferenceDigest) ||
			!validProviderIngressCurrentnessV1(item.Currentness) ||
			!validProviderIngressInvestigationV1(item.InvestigationState) ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) ||
			item.ComparedSnapshotBindingDigest != "" {
			return ErrPrivateStateIntegrity
		}
		if item.ClaimType != "" {
			if _, err := domaincaseentity.NewCaseClaimTypedStateV1(item.ClaimType); err != nil {
				return ErrPrivateStateIntegrity
			}
		}
		switch item.Kind {
		case ProviderIngressLongitudinalCurrentVerifiedFactV1:
			if item.Currentness != domaincaseentity.SnapshotCurrentV1 ||
				item.InvestigationState != domaincaseentity.InvestigationConfirmedV1 ||
				item.EvidenceReferenceCount == 0 || item.CounterEvidenceReferenceCount != 0 ||
				item.ClaimType == "relationship" {
				return ErrPrivateStateIntegrity
			}
		case ProviderIngressLongitudinalHistoricalComparisonFactV1:
			if item.Currentness == domaincaseentity.SnapshotCurrentV1 ||
				item.InvestigationState != domaincaseentity.InvestigationConfirmedV1 ||
				item.EvidenceReferenceCount == 0 || item.CounterEvidenceReferenceCount != 0 ||
				item.ClaimType == "relationship" {
				return ErrPrivateStateIntegrity
			}
		case ProviderIngressLongitudinalKeyRelationshipV1:
			if item.ClaimType != "relationship" ||
				item.InvestigationState == domaincaseentity.InvestigationRejectedV1 ||
				item.CounterEvidenceReferenceCount != 0 ||
				item.InvestigationState == domaincaseentity.InvestigationConfirmedV1 && item.EvidenceReferenceCount == 0 {
				return ErrPrivateStateIntegrity
			}
		case ProviderIngressLongitudinalCounterevidenceRefutedV1:
			if item.CounterEvidenceReferenceCount == 0 {
				return ErrPrivateStateIntegrity
			}
		case ProviderIngressLongitudinalEvidenceClaimReferenceV1:
			if item.InvestigationState != domaincaseentity.InvestigationOpenV1 {
				return ErrPrivateStateIntegrity
			}
		default:
			return ErrPrivateStateIntegrity
		}
	case ProviderIngressLongitudinalReferenceEvidenceV1:
		if item.Kind != ProviderIngressLongitudinalEvidenceClaimReferenceV1 ||
			!domainsecurity.IsSHA256Hex(item.ReferenceDigest) ||
			!validProviderIngressCurrentnessV1(item.Currentness) ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) ||
			item.InvestigationState != "" || item.ClaimType != "" ||
			item.EvidenceReferenceCount != 0 || item.CounterEvidenceReferenceCount != 0 ||
			item.ComparedSnapshotBindingDigest != "" {
			return ErrPrivateStateIntegrity
		}
	case ProviderIngressLongitudinalReferenceContinuationV1:
		if item.Kind != ProviderIngressLongitudinalEvidenceClaimReferenceV1 || item.ReferenceDigest != "" ||
			!domainsecurity.IsSHA256Hex(item.Digest) || !validProviderIngressCurrentnessV1(item.Currentness) ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) || item.InvestigationState != "" ||
			item.ClaimType != "" || item.EvidenceReferenceCount != 0 ||
			item.CounterEvidenceReferenceCount != 0 || item.ComparedSnapshotBindingDigest != "" {
			return ErrPrivateStateIntegrity
		}
	case ProviderIngressLongitudinalReferenceDataGapV1, ProviderIngressLongitudinalReferenceOpenQuestionV1:
		if item.Kind != ProviderIngressLongitudinalDataGapV1 ||
			!domainsecurity.IsSHA256Hex(item.ReferenceDigest) ||
			item.Digest != "" || item.Currentness != "" ||
			item.InvestigationState != domaincaseentity.InvestigationOpenV1 || item.ClaimType != "" ||
			item.EvidenceReferenceCount != 0 || item.CounterEvidenceReferenceCount != 0 ||
			item.SnapshotBindingDigest != "" || item.ComparedSnapshotBindingDigest != "" {
			return ErrPrivateStateIntegrity
		}
	case ProviderIngressLongitudinalReferenceSnapshotV1:
		if item.Kind != ProviderIngressLongitudinalSnapshotDifferenceV1 || item.ReferenceDigest != "" || item.Digest != "" ||
			item.Currentness == domaincaseentity.SnapshotCurrentV1 || !validProviderIngressCurrentnessV1(item.Currentness) ||
			item.InvestigationState != "" || item.ClaimType != "" ||
			item.EvidenceReferenceCount != 0 || item.CounterEvidenceReferenceCount != 0 ||
			!domainsecurity.IsSHA256Hex(item.SnapshotBindingDigest) ||
			!domainsecurity.IsSHA256Hex(item.ComparedSnapshotBindingDigest) ||
			item.SnapshotBindingDigest == item.ComparedSnapshotBindingDigest {
			return ErrPrivateStateIntegrity
		}
	default:
		return ErrPrivateStateIntegrity
	}
	return nil
}

func providerIngressLongitudinalPriorityIndexV1(kind string) int {
	for index, current := range providerIngressLongitudinalPriorityV1 {
		if current == kind {
			return index
		}
	}
	return -1
}

func validProviderIngressCurrentnessV1(value string) bool {
	switch value {
	case domaincaseentity.SnapshotCurrentV1, domaincaseentity.SnapshotHistoricalV1,
		domaincaseentity.SnapshotStaleV1, domaincaseentity.SnapshotSupersededV1:
		return true
	default:
		return false
	}
}

func validProviderIngressInvestigationV1(value string) bool {
	return value == domaincaseentity.InvestigationConfirmedV1 ||
		value == domaincaseentity.InvestigationRejectedV1 ||
		value == domaincaseentity.InvestigationOpenV1
}

func cloneProviderIngressLongitudinalStateV1(state ProviderIngressLongitudinalStateV1) ProviderIngressLongitudinalStateV1 {
	if state.SchemaVersion == 0 {
		return ProviderIngressLongitudinalStateV1{}
	}
	return ProviderIngressLongitudinalStateV1{
		SchemaVersion: state.SchemaVersion, ScopeBindingDigest: state.ScopeBindingDigest,
		SelectionBudget: state.SelectionBudget,
		Items:           append([]ProviderIngressLongitudinalItemV1{}, state.Items...),
		OmittedTotal:    state.OmittedTotal,
		OmittedCoverage: append([]ProviderIngressLongitudinalOmittedCoverageV1{}, state.OmittedCoverage...),
		Currentness:     state.Currentness,
		Claims:          append([]ProviderIngressClaimDigestV1{}, state.Claims...),
		Evidence:        append([]ProviderIngressEvidenceDigestV1{}, state.Evidence...),
		Continuations:   append([]ProviderIngressContinuationDigestV1{}, state.Continuations...),
	}
}
