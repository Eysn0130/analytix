package evidence

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRegistryPublicationProjectsCompleteAccountFlowAggregateFacts(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	receipt := seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "one", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
	candidates, err := VerifiedPublicationCandidatesFromRegistrySnapshot(context.Background(), issuer.Registry, input.Context, evidenceIssuerTime())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.Claims) != 3 || len(candidates.NoHitReceiptIDs) != 0 {
		t.Fatalf("account-flow registry projection published wrong candidates: %#v", candidates)
	}
	for _, claim := range candidates.Claims {
		if claim.EvidenceIDs[0] != receipt.ReceiptID || claim.NormalizedPayload.Granularity != accountFlowReceiptGranularity {
			t.Fatalf("account-flow candidate was not bound to aggregate receipt fact: %#v", claim)
		}
	}
}

func TestFinalGateIsolatesProviderBodyOnlyForCompleteCurrentAccountFlowGroup(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "wrong_model_amount", body: "My arithmetic says the answer is 999."},
		{name: "no_amount_explanation", body: "I summarized the available account-flow evidence."},
	} {
		t.Run(test.name, func(t *testing.T) {
			issuer, input := evidenceIssuerFixture(t)
			receipt := seedAccountFlowAggregateForFinalGateTest(t, issuer, input, test.name, "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
			slot, err := domainordinaryresult.NewResultSlotV1(test.body)
			if err != nil {
				t.Fatalf("provider fixture did not pass the ordinary projection: %v", err)
			}
			boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
				Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
				CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer || boundary.Envelope.OrdinaryResult != nil ||
				len(boundary.Envelope.EvidenceReceiptIDs) != 1 || boundary.Envelope.EvidenceReceiptIDs[0] != receipt.ReceiptID ||
				strings.Contains(boundary.Text, test.body) || strings.Contains(boundary.Text, "999") ||
				!strings.Contains(boundary.Text, "流入 100 CNY") || !strings.Contains(boundary.Text, "有符号净额 100 CNY") {
				t.Fatalf("complete account-flow group did not isolate provider prose from the exact host result: %#v", boundary)
			}
		})
	}
}

func TestFinalGateDoesNotLeakRejectedAccountFlowProviderBody(t *testing.T) {
	longRef := "cer1_" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "long_ref", body: "account " + longRef + " was included in the draft"},
		{name: "pii", body: "the contact phone is 13800138000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			issuer, input := evidenceIssuerFixture(t)
			seedAccountFlowAggregateForFinalGateTest(t, issuer, input, test.name, "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
			projected, err := domainordinaryresult.NewResultSlotV1(test.body)
			if err != nil {
				projected, err = domainordinaryresult.NewHostFixedResultSlotV1(domainordinaryresult.HostFixedProtectedFactBlockedTextV1)
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(projected.Text, longRef) || strings.Contains(projected.Text, "13800138000") {
				t.Fatalf("unsafe provider body survived the ordinary projection: %#v", projected)
			}
			boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
				Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &projected,
				CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if boundary.Envelope.OrdinaryResult != nil || strings.Contains(boundary.Text, longRef) ||
				strings.Contains(boundary.Text, "13800138000") || strings.Contains(boundary.Text, projected.Text) ||
				!strings.Contains(boundary.Text, "有符号净额 100 CNY") {
				t.Fatalf("unsafe provider body or its projection was concatenated with the exact account-flow answer: %#v", boundary)
			}
		})
	}
}

func TestFinalGatePreservesExistingHandlingOutsideCompleteAccountFlowGroups(t *testing.T) {
	t.Run("missing_claim", func(t *testing.T) {
		issuer, input := evidenceIssuerFixture(t)
		seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "missing", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
		candidates, err := VerifiedPublicationCandidatesFromRegistrySnapshot(context.Background(), issuer.Registry, input.Context, evidenceIssuerTime())
		if err != nil || len(candidates.Claims) != 3 {
			t.Fatalf("account-flow candidates unavailable: %#v err=%v", candidates, err)
		}
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied a bounded non-numeric explanation.")
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			Claims: candidates.Claims[:2], IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.Variant != domainevidence.EvidenceBackedAnswer || envelope.AccountFlowOutcome != nil || envelope.OrdinaryResult == nil ||
			envelope.OrdinaryResult.Text != slot.Text {
			t.Fatalf("incomplete account-flow claim group changed existing ordinary-result handling: %#v err=%v", envelope, err)
		}
		envelope, err = (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: TerminalProviderFailure, OrdinaryResult: &slot,
			Claims: candidates.Claims, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || envelope.OrdinaryResult == nil || len(envelope.Claims) != 0 {
			t.Fatalf("failed account-flow terminal changed existing fail-closed handling: %#v err=%v", envelope, err)
		}
	})

	t.Run("cross_query", func(t *testing.T) {
		for _, test := range []struct {
			name string
			body string
		}{
			{name: "wrong_combined_amount", body: "My arithmetic says the combined answer is 999."},
			{name: "amount_free_explanation", body: "I summarized the two account-flow periods."},
		} {
			t.Run(test.name, func(t *testing.T) {
				issuer, input := evidenceIssuerFixture(t)
				seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "january", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
				seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "february", "2026-02-01T00:00:00Z", "2026-02-28T23:59:59Z", "7", "2", "2")
				slot, err := domainordinaryresult.NewResultSlotV1(test.body)
				if err != nil {
					t.Fatal(err)
				}
				boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
					Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
					CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
				})
				if err != nil || boundary.Envelope.Variant != domainevidence.PartialEvidenceAnswer || boundary.Envelope.AccountFlowOutcome != nil || boundary.Envelope.OrdinaryResult != nil ||
					len(boundary.Envelope.EvidenceReceiptIDs) != 2 || strings.Contains(boundary.Text, test.body) || strings.Contains(boundary.Text, "999") ||
					strings.Count(boundary.Text, "资金汇总") != 2 || strings.Count(boundary.Text, "有符号净额 100 CNY") != 1 ||
					strings.Count(boundary.Text, "有符号净额 5 CNY") != 1 || strings.Contains(boundary.Text, "有符号净额 105 CNY") {
					t.Fatalf("complete cross-query groups leaked provider prose or merged host arithmetic: %#v err=%v", boundary, err)
				}
			})
		}
	})

	t.Run("cross_receipt_incomplete_groups", func(t *testing.T) {
		issuer, input := evidenceIssuerFixture(t)
		january := seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "january-incomplete", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
		february := seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "february-incomplete", "2026-02-01T00:00:00Z", "2026-02-28T23:59:59Z", "7", "2", "2")
		candidates, err := VerifiedPublicationCandidatesFromRegistrySnapshot(context.Background(), issuer.Registry, input.Context, evidenceIssuerTime())
		if err != nil || len(candidates.Claims) != 6 {
			t.Fatalf("cross-receipt candidates unavailable: %#v err=%v", candidates, err)
		}
		byReceipt := map[string][]domainevidence.ClaimRecord{}
		for _, claim := range candidates.Claims {
			byReceipt[claim.EvidenceIDs[0]] = append(byReceipt[claim.EvidenceIDs[0]], claim)
		}
		if len(byReceipt[january.ReceiptID]) != 3 || len(byReceipt[february.ReceiptID]) != 3 {
			t.Fatalf("cross-receipt fixture did not retain two complete source groups: %#v", byReceipt)
		}
		mixed := append([]domainevidence.ClaimRecord{}, byReceipt[january.ReceiptID][:2]...)
		mixed = append(mixed, byReceipt[february.ReceiptID][0])
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied a bounded explanation for incomplete receipt groups.")
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			Claims: mixed, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.Variant != domainevidence.PartialEvidenceAnswer || envelope.AccountFlowOutcome != nil || envelope.OrdinaryResult == nil ||
			envelope.OrdinaryResult.Text != slot.Text {
			t.Fatalf("cross-receipt incomplete groups suppressed the existing ordinary result: %#v err=%v", envelope, err)
		}
	})

	t.Run("rejected_claim_alongside_complete_group", func(t *testing.T) {
		issuer, input := evidenceIssuerFixture(t)
		seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "with-rejected", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
		candidates, err := VerifiedPublicationCandidatesFromRegistrySnapshot(context.Background(), issuer.Registry, input.Context, evidenceIssuerTime())
		if err != nil || len(candidates.Claims) != 3 {
			t.Fatalf("account-flow candidates unavailable: %#v err=%v", candidates, err)
		}
		rejected := candidates.Claims[0]
		rejected.SupportState = domainevidence.ClaimUnresolved
		claims := append([]domainevidence.ClaimRecord{}, candidates.Claims...)
		claims = append(claims, rejected)
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied a bounded explanation with a rejected claim.")
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			Claims: claims, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.Variant != domainevidence.PartialEvidenceAnswer || envelope.OrdinaryResult == nil ||
			envelope.OrdinaryResult.Text != slot.Text {
			t.Fatalf("a rejected claim was ignored when deciding exact-only isolation: %#v err=%v", envelope, err)
		}
	})

	t.Run("mixed_account_flow_and_other_tool", func(t *testing.T) {
		issuer, input := evidenceIssuerFixture(t)
		seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "mixed-tool", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
		seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied a mixed-tool explanation.")
		if err != nil {
			t.Fatal(err)
		}
		boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || boundary.Envelope.Variant != domainevidence.PartialEvidenceAnswer || boundary.Envelope.OrdinaryResult == nil ||
			!strings.Contains(boundary.Text, slot.Text) {
			t.Fatalf("mixed account-flow and other-tool claims were treated as exact-only groups: %#v err=%v", boundary, err)
		}
	})

	t.Run("partial_other_tool_alongside_complete_group", func(t *testing.T) {
		issuer, input := evidenceIssuerFixture(t)
		seedAccountFlowAggregateForFinalGateTest(t, issuer, input, "mixed-partial", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1")
		partialInput := input
		partialInput.Material.QueryHash = domainsecurity.SHA256Hex([]byte("partial-other-tool-query"))
		partialInput.Material.PaginationCompleteness = domainevidence.PaginationPartial
		seedPreauthorizedRegistryForGateUnitTest(t, issuer, partialInput)
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied a mixed partial explanation.")
		if err != nil {
			t.Fatal(err)
		}
		boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || boundary.Envelope.Variant != domainevidence.PartialEvidenceAnswer || boundary.Envelope.OrdinaryResult == nil ||
			!strings.Contains(boundary.Text, slot.Text) {
			t.Fatalf("a partial non-account-flow claim was ignored when deciding exact-only isolation: %#v err=%v", boundary, err)
		}
	})

	t.Run("unbound_result_lineage", func(t *testing.T) {
		issuer, input := evidenceIssuerFixture(t)
		seedAccountFlowAggregateWithLineageForFinalGateTest(t, issuer, input, "legacy-lineage", "2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z", "100", "0", "1", false)
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied an explanation for a legacy unbound result.")
		if err != nil {
			t.Fatal(err)
		}
		boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer || boundary.Envelope.AccountFlowOutcome != nil || boundary.Envelope.OrdinaryResult == nil ||
			!strings.Contains(boundary.Text, slot.Text) {
			t.Fatalf("claim group without native query/result lineage was treated as the complete exact group: %#v err=%v", boundary, err)
		}
	})

	t.Run("ordinary_chat", func(t *testing.T) {
		_, input := evidenceIssuerFixture(t)
		slot, err := domainordinaryresult.NewResultSlotV1("Updated the focused Go test successfully.")
		if err != nil {
			t.Fatal(err)
		}
		boundary, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			CaseSlotIntent: CaseSlotNotRequestedV1, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || boundary.Text != slot.Text || boundary.Envelope.OrdinaryResult == nil {
			t.Fatalf("ordinary chat was changed by account-flow isolation: %#v err=%v", boundary, err)
		}
	})

	t.Run("other_tool_claim", func(t *testing.T) {
		issuer, input, _ := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
		seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
		slot, err := domainordinaryresult.NewResultSlotV1("The provider supplied a non-account-flow explanation.")
		if err != nil {
			t.Fatal(err)
		}
		boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
			Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
			CaseSlotIntent: CaseSlotRequestedV1, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer || boundary.Envelope.OrdinaryResult == nil ||
			!strings.Contains(boundary.Text, slot.Text) {
			t.Fatalf("a non-account-flow tool result was affected by account-flow body isolation: %#v err=%v", boundary, err)
		}
	})
}

func seedAccountFlowAggregateForFinalGateTest(
	t *testing.T,
	issuer Issuer,
	input IssueEvidenceInput,
	suffix, start, end, inflow, outflow, count string,
) domainevidence.EvidenceReceipt {
	t.Helper()
	return seedAccountFlowAggregateWithLineageForFinalGateTest(t, issuer, input, suffix, start, end, inflow, outflow, count, true)
}

func seedAccountFlowAggregateWithLineageForFinalGateTest(
	t *testing.T,
	issuer Issuer,
	input IssueEvidenceInput,
	suffix, start, end, inflow, outflow, count string,
	exactResultBinding bool,
) domainevidence.EvidenceReceipt {
	t.Helper()
	input.Grant.ToolName = fundsAccountFlowCanonicalTool
	input.Material.Granularity = accountFlowReceiptGranularity
	input.Material.QueryHash = domainsecurity.SHA256Hex([]byte("account-flow-query:" + suffix))
	input.Material.PIIClassification = domainevidence.PIINone
	input.Material.QueryRange.AccountIDs = []string{"00123456789012345678"}
	input.Material.QueryRange.Directions = []string{"in", "out"}
	input.Material.QueryRange.StartAt = start
	input.Material.QueryRange.EndAt = end
	input.Material.QueryRange.FiltersHash = domainsecurity.SHA256Hex([]byte("account-flow-filters:" + suffix))
	input.Material.SourceRecordIDs = []string{"row-account-flow-" + suffix}
	amountPayload := func(amount, direction string) domainevidence.NormalizedClaimPayload {
		return domainevidence.NormalizedClaimPayload{
			SubjectID: "entity-a", EntityID: "entity-a", AccountID: "00123456789012345678",
			AmountMinor: amount, Currency: "CNY", Direction: direction,
			StartAt: start, EndAt: end, Granularity: accountFlowReceiptGranularity,
		}
	}
	countPayload := domainevidence.NormalizedClaimPayload{
		SubjectID: "entity-a", EntityID: "entity-a", Count: count,
		StartAt: start, EndAt: end, Granularity: accountFlowReceiptGranularity,
	}
	material := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{
			{FactID: accountFlowAggregateFactPrefix + suffix + "-in", ClaimType: domainevidence.ClaimAmount, NormalizedPayload: amountPayload(inflow, "in")},
			{FactID: accountFlowAggregateFactPrefix + suffix + "-out", ClaimType: domainevidence.ClaimAmount, NormalizedPayload: amountPayload(outflow, "out")},
			{FactID: accountFlowAggregateFactPrefix + suffix + "-count", ClaimType: domainevidence.ClaimCount, NormalizedPayload: countPayload},
		},
	}
	canonical, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	input.Material.CanonicalEvidence = canonical
	nativeResultHash := domainsecurity.SHA256Hex([]byte("account-flow-native-result:" + suffix))
	if exactResultBinding {
		input.Material.TransformationLineage = []domainevidence.TransformationLineageStep{
			{StepID: accountFlowNativeBindStepIDV1, Transformer: accountFlowNativeBinderV1, TransformerVersion: "1", InputHash: input.RawResult.RawSHA256, OutputHash: nativeResultHash},
			{StepID: accountFlowNormalizeStepIDV1, Transformer: accountFlowNormalizerV1, TransformerVersion: "1", InputHash: nativeResultHash, OutputHash: domainsecurity.CanonicalJSONHash(canonical)},
		}
	} else {
		input.Material.TransformationLineage = []domainevidence.TransformationLineageStep{{
			StepID: "legacy-account-flow-normalize-fixture", Transformer: "fixture-normalizer", TransformerVersion: "1",
			InputHash: input.RawResult.RawSHA256, OutputHash: domainsecurity.CanonicalJSONHash(canonical),
		}}
	}
	return seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
}
