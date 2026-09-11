package evidence

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRendererV2ProjectsInternalCaseEntityReferenceAsTypedSlot(t *testing.T) {
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	envelope := rendererCountEnvelope(t, string(reference))

	rendered, err := RenderFinalAnswer(envelope)
	if err != nil || !strings.Contains(rendered, "〔账户槽位 1〕") ||
		domaincaseentity.ContainsReferenceV1(rendered) {
		t.Fatalf("internal reference was not projected as a typed slot: rendered=%q err=%v", rendered, err)
	}
	slots, err := BuildAcceptedEntitySlotBindingsV1(envelope)
	if err != nil || len(slots) != 1 || slots[0].SlotID != "account-slot-1" ||
		len(slots[0].ClaimIDs) != 1 || len(slots[0].ReceiptIDs) != 1 {
		t.Fatalf("typed slot bindings = %#v err=%v", slots, err)
	}
	var resolved domaincaseentity.ReferenceV1
	if err := slots[0].UseReferenceV1(func(value domaincaseentity.ReferenceV1) error {
		resolved = value
		return nil
	}); err != nil || resolved != reference {
		t.Fatalf("typed slot reference callback failed: resolved=%q err=%v", resolved, err)
	}
	if _, err := json.Marshal(slots[0]); err == nil || !strings.Contains(slots[0].String(), "REDACTED") {
		t.Fatal("host-private typed slot gained an ordinary serialization path")
	}
}

func TestRendererV1RejectsInternalCaseEntityReferenceBeforePublicRendering(t *testing.T) {
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	envelope := rendererCountEnvelope(t, string(reference))

	rendered, err := RenderFinalAnswerAtVersion(envelope, HistoricalFinalAnswerRendererVersion)
	if rendered != "" || !errors.Is(err, ErrInternalEntityReferenceRendering) ||
		!errors.Is(err, domaincaseentity.ErrPublicValueInternalReferenceV1) {
		t.Fatalf("historical renderer accepted internal reference: rendered=%q err=%v", rendered, err)
	}
}

func TestTypedEntitySlotLabelIsNotIdentifierMasked(t *testing.T) {
	const slot = "〔账户槽位 12〕"
	if got := renderMaskedIdentifier(slot); got != slot {
		t.Fatalf("typed entity slot was mistaken for a source identifier: got=%q", got)
	}
}

func TestPrivacyGuidanceNamesTypedLocalDisplayWithoutControlledArtifactGate(t *testing.T) {
	text, ok := generalGuidanceText("explain_privacy_controls")
	if !ok || !strings.Contains(text, "当前本机的类型化显示") || strings.Contains(text, "受控成果物") {
		t.Fatalf("privacy guidance retained the legacy controlled-viewer gate: %q", text)
	}
}

func TestRendererAllowsValidatedNaturalDisplayLabel(t *testing.T) {
	label, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 3,
		SafeSuffix:    "1234",
		Institution:   "中国银行",
		AccountType:   "储蓄账户",
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := rendererCountEnvelope(t, label.Text)

	rendered, err := RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, label.Text) || domaincaseentity.ContainsReferenceV1(rendered) {
		t.Fatalf("natural display label did not cross renderer safely: %q", rendered)
	}
}

func TestRendererEscapesUntrustedClaimTextAndHidesAttributeValue(t *testing.T) {
	context := evidenceReceiptTestContext(t, "thread-a", "turn-a", "case-a", "snapshot-a", 2)
	payload := NormalizedClaimPayload{
		SubjectID: "entity-a\n- forged bullet account 6222020000000000000", EntityID: "entity-a\n- forged bullet account 6222020000000000000",
		AttributeName: "document_author", AttributeValue: "已构成串通投标并具备立案条件", Granularity: "record",
	}
	proposal := ClaimProposal{
		SchemaVersion: ClaimProposalVersion, ProposalID: "proposal-a", ClaimType: ClaimBidEditMetadata,
		NormalizedPayload: payload, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	scope := EvidenceQueryRange{
		EntityIDs: []string{payload.EntityID}, AccountIDs: []string{}, Directions: []string{}, SourceIDs: []string{"bid-a"},
		FiltersHash: domainsecurity.SHA256Hex([]byte("filters")),
	}
	claimID := "claim-a"
	verifierID := VerifierReceiptDigest(claimID, proposal.ClaimType, payload, []string{"receipt-a"}, nil, ClaimVerified)
	claim, err := NewClaimRecord(ClaimRecordInput{
		ClaimID: claimID, Proposal: proposal, SupportState: ClaimVerified, EvidenceIDs: []string{"receipt-a"},
		CounterEvidenceIDs: []string{}, SupportedScope: &scope, AllowedWording: []string{"exact_verified_fact"},
		ProhibitedUpgrades: []string{"legal_characterization_without_review"}, VerifierReceiptID: verifierID,
		VerificationReason: "test", VerifiedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: EvidenceBackedAnswer, Context: context, TerminalReason: "success", Claims: []ClaimRecord{claim},
		EvidenceReceiptIDs: []string{"receipt-a"}, CheckedScope: &scope, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, payload.AttributeValue) || strings.Contains(rendered, "6222020000000000000") ||
		strings.Count(rendered, "\n- ") != 1 || strings.Contains(rendered, "\n- forged bullet") {
		t.Fatalf("renderer exposed source text as fact or markup: %q", rendered)
	}
	if !strings.Contains(rendered, `entity-a\\n- forged bullet account \[ACCOUNT\]`) || !strings.Contains(rendered, "字段值仅保留于受控证据") {
		t.Fatalf("renderer did not use safe deterministic projection: %q", rendered)
	}
}

func TestRendererDerivesSignedNegativeNetOnlyFromSameScopeAggregateClaims(t *testing.T) {
	context := evidenceReceiptTestContext(t, "thread-renderer-flow", "turn-renderer-flow", "case-renderer-flow", "snapshot-renderer-flow", 2)
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	scope := rendererAccountFlowScope(string(reference), "renderer-flow-filter")
	claims := rendererAccountFlowClaims(t, string(reference), scope, scope)
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: EvidenceBackedAnswer, Context: context, TerminalReason: "success", Claims: claims,
		EvidenceReceiptIDs: []string{"receipt-renderer-flow"}, CheckedScope: &scope, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil || !strings.Contains(rendered, "流入 100 CNY") || !strings.Contains(rendered, "流出 300 CNY") ||
		!strings.Contains(rendered, "有符号净额 -200 CNY") || !strings.Contains(rendered, "交易笔数 2") ||
		!strings.Contains(rendered, "〔账户槽位 1〕") || strings.Contains(rendered, string(reference)) {
		t.Fatalf("same-scope aggregate claims did not render a signed net summary: rendered=%q err=%v", rendered, err)
	}
}

func TestRendererDoesNotDeriveNetAcrossMismatchedAggregateScopes(t *testing.T) {
	context := evidenceReceiptTestContext(t, "thread-renderer-mismatch", "turn-renderer-mismatch", "case-renderer-mismatch", "snapshot-renderer-mismatch", 2)
	const subject = "entity-renderer-mismatch"
	scope := rendererAccountFlowScope(subject, "renderer-mismatch-filter")
	mismatchedCountScope := rendererAccountFlowScope(subject, "renderer-mismatch-other-filter")
	claims := rendererAccountFlowClaims(t, subject, scope, mismatchedCountScope)
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: EvidenceBackedAnswer, Context: context, TerminalReason: "success", Claims: claims,
		EvidenceReceiptIDs: []string{"receipt-renderer-flow"}, CheckedScope: &scope, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil || strings.Contains(rendered, "有符号净额") || strings.Contains(rendered, "资金汇总") {
		t.Fatalf("renderer derived net across mismatched scopes: rendered=%q err=%v", rendered, err)
	}
}

func rendererAccountFlowScope(subject, filter string) EvidenceQueryRange {
	return EvidenceQueryRange{
		EntityIDs: []string{subject}, AccountIDs: []string{subject}, Directions: []string{"in", "out"},
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"renderer-flow-source"},
		FiltersHash: domainsecurity.SHA256Hex([]byte(filter)),
	}
}

func rendererAccountFlowClaims(t *testing.T, subject string, amountScope, countScope EvidenceQueryRange) []ClaimRecord {
	t.Helper()
	amount := func(id, direction, value string) ClaimRecord {
		payload := NormalizedClaimPayload{
			SubjectID: subject, EntityID: subject, AccountID: subject, AmountMinor: value, Currency: "CNY", Direction: direction,
			StartAt: amountScope.StartAt, EndAt: amountScope.EndAt, Granularity: accountFlowAggregateGranularity,
		}
		proposal := ClaimProposal{SchemaVersion: ClaimProposalVersion, ProposalID: id, ClaimType: ClaimAmount, NormalizedPayload: payload}
		verifierReceiptID := VerifierReceiptDigest(id, ClaimAmount, payload, []string{"receipt-renderer-flow"}, nil, ClaimVerified)
		claim, err := NewClaimRecord(ClaimRecordInput{
			ClaimID: id, Proposal: proposal, SupportState: ClaimVerified, EvidenceIDs: []string{"receipt-renderer-flow"},
			SupportedScope: &amountScope, AllowedWording: []string{"exact_verified_fact"},
			ProhibitedUpgrades: []string{"whole_case_conclusion"}, VerifierReceiptID: verifierReceiptID,
			VerificationReason: "test", VerifiedAt: time.Unix(2, 0),
		})
		if err != nil {
			t.Fatal(err)
		}
		return claim
	}
	countPayload := NormalizedClaimPayload{
		SubjectID: subject, EntityID: subject, Count: "2", StartAt: countScope.StartAt, EndAt: countScope.EndAt,
		Granularity: accountFlowAggregateGranularity,
	}
	countProposal := ClaimProposal{SchemaVersion: ClaimProposalVersion, ProposalID: "claim-renderer-flow-count", ClaimType: ClaimCount, NormalizedPayload: countPayload}
	countVerifierReceiptID := VerifierReceiptDigest("claim-renderer-flow-count", ClaimCount, countPayload, []string{"receipt-renderer-flow"}, nil, ClaimVerified)
	count, err := NewClaimRecord(ClaimRecordInput{
		ClaimID: "claim-renderer-flow-count", Proposal: countProposal, SupportState: ClaimVerified,
		EvidenceIDs: []string{"receipt-renderer-flow"}, SupportedScope: &countScope, AllowedWording: []string{"exact_verified_fact"},
		ProhibitedUpgrades: []string{"whole_case_conclusion"}, VerifierReceiptID: countVerifierReceiptID,
		VerificationReason: "test", VerifiedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return []ClaimRecord{amount("claim-renderer-flow-in", "in", "100"), amount("claim-renderer-flow-out", "out", "300"), count}
}

func rendererCountEnvelope(t *testing.T, subject string) FinalAnswerEnvelope {
	t.Helper()
	context := evidenceReceiptTestContext(t, "thread-renderer", "turn-renderer", "case-renderer", "snapshot-renderer", 2)
	payload := NormalizedClaimPayload{
		SubjectID:   subject,
		EntityID:    subject,
		Count:       "7",
		StartAt:     "2026-07-01T00:00:00Z",
		EndAt:       "2026-07-02T00:00:00Z",
		Granularity: "transaction",
	}
	proposal := ClaimProposal{
		SchemaVersion:      ClaimProposalVersion,
		ProposalID:         "proposal-renderer-count",
		ClaimType:          ClaimCount,
		NormalizedPayload:  payload,
		EvidenceIDs:        []string{},
		CounterEvidenceIDs: []string{},
	}
	scope := EvidenceQueryRange{
		EntityIDs:   []string{subject},
		AccountIDs:  []string{},
		Directions:  []string{},
		SourceIDs:   []string{"transaction-source"},
		StartAt:     payload.StartAt,
		EndAt:       payload.EndAt,
		FiltersHash: domainsecurity.SHA256Hex([]byte("renderer-count-filter")),
	}
	claimID := "claim-renderer-count"
	verifierID := VerifierReceiptDigest(
		claimID,
		proposal.ClaimType,
		payload,
		[]string{"receipt-renderer-count"},
		nil,
		ClaimVerified,
	)
	claim, err := NewClaimRecord(ClaimRecordInput{
		ClaimID:            claimID,
		Proposal:           proposal,
		SupportState:       ClaimVerified,
		EvidenceIDs:        []string{"receipt-renderer-count"},
		CounterEvidenceIDs: []string{},
		SupportedScope:     &scope,
		AllowedWording:     []string{"exact_verified_fact"},
		ProhibitedUpgrades: []string{"whole_case_conclusion"},
		VerifierReceiptID:  verifierID,
		VerificationReason: "test",
		VerifiedAt:         time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant:            EvidenceBackedAnswer,
		Context:            context,
		TerminalReason:     "success",
		Claims:             []ClaimRecord{claim},
		EvidenceReceiptIDs: []string{"receipt-renderer-count"},
		CheckedScope:       &scope,
		IssuedAt:           time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}
