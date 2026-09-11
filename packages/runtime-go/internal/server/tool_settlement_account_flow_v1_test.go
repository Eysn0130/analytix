package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

type accountFlowProviderSettlementMCPStubV1 struct {
	*providerStepIntegrationMCP
	semantic  domainnative.AccountFlowProviderSemanticResultV1
	discarded int
}

func (stub *accountFlowProviderSettlementMCPStubV1) HostFundsAccountFlowProviderSemanticV1(
	result domainmcp.LosslessToolResult,
) (domainnative.AccountFlowProviderSemanticResultV1, bool) {
	return stub.semantic, domainmcp.ValidLosslessToolResult(result)
}

func (stub *accountFlowProviderSettlementMCPStubV1) DiscardHostFundsAccountFlowEvidenceV1(
	domainmcp.LosslessToolResult,
) {
	stub.discarded++
}

func TestAccountFlowSettlementKeepsPublicMetadataAndBindsProviderSafeSemantics(t *testing.T) {
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-account-flow-provider", TurnID: "turn-account-flow-provider",
		WorkspaceRealPath: "/workspace", CaseID: "case-account-flow-provider",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("account-flow-provider-binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("account-flow-provider-manifest")),
		ContextEpoch:       3, IssuedAt: time.Unix(1, 0),
	})
	call := provider.ToolCall{
		ID:        serverTestHostToolCallID("account-flow-provider-settlement"),
		Name:      "mcp__analytix_funds__analyze_account_flows",
		Arguments: json.RawMessage(`{"subject_alias":"acct:1","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.000000Z","evidence_row_limit":1}`),
	}
	semantic := domainnative.AccountFlowProviderSemanticResultV1{
		SubjectAlias:   "acct:1",
		StartInclusive: "2026-01-01T00:00:00.000000Z",
		EndInclusive:   "2026-01-31T23:59:59.000000Z",
		Timezone:       "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: "100", OutflowMinor: "0", NetMinor: "100",
		TransactionCount: 1, EvidenceTransactionCount: 1, EvidenceRowLimit: 1,
		AggregateComplete: true, EvidenceRowsComplete: true, CounterpartySemanticsComplete: false,
		Currentness: domainnative.AccountFlowProviderCurrentnessCurrentV1,
		Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{
			State:                  domainnative.AccountFlowCoveragePartialV1,
			Gaps:                   []string{domainnative.AccountFlowGapCounterpartyResolutionV1},
			NormalizedSnapshotRows: 1, AcceptedSnapshotRows: 1, ObservedMatchingRows: 1,
		},
		QueryHash:  domainsecurity.SHA256Hex([]byte("account-flow-provider-query")),
		ResultHash: domainsecurity.SHA256Hex([]byte("account-flow-provider-result")),
		Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{{
			EvidenceRef:  "srow1_" + domainsecurity.SHA256Hex([]byte("account-flow-provider-row")),
			Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
			OccurredAt:   "2026-01-05T10:30:00.000000Z",
			Direction:    domainnative.AccountFlowDirectionInflowV1,
			AmountMinor:  "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		}},
	}
	outcome, err := domainnative.NewAccountFlowProviderOutcomeV1(
		semantic.SubjectAlias, semantic.AggregateComplete, semantic.EvidenceRowsComplete, semantic.QueryHash, semantic.ResultHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	semantic.Outcome = outcome
	stub := &accountFlowProviderSettlementMCPStubV1{
		providerStepIntegrationMCP: &providerStepIntegrationMCP{admissionFailureMCP: &admissionFailureMCP{}},
		semantic:                   semantic,
	}
	handler := &runtimeServerHandler{mcp: stub}
	pending := runtimePendingToolCall{Call: call, SecurityContext: securityContext}
	raw := mcpprotocol.ExtractLosslessToolResult([]byte(`{"content":[],"structuredContent":{"schemaVersion":1,"purpose":"analytix.funds-account-flow-analysis/v1","semanticStatus":"partial","data":{}}}`))
	output := map[string]any{
		"executed":                     true,
		"transportStatus":              "success",
		"semanticStatus":               "partial",
		"result":                       map[string]any{"purpose": "analytix.funds-account-flow-analysis/v1"},
		"hostOnly":                     map[string]any{"path": "/private/case.duckdb", "sql": "SELECT secret"},
		domainmcp.HostRawToolResultKey: raw,
	}
	attempt := apploop.CaptureToolResultProviderAttemptV1(apploop.ToolResultProviderAttemptInputV1{Pending: pending, Output: output, Source: handler.mcp, ProjectExact: subagentapp.ProjectForegroundParentToolOutputV1})
	prepared, err := attempt.PrepareSettlementV1(output, false)
	if err != nil {
		t.Fatalf("exact account-flow provider output unavailable: %v", err)
	}
	projection, message := prepared.Projection, prepared.Message
	modelContent := message.Content
	publicBody, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"cer1_" + strings.Repeat("a", 64), semantic.QueryHash, "/private/case.duckdb", "SELECT secret"} {
		if strings.Contains(string(publicBody), forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, publicBody)
		}
	}
	for _, expected := range []string{semantic.SubjectAlias, `"netMinor":"100"`, semantic.Transactions[0].EvidenceRef} {
		if !strings.Contains(modelContent, expected) {
			t.Fatalf("provider semantic output lost %q: %s", expected, modelContent)
		}
	}
	if message.PrivateProviderSemanticBinding == nil {
		t.Fatalf("provider semantic binding failed: message=%#v", message)
	}
	projected, err := privacyprojectionapp.ProjectProviderRequestForEffect(
		securityContext,
		provider.Request{Messages: []provider.Message{{Role: "assistant", ToolCalls: []provider.ToolCall{call}}, message}},
		false,
	)
	if err != nil {
		t.Fatalf("final provider privacy projection failed: %v", err)
	}
	if projected.Messages[1].Content != modelContent ||
		projected.Messages[1].PrivateProviderSemanticBinding != nil {
		t.Fatalf("provider semantic projection changed or leaked binding: %#v", projected.Messages[1])
	}
	attempt.DiscardPrivate()
	if stub.discarded != 1 {
		t.Fatalf("private carrier was not disposed: %d", stub.discarded)
	}
	if !strings.Contains(modelContent, `"semanticStatus":"partial"`) {
		t.Fatalf("semantic status changed: %s", modelContent)
	}
}
