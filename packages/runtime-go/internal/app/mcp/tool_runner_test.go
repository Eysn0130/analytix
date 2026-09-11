package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type toolCallerStub struct {
	name       string
	allowStale bool
	args       []map[string]any
	result     map[string]any
}

func (s *toolCallerStub) CallTool(name string, allowStale bool, args ...map[string]any) map[string]any {
	s.name = name
	s.allowStale = allowStale
	s.args = args
	if s.result != nil {
		return s.result
	}
	return map[string]any{"executed": true}
}

type contextToolCallerStub struct {
	toolCallerStub
	contextCalled bool
}

type boundToolCallerStub struct {
	toolCallerStub
	boundCalled    bool
	epoch          uint64
	securityDigest string
	grantID        string
	hostErr        error
}

type fileWritingBoundToolCallerStub struct {
	toolCallerStub
	calls      int
	sideEffect bool
}

func (stub *fileWritingBoundToolCallerStub) CallToolSecurityBoundContext(_ context.Context, name string, allowStale bool, _ domainmcp.HostContextEnvelope, args ...map[string]any) map[string]any {
	stub.calls++
	stub.sideEffect = true
	return stub.CallTool(name, allowStale, args...)
}

func (s *boundToolCallerStub) CallToolSecurityBoundContext(_ context.Context, name string, allowStale bool, envelope domainmcp.HostContextEnvelope, args ...map[string]any) map[string]any {
	s.boundCalled = true
	securityContext, grant, err := envelope.Authority()
	s.hostErr = err
	s.epoch = grant.ConnectionEpoch
	s.grantID = grant.GrantID
	s.securityDigest = securityContext.ContextDigest
	return s.CallTool(name, allowStale, args...)
}

func (s *contextToolCallerStub) CallToolContext(ctx context.Context, name string, allowStale bool, args ...map[string]any) map[string]any {
	s.contextCalled = true
	return s.CallTool(name, allowStale, args...)
}

func TestExecuteToolUsesTypedHostContextEnvelope(t *testing.T) {
	caller := &boundToolCallerStub{}
	input := toolRunInputForTest(t, "mcp__analytix_funds__lookup", map[string]any{"query": "hello"})
	output, isError := ExecuteTool(context.Background(), caller, input)
	if isError || output["executed"] != true {
		t.Fatalf("unexpected output=%#v isError=%t", output, isError)
	}
	if caller.name != "mcp__analytix_funds__lookup" || !caller.allowStale || len(caller.args) != 1 || !caller.boundCalled || caller.hostErr != nil ||
		caller.epoch != 2 || caller.securityDigest != input.SecurityContext.ContextDigest || caller.grantID != input.ExecutionGrant.GrantID {
		t.Fatalf("unexpected call: %#v", caller)
	}
	arg := caller.args[0]
	if arg["query"] != "hello" || len(arg) != 1 {
		t.Fatalf("host context leaked into provider arguments: %#v", arg)
	}
	if outcome, ok := output["toolOutcome"].(map[string]any); !ok || outcome["safeToAnswer"] != false || outcome["sourceAssertionsAuthoritative"] != false {
		t.Fatalf("host-normalized ToolOutcomeV1 missing: %#v", output)
	}
}

func TestPendingExecutionGrantNeverReachesMCP(t *testing.T) {
	caller := &boundToolCallerStub{}
	input := toolRunInputForTest(t, "mcp__analytix_funds__lookup", map[string]any{"query": "blocked"})
	original := input.ExecutionGrant
	input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: input.SecurityContext, Provider: original.Provider, ServerIdentity: original.ServerIdentity,
		ToolName: original.ToolName, ToolCallID: original.ToolCallID, ConnectionEpoch: original.ConnectionEpoch,
		ArgsHash: original.ArgsHash, SchemaHash: original.SchemaHash, ScopeHash: original.ScopeHash,
		ReadOnly: original.ReadOnly, ApprovalState: "pending", IssuedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
	})
	output, isError := ExecuteTool(context.Background(), caller, input)
	if !isError || output["code"] != "mcp_execution_approval_pending" || output["executed"] != false || caller.boundCalled || len(caller.args) != 0 {
		t.Fatalf("pending grant reached MCP execution: output=%#v caller=%#v", output, caller)
	}
}

func TestNonCanonicalProviderCallIdentityNeverReachesMCPTransport(t *testing.T) {
	for _, callID := range []string{
		"provider_call_6222020202020202020",
		" " + mcpTestHostToolCallID(t, "whitespace") + " ",
	} {
		caller := &boundToolCallerStub{}
		input := toolRunInputForTest(t, "mcp__analytix_funds__lookup", map[string]any{"query": "blocked"})
		original := input.ExecutionGrant
		input.Call.ID = callID
		input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
			Context: input.SecurityContext, Provider: original.Provider, ServerIdentity: original.ServerIdentity,
			ToolName: original.ToolName, ToolCallID: callID, ConnectionEpoch: original.ConnectionEpoch,
			ArgsHash: original.ArgsHash, SchemaHash: original.SchemaHash, ScopeHash: original.ScopeHash,
			ReadOnly: original.ReadOnly, ApprovalState: original.ApprovalState,
			IssuedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
		})
		output, isError := ExecuteTool(context.Background(), caller, input)
		body, _ := json.Marshal(output)
		if !isError || output["code"] != "mcp_execution_authority_invalid" || caller.boundCalled || len(caller.args) != 0 || strings.Contains(string(body), callID) {
			t.Fatalf("non-canonical provider identity reached MCP transport: call=%q output=%s caller=%#v", callID, body, caller)
		}
	}
}

func TestCaseContextOrdinaryMCPWriteUsesNormalApprovalGrant(t *testing.T) {
	for _, approvalState := range []string{"not_required", "approved"} {
		t.Run(approvalState, func(t *testing.T) {
			caller := &fileWritingBoundToolCallerStub{}
			input := toolRunInputForTest(t, "mcp__server__generate_bundle", map[string]any{})
			original := input.ExecutionGrant
			input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
				Context: input.SecurityContext, Provider: original.Provider, ServerIdentity: original.ServerIdentity,
				ToolName: original.ToolName, ToolCallID: original.ToolCallID, ConnectionEpoch: original.ConnectionEpoch,
				ArgsHash: original.ArgsHash, SchemaHash: original.SchemaHash, ScopeHash: original.ScopeHash,
				ReadOnly: false, ApprovalState: approvalState, IssuedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
			})
			output, isError := ExecuteTool(context.Background(), caller, input)
			if isError || output["executed"] != true || caller.calls != 1 || !caller.sideEffect {
				t.Fatalf("case context replaced an ordinary MCP write: state=%s output=%#v caller=%#v", approvalState, output, caller)
			}
		})
	}
}

func TestCaseDataMCPWriteNeverReachesTransport(t *testing.T) {
	for _, approvalState := range []string{"not_required", "approved"} {
		t.Run(approvalState, func(t *testing.T) {
			caller := &fileWritingBoundToolCallerStub{}
			input := toolRunInputForTest(t, "mcp__analytix_funds__generate_bundle", map[string]any{})
			original := input.ExecutionGrant
			input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
				Context: input.SecurityContext, Provider: original.Provider, ServerIdentity: original.ServerIdentity,
				ToolName: original.ToolName, ToolCallID: original.ToolCallID, ConnectionEpoch: original.ConnectionEpoch,
				ArgsHash: original.ArgsHash, SchemaHash: original.SchemaHash, ScopeHash: original.ScopeHash,
				ReadOnly: false, ApprovalState: approvalState, IssuedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
			})
			output, isError := ExecuteTool(context.Background(), caller, input)
			if !isError || output["code"] != "publication_receipt_required" || output["executed"] != false || caller.calls != 0 || caller.sideEffect {
				t.Fatalf("case-data MCP write reached transport: state=%s output=%#v caller=%#v", approvalState, output, caller)
			}
		})
	}
}

func TestWitnessedBoundaryClassifiesOrdinaryAndFundsMCPBeforeTransport(t *testing.T) {
	arguments := map[string]any{
		"_analytix": map[string]any{"caseId": "forged"},
	}
	ordinary := toolRunInputForTest(t, "mcp__docs__lookup", arguments)
	ordinary = toolRunInputWithContext(
		t,
		ordinary,
		mcpWitnessedBoundaryContext(t, ordinary.SecurityContext, time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)),
	)
	ordinaryCaller := &boundToolCallerStub{}
	output, isError := ExecuteTool(context.Background(), ordinaryCaller, ordinary)
	if !isError || output["code"] != "mcp_provider_authority_rejected" || ordinaryCaller.boundCalled {
		t.Fatalf("ordinary MCP did not pass per-call context admission before argument authority rejection: output=%#v caller=%#v", output, ordinaryCaller)
	}

	funds := toolRunInputForTest(t, "mcp__analytix_funds__lookup", arguments)
	funds = toolRunInputWithContext(t, funds, ordinary.SecurityContext)
	fundsCaller := &boundToolCallerStub{}
	output, isError = ExecuteTool(context.Background(), fundsCaller, funds)
	if !isError || output["code"] != "mcp_execution_authority_invalid" || fundsCaller.boundCalled {
		t.Fatalf("funds MCP escaped boundary-only context authority: output=%#v caller=%#v", output, fundsCaller)
	}
}

func TestOrdinaryMCPRejectsQuarantinedAndAuditOnlyContexts(t *testing.T) {
	base := toolRunInputForTest(t, "mcp__docs__lookup", map[string]any{})
	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: base.SecurityContext.ThreadID, TurnID: base.SecurityContext.TurnID,
		WorkspaceRealPath: base.SecurityContext.WorkspaceRealPath,
		ContextEpoch:      base.SecurityContext.ContextEpoch, IssuedAt: time.Date(2026, 7, 27, 11, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: base.SecurityContext.ThreadID, TurnID: base.SecurityContext.TurnID,
		WorkspaceRealPath: base.SecurityContext.WorkspaceRealPath,
		CaseID:            "case-v1", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-v1")),
		DatasetSnapshotID: "snapshot-v1", SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-v1")),
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 27, 11, 30, 0, 0, time.UTC),
	})
	for name, securityContext := range map[string]domainsecurity.TurnSecurityContext{
		"quarantined": quarantined,
		"audit-only":  legacy,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &boundToolCallerStub{}
			output, isError := ExecuteTool(context.Background(), caller, toolRunInputWithContext(t, base, securityContext))
			if !isError || output["code"] != "mcp_execution_authority_invalid" || caller.boundCalled {
				t.Fatalf("%s context reached MCP transport: output=%#v caller=%#v", name, output, caller)
			}
		})
	}
}

func TestExecuteToolRejectsProviderAuthorityEvenWhenSchemaCouldAllowIt(t *testing.T) {
	for _, key := range []string{"_analytix", "__analytix", "analytix_runtime_context"} {
		t.Run(key, func(t *testing.T) {
			caller := &boundToolCallerStub{}
			input := toolRunInputForTest(t, "mcp__demo__lookup", map[string]any{
				"query": "hello", key: map[string]any{"caseId": "forged", "contextEpoch": 999},
			})
			output, isError := ExecuteTool(context.Background(), caller, input)
			if !isError || output["code"] != "mcp_provider_authority_rejected" || caller.boundCalled {
				t.Fatalf("provider authority reached MCP transport: key=%s output=%#v caller=%#v", key, output, caller)
			}
		})
	}
}

func TestExecuteToolRejectsCallerWithoutConnectionEpochEnforcement(t *testing.T) {
	caller := &contextToolCallerStub{}
	output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, "mcp__demo__lookup", map[string]any{}))
	if !isError || caller.contextCalled || output["code"] != "mcp_execution_authority_unavailable" {
		t.Fatalf("unbound caller must fail closed, output=%#v caller=%#v", output, caller)
	}
}

func TestExecuteToolRequiresConnectionEpochAwareCaller(t *testing.T) {
	caller := &toolCallerStub{}
	input := toolRunInputForTest(t, "mcp__analytix_funds__lookup", map[string]any{})
	output, isError := ExecuteTool(context.Background(), caller, input)
	if !isError || output["executed"] != false || output["code"] != "mcp_execution_authority_unavailable" || len(caller.args) != 0 || output["toolOutcome"] == nil {
		t.Fatalf("caller without epoch enforcement must fail before execution: output=%#v caller=%#v", output, caller)
	}
}

func TestExecuteToolRejectsGrantToolAndExecutionContextSubstitution(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*ToolRunInput)
	}{
		{"tool name", func(input *ToolRunInput) { input.ToolName = "mcp__analytix_funds__other" }},
		{"call name", func(input *ToolRunInput) { input.Call.Name = "mcp__analytix_funds__other" }},
		{"workspace", func(input *ToolRunInput) { input.Workspace = "/tmp/other" }},
		{"thread", func(input *ToolRunInput) { input.ThreadID = "thread-other" }},
		{"turn", func(input *ToolRunInput) { input.TurnID = "turn-other" }},
		{"arguments", func(input *ToolRunInput) { input.Arguments["query"] = "substituted" }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			caller := &boundToolCallerStub{}
			input := toolRunInputForTest(t, "mcp__analytix_funds__lookup", map[string]any{"query": "original"})
			test.mutate(&input)
			output, isError := ExecuteTool(context.Background(), caller, input)
			if !isError || output["code"] != "mcp_execution_authority_invalid" || caller.boundCalled {
				t.Fatalf("substitution reached MCP execution: output=%#v caller=%#v", output, caller)
			}
		})
	}
}

func TestExecuteToolPreservesExactProviderArguments(t *testing.T) {
	caller := &boundToolCallerStub{}
	input := toolRunInputForRawTest(t, "mcp__analytix_funds__lookup", json.RawMessage(`{"amount":9007199254740993,"ratio":1.2300,"account":"0012300"}`))
	output, isError := ExecuteTool(context.Background(), caller, input)
	if isError || output["executed"] != true || len(caller.args) != 1 {
		t.Fatalf("exact MCP argument execution failed: output=%#v caller=%#v", output, caller)
	}
	amount, amountOK := caller.args[0]["amount"].(json.Number)
	ratio, ratioOK := caller.args[0]["ratio"].(json.Number)
	if !amountOK || amount.String() != "9007199254740993" || !ratioOK || ratio.String() != "1.2300" || caller.args[0]["account"] != "0012300" {
		t.Fatalf("MCP arguments lost lexical precision: %#v", caller.args[0])
	}
}

func TestExecuteToolMarksNotExecutedAsError(t *testing.T) {
	caller := &boundToolCallerStub{toolCallerStub: toolCallerStub{result: map[string]any{"executed": false, "code": "mcp_tool_missing"}}}
	output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, "mcp__demo__missing", map[string]any{}))
	if !isError || output["code"] != "mcp_tool_missing" {
		t.Fatalf("unexpected not-executed result: %#v isError=%t", output, isError)
	}
}

func TestExecuteToolMarksMCPSemanticFailureAsError(t *testing.T) {
	caller := &boundToolCallerStub{toolCallerStub: toolCallerStub{result: map[string]any{
		"executed": true,
		"isError":  true,
		"code":     "mcp_semantic_failure",
		"result": map[string]any{
			"isError":   true,
			"errorCode": "PUBLICATION_RECEIPT_REQUIRED",
		},
	}}}
	output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, "mcp__demo__lookup", map[string]any{}))
	if !isError || output["code"] != "mcp_semantic_failure" {
		t.Fatalf("semantic MCP failure must not become a successful tool outcome: %#v isError=%t", output, isError)
	}
}

func TestWriteReportFalseProducesNoFile(t *testing.T) {
	for name, arguments := range map[string]map[string]any{
		"false":   {"write_report": false},
		"true":    {"write_report": true},
		"missing": {},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &fileWritingBoundToolCallerStub{}
			output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, "mcp__analytix_funds__run_full_case_analysis", arguments))
			if !isError || output["code"] != "publication_receipt_required" || output["executed"] != false || caller.calls != 0 || caller.sideEffect || len(caller.args) != 0 {
				t.Fatalf("quarantined report tool reached MCP execution: output=%#v caller=%#v", output, caller)
			}
		})
	}
}

func TestExecuteToolQuarantinesHiddenArtifactToolsAcrossServerAliases(t *testing.T) {
	for _, toolName := range []string{
		"mcp__analytix-fund-analysis__create_case_notebook",
		"mcp__spoofed_funds__export_cleaned_case_data",
		"mcp__spoofed_funds__run_full_case_analysis",
	} {
		t.Run(toolName, func(t *testing.T) {
			caller := &fileWritingBoundToolCallerStub{}
			output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, toolName, map[string]any{}))
			if !isError || output["code"] != "publication_receipt_required" || caller.calls != 0 {
				t.Fatalf("hidden artifact tool reached a remote server: output=%#v caller=%#v", output, caller)
			}
		})
	}
}

func TestExecuteToolKeepsRawEvidenceInProcessOnly(t *testing.T) {
	raw := json.RawMessage(`{"content":[],"structuredContent":{"amount":9007199254740993}}`)
	caller := &boundToolCallerStub{toolCallerStub: toolCallerStub{result: map[string]any{
		"executed": true,
		domainmcp.HostRawToolResultKey: domainmcp.LosslessToolResult{
			RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw),
		},
	}}}
	output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, "mcp__demo__query", map[string]any{}))
	lossless, ok := output[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if isError || !ok || string(lossless.RawResult) != string(raw) {
		t.Fatalf("lossless evidence carrier was not preserved in process: output=%#v", output)
	}
	body, err := json.Marshal(lossless)
	if err != nil || string(body) != "{}" {
		t.Fatalf("lossless evidence carrier must not serialize raw bytes: body=%s err=%v", body, err)
	}
}

func TestExecuteToolConsumesExactHostObservationWithoutPromotingRemoteAuthority(t *testing.T) {
	raw := json.RawMessage(`{
		"content":[],
		"structuredContent":{"ok":true},
		"safeToAnswer":false,
		"semanticStatus":"blocked",
		"blocker":{"code":"SOURCE_NOT_READY"},
		"partialCoverage":{"coverageStatus":"partial"},
		"evidenceReceipts":[{"receiptId":"remote-candidate"}],
		"_meta":{
			"analytix_evidence_ledger":{"status":"unsupported"},
			"analytix_tool_outcome":{
				"reportedSemanticStatus":"success",
				"caseId":"case-remote",
				"contextEpoch":99,
				"datasetSnapshotId":"snapshot-remote",
				"serverIdentity":"server-remote"
			}
		}
	}`)
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	lossless := domainmcp.LosslessToolResult{
		Value: map[string]any{"ok": true}, RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw),
	}
	observation, err := InspectToolResultOutput(lossless, schema)
	if err != nil {
		t.Fatal(err)
	}
	lossless.Observation = &observation
	caller := &boundToolCallerStub{toolCallerStub: toolCallerStub{result: map[string]any{
		"executed": true, "transportStatus": "success", "result": map[string]any{"ok": true},
		domainmcp.HostRawToolResultKey: lossless,
	}}}
	output, isError := ExecuteTool(context.Background(), caller, toolRunInputForTest(t, "mcp__demo__query", map[string]any{}))
	outcome, ok := output["toolOutcome"].(map[string]any)
	if !isError || !ok || outcome["safeToAnswer"] != false || outcome["sourceAssertionsAuthoritative"] != false ||
		outcome["semanticStatus"] != "blocked" || outcome["reportedSafeToAnswer"] != false ||
		outcome["reportedSemanticStatus"] != "success" || outcome["reportedCaseId"] != "case-remote" ||
		outcome["reportedDatasetSnapshotId"] != "snapshot-remote" || outcome["reportedServerIdentity"] != "server-remote" {
		t.Fatalf("exact host observation was lost or promoted: output=%#v", output)
	}
	candidates, _ := outcome["candidateEvidenceReceipts"].([]any)
	meta, _ := outcome["untrustedMeta"].(map[string]any)
	ledger, _ := meta["analytix_evidence_ledger"].(map[string]any)
	if len(candidates) != 1 || ledger["status"] != "unsupported" {
		t.Fatalf("host observation audit material was lost: outcome=%#v", outcome)
	}
	providerResult, _ := output["result"].(map[string]any)
	for _, key := range []string{"safeToAnswer", "semanticStatus", "blocker", "partialCoverage", "evidenceReceipts", "_meta"} {
		if _, leaked := providerResult[key]; leaked {
			t.Fatalf("host-private MCP assertion %q entered provider result: %#v", key, providerResult)
		}
	}
}

func toolRunInputForTest(t *testing.T, toolName string, arguments map[string]any) ToolRunInput {
	t.Helper()
	body, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	return toolRunInputForRawTest(t, toolName, body)
}

func toolRunInputForRawTest(t *testing.T, toolName string, body json.RawMessage) ToolRunInput {
	t.Helper()
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	policyDigest := domainsecurity.SHA256Hex([]byte("tool-runner-test-risk-policy"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("tool-runner-test-binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding("thread-1", "/tmp/work", domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	contextValue, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/tmp/work", CaseID: "case_1",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("tool-runner"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: now,
		PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := DecodeArguments(body)
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: mcpTestHostToolCallID(t, toolName), Name: toolName, Arguments: body}
	serverID := strings.SplitN(toolName, "__", 3)[1]
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, serverID, "1.0.0", domainsecurity.SHA256Hex([]byte("tool-runner-test-instance")), 2)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextValue, Provider: "provider-1", ServerIdentity: serverIdentity, ToolName: toolName,
		ToolCallID: call.ID, ConnectionEpoch: 2, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	return ToolRunInput{
		ToolName: toolName, Arguments: arguments, Workspace: "/tmp/work", ThreadID: contextValue.ThreadID, TurnID: contextValue.TurnID,
		SecurityContext: contextValue, ExecutionGrant: grant, Call: call,
	}
}

func toolRunInputWithContext(t *testing.T, input ToolRunInput, securityContext domainsecurity.TurnSecurityContext) ToolRunInput {
	t.Helper()
	grant := input.ExecutionGrant
	input.SecurityContext = securityContext
	input.Workspace = securityContext.WorkspaceRealPath
	input.ThreadID = securityContext.ThreadID
	input.TurnID = securityContext.TurnID
	input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: grant.Provider, ServerIdentity: grant.ServerIdentity,
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ConnectionEpoch: grant.ConnectionEpoch,
		ArgsHash: domainsecurity.CanonicalJSONHash(input.Call.Arguments), SchemaHash: grant.SchemaHash, ScopeHash: grant.ScopeHash,
		ReadOnly: grant.ReadOnly, ApprovalState: grant.ApprovalState,
		IssuedAt: time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC),
	})
	if err := domainsecurity.ValidateExecutionGrant(input.ExecutionGrant); err != nil {
		t.Fatal(err)
	}
	return input
}

func mcpWitnessedBoundaryContext(t *testing.T, base domainsecurity.TurnSecurityContext, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: base.ThreadID, TurnID: base.TurnID, WorkspaceRealPath: base.WorkspaceRealPath,
		ContextEpoch: base.ContextEpoch, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		quarantined.ThreadID,
		quarantined.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		quarantined.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: quarantined.ThreadID, TurnID: quarantined.TurnID, WorkspaceRealPath: quarantined.WorkspaceRealPath,
		TenantID: quarantined.TenantID, UserID: quarantined.UserID, CaseID: quarantined.CaseID,
		CaseBindingHash: quarantined.CaseBindingHash, DatasetSnapshotID: quarantined.DatasetSnapshotID,
		SourceManifestHash: quarantined.SourceManifestHash, ContextEpoch: quarantined.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: quarantined.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func TestIsToolName(t *testing.T) {
	if !IsToolName("mcp__demo__lookup") || IsToolName(" mcp__demo__lookup ") || IsToolName("MCP__demo__lookup") || IsToolName("read_file") {
		t.Fatal("MCP tool prefix detection failed")
	}
}

func TestDecodeArgumentsRejectsAmbiguousProviderJSON(t *testing.T) {
	for _, body := range []json.RawMessage{
		json.RawMessage(`{"account":"first","account":"second"}`),
		json.RawMessage(`{"account":"x"}{"account":"y"}`),
		json.RawMessage(`{"account":"\uD800"}`),
		json.RawMessage(`{"amount":1e10001}`),
	} {
		if _, err := DecodeArguments(body); err == nil {
			t.Fatalf("ambiguous provider arguments passed: %s", body)
		}
	}
}
