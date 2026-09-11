package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type providerAttemptFundsSourceV1 struct {
	semantic           domainnative.AccountFlowProviderSemanticResultV1
	active             bool
	discardInvocations int
}

func (source *providerAttemptFundsSourceV1) HostFundsAccountFlowProviderSemanticV1(
	result domainmcp.LosslessToolResult,
) (domainnative.AccountFlowProviderSemanticResultV1, bool) {
	return source.semantic, source.active && domainmcp.ValidLosslessToolResult(result)
}

func (source *providerAttemptFundsSourceV1) DiscardHostFundsAccountFlowEvidenceV1(
	domainmcp.LosslessToolResult,
) {
	source.discardInvocations++
	source.active = false
}

type providerAttemptFundsDisposerOnlyV1 struct {
	discardInvocations int
}

func (source *providerAttemptFundsDisposerOnlyV1) DiscardHostFundsAccountFlowEvidenceV1(
	domainmcp.LosslessToolResult,
) {
	source.discardInvocations++
}

func TestToolResultProviderAttemptCapturesSemanticBeforeEvidenceBurnsCarrier(t *testing.T) {
	pending, output, lossless, semantic := providerAttemptAccountFlowFixtureV1(t)
	source := &providerAttemptFundsSourceV1{semantic: semantic, active: true}
	attempt := CaptureToolResultProviderAttemptV1(ToolResultProviderAttemptInputV1{
		Pending: pending, Output: output, Source: source,
	})
	source.DiscardHostFundsAccountFlowEvidenceV1(lossless)
	if source.discardInvocations != 1 {
		t.Fatalf("evidence carrier was not burned: %d", source.discardInvocations)
	}

	prepared, err := attempt.PrepareSettlementV1(output, false)
	if err != nil {
		t.Fatalf("prepare captured account-flow semantic after evidence burn: %v", err)
	}
	if prepared.Message.PrivateProviderSemanticBinding == nil {
		t.Fatal("captured account-flow semantic lost its provider binding")
	}
	for _, expected := range []string{semantic.SubjectAlias, semantic.QueryHash, `"netMinor":"100"`} {
		if !strings.Contains(prepared.Message.Content, expected) {
			t.Fatalf("provider message lost %q: %s", expected, prepared.Message.Content)
		}
	}
	publicBody, err := json.Marshal(prepared.PublicOutput)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", semantic.QueryHash, "/private/case.duckdb"} {
		if strings.Contains(string(publicBody), forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, publicBody)
		}
	}

	attempt.DiscardPrivate()
	attempt.DiscardPrivate()
	if source.discardInvocations != 2 {
		t.Fatalf("attempt disposer was not idempotent: %d", source.discardInvocations)
	}
}

func TestToolResultProviderAttemptDisposesCarrierAfterCaptureFailure(t *testing.T) {
	pending, output, _, _ := providerAttemptAccountFlowFixtureV1(t)
	source := &providerAttemptFundsDisposerOnlyV1{}
	attempt := CaptureToolResultProviderAttemptV1(ToolResultProviderAttemptInputV1{
		Pending: pending, Output: output, Source: source,
	})
	if _, err := attempt.PrepareSettlementV1(output, false); err == nil {
		t.Fatal("missing semantic reader did not fail closed")
	}
	attempt.DiscardPrivate()
	attempt.DiscardPrivate()
	if source.discardInvocations != 1 {
		t.Fatalf("failed capture disposer was not idempotent: %d", source.discardInvocations)
	}
}

func TestToolResultProviderAttemptDoesNotInjectSemanticIntoErrorSettlement(t *testing.T) {
	pending, output, _, semantic := providerAttemptAccountFlowFixtureV1(t)
	source := &providerAttemptFundsSourceV1{semantic: semantic, active: true}
	attempt := CaptureToolResultProviderAttemptV1(ToolResultProviderAttemptInputV1{
		Pending: pending, Output: output, Source: source,
	})
	defer attempt.DiscardPrivate()
	failure := map[string]any{"executed": false, "isError": true, "code": "evidence_preparation_failed"}
	prepared, err := attempt.PrepareSettlementV1(failure, true)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Message.PrivateProviderSemanticBinding != nil ||
		strings.Contains(prepared.Message.Content, semantic.SubjectAlias) {
		t.Fatalf("error settlement received captured semantic: %#v", prepared.Message)
	}
}

func TestToolResultProviderAttemptDisposesCarrierFromInitialError(t *testing.T) {
	pending, output, _, semantic := providerAttemptAccountFlowFixtureV1(t)
	source := &providerAttemptFundsSourceV1{semantic: semantic, active: true}
	attempt := CaptureToolResultProviderAttemptV1(ToolResultProviderAttemptInputV1{
		Pending: pending, Output: output, IsError: true, Source: source,
	})
	attempt.DiscardPrivate()
	attempt.DiscardPrivate()
	if source.discardInvocations != 1 {
		t.Fatalf("initial error carrier disposer was not idempotent: %d", source.discardInvocations)
	}
}

func TestToolResultProviderAttemptBurnsForegroundCarrierWithoutInjectingItIntoError(t *testing.T) {
	privateCanary := "HOST_PRIVATE_CASE_RESULT_8F03"
	closed := map[string]any{
		"kind": "subagent_task", "status": "failed", "code": "foreground_handoff_projection_invalid",
		"factAnswerAllowed": false, "evidenceAuthority": false,
		"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
	}
	pending := appmodel.PendingToolCall{Call: domainmodel.ToolCall{ID: "case-task-call", Name: "task"}}
	publicCalls, privateCalls := 0, 0
	attempt := CaptureToolResultProviderAttemptV1(ToolResultProviderAttemptInputV1{
		Pending: pending, Output: map[string]any{"private": privateCanary}, IsError: true,
		ProjectExactPublic: func(appmodel.PendingToolCall, any) (map[string]any, bool, bool) {
			publicCalls++
			return closed, true, false
		},
		ProjectExact: func(appmodel.PendingToolCall, any) (map[string]any, map[string]any, bool, bool) {
			privateCalls++
			return map[string]any{"caseResult": privateCanary}, closed, true, true
		},
	})
	prepared, err := attempt.PrepareSettlementV1(map[string]any{"private": privateCanary}, true)
	if err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(map[string]any{
		"public": prepared.PublicOutput, "projection": prepared.Projection, "message": prepared.Message,
	})
	if err != nil {
		t.Fatal(err)
	}
	if publicCalls != 1 || privateCalls != 1 || strings.Contains(string(serialized), privateCanary) ||
		!strings.Contains(string(serialized), "foreground_handoff_projection_invalid") {
		t.Fatalf("error projection leaked or replayed the foreground carrier: publicCalls=%d privateCalls=%d bytes=%s", publicCalls, privateCalls, serialized)
	}
}

func TestPersistableForegroundOutputWithoutExactProjectorFailsClosed(t *testing.T) {
	securityContext := newLoopCaseContextV2(
		t, "thread-foreground-persist", "turn-foreground-persist", "/workspace", "case-foreground-persist",
	)
	pending := appmodel.PendingToolCall{
		SecurityContext: securityContext,
		Call: domainmodel.ToolCall{
			ID: loopTestHostToolCallID("foreground-persist"), Name: "task",
			Arguments: json.RawMessage(`{"prompt":"bounded question","max_steps":2,"token_budget":512,"time_budget_ms":30000}`),
		},
	}
	privateCanary := "HOST_PRIVATE_CASE_RESULT_E6D1"
	projected := PersistableToolOutputV1(pending, map[string]any{
		"kind": "subagent_task", "status": "completed", "caseResult": privateCanary,
	}, nil)
	serialized, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), privateCanary) || strings.Contains(string(serialized), "caseResult") ||
		!strings.Contains(string(serialized), "foreground_handoff_projection_invalid") {
		t.Fatalf("missing exact projector used a generic foreground fallback: %s", serialized)
	}
}

func providerAttemptAccountFlowFixtureV1(
	t *testing.T,
) (appmodel.PendingToolCall, map[string]any, domainmcp.LosslessToolResult, domainnative.AccountFlowProviderSemanticResultV1) {
	t.Helper()
	securityContext := newLoopCaseContextV2(
		t,
		"thread-account-flow-attempt",
		"turn-account-flow-attempt",
		"/workspace",
		"case-account-flow-attempt",
	)
	call := domainmodel.ToolCall{
		ID:   loopTestHostToolCallID("account-flow-provider-attempt"),
		Name: accountFlowProviderToolNameV1,
		Arguments: json.RawMessage(
			`{"subject_alias":"acct:1","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.000000Z","evidence_row_limit":1}`,
		),
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
	rawResult := json.RawMessage(`{"content":[],"structuredContent":{"schemaVersion":1,"purpose":"analytix.funds-account-flow-analysis/v1","semanticStatus":"partial","data":{}}}`)
	rawDigest := sha256.Sum256(rawResult)
	lossless := domainmcp.LosslessToolResult{
		RawResult: rawResult, RawSHA256: hex.EncodeToString(rawDigest[:]),
	}
	output := map[string]any{
		"executed": true, "transportStatus": "success", "semanticStatus": "partial",
		"result":                       map[string]any{"purpose": "analytix.funds-account-flow-analysis/v1"},
		"hostOnly":                     map[string]any{"path": "/private/case.duckdb"},
		domainmcp.HostRawToolResultKey: lossless,
	}
	return appmodel.PendingToolCall{Call: call, SecurityContext: securityContext}, output, lossless, semantic
}
