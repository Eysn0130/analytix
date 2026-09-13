package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type caseFundScopeMCPStub struct {
	runtimeToolSchemaMCPStub
}

func (m caseFundScopeMCPStub) LiveToolsForSecurityContext(domainsecurity.TurnSecurityContext) []string {
	return append([]string(nil), m.live...)
}

func (m caseFundScopeMCPStub) ToolReadOnlyHint(string) bool {
	return true
}

type delegatedManifestCountingProvider struct {
	calls atomic.Int64
}

func (*delegatedManifestCountingProvider) RequiresDurablePipelineStagesV1() {}

func (p *delegatedManifestCountingProvider) Stream(context.Context, domainmodel.Request) (domainmodel.Result, error) {
	p.calls.Add(1)
	return domainmodel.Result{}, nil
}

func TestSubagentSchemaSwapBeforeFirstProviderDispatchRejected(t *testing.T) {
	stringSchema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`)
	integerSchema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"integer"}},"required":["q"],"additionalProperties":false}`)
	output := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	identity := delegatedServerManifestIdentity(t, 7, "schema-instance")
	expectedAdvertisement := toolcatalogapp.MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: stringSchema, OutputSchema: output,
		ReadOnly: true, ConnectionEpoch: 7, ServerIdentity: identity,
	}
	actualAdvertisement := expectedAdvertisement
	actualAdvertisement.InputSchema = integerSchema
	expectedSchemas := []domainmodel.ToolSchema{{
		Name: expectedAdvertisement.Name, Description: expectedAdvertisement.Description,
		Parameters: stringSchema, OutputSchema: output, Source: "mcp",
	}}
	assertDelegatedManifestRejectedBeforeProvider(t, expectedAdvertisement, actualAdvertisement, expectedSchemas, "subagent_tool_schema_mismatch")
}

func TestSubagentMCPIdentityEpochSwapWithSameSchemaRejected(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	expectedAdvertisement := toolcatalogapp.MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: closed, OutputSchema: closed,
		ReadOnly: true, ConnectionEpoch: 7, ServerIdentity: delegatedServerManifestIdentity(t, 7, "identity-before"),
	}
	actualAdvertisement := expectedAdvertisement
	actualAdvertisement.ConnectionEpoch = 8
	actualAdvertisement.ServerIdentity = delegatedServerManifestIdentity(t, 8, "identity-after")
	expectedSchemas := []domainmodel.ToolSchema{{
		Name: expectedAdvertisement.Name, Description: expectedAdvertisement.Description,
		Parameters: closed, OutputSchema: closed, Source: "mcp",
	}}
	assertDelegatedManifestRejectedBeforeProvider(t, expectedAdvertisement, actualAdvertisement, expectedSchemas, "subagent_mcp_authority_mismatch")
}

func TestSubagentMCPReadOnlyPolicySwapWithSameSchemaRejected(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	expectedAdvertisement := toolcatalogapp.MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: closed, OutputSchema: closed,
		ReadOnly: true, ConnectionEpoch: 5, ServerIdentity: delegatedServerManifestIdentity(t, 5, "policy-instance"),
	}
	actualAdvertisement := expectedAdvertisement
	actualAdvertisement.ReadOnly = false
	expectedSchemas := []domainmodel.ToolSchema{{
		Name: expectedAdvertisement.Name, Description: expectedAdvertisement.Description,
		Parameters: closed, OutputSchema: closed, Source: "mcp",
	}}
	assertDelegatedManifestRejectedBeforeProvider(t, expectedAdvertisement, actualAdvertisement, expectedSchemas, "subagent_mcp_authority_mismatch")
}

func TestSubagentToolRemovalBeforeFirstProviderDispatchRejected(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	expectedAdvertisement := toolcatalogapp.MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: closed, OutputSchema: closed,
		ReadOnly: true, ConnectionEpoch: 6, ServerIdentity: delegatedServerManifestIdentity(t, 6, "removal-instance"),
	}
	expectedSchemas := []domainmodel.ToolSchema{{
		Name: expectedAdvertisement.Name, Description: expectedAdvertisement.Description,
		Parameters: closed, OutputSchema: closed, Source: "mcp",
	}}
	assertDelegatedManifestRejectedBeforeProvider(t, expectedAdvertisement, toolcatalogapp.MCPToolAdvertisementV1{}, expectedSchemas, "subagent_tool_schema_mismatch")
}

func TestCaseFundChildDelegationRejectsNonFundsScope(t *testing.T) {
	workspace := writeDelegatedCaseFundBinding(t)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-fund-child", TurnID: "turn-case-fund-child", WorkspaceRealPath: workspace,
		ContextEpoch: 1, SourceManifestHash: domainsecurity.SHA256Hex([]byte("case-fund-child-source")),
		IssuedAt: time.Date(2026, 7, 21, 1, 0, 0, 0, time.UTC),
	})
	fundsTool := "mcp__analytix_funds__count_case_rows"
	countingProvider := &delegatedManifestCountingProvider{}
	handler := &runtimeServerHandler{
		provider: countingProvider,
		mcp: caseFundScopeMCPStub{runtimeToolSchemaMCPStub: runtimeToolSchemaMCPStub{
			tools: []string{fundsTool}, live: []string{fundsTool},
		}},
	}
	input := runtimeAgentLoopInput{
		ThreadID: "thread-case-fund-child", TurnID: "turn-case-fund-child",
		Request:    startRuntimeTurnRequest{Prompt: "请分析当前案件账户资金流水"},
		ProviderID: "provider", Model: "model", Workspace: workspace,
		ApprovalPolicy: "never", SandboxMode: "workspace-write",
		ToolScope: []string{"read", "grep", "mcp__memory__search"}, SubagentDepth: 1,
		SecurityContext: securityContext,
	}
	for name, run := range map[string]func() (runtimeAgentLoopResult, error){
		"initial": func() (runtimeAgentLoopResult, error) {
			return handler.runRuntimeAgentLoop(context.Background(), input)
		},
		"approval or user-input continuation": func() (runtimeAgentLoopResult, error) {
			return handler.runRuntimeAgentLoopWithMessages(
				context.Background(),
				input,
				[]provider.Message{{Role: "user", Content: input.Request.Prompt}},
			)
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := run()
			if err != nil {
				t.Fatal(err)
			}
			if result.AssistantText != apploop.CaseFundSourceUnavailableAnswer() {
				t.Fatalf("delegated non-funds scope must use the fixed host boundary: %#v", result)
			}
		})
	}
	if calls := countingProvider.calls.Load(); calls != 0 {
		t.Fatalf("delegated non-funds scope reached provider.Stream: %d", calls)
	}
}

func assertDelegatedManifestRejectedBeforeProvider(
	t *testing.T,
	expectedAdvertisement toolcatalogapp.MCPToolAdvertisementV1,
	actualAdvertisement toolcatalogapp.MCPToolAdvertisementV1,
	expectedSchemas []domainmodel.ToolSchema,
	want string,
) {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-delegated-manifest"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: 1,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("delegated-source-manifest")),
		IssuedAt:           time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC),
	})
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "items": []any{},
		"securityContext": turnsecurityapp.PublicRecord(securityContext),
		"createdAt":       time.Now().UTC().Format(time.RFC3339Nano),
	}, "provider", nil); err != nil {
		t.Fatal(err)
	}
	thread, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	toolScope := []string{expectedAdvertisement.Name}
	expectedManifest, err := toolcatalogapp.BuildDelegatedToolManifestV1(toolScope, expectedSchemas, []toolcatalogapp.MCPToolAdvertisementV1{expectedAdvertisement})
	if err != nil {
		t.Fatal(err)
	}
	countingProvider := &delegatedManifestCountingProvider{}
	actualAdvertisements := []toolcatalogapp.MCPToolAdvertisementV1(nil)
	actualToolNames := []string(nil)
	if actualAdvertisement.Name != "" {
		actualAdvertisements = []toolcatalogapp.MCPToolAdvertisementV1{actualAdvertisement}
		actualToolNames = []string{actualAdvertisement.Name}
	}
	handler := &runtimeServerHandler{
		store: store, provider: countingProvider,
		mcp: runtimeToolSchemaMCPStub{
			tools: actualToolNames, advertisements: actualAdvertisements,
		},
	}
	_, loopErr := handler.runRuntimeAgentLoopWithMessages(context.Background(), runtimeAgentLoopInput{
		ThreadID: threadID, TurnID: turnID, Thread: thread,
		Request:    startRuntimeTurnRequest{Prompt: "Lookup the delegated docs source."},
		ProviderID: "provider", Model: "model", Workspace: workspace,
		ApprovalPolicy: "never", SandboxMode: "workspace-write",
		ToolScope: toolScope, SubagentDepth: 1, SecurityContext: securityContext,
		DelegatedToolManifest: expectedManifest,
	}, []provider.Message{{Role: "user", Content: "Lookup the delegated docs source."}})
	if loopErr == nil || loopErr.Error() != want {
		t.Fatalf("delegated manifest mismatch error=%v want=%s", loopErr, want)
	}
	if calls := countingProvider.calls.Load(); calls != 0 {
		t.Fatalf("delegated manifest mismatch reached provider.Stream: %d", calls)
	}
}

func delegatedServerManifestIdentity(t *testing.T, epoch uint64, instance string) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte(instance)), epoch,
	)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func writeDelegatedCaseFundBinding(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	realPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(realPath, ".analytix")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": realPath, "caseId": "case_fund_child", "source": "analytix-data-analysis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "case-project.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return realPath
}
