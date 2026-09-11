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

func TestNeedsEvidenceRendererDoesNotClaimOrdinaryResultContainsCaseFacts(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	slot, err := domainordinaryresult.NewResultSlotV1("The ordinary read completed.")
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := (FinalEvidenceGate{}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
		MissingScope: []string{"current_case_facts"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	want := "The ordinary read completed.\n\n本轮未发布任何未经核验的案件事实。若需案件事实，请确认当前资金分析来源和同案证据可用，然后重新发起核验。"
	if err != nil || rendered != want || strings.Contains(rendered, "其中的案件事实") {
		t.Fatalf("ordinary result acquired a misleading case-fact claim:\nwant=%q\ngot =%q\nerr=%v", want, rendered, err)
	}
}

func TestLegacyEmptyResultCannotBecomeZeroOrVerifiedNoHit(t *testing.T) {
	for _, pagination := range []domainevidence.PaginationCompleteness{domainevidence.PaginationPartial, domainevidence.PaginationComplete} {
		issuer, input := noHitEvidenceFixture(t, pagination)
		if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Marker.SettlementID != "" {
			t.Fatalf("legacy %s empty result prepared evidence: prepared=%#v err=%v", pagination, prepared, err)
		}
		envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: TerminalSuccess, CheckedScope: &input.Material.QueryRange,
			MissingScope: []string{"registry_backed_dataset_snapshot_v2"}, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(envelope.EvidenceReceiptIDs) != 0 {
			t.Fatalf("legacy %s empty result escaped boundary-only: envelope=%#v err=%v", pagination, envelope, err)
		}
		rendered, err := RenderFinalAnswer(envelope)
		if err != nil || strings.Contains(rendered, "未在已查范围发现") || strings.Contains(rendered, "金额为零") {
			t.Fatalf("legacy empty result rendered a zero/no-hit upgrade: %q err=%v", rendered, err)
		}
	}
}

func TestLegacyCaseBoundaryCannotPublishNoHitOrPartialFacts(t *testing.T) {
	for _, pagination := range []domainevidence.PaginationCompleteness{domainevidence.PaginationComplete, domainevidence.PaginationPartial} {
		issuer, input := noHitEvidenceFixture(t, pagination)
		if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Marker.SettlementID != "" {
			t.Fatalf("legacy %s no-hit prepared evidence: prepared=%#v err=%v", pagination, prepared, err)
		}
		boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
			Context: input.Context, TerminalReason: TerminalSuccess, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer ||
			len(boundary.Envelope.EvidenceReceiptIDs) != 0 || strings.Contains(boundary.Text, "未在已查范围发现") {
			t.Fatalf("legacy %s no-hit reached case publication: boundary=%#v err=%v", pagination, boundary, err)
		}
	}
}

func TestLegacyPartialCoverageCannotReachCaseBoundary(t *testing.T) {
	issuer, input, _ := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationPartial)
	if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Marker.SettlementID != "" {
		t.Fatalf("legacy partial candidate prepared evidence: prepared=%#v err=%v", prepared, err)
	}
	boundary, err := FinalizeCaseBoundary(context.Background(), issuer.Registry, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSuccess, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(boundary.Envelope.Claims) != 0 ||
		strings.Contains(boundary.Text, "4200000") {
		t.Fatalf("legacy partial coverage reached case publication: boundary=%#v err=%v", boundary, err)
	}
}

func TestLegacyPartialClaimCannotBecomeWholeCaseConclusion(t *testing.T) {
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationPartial)
	proposal.EvidenceIDs = []string{"evr_provider_forged"}
	claim, err := claimVerifierForTest(issuer).Verify(context.Background(), input.Context, proposal)
	if err != nil || claim.SupportState != domainevidence.ClaimUnresolved || len(claim.EvidenceIDs) != 0 {
		t.Fatalf("legacy partial claim acquired support: claim=%#v err=%v", claim, err)
	}
	envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess, Claims: []domainevidence.ClaimRecord{claim},
		MissingScope: []string{"registry_backed_dataset_snapshot_v2"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(envelope.Claims) != 0 {
		t.Fatalf("legacy partial claim escaped boundary-only: envelope=%#v err=%v", envelope, err)
	}
}

func TestLegacyOneAccountCandidateCannotBecomeWholeCaseConclusion(t *testing.T) {
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
	claim := forgedVerifiedClaim(t, proposal, "evr_provider_forged", input.Material.QueryRange)
	envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess, Claims: []domainevidence.ClaimRecord{claim},
		MissingScope: []string{"registry_backed_dataset_snapshot_v2"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(envelope.Claims) != 0 {
		t.Fatalf("legacy one-account candidate escaped boundary-only: envelope=%#v err=%v", envelope, err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil || strings.Contains(rendered, "4200000") || strings.Contains(rendered, "****5678") {
		t.Fatalf("legacy one-account candidate reached renderer: %q err=%v", rendered, err)
	}
}

func TestFinalGateRejectsForgedSupportedScope(t *testing.T) {
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
	broad := input.Material.QueryRange
	broad.EntityIDs = []string{}
	broad.AccountIDs = []string{}
	broad.Directions = []string{}
	broad.StartAt, broad.EndAt = "", ""
	claim := forgedVerifiedClaim(t, proposal, "evr_provider_forged", broad)
	envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess, Claims: []domainevidence.ClaimRecord{claim},
		CheckedScope: &broad, RequestedScope: &broad, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(envelope.Claims) != 0 {
		t.Fatalf("forged broad claim scope reached publication: envelope=%#v err=%v", envelope, err)
	}
}

func TestIssuerRejectsCanonicalFactMetadataMismatchAndScopeEscape(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*IssueEvidenceInput)
	}{
		{name: "currency", mutate: func(input *IssueEvidenceInput) { input.Material.Currency = "USD" }},
		{name: "granularity", mutate: func(input *IssueEvidenceInput) { input.Material.Granularity = "monthly_summary" }},
		{name: "entity_scope", mutate: func(input *IssueEvidenceInput) { input.Material.QueryRange.EntityIDs = []string{"entity-b"} }},
		{name: "account_scope", mutate: func(input *IssueEvidenceInput) { input.Material.QueryRange.AccountIDs = []string{"other-account"} }},
		{name: "direction_scope", mutate: func(input *IssueEvidenceInput) { input.Material.QueryRange.Directions = []string{"in"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			issuer, input, _ := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
			test.mutate(&input)
			if _, err := issuer.Prepare(context.Background(), input); err == nil {
				t.Fatal("issuer accepted a canonical fact outside receipt metadata or scope")
			}
		})
	}
}

func TestRestrictedPIINeverReachesFinalRenderer(t *testing.T) {
	for _, classification := range []domainevidence.PIIClassification{domainevidence.PIIRestricted, domainevidence.PIIControlled} {
		t.Run(string(classification), func(t *testing.T) {
			issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
			input.Material.PIIClassification = classification
			claim := forgedVerifiedClaim(t, proposal, "evr_provider_forged", input.Material.QueryRange)
			envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
				Context: input.Context, TerminalReason: TerminalSuccess, Claims: []domainevidence.ClaimRecord{claim}, IssuedAt: evidenceIssuerTime(),
			})
			if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(envelope.Claims) != 0 {
				t.Fatalf("protected PII reached ordinary final answer: envelope=%#v err=%v", envelope, err)
			}
		})
	}
}

func forgedVerifiedClaim(t *testing.T, proposal domainevidence.ClaimProposal, receiptID string, scope domainevidence.EvidenceQueryRange) domainevidence.ClaimRecord {
	t.Helper()
	claimID := "claim-forged"
	verifierID := domainevidence.VerifierReceiptDigest(
		claimID, proposal.ClaimType, proposal.NormalizedPayload, []string{receiptID}, nil, domainevidence.ClaimVerified,
	)
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: claimID, Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{receiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &scope,
		AllowedWording:     []string{"exact_verified_fact"},
		ProhibitedUpgrades: []string{"legal_characterization_without_review", "zero_or_nonexistence_upgrade"},
		VerifierReceiptID:  verifierID, VerificationReason: "forged_public_hash", VerifiedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return claim
}

func TestFinalAnswerEnvelopeRejectsUnknownVariantAndProperty(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess, GeneralGuidance: []string{"explain_evidence_requirements"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := domainevidence.FinalAnswerEnvelopeRecord(envelope)
	record["unexpected"] = true
	body, _ := json.Marshal(record)
	if _, err := domainevidence.ParseFinalAnswerEnvelope(body); err == nil {
		t.Fatal("unknown final-answer property was accepted")
	}
	record = domainevidence.FinalAnswerEnvelopeRecord(envelope)
	record["variant"] = "FreeTextAnswer"
	body, _ = json.Marshal(record)
	if _, err := domainevidence.ParseFinalAnswerEnvelope(body); err == nil {
		t.Fatal("free-text final-answer variant was accepted")
	}
}

func TestFinalAnswerDeterministicRendererGolden(t *testing.T) {
	// These are closed envelope-shape fixtures for the deterministic renderer;
	// they do not pass through Issuer/FinalEvidenceGate and do not prove legacy
	// snapshot issuance or publication authority.
	_, verifiedInput, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
	receiptID := "evr_renderer_shape_fixture"
	verifiedClaim := forgedVerifiedClaim(t, proposal, receiptID, verifiedInput.Material.QueryRange)
	verifiedEnvelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.EvidenceBackedAnswer, Context: verifiedInput.Context, TerminalReason: string(TerminalSuccess),
		Claims: []domainevidence.ClaimRecord{verifiedClaim}, EvidenceReceiptIDs: []string{receiptID},
		CheckedScope: &verifiedInput.Material.QueryRange, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	partialClaimID := "claim-renderer-partial"
	partialClaim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: partialClaimID, Proposal: proposal, SupportState: domainevidence.ClaimPartial,
		EvidenceIDs: []string{receiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &verifiedInput.Material.QueryRange,
		AllowedWording: []string{"within_checked_scope_only"}, ProhibitedUpgrades: []string{"whole_case_conclusion"},
		VerifierReceiptID: domainevidence.VerifierReceiptDigest(
			partialClaimID, proposal.ClaimType, proposal.NormalizedPayload, []string{receiptID}, nil, domainevidence.ClaimPartial,
		), VerificationReason: "renderer_shape_fixture", VerifiedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	partialEnvelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.PartialEvidenceAnswer, Context: verifiedInput.Context, TerminalReason: string(TerminalSuccess),
		Claims: []domainevidence.ClaimRecord{partialClaim}, EvidenceReceiptIDs: []string{receiptID},
		CheckedScope: &verifiedInput.Material.QueryRange, MissingScope: []string{"remaining_months"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	noHitEnvelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.VerifiedNoHitAnswer, Context: verifiedInput.Context, TerminalReason: string(TerminalSuccess),
		EvidenceReceiptIDs: []string{receiptID}, CheckedScope: &verifiedInput.Material.QueryRange,
		NoHitWording: domainevidence.VerifiedNoHitWording, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}

	boundaryGate := FinalEvidenceGate{}
	sourceEnvelope, err := boundaryGate.Finalize(context.Background(), FinalGateInput{
		Context: verifiedInput.Context, TerminalReason: TerminalSourceUnavailable, SourceUnavailable: true,
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	needsEnvelope, err := boundaryGate.Finalize(context.Background(), FinalGateInput{
		Context: verifiedInput.Context, TerminalReason: TerminalProviderFailure, MissingScope: []string{"current_case_facts"},
		AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	guidanceEnvelope, err := boundaryGate.Finalize(context.Background(), FinalGateInput{
		Context: verifiedInput.Context, TerminalReason: TerminalSuccess, GeneralGuidance: []string{"explain_evidence_requirements"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		envelope domainevidence.FinalAnswerEnvelope
		want     string
	}{
		{"verified", verifiedEnvelope, "已核验证据支持以下案件事实：\n- 主体 entity-a 的金额（最小货币单位）为 4200000 CNY。（限定范围：实体 entity-a；账户 ****5678；方向 out；时间 2026-01-01T00:00:00Z 至 2026-01-31T23:59:59Z；粒度 transaction；已核验来源 1 项）"},
		{"partial", partialEnvelope, "仅在已查范围内可确认以下内容，不得外推为全案结论：\n- 主体 entity-a 的金额（最小货币单位）为 4200000 CNY。（限定范围：实体 entity-a；账户 ****5678；方向 out；时间 2026-01-01T00:00:00Z 至 2026-01-31T23:59:59Z；粒度 transaction；已核验来源 1 项）\n未覆盖范围：remaining_months"},
		{"no hit", noHitEnvelope, "未在已查范围发现匹配记录；这不等同于范围外不存在、金额为零、无关联或无异常。"},
		{"source unavailable", sourceEnvelope, domainevidence.CaseSourceUnavailableText},
		{"needs evidence", needsEnvelope, domainevidence.CaseUnverifiedText},
		{"guidance", guidanceEnvelope, "案件事实必须由同案、同轮、同数据快照的宿主证据回执支持。"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := RenderFinalAnswer(test.envelope)
			if err != nil || got != test.want {
				t.Fatalf("renderer golden mismatch:\nwant=%q\ngot =%q\nerr=%v", test.want, got, err)
			}
		})
	}
}

func TestFinalEvidenceGateAcceptsClosedTerminalReasonTable(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	seen := map[TerminalReason]bool{}
	for _, reason := range AllTerminalReasons() {
		if seen[reason] {
			t.Fatalf("duplicate terminal reason %q", reason)
		}
		seen[reason] = true
		envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: reason, SourceUnavailable: true, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.TerminalReason != string(reason) || envelope.Variant != domainevidence.SourceUnavailableAnswer {
			t.Fatalf("terminal reason %q did not route through final gate: envelope=%#v err=%v", reason, envelope, err)
		}
	}
}

func TestFailureTerminalReasonsCannotPublishVerifiedClaims(t *testing.T) {
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
	claim := forgedVerifiedClaim(t, proposal, "evr_failure_terminal_fixture", input.Material.QueryRange)
	for _, reason := range []TerminalReason{TerminalSemanticFailure, TerminalProviderFailure, TerminalCancel, TerminalTimeout, TerminalStreamAbort, TerminalRecovery, TerminalRestart, TerminalReportFallback, TerminalStepLimit, TerminalToolFailure, TerminalApprovalDenied, TerminalInputCancelled} {
		envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
			Context: input.Context, TerminalReason: reason, Claims: []domainevidence.ClaimRecord{claim}, IssuedAt: evidenceIssuerTime(),
		})
		if err != nil || envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(envelope.Claims) != 0 || len(envelope.EvidenceReceiptIDs) != 0 {
			t.Fatalf("failure terminal %q published evidence: envelope=%#v err=%v", reason, envelope, err)
		}
	}
}

func noHitEvidenceFixture(t *testing.T, pagination domainevidence.PaginationCompleteness) (Issuer, IssueEvidenceInput) {
	t.Helper()
	issuer, input := evidenceIssuerFixture(t)
	material := domainevidence.CanonicalEvidenceMaterial{SchemaVersion: domainevidence.CanonicalEvidenceVersion, Facts: []domainevidence.CanonicalEvidenceFact{}}
	canonical, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	input.Material.CanonicalEvidence = canonical
	input.Material.PaginationCompleteness = pagination
	alignEvidenceOutcomeWithPagination(&input, pagination)
	input.Material.SourceRecordIDs = []string{}
	input.Material.TransformationLineage[0].OutputHash = domainsecurity.CanonicalJSONHash(canonical)
	return issuer, input
}
