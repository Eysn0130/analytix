package evidence

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCaseForegroundAnswerSlotResolverBindsExactCurrentReceiptGroup(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	output, receipt := seedCaseForegroundAccountFlowGroupV1(t, issuer, input, "exact", false)
	bindings, err := ResolveCurrentCaseForegroundAnswerSlotsV1(
		context.Background(), issuer.Registry, input.Context, []domainnative.AccountFlowProviderModelOutputV1{output},
	)
	if err != nil || len(bindings) != 1 || len(bindings[0].Claims) != 3 || len(bindings[0].Evidence) != 1 ||
		bindings[0].Evidence[0].Digest != receipt.ReceiptDigest {
		t.Fatalf("exact current claim/evidence group was not resolved: bindings=%#v err=%v", bindings, err)
	}
	result := caseForegroundResultForBindingV1(t, bindings[0])
	if err := ValidateCurrentCaseForegroundChildResultV1(context.Background(), issuer.Registry, input.Context, result); err != nil {
		t.Fatalf("exact current child selection was rejected: %v", err)
	}

	empty := result
	empty.Claims = []domainjob.CaseDelegatedClaimReferenceV1{}
	empty.Evidence = []domainjob.CaseDelegatedEvidenceReferenceV1{}
	if err := ValidateCurrentCaseForegroundChildResultV1(context.Background(), issuer.Registry, input.Context, empty); err == nil {
		t.Fatal("empty claim/evidence selection acquired current registry authority")
	}
}

func TestCaseForegroundAnswerSlotResolverRejectsMixedCurrentReceiptGroups(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	firstOutput, _ := seedCaseForegroundAccountFlowGroupV1(t, issuer, input, "first", false)
	secondOutput, _ := seedCaseForegroundAccountFlowGroupV1(t, issuer, input, "second", false)
	first, firstErr := ResolveCurrentCaseForegroundAnswerSlotsV1(
		context.Background(), issuer.Registry, input.Context, []domainnative.AccountFlowProviderModelOutputV1{firstOutput},
	)
	second, secondErr := ResolveCurrentCaseForegroundAnswerSlotsV1(
		context.Background(), issuer.Registry, input.Context, []domainnative.AccountFlowProviderModelOutputV1{secondOutput},
	)
	if firstErr != nil || secondErr != nil || len(first) != 1 || len(second) != 1 {
		t.Fatalf("two current receipt groups were not independently resolvable: first=%#v second=%#v err=%v/%v", first, second, firstErr, secondErr)
	}

	mixedClaims := caseForegroundResultForBindingV1(t, first[0])
	mixedClaims.Claims[2] = second[0].Claims[2]
	sort.Slice(mixedClaims.Claims, func(left, right int) bool { return mixedClaims.Claims[left].Digest < mixedClaims.Claims[right].Digest })
	if err := domainjob.ValidateCaseForegroundChildResultShapeV1(mixedClaims); err != nil {
		t.Fatalf("mixed-claim hostile fixture did not retain the closed shape: %v", err)
	}
	if err := ValidateCurrentCaseForegroundChildResultV1(context.Background(), issuer.Registry, input.Context, mixedClaims); err == nil {
		t.Fatal("claims from two current receipt groups were mixed into one answer slot")
	}

	mixedEvidence := caseForegroundResultForBindingV1(t, first[0])
	mixedEvidence.Evidence = append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, second[0].Evidence...)
	if err := ValidateCurrentCaseForegroundChildResultV1(context.Background(), issuer.Registry, input.Context, mixedEvidence); err == nil {
		t.Fatal("a current evidence receipt from another query was attached to the selected slot")
	}
}

func TestCaseForegroundAnswerSlotResolverPreservesClosedPartialGaps(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	output, _ := seedCaseForegroundAccountFlowGroupV1(t, issuer, input, "partial", true)
	bindings, err := ResolveCurrentCaseForegroundAnswerSlotsV1(
		context.Background(), issuer.Registry, input.Context, []domainnative.AccountFlowProviderModelOutputV1{output},
	)
	if err != nil || len(bindings) != 1 || bindings[0].AnswerSlot.AggregateComplete ||
		bindings[0].AnswerSlot.EvidenceRowsComplete || len(bindings[0].AnswerSlot.Gaps) != 3 {
		t.Fatalf("partial host semantic did not remain a closed typed proposal: bindings=%#v err=%v", bindings, err)
	}
	for _, claim := range bindings[0].Claims {
		if claim.InvestigationState != domaincaseentity.InvestigationOpenV1 {
			t.Fatalf("partial receipt claim was not delegated as open: %#v", bindings[0].Claims)
		}
	}
	result := caseForegroundResultForBindingV1(t, bindings[0])
	if err := ValidateCurrentCaseForegroundChildResultV1(context.Background(), issuer.Registry, input.Context, result); err != nil {
		t.Fatalf("current partial/gap typed selection was rejected: %v", err)
	}
}

func caseForegroundResultForBindingV1(t *testing.T, binding domainjob.CaseDelegatedAnswerSlotBindingV1) domainjob.CaseForegroundChildResultV1 {
	t.Helper()
	result := domainjob.CaseForegroundChildResultV1{
		SchemaVersion:    domainjob.CaseForegroundChildResultSchemaVersionV1,
		Purpose:          domainjob.CaseForegroundChildResultPurposeV1,
		DelegationDigest: domainsecurity.SHA256Hex([]byte("case-foreground-test-delegation")),
		EntityAliases:    []domaincaseentity.ModelEntityAliasV1{domaincaseentity.ModelEntityAliasV1(binding.AnswerSlot.SubjectAlias)},
		Claims:           append([]domainjob.CaseDelegatedClaimReferenceV1{}, binding.Claims...),
		Evidence:         append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, binding.Evidence...),
		Gaps:             append([]string{}, binding.AnswerSlot.Gaps...),
		AnswerSlots:      []domainnative.AccountFlowDelegatedAnswerSlotV1{binding.AnswerSlot},
		Currentness:      domaincaseentity.SnapshotCurrentV1,
	}
	if err := domainjob.ValidateCaseForegroundChildResultShapeV1(result); err != nil {
		t.Fatalf("case foreground result fixture is invalid: %v", err)
	}
	return result
}

func seedCaseForegroundAccountFlowGroupV1(
	t *testing.T,
	issuer Issuer,
	input IssueEvidenceInput,
	suffix string,
	partial bool,
) (domainnative.AccountFlowProviderModelOutputV1, domainevidence.EvidenceReceipt) {
	t.Helper()
	const subject = "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	start := "2026-01-01T00:00:00.000000Z"
	end := "2026-01-31T23:59:59.000000Z"
	receiptStart := "2026-01-01T00:00:00Z"
	receiptEnd := "2026-01-31T23:59:59Z"
	occurredAt := "2026-01-05T10:30:00.000000Z"
	if suffix == "second" {
		start, end = "2026-02-01T00:00:00.000000Z", "2026-02-28T23:59:59.000000Z"
		receiptStart, receiptEnd = "2026-02-01T00:00:00Z", "2026-02-28T23:59:59Z"
		occurredAt = "2026-02-05T10:30:00.000000Z"
	}
	queryHash := domainsecurity.SHA256Hex([]byte("case-foreground-query:" + suffix))
	resultHash := domainsecurity.SHA256Hex([]byte("case-foreground-result:" + suffix))
	// The evidence app consumes only the pseudonymous receipt reference; the
	// restricted source-row type and reverse mapping remain in the domain owner.
	sourceRecordID := "srow1_" + domainsecurity.SHA256Hex([]byte("case-foreground-source-row:"+suffix))
	factIDs := []string{
		accountFlowAggregateFactPrefix + domainsecurity.SHA256Hex([]byte(suffix+":in")),
		accountFlowAggregateFactPrefix + domainsecurity.SHA256Hex([]byte(suffix+":out")),
		accountFlowAggregateFactPrefix + domainsecurity.SHA256Hex([]byte(suffix+":count")),
	}
	inflow, outflow, count := "100", "0", "1"
	transactionCount := uint64(1)
	coverage := domainnative.AccountFlowProviderSemanticCoverageV1{
		State:                  domainnative.AccountFlowCoveragePartialV1,
		Gaps:                   []string{domainnative.AccountFlowGapCounterpartyResolutionV1},
		NormalizedSnapshotRows: 1, AcceptedSnapshotRows: 1, ObservedMatchingRows: 1,
	}
	aggregateComplete, evidenceRowsComplete := true, true
	if partial {
		outflow, count, transactionCount = "20", "2", 2
		aggregateComplete, evidenceRowsComplete = false, false
		coverage = domainnative.AccountFlowProviderSemanticCoverageV1{
			State: domainnative.AccountFlowCoveragePartialV1,
			Gaps: []string{
				domainnative.AccountFlowGapDuplicateSourceRowsV1,
				domainnative.AccountFlowGapEvidenceRowLimitV1,
				domainnative.AccountFlowGapCounterpartyResolutionV1,
			},
			NormalizedSnapshotRows: 3, AcceptedSnapshotRows: 2, DuplicateSnapshotRows: 1, ObservedMatchingRows: 2,
		}
	}
	semantic := domainnative.AccountFlowProviderSemanticResultV1{
		SubjectAlias: "acct:1", StartInclusive: start, EndInclusive: end,
		Timezone: "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: inflow, OutflowMinor: outflow, NetMinor: func() string {
			if partial {
				return "80"
			}
			return "100"
		}(),
		TransactionCount: transactionCount, EvidenceTransactionCount: 1, EvidenceRowLimit: 1,
		AggregateComplete: aggregateComplete, EvidenceRowsComplete: evidenceRowsComplete,
		CounterpartySemanticsComplete: false,
		Currentness:                   domainnative.AccountFlowProviderCurrentnessCurrentV1, Coverage: coverage,
		Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{{
			EvidenceRef:  sourceRecordID,
			Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
			OccurredAt:   occurredAt, Direction: domainnative.AccountFlowDirectionInflowV1,
			AmountMinor: "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		}},
		QueryHash: queryHash, ResultHash: resultHash,
	}
	semantic.Outcome, _ = domainnative.NewAccountFlowProviderOutcomeV1("acct:1", aggregateComplete, evidenceRowsComplete, queryHash, resultHash)
	output := domainnative.AccountFlowProviderModelOutputV1{
		SchemaVersion: 3, Purpose: domainnative.AccountFlowProviderModelPurposeV1,
		SemanticStatus: domainevidence.SemanticPartial, Data: semantic,
	}
	if _, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(output); err != nil {
		t.Fatalf("account-flow provider fixture is invalid: %v", err)
	}
	facts := []domainevidence.CanonicalEvidenceFact{
		{FactID: factIDs[0], ClaimType: domainevidence.ClaimAmount, NormalizedPayload: domainevidence.NormalizedClaimPayload{
			SubjectID: subject, EntityID: subject, AccountID: subject, AmountMinor: inflow, Currency: "CNY", Direction: "in",
			StartAt: receiptStart, EndAt: receiptEnd, Granularity: accountFlowReceiptGranularity,
		}},
		{FactID: factIDs[1], ClaimType: domainevidence.ClaimAmount, NormalizedPayload: domainevidence.NormalizedClaimPayload{
			SubjectID: subject, EntityID: subject, AccountID: subject, AmountMinor: outflow, Currency: "CNY", Direction: "out",
			StartAt: receiptStart, EndAt: receiptEnd, Granularity: accountFlowReceiptGranularity,
		}},
		{FactID: factIDs[2], ClaimType: domainevidence.ClaimCount, NormalizedPayload: domainevidence.NormalizedClaimPayload{
			SubjectID: subject, EntityID: subject, Count: count, StartAt: receiptStart, EndAt: receiptEnd, Granularity: accountFlowReceiptGranularity,
		}},
	}
	binding, err := domainevidence.NewAcceptedSlotSourceBindingV1(domainevidence.AcceptedSlotSourceBindingInputV1{
		FactIDs: factIDs, EntityReference: subject, SourceRecordID: sourceRecordID,
		SourceFileID: "0123456789abcdefabcd", SourceRowNumber: 7, Field: domainevidence.AcceptedSlotSourceFieldAccountV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	bindingDigest, err := domainevidence.AcceptedSlotSourceBindingSetDigestV1([]domainevidence.AcceptedSlotSourceBindingV1{binding})
	if err != nil {
		t.Fatal(err)
	}
	material := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersionV3, Purpose: domainevidence.CanonicalEvidencePurposeV3,
		Facts: facts, AcceptedSlotSourceBindings: []domainevidence.AcceptedSlotSourceBindingV1{binding},
		AcceptedSlotSourceBindingSetDigest: bindingDigest,
	}
	canonical, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	queryScope, err := domainevidence.NewAccountFlowQueryScopeRefV1(input.Context.ContextDigest, queryHash)
	if err != nil {
		t.Fatal(err)
	}
	input.Grant.ToolName = fundsAccountFlowCanonicalTool
	input.Material.CanonicalEvidence = canonical
	input.Material.SourceType = "transactions"
	input.Material.QueryHash = queryHash
	input.Material.QueryRange = domainevidence.EvidenceQueryRange{
		EntityIDs: []string{subject}, AccountIDs: []string{subject}, Directions: []string{"in", "out"},
		StartAt: receiptStart, EndAt: receiptEnd, SourceIDs: []string{queryScope},
		FiltersHash: domainsecurity.SHA256Hex([]byte("case-foreground-filters:" + suffix)),
	}
	input.Material.Granularity = accountFlowReceiptGranularity
	input.Material.Currency = "CNY"
	input.Material.Timezone = "Z"
	input.Material.PaginationCompleteness = domainevidence.PaginationComplete
	if partial {
		input.Material.PaginationCompleteness = domainevidence.PaginationPartial
		alignEvidenceOutcomeWithPagination(&input, domainevidence.PaginationPartial)
	}
	input.Material.SourceRecordIDs = []string{sourceRecordID}
	input.Material.TransformationLineage = []domainevidence.TransformationLineageStep{
		{StepID: accountFlowNativeBindStepIDV1, Transformer: accountFlowNativeBinderV1, TransformerVersion: "1", InputHash: input.RawResult.RawSHA256, OutputHash: resultHash},
		{StepID: accountFlowNormalizeStepIDV1, Transformer: accountFlowNormalizerV1, TransformerVersion: "1", InputHash: resultHash, OutputHash: domainsecurity.CanonicalJSONHash(canonical)},
	}
	input.Material.PIIClassification = domainevidence.PIINone
	for _, fact := range facts {
		if !domainevidence.SourceTypeSupportsClaim(input.Material.SourceType, fact.ClaimType) ||
			!domainevidence.EvidenceQueryRangeSupportsClaimPayload(input.Material.QueryRange, fact.NormalizedPayload) {
			t.Fatalf("account-flow fact fixture exceeds its query scope: fact=%#v scope=%#v", fact, input.Material.QueryRange)
		}
	}
	canonicalEvidence, err := domainevidence.CanonicalEvidenceBytes(input.Material.CanonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(evidenceReceiptDraftInput(
		input, canonicalEvidence, domainevidence.EvidenceSettlementReceiptID(domainsecurity.SHA256Hex([]byte("case-foreground-settlement:"+suffix))), evidenceIssuerTime(),
	))
	if err != nil {
		t.Fatal(err)
	}
	parsedMaterial, err := domainevidence.ParseCanonicalEvidenceMaterial(canonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range parsedMaterial.Facts {
		if !domainevidence.SourceTypeSupportsClaim(draft.SourceType, fact.ClaimType) ||
			!domainevidence.EvidenceQueryRangeSupportsClaimPayload(draft.QueryRange, fact.NormalizedPayload) ||
			!domainevidence.EvidenceReceiptMetadataSupportsClaim(draft, fact.ClaimType, fact.NormalizedPayload) {
			t.Fatalf("canonical account-flow fact exceeds draft receipt: fact=%#v receipt=%#v", fact, draft)
		}
	}
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	if receipt.QueryHash != queryHash || receipt.TransformationLineage[0].OutputHash != resultHash {
		t.Fatalf("account-flow receipt fixture detached from provider semantics: %#v", receipt)
	}
	return output, receipt
}
