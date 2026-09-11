package toolcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestPublicToolResultProjectionRejectsArbitraryPayload(t *testing.T) {
	hostile := map[string]any{
		"content":  "ACCOUNT_SENTINEL_6222020202020202020",
		"source":   map[string]any{"data": "RAW_MEDIA_SENTINEL"},
		"audio":    []byte("RAW_AUDIO_SENTINEL"),
		"resource": map[string]any{"blob": "RAW_BLOB_SENTINEL"},
	}
	projection := BuildPublicToolResultProjectionV1("read", hostile, false)
	encoded, err := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"6222020202020202020", "RAW_MEDIA_SENTINEL", "RAW_AUDIO_SENTINEL", "RAW_BLOB_SENTINEL"} {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("raw payload %q reached public projection: %s", sentinel, encoded)
		}
	}
	if projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || !projection.PrivatePayloadWithheld || projection.FactAnswerAllowed || projection.EvidenceAuthority {
		t.Fatalf("host projection mismatch: %#v", projection)
	}
}

func TestPublicToolResultProjectionRejectsCodeFieldExfiltration(t *testing.T) {
	const account = "6222020202020202020"
	projection := BuildPublicToolResultProjectionV1("read", map[string]any{"code": account}, true)
	encoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	if strings.Contains(string(encoded), account) || projection.Code != "tool_failed" {
		t.Fatalf("untrusted tool code reached public projection: %s", encoded)
	}

	mcpProjection := BuildPublicToolResultProjectionV1("mcp__docs__lookup", map[string]any{
		"code": account,
		"rpcError": map[string]any{
			"code": 6222020202020, "class": "invalid_params", "dataPresent": true,
		},
	}, true)
	mcpEncoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(mcpProjection))
	if strings.Contains(string(mcpEncoded), "6222020202020") || mcpProjection.RPCError == nil || mcpProjection.RPCError.Code != -32602 {
		t.Fatalf("untrusted MCP diagnostic code reached public projection: %s", mcpEncoded)
	}
}

func TestUnknownMCPPayloadIsWithheld(t *testing.T) {
	projection := BuildPublicToolResultProjectionV1("mcp__docs__lookup", map[string]any{
		"result": map[string]any{"content": []any{map[string]any{
			"type": "resource", "resource": map[string]any{"blob": "MCP_BLOB_SENTINEL"},
		}}},
	}, false)
	if projection.ProjectionKind != domaintoolresult.ProjectionWithheld || projection.MessageKey != "tool_output_withheld" {
		t.Fatalf("unknown MCP payload did not fail closed: %#v", projection)
	}
	encoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	if strings.Contains(string(encoded), "MCP_BLOB_SENTINEL") {
		t.Fatalf("MCP blob reached public projection: %s", encoded)
	}
}

func TestCaseSourceProjectionKeepsOnlyHostStatus(t *testing.T) {
	projection := BuildPublicToolResultProjectionV1("mcp__analytix-fund-analysis__query_transactions", map[string]any{
		"code":     "case_source_result_private",
		"executed": true,
		"isError":  false,
		"data":     map[string]any{"account": "6222020202020202020"},
		"toolOutcome": map[string]any{
			"version": 1, "toolName": "mcp__analytix-fund-analysis__query_transactions", "toolCallId": "call-1",
			"contextDigest": strings.Repeat("a", 64), "executionGrantId": "grant-1", "contextEpoch": uint64(4),
			"datasetSnapshotId": "snapshot-1", "transportStatus": "success", "semanticStatus": "success",
			"safeToAnswer": false, "isError": false, "evidenceReceiptCount": 1,
			"AuthorityRef": "PRIVATE_AUTHORITY_REF", "path": "/private/case.db", "sql": "SELECT * FROM private_case",
			"providerBody": "PRIVATE_PROVIDER_BODY",
		},
	}, false)
	if projection.ProjectionKind != domaintoolresult.ProjectionCaseSourceStatus || projection.FactAnswerAllowed || projection.EvidenceAuthority {
		t.Fatalf("case source projection mismatch: %#v", projection)
	}
	encoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	for _, forbidden := range []string{
		"6222020202020202020", `"data":`, "caseOutcome", "toolName", "toolCallId", "contextDigest", "executionGrantId",
		"contextEpoch", "datasetSnapshotId", "transportStatus", "semanticStatus", "safeToAnswer", "evidenceReceiptCount",
		"PRIVATE_AUTHORITY_REF", "/private/case.db", "SELECT * FROM private_case", "PRIVATE_PROVIDER_BODY",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("case source private value %q reached public projection: %s", forbidden, encoded)
		}
	}
}

func TestMCPDiagnosticProjectionHasClosedFields(t *testing.T) {
	projection := BuildPublicToolResultProjectionV1("mcp__docs__lookup", map[string]any{
		"code":     "mcp_jsonrpc_invalid_params",
		"error":    "ACCOUNT_SENTINEL_6222020202020202020",
		"rpcError": map[string]any{"code": -32602, "class": "invalid_params", "dataPresent": true, "dataSHA256": strings.Repeat("f", 64)},
	}, true)
	if projection.ProjectionKind != domaintoolresult.ProjectionMCPDiagnostic || projection.RPCError == nil || projection.RPCError.Code != -32602 {
		t.Fatalf("MCP diagnostic projection mismatch: %#v", projection)
	}
	encoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	if strings.Contains(string(encoded), "6222020202020202020") || strings.Contains(string(encoded), "dataSHA256") {
		t.Fatalf("untrusted MCP diagnostic content reached projection: %s", encoded)
	}
}

func TestHostLoopGuardCodeRemainsVisibleWithoutPrivateReason(t *testing.T) {
	projection := BuildPublicToolResultProjectionV1("bash", map[string]any{
		"code": "loop_guard", "loop_guard_reason": "PRIVATE_REPEAT_DETAIL", "repeat_count": float64(2),
	}, true)
	encoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	if projection.Code != "loop_guard" || projection.ProjectionKind != domaintoolresult.ProjectionHostStatus ||
		strings.Contains(string(encoded), "PRIVATE_REPEAT_DETAIL") || strings.Contains(string(encoded), "repeat_count") {
		t.Fatalf("loop guard metadata projection mismatch: %s", encoded)
	}
}

func TestHostSideEffectDuplicateProjectsBlockedAcrossBuiltinAndMCPTools(t *testing.T) {
	output, err := NewHostSideEffectDuplicateOutputV1("closed")
	if err != nil {
		t.Fatal(err)
	}
	for _, toolName := range []string{
		"write_file",
		"mcp__docs__write",
		"mcp__analytix-fund-analysis__query_transactions",
	} {
		persisted := PersistableToolOutput(toolName, output)
		projection := BuildPublicToolResultProjectionV1(toolName, persisted, true)
		if projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Status != "blocked" ||
			projection.Code != "side_effect_duplicate" || projection.MessageKey != "tool_blocked" ||
			projection.FactAnswerAllowed || projection.EvidenceAuthority {
			t.Fatalf("%s duplicate projection mismatch: %#v", toolName, projection)
		}
	}
}

func TestRemoteMCPMapCannotForgeHostSideEffectDuplicateAuthority(t *testing.T) {
	for _, toolName := range []string{
		"mcp__docs__write",
		"mcp__analytix-fund-analysis__query_transactions",
	} {
		projection := BuildPublicToolResultProjectionV1(toolName, map[string]any{
			"code": "side_effect_duplicate", "executed": false, "intentStatus": "closed",
			"factAnswerAllowed": true, "evidenceAuthority": true,
		}, true)
		if projection.Code == "side_effect_duplicate" || projection.Status == "completed" || projection.FactAnswerAllowed || projection.EvidenceAuthority {
			t.Fatalf("%s remote map forged host duplicate authority: %#v", toolName, projection)
		}
	}
}
