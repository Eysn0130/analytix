package caseentity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Independent full enumeration/sort oracle retained from the pre-Top32 implementation.
// It never calls the bounded selector or its comparison/coverage implementation.
func TestLongitudinalTop32MatchesFullSortOracleV1(t *testing.T) {
	for _, extra := range []int{0, 19, 20, 21, 256, 2048, 4096} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			index, _, _ := longitudinalPriorityIndexV1(t, extra)
			got, err := providerIngressLongitudinalStateFromIndexV1(index)
			want, oracleErr := fullSortLongitudinalOracleV1(index)
			actual, marshalErr := json.Marshal(got)
			expected, oracleMarshalErr := json.Marshal(want)
			if err != nil || oracleErr != nil || marshalErr != nil || oracleMarshalErr != nil || !bytes.Equal(actual, expected) {
				t.Fatalf("selection differs from full-sort oracle: errors=%v/%v/%v/%v", err, oracleErr, marshalErr, oracleMarshalErr)
			}
			if uint32(len(got.Items))+got.OmittedTotal != uint32(len(index.Claims)+len(index.Evidence)+len(index.Continuations)+len(index.DataGapReferences)+len(index.OpenQuestionReferences)+len(index.Snapshots)-1) {
				t.Fatal("candidate count lost during bounded selection")
			}
			// Validation must include omitted records and reject duplicates, bad
			// support before any model context is available. These mutations also
			// invalidate the owner digest; deeper validation has separate domain tests.
			for _, mutate := range []func(*domaincaseentity.ThreadCaseContextRecord){
				func(r *domaincaseentity.ThreadCaseContextRecord) { r.Evidence = append(r.Evidence, r.Evidence[0]) },
				func(r *domaincaseentity.ThreadCaseContextRecord) { r.Claims = append(r.Claims, r.Claims[0]) },
				func(r *domaincaseentity.ThreadCaseContextRecord) {
					r.Evidence[len(r.Evidence)-1].EvidenceDigest = "invalid"
				},
				func(r *domaincaseentity.ThreadCaseContextRecord) {
					r.Claims[len(r.Claims)-1].EvidenceReferences = []string{"evr_" + domainsecurity.SHA256Hex([]byte("missing"))}
				},
			} {
				body, _ := json.Marshal(index)
				var bad domaincaseentity.ThreadCaseContextRecord
				if err := json.Unmarshal(body, &bad); err != nil {
					t.Fatal(err)
				}
				mutate(&bad)
				got, gotErr := providerIngressLongitudinalStateFromIndexV1(bad)
				want, wantErr := fullSortLongitudinalOracleV1(bad)
				if gotErr == nil || wantErr == nil || !reflect.DeepEqual(got, want) {
					t.Fatal("invalid owner bypassed complete validation")
				}
			}
		})
	}
}

func TestLongitudinalTop32UnorderedAndDuplicateMultisetV1(t *testing.T) {
	random := rand.New(rand.NewSource(20261001))
	for _, size := range []int{0, 1, 31, 32, 33, 257, 8192 + 16384} {
		values := make([]providerIngressLongitudinalCandidateV1, size)
		for i := range values {
			values[i] = providerIngressLongitudinalCandidateV1{priority: random.Intn(7), key: fmt.Sprintf("%08d", random.Intn(128))}
		}
		for _, order := range []string{"shuffled", "ascending", "descending"} {
			t.Run(fmt.Sprintf("%d/%s", size, order), func(t *testing.T) {
				input := append([]providerIngressLongitudinalCandidateV1(nil), values...)
				less := func(a, b providerIngressLongitudinalCandidateV1) bool {
					if a.priority != b.priority {
						return a.priority < b.priority
					}
					return a.key < b.key
				}
				switch order {
				case "shuffled":
					random.Shuffle(len(input), func(i, j int) { input[i], input[j] = input[j], input[i] })
				case "ascending":
					sort.Slice(input, func(i, j int) bool { return less(input[i], input[j]) })
				case "descending":
					sort.Slice(input, func(i, j int) bool { return less(input[j], input[i]) })
				}
				want := append([]providerIngressLongitudinalCandidateV1{}, input...)
				sort.Slice(want, func(i, j int) bool { return less(want[i], want[j]) })
				if len(want) > 32 {
					want = want[:32]
				}
				got := make([]providerIngressLongitudinalCandidateV1, 0, 32)
				for _, v := range input {
					got = retainProviderIngressTopCandidateV1(got, v)
					if len(got) > 32 || cap(got) > 32 {
						t.Fatal("candidate storage exceeded budget")
					}
				}
				sort.Slice(got, func(i, j int) bool { return less(got[i], got[j]) })
				if !reflect.DeepEqual(got, want) {
					t.Fatal("bounded multiset selection differs from independent sort")
				}
			})
		}
	}
}

func BenchmarkLongitudinalSelectionV1(b *testing.B) {
	for _, extra := range []int{0, 20, 256, 2048, 4096} {
		index, _, _ := longitudinalPriorityIndexV1(b, extra)
		for _, algorithm := range []string{"full-sort", "top32"} {
			b.Run(fmt.Sprintf("evidence-%d/%s", len(index.Evidence), algorithm), func(b *testing.B) {
				selectState := fullSortLongitudinalOracleV1
				if algorithm == "top32" {
					selectState = providerIngressLongitudinalStateFromIndexV1
				}
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := selectState(index); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func fullSortLongitudinalOracleV1(
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
