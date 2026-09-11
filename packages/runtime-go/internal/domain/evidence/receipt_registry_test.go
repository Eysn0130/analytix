package evidence

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestEvidenceReceiptDraftRejectsRawProviderCallIdentityWithoutEcho(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-raw", "turn-raw", "case-raw", "snapshot-raw", 4)
	rawProviderID := "provider_call_6222020202020202020"
	receipt, err := NewEvidenceReceiptDraft(EvidenceReceiptInput{Context: securityContext, ToolCallID: rawProviderID})
	if err == nil || receipt.ReceiptID != "" || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider call identity reached evidence receipt draft: receipt=%#v err=%v", receipt, err)
	}
}

func TestEvidenceReceiptRegistryRequiresExactCurrentMembershipAndSupportsRevocation(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-a", "turn-a", "case-a", "snapshot-a", 4)
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	material := evidenceReceiptTestMaterial(t, "4200000")
	proof := evidenceReceiptTestSettlementProof("receipt-host-a")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	registered, receipt, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RegistrySequence != 1 || receipt.RegistryIntegrityProof == "" {
		t.Fatalf("registered receipt lacks registry proof: %#v", receipt)
	}
	canonical, _ := CanonicalEvidenceBytes(material)
	resolved, err := VerifyEvidenceReceiptMembership(registered, securityContext, receipt.ReceiptID)
	if err != nil || string(resolved.CanonicalEvidence) != string(canonical) || resolved.Revoked {
		t.Fatalf("registered receipt did not resolve exactly: resolved=%#v err=%v", resolved, err)
	}
	outcome := NewToolOutcome(ToolOutcomeInput{
		ToolName: receipt.ToolName, ToolCallID: receipt.ToolCallID, ContextDigest: receipt.ContextDigest,
		ExecutionGrantID: receipt.ExecutionGrantID, CaseID: receipt.CaseID, ContextEpoch: receipt.ContextEpoch,
		DatasetSnapshotID: receipt.DatasetSnapshotID, ServerIdentity: receipt.ServerIdentity,
		TransportStatus: TransportSuccess, SemanticStatus: SemanticSuccess, IssuedAt: evidenceReceiptTestTime(),
	})
	accepted, err := ToolOutcomeWithRegisteredEvidence(outcome, []RegisteredEvidence{resolved})
	if err != nil || !accepted.SafeToAnswer || len(accepted.EvidenceReceiptIDs) != 1 {
		t.Fatalf("registered evidence did not upgrade the host outcome: outcome=%#v err=%v", accepted, err)
	}
	mismatchedIdentity := NewToolOutcome(ToolOutcomeInput{
		ToolName: receipt.ToolName, ToolCallID: receipt.ToolCallID, ContextDigest: receipt.ContextDigest,
		ExecutionGrantID: receipt.ExecutionGrantID, CaseID: receipt.CaseID, ContextEpoch: receipt.ContextEpoch,
		DatasetSnapshotID: receipt.DatasetSnapshotID, ServerIdentity: domainEvidenceTestIdentity(t, "analytix_funds", "spoofed", "1.0.0", 3),
		TransportStatus: TransportSuccess, SemanticStatus: SemanticSuccess, IssuedAt: evidenceReceiptTestTime(),
	})
	if _, err := ToolOutcomeWithRegisteredEvidence(mismatchedIdentity, []RegisteredEvidence{resolved}); err == nil {
		t.Fatal("receipt from another verified server identity upgraded the tool outcome")
	}
	otherTurn := evidenceReceiptTestContext(t, "thread-a", "turn-b", "case-a", "snapshot-a", 4)
	if _, err := VerifyEvidenceReceiptMembership(registered, otherTurn, receipt.ReceiptID); err == nil {
		t.Fatal("receipt from another turn was accepted")
	}
	revoked, err := RevokeEvidenceReceipt(registered, receipt.ReceiptID, "source_retracted", evidenceReceiptTestTime().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyEvidenceReceiptMembership(revoked, securityContext, receipt.ReceiptID); err == nil {
		t.Fatal("revoked receipt remained authoritative")
	}
}

func TestLegacyMCPIdentityCannotRemainFactAuthority(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-legacy", "turn-legacy", "case-legacy", "snapshot-legacy", 4)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "4200000")
	proof := evidenceReceiptTestSettlementProof("receipt-legacy")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	draft.ServerIdentity = "mcpv1:" + encode("analytix_funds") + ":" + encode("analytix-fund-analysis") + ":" + encode("1.0.0") + ":" + domainsecurity.SHA256Hex([]byte("legacy-instance")) + ":3"
	draft.ReceiptDigest = evidenceReceiptDigest(draft)
	if _, _, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime()); err == nil {
		t.Fatal("legacy MCP identity remained live evidence authority")
	}
}

func TestPartialReceiptCannotAuthorizeSuccessfulToolOutcome(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-partial", "turn-partial", "case-partial", "snapshot-partial", 4)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	material := evidenceReceiptTestMaterial(t, "4200000")
	proof := evidenceReceiptTestSettlementProof("receipt-partial")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	draft.PaginationCompleteness = PaginationPartial
	draft.ReceiptDigest = evidenceReceiptDigest(draft)
	registry, receipt, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := VerifyEvidenceReceiptMembership(registry, securityContext, receipt.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	outcome := NewToolOutcome(ToolOutcomeInput{
		ToolName: receipt.ToolName, ToolCallID: receipt.ToolCallID, ContextDigest: receipt.ContextDigest,
		ExecutionGrantID: receipt.ExecutionGrantID, CaseID: receipt.CaseID, ContextEpoch: receipt.ContextEpoch,
		DatasetSnapshotID: receipt.DatasetSnapshotID, ServerIdentity: receipt.ServerIdentity,
		TransportStatus: TransportSuccess, SemanticStatus: SemanticSuccess, IssuedAt: evidenceReceiptTestTime(),
	})
	if _, err := ToolOutcomeWithRegisteredEvidence(outcome, []RegisteredEvidence{resolved}); err == nil {
		t.Fatal("partial receipt authorized a semantic-success tool outcome")
	}
}

func TestEvidenceReceiptRegistryFailsClosedOnFakeMismatchTamperAndSequenceGap(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-a", "turn-a", "case-a", "snapshot-a", 4)
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	if _, err := VerifyEvidenceReceiptMembership(registry, securityContext, "provider-forged-receipt"); err == nil {
		t.Fatal("non-empty provider receipt id became evidence")
	}
	material := evidenceReceiptTestMaterial(t, "4200000")
	proof := evidenceReceiptTestSettlementProof("receipt-host-a")
	draft := evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(proof.SettlementID))
	registry, _, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	tampered := registry
	tampered.Entries = append([]EvidenceReceiptRegistryEntry(nil), registry.Entries...)
	tampered.Entries[0].CanonicalEvidence = evidenceReceiptTestMaterial(t, "9900000")
	if err := ValidateEvidenceReceiptRegistry(tampered); err == nil {
		t.Fatal("tampered canonical evidence passed registry validation")
	}
	gapped := registry
	gapped.Entries = append([]EvidenceReceiptRegistryEntry(nil), registry.Entries...)
	gapped.Entries[0].Sequence = 2
	gapped.Entries[0].EntryDigest = evidenceRegistryEntryDigest(gapped.Entries[0])
	gapped.StateDigest = evidenceRegistryStateDigest(gapped)
	if err := ValidateEvidenceReceiptRegistry(gapped); err == nil {
		t.Fatal("registry sequence gap passed validation")
	}
	wrongSnapshot := evidenceReceiptTestContext(t, "thread-a", "turn-a", "case-a", "snapshot-b", 4)
	if _, err := VerifyEvidenceReceiptMembership(registry, wrongSnapshot, draft.ReceiptID); err == nil {
		t.Fatal("receipt crossed dataset snapshots")
	}
}

func TestEvidenceCanonicalJSONPreservesLargeNumberAndMapOrder(t *testing.T) {
	left, err := CanonicalEvidenceBytes(json.RawMessage(`{"b":2,"a":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	right, err := CanonicalEvidenceBytes(json.RawMessage(`{"a":9007199254740993,"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != `{"a":9007199254740993,"b":2}` || string(left) != string(right) ||
		domainsecurity.CanonicalJSONHash(left) != domainsecurity.CanonicalJSONHash(right) {
		t.Fatalf("canonical evidence was not byte-stable: left=%s right=%s", left, right)
	}
}

func TestCanonicalEvidenceBytesRejectsDuplicateKeysAndInvalidUnicode(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"schemaVersion":1,"schemaVersion":2,"facts":[]}`),
		json.RawMessage(`{"schemaVersion":1,"facts":[],"text":"\uD800"}`),
		json.RawMessage(append([]byte(`{"schemaVersion":1,"facts":[],"text":"`), append([]byte{0xff}, []byte(`"}`)...)...)),
	} {
		if canonical, err := CanonicalEvidenceBytes(raw); err == nil || len(canonical) != 0 {
			t.Fatalf("ambiguous canonical evidence passed: raw=%q canonical=%s err=%v", raw, canonical, err)
		}
	}
}

func TestTransactionDatasetInventorySupportsOnlyExactTableRowCount(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-a", "turn-count", "case-a", "snapshot-count", 5)
	materialBody, err := json.Marshal(CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersion,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-count", ClaimType: ClaimCount,
			NormalizedPayload: NormalizedClaimPayload{
				SubjectID: "dataset:analysis_txn_detail_idx", EntityID: "dataset:analysis_txn_detail_idx",
				Count: "3", Granularity: "dataset_table_rows",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	material, err := CanonicalEvidenceBytes(materialBody)
	if err != nil {
		t.Fatal(err)
	}
	proof := evidenceReceiptTestSettlementProof("receipt-count")
	toolCallID := evidenceTestHostToolCallID(t, "count")
	draft, err := NewEvidenceReceiptDraft(EvidenceReceiptInput{
		ReceiptID: EvidenceSettlementReceiptID(proof.SettlementID), Context: securityContext, ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant-count")), ToolCallID: toolCallID,
		ServerIdentity: domainEvidenceTestIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 7), ServerVersion: "0.16.16", ConnectionEpoch: 7,
		ToolName: "mcp__analytix_funds__count_case_rows", ArgsHash: domainsecurity.SHA256Hex([]byte("args-count")),
		ResultHash: domainsecurity.CanonicalJSONHash(material), SourceType: SourceTypeTransactionDatasetInventory,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, QueryHash: domainsecurity.SHA256Hex([]byte("query-count")),
		QueryRange: EvidenceQueryRange{
			EntityIDs: []string{"dataset:analysis_txn_detail_idx"}, AccountIDs: []string{}, Directions: []string{},
			SourceIDs: []string{"analysis_txn_detail_idx@snapshot-count"}, FiltersHash: domainsecurity.SHA256Hex([]byte("no-filters")),
		},
		Granularity: "dataset_table_rows", Currency: "", Timezone: "Asia/Shanghai", PaginationCompleteness: PaginationComplete,
		SourceRecordIDs: []string{"analysis_txn_detail_idx@snapshot-count"}, RawSHA256: domainsecurity.SHA256Hex([]byte("raw-count")),
		TransformationLineage: []TransformationLineageStep{}, PIIClassification: PIINone, IssuedAt: evidenceReceiptTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := NewEvidenceReceiptRegistry(securityContext)
	registry, receipt, err := RegisterEvidenceReceipt(registry, draft, material, proof, evidenceReceiptTestTime())
	if err != nil {
		t.Fatal(err)
	}
	if !SourceTypeSupportsClaim(receipt.SourceType, ClaimCount) || SourceTypeSupportsClaim(receipt.SourceType, ClaimAmount) {
		t.Fatal("dataset inventory source capability exceeded exact row-count claims")
	}
	if _, err := VerifyEvidenceReceiptMembership(registry, securityContext, receipt.ReceiptID); err != nil {
		t.Fatalf("exact dataset table row-count receipt was not registered: %v", err)
	}
	wrong := draft
	wrong.PaginationCompleteness = PaginationPartial
	wrong.ReceiptDigest = evidenceReceiptDigest(wrong)
	if err := ValidateEvidenceReceiptDraft(wrong); err == nil {
		t.Fatal("partial dataset inventory became an exact table-row receipt")
	}
}

func evidenceReceiptTestDraft(t *testing.T, securityContext domainsecurity.TurnSecurityContext, material json.RawMessage, receiptID string) EvidenceReceipt {
	t.Helper()
	canonical, err := CanonicalEvidenceBytes(material)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := NewEvidenceReceiptDraft(EvidenceReceiptInput{
		ReceiptID: receiptID, Context: securityContext, ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")), ToolCallID: evidenceTestHostToolCallID(t, "receipt-a"),
		ServerIdentity: domainEvidenceTestIdentity(t, "analytix_funds", "analytix-fund-analysis", "1.0.0", 3), ServerVersion: "1.0.0", ConnectionEpoch: 3,
		ToolName: "mcp__analytix_funds__query", ArgsHash: domainsecurity.SHA256Hex([]byte("args")),
		ResultHash: domainsecurity.CanonicalJSONHash(canonical), SourceType: "transactions", DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash: domainsecurity.SHA256Hex([]byte("query")), QueryRange: EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{"00123456789012345678"}, Directions: []string{"out"},
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"bank-flow-a"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("filters")),
		}, Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai", PaginationCompleteness: PaginationComplete,
		SourceRecordIDs: []string{"row-a"}, RawSHA256: domainsecurity.SHA256Hex([]byte("raw")),
		TransformationLineage: []TransformationLineageStep{}, PIIClassification: PIIMasked, IssuedAt: evidenceReceiptTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func evidenceReceiptTestSettlementProof(label string) EvidenceSettlementProof {
	return EvidenceSettlementProof{
		SettlementID:         domainsecurity.SHA256Hex([]byte("settlement:" + label)),
		PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("prepared:" + label)),
	}
}

func evidenceReceiptTestMaterial(t *testing.T, amountMinor string) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersion,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-a", ClaimType: ClaimAmount,
			NormalizedPayload: NormalizedClaimPayload{
				SubjectID: "entity-a", EntityID: "entity-a", AccountID: "00123456789012345678",
				AmountMinor: amountMinor, Currency: "CNY", Direction: "out",
				StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func evidenceReceiptTestContext(t *testing.T, threadID string, turnID string, caseID string, snapshotID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	snapshotID = securitycontexttest.DatasetSnapshotID(snapshotID)
	policyDigest := domainsecurity.SHA256Hex([]byte("domain-evidence-test-policy:\x00" + threadID))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("domain-evidence-binding:\x00" + threadID + "\x00" + turnID)),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, "/workspace", domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	context, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace", CaseID: caseID,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-" + caseID)), DatasetSnapshotID: snapshotID,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: epoch, IssuedAt: evidenceReceiptTestTime(),
		PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
		t.Fatal(err)
	}
	return context
}

func evidenceReceiptTestTime() time.Time {
	return time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
}
