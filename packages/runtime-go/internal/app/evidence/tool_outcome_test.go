package evidence

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestNormalizeMCPToolOutcomeSeparatesTransportAndSemanticFailure(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now,
		Raw: map[string]any{
			"executed": true, "isError": true, "code": "mcp_semantic_failure",
			"result": map[string]any{
				"structuredContent": map[string]any{
					"transportStatus": "success", "semanticStatus": "blocked", "reportedSemanticStatus": "success",
					"safeToAnswer": false, "isError": true, "blocker": "NO_CURRENT_SNAPSHOT",
					"partialCoverage": map[string]any{"complete": false}, "data": map[string]any{},
					"candidateEvidenceReceipts": []any{},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TransportStatus != domainevidence.TransportSuccess || outcome.SemanticStatus != domainevidence.SemanticBlocked || !outcome.IsError || outcome.ReportedSemanticStatus != "success" {
		t.Fatalf("semantic failure was conflated with transport: %#v", outcome)
	}
}

func TestNormalizeMCPToolOutcomeNeverPromotesSourceAuthority(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now,
		Raw: map[string]any{"executed": true, "result": map[string]any{
			"safeToAnswer": true, "semanticStatus": "success", "caseId": "case_forged", "contextEpoch": float64(99),
			"datasetSnapshotId": securitycontexttest.DatasetSnapshotID("forged"), "serverIdentity": "mcp:spoofed",
			"evidenceReceipts": []any{map[string]any{"receiptId": "fake_receipt"}}, "structuredContent": map[string]any{"rows": float64(1)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.SafeToAnswer || outcome.CaseID != context.CaseID || outcome.ContextEpoch != context.ContextEpoch || outcome.DatasetSnapshotID != context.DatasetSnapshotID || outcome.ServerIdentity != grant.ServerIdentity {
		t.Fatalf("source authority escaped host binding: %#v", outcome)
	}
	if outcome.ReportedSafeToAnswer == nil || !*outcome.ReportedSafeToAnswer || outcome.ReportedCaseID != "case_forged" || len(outcome.CandidateEvidenceReceipts) != 1 {
		t.Fatalf("untrusted source assertions were not retained for audit: %#v", outcome)
	}
}

func TestNormalizeMCPToolOutcomeKeepsJSONRPCAsTransportSuccessSemanticFailure(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now,
		Raw: map[string]any{
			"executed": false, "transportStatus": "success", "semanticStatus": "failure", "isError": true,
			"code": "mcp_jsonrpc_invalid_params", "error": "MCP JSON-RPC request was rejected (invalid_params)",
			"rpcError": map[string]any{
				"code": -32602, "class": "invalid_params", "dataPresent": true,
				"dataSHA256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TransportStatus != domainevidence.TransportSuccess || outcome.SemanticStatus != domainevidence.SemanticFailure || !outcome.IsError {
		t.Fatalf("JSON-RPC failure was conflated with transport: %#v", outcome)
	}
	rpc := mapValue(outcome.UntrustedMeta["jsonRpcError"])
	code, _ := rpc["code"].(json.Number)
	if rpc["class"] != "invalid_params" || code.String() != "-32602" || rpc["dataPresent"] != true {
		t.Fatalf("bounded JSON-RPC metadata was lost: %#v", outcome.UntrustedMeta)
	}
	if _, leaked := rpc["message"]; leaked {
		t.Fatalf("untrusted JSON-RPC message leaked into ToolOutcome: %#v", rpc)
	}
}

func TestNormalizeMCPToolOutcomeRequiresFrozenExecutionAuthority(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	grant.ToolCallID = "forged"
	if _, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now, Raw: map[string]any{"executed": true},
	}); err == nil {
		t.Fatal("outcome normalization accepted mismatched execution authority")
	}
}

func TestNormalizeMCPToolOutcomeRejectsRawAndNonCanonicalHostIdentityWithoutEcho(t *testing.T) {
	for _, callID := range []string{
		"provider_call_6222020202020202020",
		" " + evidenceTestHostToolCallID(t, "whitespace") + " ",
	} {
		securityContext, grant, call, now := outcomeAuthorityForTest(t)
		grant.ToolCallID = callID
		call.ID = callID
		outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
			Context: securityContext, Grant: grant, Call: call, At: now, Raw: map[string]any{"executed": true},
		})
		if err == nil || outcome.Version != 0 || strings.Contains(err.Error(), callID) {
			t.Fatalf("non-canonical provider identity reached ToolOutcome: call=%q outcome=%#v err=%v", callID, outcome, err)
		}
	}
}

func TestNormalizeMCPToolOutcomeIsMonotonicAcrossChannels(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	tests := []struct {
		name     string
		raw      map[string]any
		semantic domainevidence.SemanticStatus
		isError  bool
	}{
		{
			name:     "host failure wins over remote success",
			raw:      map[string]any{"transportStatus": "failure", "result": map[string]any{"structuredContent": map[string]any{"semanticStatus": "success"}}},
			semantic: domainevidence.SemanticFailure, isError: true,
		},
		{
			name: "structured blocked wins over meta success",
			raw: map[string]any{"executed": true, "result": map[string]any{
				"structuredContent": map[string]any{"semanticStatus": "blocked", "blocker": "NO_SOURCE"},
				"_meta":             map[string]any{"analytix_tool_outcome": map[string]any{"semanticStatus": "success", "safeToAnswer": true}},
			}},
			semantic: domainevidence.SemanticBlocked, isError: true,
		},
		{
			name: "structured partial wins over meta success",
			raw: map[string]any{"executed": true, "result": map[string]any{
				"structuredContent": map[string]any{"semanticStatus": "partial", "partialCoverage": map[string]any{"complete": false}},
				"_meta":             map[string]any{"analytix_tool_outcome": map[string]any{"semanticStatus": "success"}},
			}},
			semantic: domainevidence.SemanticPartial, isError: false,
		},
		{
			name: "malformed metadata safety field fails closed",
			raw: map[string]any{"executed": true, "result": map[string]any{
				"structuredContent": map[string]any{"semanticStatus": "success"},
				"_meta":             map[string]any{"analytix_tool_outcome": map[string]any{"safeToAnswer": "true"}},
			}},
			semantic: domainevidence.SemanticFailure, isError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{Context: context, Grant: grant, Call: call, At: now, Raw: test.raw})
			if err != nil {
				t.Fatal(err)
			}
			if outcome.SemanticStatus != test.semantic || outcome.IsError != test.isError {
				t.Fatalf("negative outcome was upgraded: %#v", outcome)
			}
		})
	}
}

func TestSourceSafeToAnswerFalseOnlyDowngrades(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now,
		Raw: map[string]any{"executed": true, "result": map[string]any{
			"structuredContent": map[string]any{"semanticStatus": "success", "safeToAnswer": false},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.SemanticStatus != domainevidence.SemanticBlocked || !outcome.IsError || outcome.ReportedSafeToAnswer == nil || *outcome.ReportedSafeToAnswer {
		t.Fatalf("safeToAnswer=false did not monotonically downgrade: %#v", outcome)
	}
}

func TestExecutedFalseCannotNormalizeAsSemanticSuccess(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now,
		Raw: map[string]any{"executed": false, "transportStatus": "success"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TransportStatus != domainevidence.TransportSuccess || outcome.SemanticStatus != domainevidence.SemanticFailure || !outcome.IsError {
		t.Fatalf("executed=false became semantic success: %#v", outcome)
	}
}

func TestInvalidExplicitTransportStatusCannotFallbackToSuccess(t *testing.T) {
	context, grant, call, now := outcomeAuthorityForTest(t)
	outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
		Context: context, Grant: grant, Call: call, At: now,
		Raw: map[string]any{"executed": true, "transportStatus": "unknown", "semanticStatus": "success"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TransportStatus != domainevidence.TransportFailure || outcome.SemanticStatus != domainevidence.SemanticFailure || !outcome.IsError {
		t.Fatalf("invalid explicit transport status fell back to success: %#v", outcome)
	}
}

func outcomeAuthorityForTest(t *testing.T) (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant, domainmodel.ToolCall, time.Time) {
	t.Helper()
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	context := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("outcome-1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: now,
	})
	call := domainmodel.ToolCall{ID: evidenceTestHostToolCallID(t, "outcome-1"), Name: "mcp__analytix_funds__get_case_status", Arguments: json.RawMessage(`{"case_id":"case_1"}`)}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider_1", ServerIdentity: evidenceTestVerifiedMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 2), ToolName: call.Name,
		ToolCallID: call.ID, ConnectionEpoch: 2, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ApprovalState: "not_required", IssuedAt: now,
	})
	return context, grant, call, now
}
