package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestConcreteClaimRequiresEvidenceReceipt(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	registry := &v1RejectionRegistrySpy{}
	settlements := &v1RejectionSettlementStoreSpy{}
	issuer := Issuer{
		Registry: registry, SettlementStore: settlements, Authority: newMemoryFinalAuthority(40), Now: evidenceIssuerTime,
	}
	prepared, err := issuer.Prepare(context.Background(), input)
	if err == nil || prepared.Record.ReceiptID != "" || prepared.Marker.ReceiptID != "" {
		t.Fatalf("legacy snapshot minted a concrete evidence receipt: prepared=%#v err=%v", prepared, err)
	}
	if registry.calls != 0 || settlements.calls != 0 {
		t.Fatalf("legacy snapshot crossed evidence stores: registry=%d settlements=%d", registry.calls, settlements.calls)
	}
	if len(input.Outcome.CandidateEvidenceReceipts) != 1 || input.Outcome.CandidateEvidenceReceipts[0]["receiptId"] != "provider-forged-receipt" {
		t.Fatalf("forged candidate control fixture drifted: %#v", input.Outcome.CandidateEvidenceReceipts)
	}
}

func TestLegacySnapshotCannotPrepareEvidenceReceipt(t *testing.T) {
	issuer, input := legacyEvidenceIssuerFixture(t)
	if !domainsecurity.IsDatasetSnapshotIDV1(input.Context.DatasetSnapshotID) {
		t.Fatalf("legacy quarantine fixture drifted: %q", input.Context.DatasetSnapshotID)
	}
	prepared, err := issuer.Prepare(context.Background(), input)
	if err == nil || prepared.Marker.SettlementID != "" {
		t.Fatalf("legacy snapshot prepared an evidence receipt: prepared=%#v err=%v", prepared, err)
	}
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	if registry.initialized && registry.registry.Sequence != 0 {
		t.Fatalf("legacy snapshot mutated evidence registry: %#v", registry.registry)
	}
}

func TestLegacyPreparedSettlementCannotCommitAfterRestart(t *testing.T) {
	issuer, input, _, marker := legacyPreparedSettlementFixture(t)
	authority := durableEvidenceSettlementAuthority(t, input, marker, issuer.Now().Add(time.Minute))
	receipt, err := issuer.Commit(context.Background(), CommitEvidenceInput{Context: input.Context, Marker: marker, Authority: authority})
	if err == nil || receipt.ReceiptID != "" || !strings.Contains(err.Error(), "commit authority is unavailable") {
		t.Fatalf("restart committed a legacy prepared settlement: receipt=%#v err=%v", receipt, err)
	}
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	if registry.initialized && registry.registry.Sequence != 0 {
		t.Fatalf("legacy restart commit mutated evidence registry: %#v", registry.registry)
	}
}

func TestRestartRevalidatesSourceFieldAgainstSignedRawResult(t *testing.T) {
	issuer, input := sourceBoundV2IssueFixture(t, "entity-a", "entity-b")
	input.Material.QueryRange.EntityIDs = []string{"entity-a", "entity-b"}
	record := prepareSignedSettlementWithoutIssuerSourceChecks(t, issuer, input)
	if err := domainevidence.ValidatePreparedEvidenceSettlementForExecution(record); err == nil ||
		!strings.Contains(err.Error(), "source field authority is not reproducible") {
		t.Fatalf("restart did not revalidate the signed raw source field: %v", err)
	}
}

func TestPreviouslySignedWeakV2SettlementIsAuditOnlyAfterVerifierUpgrade(t *testing.T) {
	issuer, input := sourceBoundV2IssueFixture(t, "entity-a", "entity-b")
	input.Material.QueryRange.EntityIDs = []string{"entity-a", "entity-b"}
	record := prepareSignedSettlementWithoutIssuerSourceChecks(t, issuer, input)
	if err := domainevidence.ValidatePreparedEvidenceSettlement(record); err != nil {
		t.Fatalf("historical signed record lost audit readability: %v", err)
	}
	if err := domainevidence.ValidatePreparedEvidenceSettlementForExecution(record); err == nil ||
		!strings.Contains(err.Error(), "source field authority is not reproducible") {
		t.Fatalf("historical V2 record with a mismatched source entity regained restart execution authority: %v", err)
	}
}

func sourceBoundV2IssueFixture(t *testing.T, rawEntityID, claimedEntityID string) (Issuer, IssueEvidenceInput) {
	t.Helper()
	issuer, input := evidenceIssuerFixture(t)
	const (
		exactAccount     = "0012-3456789012345678"
		canonicalAccount = "00123456789012345678"
	)
	recordBody, err := json.Marshal(map[string]any{
		"sourceRecordId": "row-a", "entityId": rawEntityID, "accountId": exactAccount,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"structuredContent": map[string]any{"rows": []json.RawMessage{recordBody}}})
	if err != nil {
		t.Fatal(err)
	}
	rawHash := domainsecurity.SHA256Hex(raw)
	binding, err := domainevidence.NewSourceFieldBindingV2(domainevidence.SourceFieldBindingInputV2{
		FactID: "fact-account", ClaimType: domainevidence.ClaimAccount,
		CanonicalEntityID: claimedEntityID, CanonicalAccountID: canonicalAccount,
		SourceRecordID: "row-a", RawArtifactSHA256: rawHash, SourceRecordSHA256: domainsecurity.CanonicalJSONHash(recordBody),
		SourceRecordPath: "/structuredContent/rows/0", SourceRecordIDPath: "/structuredContent/rows/0/sourceRecordId",
		SourceEntityIDPath: "/structuredContent/rows/0/entityId", SourceFieldPath: "/structuredContent/rows/0/accountId",
		SourceScalarKind: domainevidence.SourceFieldBindingScalarTextV2, SourceExactValue: exactAccount,
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := domainevidence.CanonicalSourceFieldBindingsV2([]domainevidence.SourceFieldBindingV2{binding})
	if err != nil {
		t.Fatal(err)
	}
	bindingDigest, err := domainevidence.SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	canonicalBody, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersionV2, Purpose: domainevidence.CanonicalEvidencePurposeV2,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-account", ClaimType: domainevidence.ClaimAccount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{SubjectID: claimedEntityID, AccountID: canonicalAccount},
		}},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: bindingDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(canonicalBody)
	if err != nil {
		t.Fatal(err)
	}
	input.RawResult = domainmcp.LosslessToolResult{RawResult: raw, RawSHA256: rawHash}
	input.Material.CanonicalEvidence = canonical
	input.Material.PIIClassification = domainevidence.PIIControlled
	input.Material.QueryRange.EntityIDs = []string{claimedEntityID}
	input.Material.QueryRange.AccountIDs = []string{canonicalAccount}
	input.Material.SourceRecordIDs = []string{"row-a"}
	input.Material.TransformationLineage = []domainevidence.TransformationLineageStep{{
		StepID: "normalize-source-bound-v2", Transformer: "host-source-bound-test-normalizer", TransformerVersion: "2",
		InputHash: rawHash, OutputHash: domainsecurity.CanonicalJSONHash(canonical),
	}}
	return issuer, input
}

func prepareSignedSettlementWithoutIssuerSourceChecks(t *testing.T, issuer Issuer, input IssueEvidenceInput) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	preparedAt := issuer.Now()
	canonical, err := domainevidence.CanonicalEvidenceBytes(input.Material.CanonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}
	draftInput := evidenceReceiptDraftInput(input, canonical, "pending", preparedAt)
	provisional, err := domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	preparedInput := domainevidence.PreparedEvidenceSettlementInput{
		Context: input.Context, Grant: input.Grant, ActiveGrantRegistrySequence: input.GrantRegistry.Sequence,
		ActiveGrantRegistryDigest: input.GrantRegistry.StateDigest, SourceProbe: input.SourceProbe, ToolOutcome: input.Outcome,
		RawResult: input.RawResult.RawResult, CanonicalEvidence: canonical, ReceiptDraft: provisional,
		QueryHash: input.Material.QueryHash, ResultItemID: input.ResultItemID, PreparedAt: preparedAt,
		AuthorityKeyID: issuer.Authority.KeyID(), AuthorityPublicKey: issuer.Authority.PublicKey(),
	}
	settlementID := domainevidence.ComputeEvidenceSettlementID(preparedInput)
	draftInput.ReceiptID = domainevidence.EvidenceSettlementReceiptID(settlementID)
	preparedInput.ReceiptDraft, err = domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainevidence.NewPreparedEvidenceSettlement(preparedInput, func(message []byte) ([]byte, error) {
		return issuer.Authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// legacyPreparedSettlementFixture reconstructs an already-durable historical
// DSV1 record for restart quarantine tests. It bypasses no production API and
// cannot prove current issuance, settlement, or publication authority.
func legacyPreparedSettlementFixture(t *testing.T) (Issuer, IssueEvidenceInput, domainevidence.PreparedEvidenceSettlement, domainevidence.HostEvidenceSettlementMarker) {
	t.Helper()
	fixture := v1EvidenceExecutionFixture(t)
	authority := newMemoryFinalAuthority(46)
	issuer := Issuer{
		Registry: &memoryEvidenceRegistry{}, SettlementStore: &memoryEvidenceSettlementStore{},
		Authority: authority, Now: evidenceIssuerTime,
	}
	legacyPrepared := v1RejectionPreparedSettlement(t, fixture, authority)
	if err := issuer.SettlementStore.PutPreparedIfAbsent(context.Background(), legacyPrepared); err != nil {
		t.Fatal(err)
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(legacyPrepared)
	if err != nil {
		t.Fatal(err)
	}
	return issuer, fixture.issue, legacyPrepared, marker
}

func TestEvidenceIssuerRejectsSettledExpiredAndMismatchedAuthority(t *testing.T) {
	_, base := evidenceIssuerFixture(t)
	cases := map[string]func(*IssueEvidenceInput){
		"settled grant": func(input *IssueEvidenceInput) {
			input.GrantRegistry, _ = domainsecurity.SettleRegisteredExecutionGrant(input.GrantRegistry, input.Grant.GrantID, evidenceIssuerTime().Add(time.Minute))
		},
		"expired grant": func(input *IssueEvidenceInput) { input.IssuedAt = evidenceIssuerTime().Add(20 * time.Minute) },
		"wrong turn probe": func(input *IssueEvidenceInput) {
			input.SourceProbe.TurnID = "turn-other"
		},
		"semantic failure": func(input *IssueEvidenceInput) {
			input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
				ToolName: input.Grant.ToolName, ToolCallID: input.Grant.ToolCallID, ContextDigest: input.Context.ContextDigest,
				ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
				DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
				TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticFailure, IsError: true, IssuedAt: evidenceIssuerTime(),
			})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			store := &memoryEvidenceRegistry{}
			input := base
			input.GrantRegistry, _ = domainsecurity.ParseExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistryRecord(base.GrantRegistry))
			mutate(&input)
			issuer := Issuer{
				Registry: store, SettlementStore: &memoryEvidenceSettlementStore{}, Authority: newMemoryFinalAuthority(43),
				Now: func() time.Time { return input.IssuedAt },
			}
			if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Record.ReceiptID != "" {
				t.Fatalf("invalid authority prepared evidence=%#v err=%v", prepared, err)
			}
		})
	}
}

func TestEvidenceIssuerReturnsNoReceiptWhenPrivateRegistryAppendFails(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	settlements := &v1RejectionSettlementStoreSpy{}
	issuer := Issuer{
		Registry: failingEvidenceRegistry{}, SettlementStore: settlements, Authority: newMemoryFinalAuthority(44), Now: evidenceIssuerTime,
	}
	prepared, err := issuer.Prepare(context.Background(), input)
	if err == nil || prepared.Record.ReceiptID != "" || prepared.Marker.ReceiptID != "" {
		t.Fatalf("legacy snapshot reached private registry append setup: prepared=%#v err=%v", prepared, err)
	}
	if settlements.calls != 0 {
		t.Fatalf("legacy snapshot wrote a prepared settlement before registry append: calls=%d", settlements.calls)
	}
}

func TestEvidenceIssuerUsesTrustedClockInsteadOfBackdatedInput(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	input.IssuedAt = evidenceIssuerTime().Add(-time.Hour)
	issuer := Issuer{
		Registry: &memoryEvidenceRegistry{}, SettlementStore: &memoryEvidenceSettlementStore{}, Authority: newMemoryFinalAuthority(45),
		Now: func() time.Time { return evidenceIssuerTime().Add(20 * time.Minute) },
	}
	if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Record.ReceiptID != "" {
		t.Fatalf("backdated caller time bypassed grant expiry: prepared=%#v err=%v", prepared, err)
	}
}

func TestEvidenceIssuerRejectsAmbiguousToolIdentity(t *testing.T) {
	for _, name := range []string{"mcp__docs__lookup", "mcp__foo__bar__lookup", "mcp____lookup"} {
		got := mcpServerID(name)
		if name == "mcp__docs__lookup" && got != "docs" {
			t.Fatalf("valid evidence tool identity failed: %q", got)
		}
		if name == "mcp__foo__bar__lookup" && got != "foo" {
			t.Fatalf("valid evidence tool identity with repeated underscores failed: %q", got)
		}
		if name == "mcp____lookup" && got != "" {
			t.Fatalf("ambiguous evidence tool identity was parsed: %q => %q", name, got)
		}
	}
}

func TestEvidenceIssuerRejectsNonStrictRawAndCanonicalMaterial(t *testing.T) {
	for name, mutate := range map[string]func(*IssueEvidenceInput){
		"duplicate raw": func(input *IssueEvidenceInput) {
			raw := json.RawMessage(`{"structuredContent":{"data":{}},"structuredContent":{"data":{"account":"00123456789012345678"}}}`)
			input.RawResult = domainmcp.LosslessToolResult{RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw)}
			input.Material.TransformationLineage[0].InputHash = input.RawResult.RawSHA256
		},
		"duplicate canonical": func(input *IssueEvidenceInput) {
			canonical := json.RawMessage(`{"schemaVersion":1,"schemaVersion":1,"facts":[]}`)
			input.Material.CanonicalEvidence = canonical
			input.Material.SourceRecordIDs = []string{}
			input.Material.TransformationLineage[0].OutputHash = domainsecurity.CanonicalJSONHash(canonical)
		},
	} {
		t.Run(name, func(t *testing.T) {
			issuer, input := evidenceIssuerFixture(t)
			mutate(&input)
			if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Record.ReceiptID != "" {
				t.Fatalf("non-strict evidence prepared authority: prepared=%#v err=%v", prepared, err)
			}
		})
	}
}

func evidenceIssuerFixture(t *testing.T) (Issuer, IssueEvidenceInput) {
	t.Helper()
	now := evidenceIssuerTime()
	securityContext := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("issuer-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: now,
	})
	return evidenceIssuerFixtureForContext(t, now, securityContext)
}

func legacyEvidenceIssuerFixture(t *testing.T) (Issuer, IssueEvidenceInput) {
	t.Helper()
	fixture := v1EvidenceExecutionFixture(t)
	return Issuer{
		Registry: &memoryEvidenceRegistry{}, SettlementStore: &memoryEvidenceSettlementStore{},
		Authority: newMemoryFinalAuthority(46), Now: evidenceIssuerTime,
	}, fixture.issue
}

func evidenceIssuerFixtureForContext(t *testing.T, now time.Time, securityContext domainsecurity.TurnSecurityContext) (Issuer, IssueEvidenceInput) {
	t.Helper()
	toolName := "mcp__analytix_funds__query"
	serverIdentity := evidenceTestVerifiedMCPIdentity(t, "analytix_funds", "analytix-fund-analysis", "1.0.0", 3)
	toolCallID := evidenceTestHostToolCallID(t, "issuer-a")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: serverIdentity,
		ToolName: toolName, ToolCallID: toolCallID, ConnectionEpoch: 3, ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"caseId":"case-a"}`)),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	grantRegistry, err := domainsecurity.RegisterExecutionGrant(domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	reportedSafe := true
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: toolName, ToolCallID: grant.ToolCallID, ContextDigest: securityContext.ContextDigest, ExecutionGrantID: grant.GrantID,
		CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ServerIdentity: grant.ServerIdentity, TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess,
		CandidateEvidenceReceipts: []map[string]any{{"receiptId": "provider-forged-receipt"}}, ReportedSafeToAnswer: &reportedSafe, IssuedAt: now,
	})
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: serverIdentity, ConnectionEpoch: 3,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog")), SpecFingerprint: domainsecurity.SHA256Hex([]byte("spec")),
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID, CheckedAt: now, Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: "analytix-fund-analysis", ServerVersion: "1.0.0",
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: securityContext.DatasetSnapshotID,
			Ready: true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"structuredContent":{"data":{"account":"00123456789012345678","amountMinor":"4200000"}}}`)
	canonicalBody, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-base", ClaimType: domainevidence.ClaimAccount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{SubjectID: "entity-a", AccountID: "00123456789012345678"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical := json.RawMessage(canonicalBody)
	store := &memoryEvidenceRegistry{}
	issuer := Issuer{
		Registry: store, SettlementStore: &memoryEvidenceSettlementStore{}, Authority: newMemoryFinalAuthority(46), Now: evidenceIssuerTime,
	}
	return issuer, IssueEvidenceInput{
		Context: securityContext, Grant: grant, GrantRegistry: grantRegistry, Outcome: outcome, SourceProbe: probe,
		RawResult: domainmcp.LosslessToolResult{RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw)},
		Material: VerifiedEvidenceMaterial{
			CanonicalEvidence: canonical, SourceType: "transactions", ServerVersion: "1.0.0",
			QueryHash: domainsecurity.SHA256Hex([]byte("host-recomputed-query")),
			QueryRange: domainevidence.EvidenceQueryRange{
				EntityIDs: []string{"entity-a"}, AccountIDs: []string{"00123456789012345678"}, Directions: []string{"out"},
				StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"bank-flow-a"},
				FiltersHash: domainsecurity.SHA256Hex([]byte("filters")),
			}, Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai",
			PaginationCompleteness: domainevidence.PaginationComplete, SourceRecordIDs: []string{"row-a"},
			TransformationLineage: []domainevidence.TransformationLineageStep{{
				StepID: "normalize-a", Transformer: "fixture-normalizer", TransformerVersion: "1.0.0",
				InputHash: domainsecurity.SHA256Hex(raw), OutputHash: domainsecurity.CanonicalJSONHash(canonical),
			}}, PIIClassification: domainevidence.PIIMasked,
		}, ResultItemID: domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, grant.ToolCallID), IssuedAt: now,
	}
}

// Preauthorized fixture; does not prove issuance/publication authority.
// seedPreauthorizedRegistryForGateUnitTest installs a domain-valid registry
// fixture only for pure evidence-consumer tests such as ClaimVerifier. It does
// not call the production Issuer and does not prove snapshot issuance,
// settlement, publication, or release authority.
func seedPreauthorizedRegistryForGateUnitTest(t *testing.T, issuer Issuer, input IssueEvidenceInput) domainevidence.EvidenceReceipt {
	t.Helper()
	registry, ok := issuer.Registry.(*memoryEvidenceRegistry)
	if !ok {
		t.Fatal("preauthorized gate fixture requires the isolated memory registry")
	}
	current, err := registry.current(input.Context)
	if err != nil {
		t.Fatal(err)
	}
	canonicalEvidence, err := domainevidence.CanonicalEvidenceBytes(input.Material.CanonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}
	proof := domainevidence.EvidenceSettlementProof{
		SettlementID:         domainsecurity.SHA256Hex([]byte("preauthorized-gate-settlement:\x00" + input.Context.ContextDigest + "\x00" + input.Material.QueryHash)),
		PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("preauthorized-gate-record:\x00" + input.Context.ContextDigest + "\x00" + input.Material.QueryHash)),
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(evidenceReceiptDraftInput(
		input, canonicalEvidence, domainevidence.EvidenceSettlementReceiptID(proof.SettlementID), evidenceIssuerTime(),
	))
	if err != nil {
		t.Fatal(err)
	}
	next, receipt, err := domainevidence.RegisterEvidenceReceipt(current, draft, canonicalEvidence, proof, evidenceIssuerTime())
	if err != nil {
		t.Fatal(err)
	}
	registry.registry = next
	registry.initialized = true
	return receipt
}

func alignEvidenceOutcomeWithPagination(input *IssueEvidenceInput, pagination domainevidence.PaginationCompleteness) {
	if input == nil || pagination != domainevidence.PaginationPartial {
		return
	}
	current := input.Outcome
	input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: current.ToolName, ToolCallID: current.ToolCallID, ContextDigest: current.ContextDigest,
		ExecutionGrantID: current.ExecutionGrantID, CaseID: current.CaseID, ContextEpoch: current.ContextEpoch,
		DatasetSnapshotID: current.DatasetSnapshotID, ServerIdentity: current.ServerIdentity,
		TransportStatus: current.TransportStatus, SemanticStatus: domainevidence.SemanticPartial, IsError: false,
		PartialCoverage: map[string]any{"paginationCompleteness": "partial"}, Data: current.Data,
		CandidateEvidenceReceipts: current.CandidateEvidenceReceipts, ReportedSemanticStatus: current.ReportedSemanticStatus,
		ReportedSafeToAnswer: current.ReportedSafeToAnswer, ReportedCaseID: current.ReportedCaseID,
		ReportedContextEpoch: current.ReportedContextEpoch, ReportedDatasetSnapshotID: current.ReportedDatasetSnapshotID,
		ReportedServerIdentity: current.ReportedServerIdentity, UntrustedMeta: current.UntrustedMeta,
		IssuedAt: evidenceIssuerTime(),
	})
}

type failingEvidenceRegistry struct{}

func (failingEvidenceRegistry) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	return domainevidence.EvidenceReceipt{}, errors.New("append failed")
}

func (failingEvidenceRegistry) Resolve(context.Context, registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	return domainevidence.RegisteredEvidence{}, errors.New("resolve failed")
}

func (failingEvidenceRegistry) Revoke(context.Context, registryport.RevokeInput) error {
	return errors.New("revoke failed")
}

func (failingEvidenceRegistry) Replay(context.Context, domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	return domainevidence.EvidenceReceiptRegistry{}, errors.New("replay failed")
}

type memoryEvidenceRegistry struct {
	registry    domainevidence.EvidenceReceiptRegistry
	initialized bool
	commitCalls int
}

func (registry *memoryEvidenceRegistry) CommitPrepared(_ context.Context, input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	current, err := registry.current(input.Context)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	next, receipt, err := domainevidence.RegisterEvidenceReceipt(current, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	registry.commitCalls++
	registry.registry = next
	return receipt, nil
}

func (registry *memoryEvidenceRegistry) Resolve(_ context.Context, query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	current, err := registry.current(query.Context)
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	return domainevidence.VerifyEvidenceReceiptMembership(current, query.Context, query.ReceiptID)
}

func (registry *memoryEvidenceRegistry) Revoke(_ context.Context, input registryport.RevokeInput) error {
	current, err := registry.current(input.Context)
	if err != nil {
		return err
	}
	registry.registry, err = domainevidence.RevokeEvidenceReceipt(current, input.ReceiptID, input.ReasonCode, input.RevokedAt)
	return err
}

func (registry *memoryEvidenceRegistry) Replay(_ context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	return registry.current(securityContext)
}

func (registry *memoryEvidenceRegistry) ListRegistries(_ context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	if !registry.initialized {
		return []registryport.InventoryRecord{}, nil
	}
	for _, securityContext := range contexts {
		if domainevidence.EvidenceReceiptRegistryMatchesContext(registry.registry, securityContext) {
			return []registryport.InventoryRecord{{Context: securityContext, Registry: registry.registry}}, nil
		}
	}
	return nil, errors.New("memory evidence registry is detached from supplied contexts")
}

func (registry *memoryEvidenceRegistry) HasRecords(context.Context) (bool, error) {
	return registry.initialized && registry.registry.Sequence > 0, nil
}

func (registry *memoryEvidenceRegistry) current(securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	if !registry.initialized {
		current, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
		if err != nil {
			return domainevidence.EvidenceReceiptRegistry{}, err
		}
		registry.registry = current
		registry.initialized = true
	}
	return registry.registry, nil
}

func evidenceIssuerTime() time.Time {
	return time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
}

func evidenceTestVerifiedMCPIdentity(t *testing.T, serverID, observedName, observedVersion string, epoch uint64) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, observedName, observedVersion, domainsecurity.SHA256Hex([]byte("evidence-test-runtime-instance")), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
