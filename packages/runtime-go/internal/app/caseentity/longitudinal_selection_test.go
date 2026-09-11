package caseentity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestProviderIngressLongitudinalSelectionUsesOnePriorityBudget(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-selection", TurnID: "turn-selection",
		TenantID: "tenant-selection", UserID: "user-selection", CaseID: "case-selection",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "selection-current", ContextEpoch: 2,
	})
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(domainsecurity.SHA256Hex([]byte("selection-entity")))
	if err != nil {
		t.Fatal(err)
	}
	typedState, err := domaincaseentity.NewCaseClaimTypedStateV1("entity")
	if err != nil {
		t.Fatal(err)
	}
	evidenceReference := "evr_" + domainsecurity.SHA256Hex([]byte("selection-evidence-reference"))
	index, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: 1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: []domaincaseentity.CaseEntityIdentityStateV1{{
			Reference:     reference,
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1,
		}},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			ContextEpoch:      securityContext.ContextEpoch,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Evidence: []domaincaseentity.CaseEvidenceStateV1{{
			EvidenceReference: evidenceReference,
			EvidenceDigest:    domainsecurity.SHA256Hex([]byte("selection-evidence")),
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Claims: []domaincaseentity.CaseClaimStateV1{{
			ClaimReference:     "claim_selection_current",
			ClaimDigest:        domainsecurity.SHA256Hex([]byte("selection-claim")),
			TypedState:         typedState,
			DatasetSnapshotID:  securityContext.DatasetSnapshotID,
			Currentness:        domaincaseentity.SnapshotCurrentV1,
			InvestigationState: domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences: []string{evidenceReference}, CounterEvidenceReferences: []string{},
		}},
		OpenQuestionReferences: []string{"question_shared_owner_id"},
		DataGapReferences:      []string{"gap_shared_owner_id"},
		Continuations: []domaincaseentity.CaseContinuationStateV1{{
			ContinuationDigest: domainsecurity.SHA256Hex([]byte("selection-continuation")),
			DatasetSnapshotID:  securityContext.DatasetSnapshotID,
			Currentness:        domaincaseentity.SnapshotCurrentV1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := providerIngressLongitudinalStateFromIndexV1(index)
	if err != nil {
		t.Fatalf("assemble typed selection: %v", err)
	}
	if state.SchemaVersion != ProviderIngressLongitudinalSchemaVersionV1 ||
		state.SelectionBudget != ProviderIngressLongitudinalSelectionBudgetV1 || len(state.Items) != 5 ||
		state.Items[0].Kind != ProviderIngressLongitudinalCurrentVerifiedFactV1 || state.OmittedTotal != 0 {
		t.Fatalf("typed selection mismatch: %#v", state)
	}
	body, err := domaincaseentity.ThreadCaseContextRecordV1Bytes(index)
	if err != nil {
		t.Fatalf("encode typed owner record: %v", err)
	}
	parsed, err := domaincaseentity.ParseThreadCaseContextRecordV1(body)
	if err != nil || len(parsed.Claims) != 1 || parsed.Claims[0].TypedState == nil ||
		parsed.Claims[0].TypedState.ClaimType != "entity" || parsed.RecordDigest != index.RecordDigest {
		t.Fatalf("typed owner record restart readback failed: parsed=%#v err=%v", parsed, err)
	}
	restartedState, err := providerIngressLongitudinalStateFromIndexV1(parsed)
	if err != nil || !reflect.DeepEqual(restartedState, state) {
		t.Fatalf("assemble restarted typed selection: %v", err)
	}
	for _, ownerReference := range []string{
		"claim_selection_current", evidenceReference, "question_shared_owner_id", "gap_shared_owner_id",
	} {
		if encoded, encodeErr := json.Marshal(state); encodeErr != nil || strings.Contains(string(encoded), ownerReference) {
			t.Fatalf("provider selection reflected raw owner reference %q: body=%s err=%v", ownerReference, encoded, encodeErr)
		}
	}

	caseBContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-selection-b", TurnID: "turn-selection-b",
		TenantID: securityContext.TenantID, UserID: securityContext.UserID, CaseID: "case-selection-b",
		CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "selection-current", ContextEpoch: 2,
	})
	if caseBContext.DatasetSnapshotID == securityContext.DatasetSnapshotID {
		t.Fatalf("case-scoped DSV2 identity was reused across cases: %s", securityContext.DatasetSnapshotID)
	}
	crossCaseSnapshotInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: caseBContext, Generation: 1,
		EntityReferences: append([]domaincaseentity.ReferenceV1(nil), index.EntityReferences...),
		EntityIdentities: append([]domaincaseentity.CaseEntityIdentityStateV1(nil), index.EntityIdentities...),
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			ContextEpoch:      caseBContext.ContextEpoch,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Evidence:               append([]domaincaseentity.CaseEvidenceStateV1(nil), index.Evidence...),
		Claims:                 append([]domaincaseentity.CaseClaimStateV1(nil), index.Claims...),
		OpenQuestionReferences: append([]string(nil), index.OpenQuestionReferences...),
		DataGapReferences:      append([]string(nil), index.DataGapReferences...),
		Continuations:          append([]domaincaseentity.CaseContinuationStateV1(nil), index.Continuations...),
	}
	if _, crossErr := domaincaseentity.NewCaseLongitudinalIndexRecordV1(crossCaseSnapshotInput); crossErr == nil {
		t.Fatal("case B owner accepted case A's immutable snapshot identity")
	}
	caseBEvidence := append([]domaincaseentity.CaseEvidenceStateV1(nil), index.Evidence...)
	for evidenceIndex := range caseBEvidence {
		caseBEvidence[evidenceIndex].DatasetSnapshotID = caseBContext.DatasetSnapshotID
	}
	caseBClaims := append([]domaincaseentity.CaseClaimStateV1(nil), index.Claims...)
	for claimIndex := range caseBClaims {
		caseBClaims[claimIndex].DatasetSnapshotID = caseBContext.DatasetSnapshotID
	}
	caseBContinuations := append([]domaincaseentity.CaseContinuationStateV1(nil), index.Continuations...)
	for continuationIndex := range caseBContinuations {
		caseBContinuations[continuationIndex].DatasetSnapshotID = caseBContext.DatasetSnapshotID
	}
	caseBIndex, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: caseBContext, Generation: 1,
		EntityReferences: append([]domaincaseentity.ReferenceV1(nil), index.EntityReferences...),
		EntityIdentities: append([]domaincaseentity.CaseEntityIdentityStateV1(nil), index.EntityIdentities...),
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: caseBContext.DatasetSnapshotID,
			ContextEpoch:      caseBContext.ContextEpoch,
			Currentness:       domaincaseentity.SnapshotCurrentV1,
		}},
		Evidence: caseBEvidence, Claims: caseBClaims,
		OpenQuestionReferences: append([]string(nil), index.OpenQuestionReferences...),
		DataGapReferences:      append([]string(nil), index.DataGapReferences...),
		Continuations:          caseBContinuations,
	})
	if err != nil {
		t.Fatalf("construct case-scoped case B index: %v", err)
	}
	caseBState, err := providerIngressLongitudinalStateFromIndexV1(caseBIndex)
	if err != nil {
		t.Fatalf("assemble case-scoped case B selection: %v", err)
	}
	caseATokens := map[string]string{}
	for _, item := range state.Items {
		if item.ReferenceDigest != "" {
			caseATokens[item.ReferenceKind] = item.ReferenceDigest
		}
	}
	for _, item := range caseBState.Items {
		if item.ReferenceDigest != "" && item.ReferenceDigest == caseATokens[item.ReferenceKind] {
			t.Fatalf("same raw %s owner ID produced a cross-case reusable provider token %s", item.ReferenceKind, item.ReferenceDigest)
		}
	}
	if state.Items[0].SnapshotBindingDigest == caseBState.Items[0].SnapshotBindingDigest ||
		state.Items[0].EvidenceBindingDigest == caseBState.Items[0].EvidenceBindingDigest {
		t.Fatal("case-scoped snapshot or evidence binding was reusable across cases")
	}
	projection := newProviderIngressProjectionWithLongitudinalStateV1(
		"analyze acct:1", state, providerIngressEntityDescriptorV1{
			reference: reference, alias: "acct:1",
			entityType:           domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			financialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		},
	)
	projectionCalls := 0
	if err := projection.UseExactWithDescriptorsAndStateV1(func(
		_ string,
		_ uint32,
		useDescriptors ProviderIngressDescriptorsUseV1,
		_ ProviderIngressLongitudinalStateV1,
	) error {
		projectionCalls++
		return useDescriptors(func(uint32, domaincaseentity.ModelEntityAliasV1, string, string, string, string) error {
			return nil
		})
	}); err != nil || projectionCalls != 1 {
		t.Fatalf("consume restarted typed projection: calls=%d err=%v", projectionCalls, err)
	}
}

func TestProviderIngressLongitudinalSelectionExactBoundaryOrderReplayAndHostileState(t *testing.T) {
	exactIndex, exactCurrent, exactHistorical := longitudinalPriorityIndexV1(t, 20)
	exact, err := providerIngressLongitudinalStateFromIndexV1(exactIndex)
	if err != nil {
		t.Fatal(err)
	}
	expectedKinds := []string{
		ProviderIngressLongitudinalCurrentVerifiedFactV1,
		ProviderIngressLongitudinalHistoricalComparisonFactV1,
		ProviderIngressLongitudinalKeyRelationshipV1,
		ProviderIngressLongitudinalCounterevidenceRefutedV1,
		ProviderIngressLongitudinalDataGapV1,
		ProviderIngressLongitudinalDataGapV1,
	}
	for len(expectedKinds) < ProviderIngressLongitudinalSelectionBudgetV1-1 {
		expectedKinds = append(expectedKinds, ProviderIngressLongitudinalEvidenceClaimReferenceV1)
	}
	expectedKinds = append(expectedKinds, ProviderIngressLongitudinalSnapshotDifferenceV1)
	if len(exact.Items) != ProviderIngressLongitudinalSelectionBudgetV1 || exact.OmittedTotal != 0 ||
		len(exact.OmittedCoverage) != len(providerIngressLongitudinalPriorityV1) {
		t.Fatalf("exact budget boundary mismatch: %#v", exact)
	}
	for index, item := range exact.Items {
		if item.Kind != expectedKinds[index] {
			t.Fatalf("priority order[%d]=%s want=%s", index, item.Kind, expectedKinds[index])
		}
	}
	if exact.Items[0].Currentness != domaincaseentity.SnapshotCurrentV1 ||
		exact.Items[0].InvestigationState != domaincaseentity.InvestigationConfirmedV1 ||
		exact.Items[0].EvidenceReferenceCount != 1 ||
		!domainsecurity.IsSHA256Hex(exact.Items[0].EvidenceBindingDigest) ||
		exact.Items[1].Currentness != domaincaseentity.SnapshotHistoricalV1 ||
		exact.Items[2].ClaimType != "relationship" ||
		exact.Items[3].InvestigationState != domaincaseentity.InvestigationRejectedV1 ||
		exact.Items[3].CounterEvidenceReferenceCount != 1 {
		t.Fatalf("selected items lost typed currentness or evidence binding: %#v", exact.Items[:4])
	}
	body, err := json.Marshal(exact)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		exactCurrent.DatasetSnapshotID, exactHistorical.DatasetSnapshotID,
		exactCurrent.CaseID, exactCurrent.CaseBindingHash,
		string(exactIndex.EntityReferences[0]),
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("typed selection reflected private owner scope %q", forbidden)
		}
	}

	overflowIndex, _, _ := longitudinalPriorityIndexV1(t, 21)
	overflow, err := providerIngressLongitudinalStateFromIndexV1(overflowIndex)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := providerIngressLongitudinalStateFromIndexV1(overflowIndex)
	if err != nil || !reflect.DeepEqual(overflow, replayed) {
		t.Fatalf("deterministic replay changed selection: err=%v", err)
	}
	lastCoverage := overflow.OmittedCoverage[len(overflow.OmittedCoverage)-1]
	if len(overflow.Items) != ProviderIngressLongitudinalSelectionBudgetV1 || overflow.OmittedTotal != 1 ||
		lastCoverage.Kind != ProviderIngressLongitudinalSnapshotDifferenceV1 || lastCoverage.Count != 1 {
		t.Fatalf("unified budget did not omit only the lowest-priority difference: %#v", overflow)
	}
	for _, item := range overflow.Items {
		if item.Kind == ProviderIngressLongitudinalSnapshotDifferenceV1 {
			t.Fatal("lower-priority snapshot difference displaced a selected reference")
		}
	}

	claimIndex := 0
	historicalIndex := 1
	hostile := map[string]func(ProviderIngressLongitudinalStateV1){
		"unknown kind": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[claimIndex].Kind = "model_memory_summary"
		},
		"duplicate item": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[1] = value.Items[0]
		},
		"priority reorder": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[0], value.Items[1] = value.Items[1], value.Items[0]
		},
		"missing reference digest": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[claimIndex].ReferenceDigest = ""
		},
		"malformed reference digest": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[claimIndex].ReferenceDigest = strings.Repeat("r", 64)
		},
		"uppercase reference digest": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[claimIndex].ReferenceDigest = strings.Repeat("A", 64)
		},
		"historical upgraded": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[historicalIndex].Currentness = domaincaseentity.SnapshotCurrentV1
		},
		"unknown claim type": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[claimIndex].ClaimType = "free_text_relationship"
		},
		"missing evidence binding": func(value ProviderIngressLongitudinalStateV1) {
			value.Items[claimIndex].EvidenceBindingDigest = ""
		},
		"omitted coverage mismatch": func(value ProviderIngressLongitudinalStateV1) {
			value.OmittedCoverage[0].Count++
		},
	}
	for name, mutate := range hostile {
		t.Run(name, func(t *testing.T) {
			value := cloneProviderIngressLongitudinalStateV1(exact)
			mutate(value)
			if validateProviderIngressLongitudinalStateV1(value) == nil {
				t.Fatal("hostile longitudinal selection was accepted")
			}
		})
	}
	corrupt := exactIndex
	corrupt.RecordDigest = domainsecurity.SHA256Hex([]byte("corrupt-authoritative-index"))
	if selected, err := providerIngressLongitudinalStateFromIndexV1(corrupt); err == nil ||
		!reflect.DeepEqual(selected, ProviderIngressLongitudinalStateV1{}) {
		t.Fatalf("corrupt authoritative index produced provider effect: state=%#v err=%v", selected, err)
	}
}

func longitudinalPriorityIndexV1(
	t *testing.T,
	extraEvidence int,
) (domaincaseentity.ThreadCaseContextRecord, domainsecurity.TurnSecurityContext, domainsecurity.TurnSecurityContext) {
	t.Helper()
	current := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-priority", TurnID: "turn-current",
		TenantID: "tenant-priority", UserID: "user-priority", CaseID: "case-priority",
		CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "priority-current", ContextEpoch: 2,
	})
	historical := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-priority", TurnID: "turn-historical",
		TenantID: current.TenantID, UserID: current.UserID, CaseID: current.CaseID,
		CaseBindingHash: current.CaseBindingHash, SnapshotSeed: "priority-historical", ContextEpoch: 1,
	})
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(domainsecurity.SHA256Hex([]byte("priority-entity")))
	if err != nil {
		t.Fatal(err)
	}
	typed := func(claimType string) *domaincaseentity.CaseClaimTypedStateV1 {
		state, stateErr := domaincaseentity.NewCaseClaimTypedStateV1(claimType)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		return state
	}
	evidence := []domaincaseentity.CaseEvidenceStateV1{
		{
			EvidenceReference: "evr_" + domainsecurity.SHA256Hex([]byte("priority-current-evidence-ref")),
			EvidenceDigest:    domainsecurity.SHA256Hex([]byte("priority-current-evidence")),
			DatasetSnapshotID: current.DatasetSnapshotID, Currentness: domaincaseentity.SnapshotCurrentV1,
		},
		{
			EvidenceReference: "evr_" + domainsecurity.SHA256Hex([]byte("priority-historical-evidence-ref")),
			EvidenceDigest:    domainsecurity.SHA256Hex([]byte("priority-historical-evidence")),
			DatasetSnapshotID: historical.DatasetSnapshotID, Currentness: domaincaseentity.SnapshotHistoricalV1,
		},
	}
	for index := 0; index < extraEvidence; index++ {
		evidence = append(evidence, domaincaseentity.CaseEvidenceStateV1{
			EvidenceReference: "evr_" + domainsecurity.SHA256Hex([]byte(fmt.Sprintf("priority-extra-ref-%02d", index))),
			EvidenceDigest:    domainsecurity.SHA256Hex([]byte(fmt.Sprintf("priority-extra-digest-%02d", index))),
			DatasetSnapshotID: current.DatasetSnapshotID, Currentness: domaincaseentity.SnapshotCurrentV1,
		})
	}
	claims := []domaincaseentity.CaseClaimStateV1{
		{
			ClaimReference: "claim_current_verified", ClaimDigest: domainsecurity.SHA256Hex([]byte("claim-current-verified")),
			TypedState: typed("entity"), DatasetSnapshotID: current.DatasetSnapshotID,
			Currentness: domaincaseentity.SnapshotCurrentV1, InvestigationState: domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences: []string{evidence[0].EvidenceReference}, CounterEvidenceReferences: []string{},
		},
		{
			ClaimReference: "claim_historical_comparison", ClaimDigest: domainsecurity.SHA256Hex([]byte("claim-historical-comparison")),
			TypedState: typed("amount"), DatasetSnapshotID: historical.DatasetSnapshotID,
			Currentness: domaincaseentity.SnapshotHistoricalV1, InvestigationState: domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences: []string{evidence[1].EvidenceReference}, CounterEvidenceReferences: []string{},
		},
		{
			ClaimReference: "claim_key_relationship", ClaimDigest: domainsecurity.SHA256Hex([]byte("claim-key-relationship")),
			TypedState: typed("relationship"), DatasetSnapshotID: current.DatasetSnapshotID,
			Currentness: domaincaseentity.SnapshotCurrentV1, InvestigationState: domaincaseentity.InvestigationOpenV1,
			EvidenceReferences: []string{}, CounterEvidenceReferences: []string{},
		},
		{
			ClaimReference: "claim_refuted", ClaimDigest: domainsecurity.SHA256Hex([]byte("claim-refuted")),
			TypedState: typed("entity"), DatasetSnapshotID: current.DatasetSnapshotID,
			Currentness: domaincaseentity.SnapshotCurrentV1, InvestigationState: domaincaseentity.InvestigationRejectedV1,
			EvidenceReferences: []string{}, CounterEvidenceReferences: []string{evidence[0].EvidenceReference},
		},
		{
			ClaimReference: "claim_open_reference", ClaimDigest: domainsecurity.SHA256Hex([]byte("claim-open-reference")),
			TypedState: typed("entity"), DatasetSnapshotID: current.DatasetSnapshotID,
			Currentness: domaincaseentity.SnapshotCurrentV1, InvestigationState: domaincaseentity.InvestigationOpenV1,
			EvidenceReferences: []string{}, CounterEvidenceReferences: []string{},
		},
	}
	index, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: current, Generation: 1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: []domaincaseentity.CaseEntityIdentityStateV1{{
			Reference: reference, EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1,
		}},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{
			{DatasetSnapshotID: historical.DatasetSnapshotID, ContextEpoch: historical.ContextEpoch, Currentness: domaincaseentity.SnapshotHistoricalV1},
			{DatasetSnapshotID: current.DatasetSnapshotID, ContextEpoch: current.ContextEpoch, Currentness: domaincaseentity.SnapshotCurrentV1},
		},
		Claims: claims, Evidence: evidence,
		OpenQuestionReferences: []string{"question_open_counterparty"},
		DataGapReferences:      []string{"gap_missing_period"},
		Continuations: []domaincaseentity.CaseContinuationStateV1{
			{ContinuationDigest: domainsecurity.SHA256Hex([]byte("priority-current-continuation")), DatasetSnapshotID: current.DatasetSnapshotID, Currentness: domaincaseentity.SnapshotCurrentV1},
			{ContinuationDigest: domainsecurity.SHA256Hex([]byte("priority-historical-continuation")), DatasetSnapshotID: historical.DatasetSnapshotID, Currentness: domaincaseentity.SnapshotHistoricalV1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index, current, historical
}
