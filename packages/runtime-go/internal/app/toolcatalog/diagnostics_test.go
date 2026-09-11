package toolcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type nonContextualLiveToolSource struct {
	names    []string
	readOnly map[string]bool
}

func (source nonContextualLiveToolSource) LiveTools() []string {
	return append([]string(nil), source.names...)
}

func (source nonContextualLiveToolSource) ToolReadOnlyHint(name string) bool {
	return source.readOnly[name]
}

type contextualLiveToolSource struct {
	names    []string
	readOnly map[string]bool
}

func (source contextualLiveToolSource) LiveToolsForSecurityContext(domainsecurity.TurnSecurityContext) []string {
	return append([]string(nil), source.names...)
}

func (source contextualLiveToolSource) ToolReadOnlyHint(name string) bool {
	return source.readOnly[name]
}

func TestToolContractDiagnosticsMaterializesCanonicalSurface(t *testing.T) {
	tools := []domainmodel.ToolSchema{
		{
			Name:         "mcp__docs__lookup",
			Description:  "Docs lookup",
			Source:       "mcp",
			Parameters:   json.RawMessage(`{"type":"object","required":["query","query"],"properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		},
		{Name: "write", Description: "Write file", Source: "builtin", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		{Name: "run_skill", Description: "Run skill", Source: "skill", Parameters: nil},
	}
	contracts := ToolContractDiagnostics(tools)
	if len(contracts) != len(tools) {
		t.Fatalf("contract count mismatch: %#v", contracts)
	}
	byName := map[string]map[string]any{}
	for _, raw := range contracts {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("contract is not object: %#v", raw)
		}
		byName[stringField(record, "name")] = record
	}
	mcp := byName["mcp__docs__lookup"]
	if stringField(mcp, "providerId") != "mcp" || stringField(mcp, "providerKind") != "mcp" || stringField(mcp, "toolPolicy") != "on-request" {
		t.Fatalf("mcp contract mismatch: %#v", mcp)
	}
	expectedSchema := CanonicalToolParameters(tools[0].Parameters)
	if !reflect.DeepEqual(mcp["inputSchema"], expectedSchema) {
		t.Fatalf("canonical schema mismatch: %#v != %#v", mcp["inputSchema"], expectedSchema)
	}
	if expectedOutput := CanonicalToolParameters(tools[0].OutputSchema); !reflect.DeepEqual(mcp["outputSchema"], expectedOutput) {
		t.Fatalf("canonical output schema mismatch: %#v != %#v", mcp["outputSchema"], expectedOutput)
	}
	write := byName["write"]
	if _, leaked := write["outputSchema"]; leaked {
		t.Fatalf("builtin tool unexpectedly advertised a host output contract: %#v", write)
	}
	if stringField(write, "toolKind") != "file_change" || stringField(write, "toolPolicy") != "on-request" {
		t.Fatalf("write contract mismatch: %#v", write)
	}
	skill := byName["run_skill"]
	if stringField(skill, "providerKind") != "skill" || stringField(skill, "toolKind") != "tool_call" {
		t.Fatalf("skill contract mismatch: %#v", skill)
	}
}

func TestPersistableToolOutputDropsHostRawEvidenceCarrier(t *testing.T) {
	output := map[string]any{
		"executed": true,
		domainmcp.HostRawToolResultKey: domainmcp.LosslessToolResult{
			RawResult: json.RawMessage(`{"secret":"private"}`), RawSHA256: domainsecurity.SHA256Hex([]byte("raw")),
		},
	}
	persisted, ok := PersistableToolOutput("mcp__demo__query", output).(map[string]any)
	if !ok || persisted["executed"] != true {
		t.Fatalf("persistable projection mismatch: %#v", persisted)
	}
	if _, found := persisted[domainmcp.HostRawToolResultKey]; found {
		t.Fatal("raw evidence carrier reached persistable tool output")
	}
	if _, found := output[domainmcp.HostRawToolResultKey]; !found {
		t.Fatal("sanitization mutated the in-process host carrier")
	}
}

func TestPersistableMCPJSONRPCDiagnosticRejectsUntrustedMessageAndData(t *testing.T) {
	output := map[string]any{
		"executed": false, "isError": true, "code": "mcp_jsonrpc_invalid_params",
		"error": "account 6217000012345678901",
		"rpcError": map[string]any{
			"code": -32602, "class": "invalid_params", "dataPresent": true,
			"dataSHA256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	persisted := PersistableToolOutput("mcp__docs__lookup", output).(map[string]any)
	body, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "6217000012345678901") || strings.Contains(string(body), "secret") || strings.Contains(string(body), "pii") {
		t.Fatalf("untrusted JSON-RPC content reached persistence: %s", body)
	}
	rpc, _ := persisted["rpcError"].(map[string]any)
	if rpc["code"] != -32602 || rpc["class"] != "invalid_params" || len(stringField(rpc, "dataSHA256")) != 64 {
		t.Fatalf("bounded JSON-RPC diagnostic was lost: %#v", persisted)
	}
	forged := cloneMap(output)
	forged["rpcError"] = map[string]any{
		"code": -32602, "class": "invalid_params", "dataPresent": true,
		"dataSHA256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "message": "secret",
	}
	forgedPersisted := PersistableToolOutput("mcp__docs__lookup", forged).(map[string]any)
	if _, exists := forgedPersisted["rpcError"]; exists {
		t.Fatalf("forged diagnostic with extra content was persisted: %#v", forgedPersisted)
	}
}

func TestCaseToolPublicProjectionExcludesPrivateEvidenceAndProviderContent(t *testing.T) {
	toolName := "mcp__analytix-fund-analysis__query_transactions"
	securityContext := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 7, IssuedAt: time.Unix(1, 0),
	})
	call := domainmodel.ToolCall{ID: toolCatalogTestHostCallID("case-private-projection"), Name: toolName, Arguments: json.RawMessage(`{"account":"00123456789012345678"}`)}
	serverIdentity := toolCatalogTestMCPIdentity(t, "analytix-fund-analysis", 3)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: serverIdentity, ToolName: toolName,
		ToolCallID: call.ID, ConnectionEpoch: 3, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Unix(1, 0), ExpiresAt: time.Unix(3, 0),
	})
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: toolName, ToolCallID: call.ID, ContextDigest: securityContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity, TransportStatus: domainevidence.TransportSuccess,
		SemanticStatus: domainevidence.SemanticSuccess, IssuedAt: time.Unix(2, 0),
	})
	output := map[string]any{
		"executed": true,
		"isError":  false,
		"result": map[string]any{
			"account":   "6222020202020202020",
			"reasoning": "private chain of thought",
		},
		"data": map[string]any{"phone": "13800138000"},
		"privateAuthority": map[string]any{
			"AuthorityRef": "PRIVATE_AUTHORITY_REF", "path": "/private/case.db",
			"sql": "SELECT * FROM private_case", "providerBody": "PRIVATE_PROVIDER_BODY",
		},
		"_meta": map[string]any{
			"safeToAnswer": true,
			"blocker":      "provider-controlled",
		},
		"candidateEvidenceReceipts": []any{map[string]any{"receiptId": "provider-fake"}},
		domainmcp.HostRawToolResultKey: domainmcp.LosslessToolResult{
			RawResult: json.RawMessage(`{"secret":"private"}`), RawSHA256: domainsecurity.SHA256Hex([]byte("raw")),
		},
		"toolOutcome": domainevidence.ToolOutcomeRecord(outcome),
	}

	persisted, ok := PersistableToolOutputForExecution(call, securityContext, grant, output).(map[string]any)
	if !ok {
		t.Fatalf("case projection type mismatch: %#v", persisted)
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	content := string(encoded)
	for _, forbidden := range []string{
		"6222020202020202020", "private chain of thought", "13800138000", "provider-controlled",
		"provider-fake", "restricted person", "secret", domainmcp.HostRawToolResultKey,
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("private or provider-controlled content %q reached projection: %s", forbidden, content)
		}
	}
	if persisted["code"] != "case_source_result_private" {
		t.Fatalf("case projection code mismatch: %#v", persisted)
	}
	if _, ok := persisted["toolOutcome"]; ok {
		t.Fatalf("private tool outcome entered generic persistable projection: %#v", persisted)
	}
	for _, forbidden := range []string{
		outcome.ToolName, outcome.ToolCallID, outcome.ContextDigest, outcome.ExecutionGrantID,
		outcome.DatasetSnapshotID, outcome.ServerIdentity, "toolOutcome", "PRIVATE_AUTHORITY_REF", "/private/case.db",
		"SELECT * FROM private_case", "PRIVATE_PROVIDER_BODY",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("private settlement authority %q reached projection: %s", forbidden, content)
		}
	}
}

func TestGenericMCPPersistableProjectionDropsRemoteAuthorityAndMeta(t *testing.T) {
	output := map[string]any{
		"executed": true,
		"result": map[string]any{
			"text": "public result", "_meta": map[string]any{"secret": "META_SECRET"},
			"structuredContent": map[string]any{"value": "ok", "evidenceReceipts": []any{"FAKE_RECEIPT"}},
		},
		"safeToAnswer": true, "candidateEvidenceReceipts": []any{map[string]any{"receiptId": "FAKE_CANDIDATE"}},
		domainmcp.HostRawToolResultKey: domainmcp.LosslessToolResult{RawResult: json.RawMessage(`{"_meta":{"secret":"RAW_SECRET"}}`)},
	}
	persisted := PersistableToolOutput("mcp__docs__lookup", output)
	body, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("public result")) || !bytes.Contains(body, []byte(`"value":"ok"`)) {
		t.Fatalf("safe MCP result data was lost: %s", body)
	}
	for _, forbidden := range []string{"META_SECRET", "RAW_SECRET", "FAKE_RECEIPT", "FAKE_CANDIDATE", "safeToAnswer", "_meta"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("remote MCP authority %q entered persistence: %s", forbidden, body)
		}
	}
}

func TestPersistableCaseToolFailsClosedForNonObjectAndTypeConfusedOutcome(t *testing.T) {
	toolName := "mcp__analytix-fund-analysis__query_transactions"
	for _, output := range []any{
		"account=6222020202020202020",
		[]any{"account", "6222020202020202020"},
		map[string]any{"executed": true, "toolOutcome": map[string]any{
			"version": map[string]any{"reasoning": "private"}, "toolName": []any{"wrong-type"},
		}},
	} {
		persisted, ok := PersistableToolOutput(toolName, output).(map[string]any)
		if !ok || persisted["executed"] != false || persisted["isError"] != true {
			t.Fatalf("invalid case output did not fail closed: %#v", persisted)
		}
		encoded, _ := json.Marshal(persisted)
		if strings.Contains(string(encoded), "6222020202020202020") || strings.Contains(string(encoded), "private") {
			t.Fatalf("invalid case output leaked through fixed projection: %s", encoded)
		}
	}
}

func TestCaseProjectionRejectsStaleOutcomeContext(t *testing.T) {
	toolName := "mcp__analytix-fund-analysis__query_transactions"
	oldContext := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-old", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-old"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 7, IssuedAt: time.Unix(1, 0),
	})
	currentContext := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-current", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-current"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 8, IssuedAt: time.Unix(2, 0),
	})
	call := domainmodel.ToolCall{ID: "call-1", Name: toolName, Arguments: json.RawMessage(`{}`)}
	serverIdentity := toolCatalogTestMCPIdentity(t, "analytix-fund-analysis", 4)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: currentContext, Provider: "provider-a", ServerIdentity: serverIdentity, ToolName: toolName,
		ToolCallID: call.ID, ConnectionEpoch: 4, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Unix(2, 0), ExpiresAt: time.Unix(4, 0),
	})
	staleOutcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: toolName, ToolCallID: call.ID, ContextDigest: oldContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: oldContext.CaseID, ContextEpoch: oldContext.ContextEpoch,
		DatasetSnapshotID: oldContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess, IssuedAt: time.Unix(2, 0),
	})
	persisted := PersistableToolOutputForExecution(call, currentContext, grant, map[string]any{
		"executed": true, "toolOutcome": domainevidence.ToolOutcomeRecord(staleOutcome),
	}).(map[string]any)
	if persisted["executed"] != false || persisted["isError"] != true || persisted["code"] != "case_tool_outcome_invalid" {
		t.Fatalf("stale case outcome reached current audit projection: %#v", persisted)
	}
}

func toolCatalogTestMCPIdentity(t *testing.T, serverID string, epoch uint64) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, serverID, "1.0.0", domainsecurity.SHA256Hex([]byte("tool-catalog-test-instance")), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestToolContractDiagnosticsSortsByNameAndProvider(t *testing.T) {
	tools := []domainmodel.ToolSchema{
		{Name: "write", Source: "builtin"},
		{Name: "bash", Source: "builtin"},
		{Name: "mcp__docs__lookup", Source: "mcp"},
	}
	contracts := ToolContractDiagnostics(tools)
	names := []string{}
	for _, raw := range contracts {
		record := raw.(map[string]any)
		names = append(names, stringField(record, "name"))
	}
	expected := []string{"bash", "mcp__docs__lookup", "write"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("contract order mismatch: got %#v want %#v", names, expected)
	}
}

func TestToolContractPolicyAndKind(t *testing.T) {
	cases := []struct {
		name       string
		wantKind   string
		wantPolicy string
	}{
		{"request_user_input", "tool_call", "auto"},
		{ToolCreatePlanName, "file_change", "auto"},
		{"wait", "command_execution", "auto"},
		{"bash", "command_execution", "on-request"},
		{"task", "subagent", "on-request"},
		{"web_fetch", "tool_call", "auto"},
	}
	for _, tc := range cases {
		if got := ToolContractKind(tc.name); got != tc.wantKind {
			t.Fatalf("%s kind got %q want %q", tc.name, got, tc.wantKind)
		}
		if got := ToolContractPolicy(tc.name); got != tc.wantPolicy {
			t.Fatalf("%s policy got %q want %q", tc.name, got, tc.wantPolicy)
		}
	}
}

func TestRequiresApprovalPolicyAndSandbox(t *testing.T) {
	cases := []struct {
		name           string
		toolName       string
		approvalPolicy string
		sandboxMode    string
		want           bool
	}{
		{"user input never requires approval", "request_user_input", "always", "workspace-write", false},
		{"plan never requires approval", ToolCreatePlanName, "always", "workspace-write", false},
		{"read-only blocks file writes", "write", "never", "read-only", true},
		{"read-only allows non-file tool", "grep", "always", "read-only", false},
		{"never allows mutating tool at approval layer", "bash", "never", "workspace-write", false},
		{"always approves all non-exempt tools", "grep", "always", "workspace-write", true},
		{"on-request approves mutating tools", "task", "on-request", "workspace-write", true},
		{"auto skips mutating tools", "task", "auto", "workspace-write", false},
	}
	for _, tc := range cases {
		if got := RequiresApproval(tc.toolName, tc.approvalPolicy, tc.sandboxMode); got != tc.want {
			t.Fatalf("%s requires approval got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestBlockedByApprovalNever(t *testing.T) {
	if !BlockedByApprovalNever("write", false, false) {
		t.Fatalf("file mutation should be blocked under approval never")
	}
	if BlockedByApprovalNever("grep", false, false) {
		t.Fatalf("read-only builtin should not be blocked under approval never")
	}
	if !BlockedByApprovalNever("mcp__docs__lookup", false, false) {
		t.Fatalf("unavailable mcp manager should block mcp tools")
	}
	if !BlockedByApprovalNever("mcp__docs__lookup", true, false) {
		t.Fatalf("mcp tool without read-only hint should be blocked")
	}
	if BlockedByApprovalNever("mcp__docs__lookup", true, true) {
		t.Fatalf("host-authorized read-only MCP tool should not be blocked")
	}
}

func TestGenericToolProgressAndStatus(t *testing.T) {
	if ShouldRecordGenericToolProgress("request_user_input") || ShouldRecordGenericToolProgress("task") || ShouldRecordGenericToolProgress(" ") {
		t.Fatalf("generic progress should skip user input, subagent, and blank tools")
	}
	if !ShouldRecordGenericToolProgress("grep") {
		t.Fatalf("generic progress should include ordinary tools")
	}
	statuses := map[string]string{
		"completed":   "success",
		"failed":      "error",
		"killed":      "error",
		"aborted":     "error",
		"interrupted": "error",
		"running":     "running",
	}
	for input, want := range statuses {
		if got := ToolProgressStatus(input); got != want {
			t.Fatalf("%s progress got %q want %q", input, got, want)
		}
	}
}

func TestToolSchemaNameSetAndNotAdvertisedOutput(t *testing.T) {
	names := ToolSchemaNameSet([]domainmodel.ToolSchema{
		{Name: "read"},
		{Name: "grep"},
	})
	if !names["read"] || !names["grep"] || names["write"] {
		t.Fatalf("tool schema name set mismatch: %#v", names)
	}
	output := ToolNotAdvertisedOutput("write")
	if stringField(output, "code") != "tool_not_advertised" || stringField(output, "error") != "write is not advertised by active tool policy" {
		t.Fatalf("not advertised output mismatch: %#v", output)
	}
}

func TestLiveToolNamesForSecurityContextFailsClosedForHighRiskSource(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_1"),
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	source := nonContextualLiveToolSource{names: []string{
		"mcp__analytix_funds__count_case_rows",
		"mcp__analytix-fund-analysis__run_full_case_analysis",
		"mcp__docs__lookup",
	}, readOnly: map[string]bool{"mcp__docs__lookup": true}}
	if got := LiveToolNamesForSecurityContext(source, context); !reflect.DeepEqual(got, []string{"mcp__docs__lookup"}) {
		t.Fatalf("non-contextual source exposed high-risk tools: %#v", got)
	}
	if got := LiveToolNamesForSecurityContext(source, domainsecurity.TurnSecurityContext{}); len(got) != 0 {
		t.Fatalf("invalid turn context exposed live MCP tools: %#v", got)
	}
}

func TestAuditOnlyV1CannotAdvertiseAnyLiveTool(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	source := contextualLiveToolSource{names: []string{"read_file", "mcp__docs__lookup"}, readOnly: map[string]bool{"mcp__docs__lookup": true}}
	if tools := LiveToolNamesForSecurityContext(source, legacy); len(tools) != 0 {
		t.Fatalf("audit-only V1 advertised live tools: %#v", tools)
	}
}

func TestLiveToolNamesForSecurityContextQuarantinesArtifactToolsFromContextualSource(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1,
		IssuedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	source := contextualLiveToolSource{names: []string{
		"mcp__analytix_funds__count_case_rows",
		"mcp__analytix_funds__analyze_account_flows",
		"mcp__analytix_funds__run_full_case_analysis",
		"mcp__spoofed__create_case_notebook",
		"mcp__spoofed__export_cleaned_case_data",
		"mcp__server__generate_bundle",
		"mcp__docs__lookup",
	}, readOnly: map[string]bool{
		"mcp__analytix_funds__count_case_rows":       true,
		"mcp__analytix_funds__analyze_account_flows": true,
		"mcp__docs__lookup":                          true,
	}}
	want := []string{"mcp__analytix_funds__analyze_account_flows", "mcp__server__generate_bundle", "mcp__docs__lookup"}
	if got := LiveToolNamesForSecurityContext(source, context); !reflect.DeepEqual(got, want) {
		t.Fatalf("context-aware source exposed host-quarantined artifact tools: got=%#v want=%#v", got, want)
	}
}

func TestLiveToolNamesForCaseContextKeepsOrdinaryMCPRegardlessOfMutationPolicy(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1,
		IssuedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	source := contextualLiveToolSource{
		names:    []string{"mcp__server__query", "mcp__server__generate_bundle", "mcp__server__missing_annotation"},
		readOnly: map[string]bool{"mcp__server__query": true, "mcp__server__generate_bundle": false},
	}
	if got := LiveToolNamesForSecurityContext(source, context); !reflect.DeepEqual(got, source.names) {
		t.Fatalf("case context replaced ordinary MCP tools: %#v", got)
	}
}

func toolCatalogCaseContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func TestPersistableToolOutputRedactsUserInputAnswers(t *testing.T) {
	output := map[string]any{
		"status":  "submitted",
		"answers": []map[string]string{{"id": "a", "value": "secret"}, {"id": "b", "value": "private"}},
	}
	persisted, ok := PersistableToolOutput("request_user_input", output).(map[string]any)
	if !ok {
		t.Fatalf("persistable output should remain an object")
	}
	if _, ok := persisted["answers"]; ok {
		t.Fatalf("persistable user-input output leaked answers: %#v", persisted)
	}
	if got := persisted["answerCount"]; got != float64(2) {
		t.Fatalf("answerCount mismatch: %#v", persisted)
	}
	if _, ok := output["answers"]; !ok {
		t.Fatalf("source output should not be mutated")
	}
	if got := PersistableToolOutput("grep", output); !reflect.DeepEqual(got, output) {
		t.Fatalf("non-user-input output should not be changed")
	}
}

func TestCancelledOutputDistinguishesTimeout(t *testing.T) {
	cancelled := CancelledOutput("grep", "call_1", context.Canceled)
	if stringField(cancelled, "code") != "tool_cancelled" || stringField(cancelled, "toolName") != "grep" || stringField(cancelled, "callId") != "call_1" {
		t.Fatalf("cancelled output mismatch: %#v", cancelled)
	}
	timedOut := CancelledOutput("grep", "call_1", context.DeadlineExceeded)
	if stringField(timedOut, "code") != "tool_timeout" {
		t.Fatalf("timeout output mismatch: %#v", timedOut)
	}
}
