package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	processadapter "analytix.local/runtime-go/internal/adapters/outbound/process"
	apploop "analytix.local/runtime-go/internal/app/loop"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/provider"
)

func TestRuntimeToolSchemasUseMCPInputSchema(t *testing.T) {
	const toolName = "mcp__docs__lookup"
	schema := json.RawMessage(`{"additionalProperties":false,"properties":{"limit":{"type":"integer"},"query":{"type":"string"}},"required":["query"],"type":"object"}`)
	handler := &runtimeServerHandler{
		mcp: runtimeToolSchemaMCPStub{
			tools:   []string{toolName},
			schemas: map[string]json.RawMessage{toolName: schema},
		},
	}

	var mcpTool *providerToolSchemaView
	for _, tool := range handler.runtimeToolSchemasFor(true, nil, false, false) {
		if tool.Name != toolName {
			continue
		}
		view := providerToolSchemaView{
			name:        tool.Name,
			description: tool.Description,
			parameters:  append(json.RawMessage(nil), tool.Parameters...),
			source:      tool.Source,
		}
		mcpTool = &view
		break
	}
	if mcpTool == nil {
		t.Fatalf("expected provider-visible MCP tool schema for %s", toolName)
	}
	if mcpTool.source != "mcp" {
		t.Fatalf("MCP tool source = %q, want mcp", mcpTool.source)
	}
	if !jsonRawMessagesEqual(mcpTool.parameters, schema) {
		t.Fatalf("MCP provider schema should use cached/input schema, got %s want %s", mcpTool.parameters, schema)
	}
	if jsonRawMessagesEqual(mcpTool.parameters, json.RawMessage(`{"type":"object","additionalProperties":true}`)) {
		t.Fatalf("MCP provider schema fell back to generic parameters: %s", mcpTool.parameters)
	}
}

func TestRuntimeEmptyDelegatedScopeAdvertisesNeitherBuiltinNorMCPTools(t *testing.T) {
	const toolName = "mcp__docs__lookup"
	handler := &runtimeServerHandler{
		mcp: runtimeToolSchemaMCPStub{
			tools: []string{toolName},
			schemas: map[string]json.RawMessage{
				toolName: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			},
		},
	}

	tools := handler.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(
		true,
		nil,
		true,
		false,
		"read the workspace with docs",
		false,
		[]string{toolName},
	)
	if len(tools) != 0 {
		t.Fatalf("empty delegated authority regained builtin or MCP schemas: %#v", runtimeToolSchemaNames(tools))
	}
}

func TestRuntimeToolSchemaHashCanonicalizesSetLikeSchemaFields(t *testing.T) {
	left := toolcatalogapp.ToolSchemaHash([]provider.ToolSchema{
		{
			Name:        "read_file",
			Description: "Read",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"mode":{"enum":["b","a"],"type":["null","string"]},"path":{"type":"string"}},"required":["path","mode"],"dependentRequired":{"mode":["path","mode"]}}`),
		},
	})
	right := toolcatalogapp.ToolSchemaHash([]provider.ToolSchema{
		{
			Name:        "read_file",
			Description: "Read",
			Parameters:  json.RawMessage(`{"required":["mode","path"],"dependentRequired":{"mode":["mode","path"]},"properties":{"path":{"type":"string"},"mode":{"type":["string","null"],"enum":["a","b"]}},"type":"object"}`),
		},
	})

	if left != right {
		t.Fatalf("subagent tool schema identity hash should ignore set-like schema field order: %s != %s", left, right)
	}
}

func TestRuntimeCommandDiagnosticsReportDeveloperTools(t *testing.T) {
	diagnostics := terminalapp.CommandDiagnosticsFor([]string{
		"go version",
		"definitely-not-an-analytix-command --version",
	}, 2*time.Second, processadapter.NewCommandProbe(), processadapter.HomeDir())
	if len(diagnostics) != 2 {
		t.Fatalf("command diagnostics length = %d, want 2: %#v", len(diagnostics), diagnostics)
	}
	byBinary := map[string]map[string]any{}
	for _, raw := range diagnostics {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("command diagnostic is not an object: %#v", raw)
		}
		byBinary[stringField(record, "binary")] = record
	}
	goRecord := byBinary["go"]
	if goRecord == nil || !boolField(goRecord, "found") || !strings.Contains(stringField(goRecord, "output"), "go version") {
		t.Fatalf("go command diagnostic should report the running Go toolchain: %#v", goRecord)
	}
	missing := byBinary["definitely-not-an-analytix-command"]
	if missing == nil || boolField(missing, "found") || stringField(missing, "error") != "not found" {
		t.Fatalf("missing command diagnostic should be explicit: %#v", missing)
	}
	if strings.Contains(stringField(goRecord, "output"), "/Users/") {
		t.Fatalf("command diagnostic output must not expose absolute home paths: %#v", goRecord)
	}
}

func TestRuntimeToolContractsMatchProviderVisibleSurface(t *testing.T) {
	const toolName = "mcp__docs__lookup"
	schema := json.RawMessage(`{"additionalProperties":false,"properties":{"limit":{"type":"integer"},"query":{"type":"string"}},"required":["query"],"type":"object"}`)
	handler := &runtimeServerHandler{
		mcp: runtimeToolSchemaMCPStub{
			tools:   []string{toolName},
			schemas: map[string]json.RawMessage{toolName: schema},
		},
	}

	visibleTools := handler.runtimeToolSchemasFor(false, nil, false, false)
	contracts := handler.runtimeToolContractDiagnostics()
	if len(contracts) != len(visibleTools) {
		t.Fatalf("tool contract count = %d, provider-visible tools = %d\ncontracts=%#v\ntools=%#v", len(contracts), len(visibleTools), contracts, visibleTools)
	}

	visibleByName := make(map[string]provider.ToolSchema, len(visibleTools))
	for _, tool := range visibleTools {
		visibleByName[tool.Name] = tool
	}
	for _, raw := range contracts {
		contract, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tool contract is not an object: %#v", raw)
		}
		if _, ok := contract["snipHint"]; ok {
			t.Fatalf("tool contract must not expose runtime-only snip hints: %#v", contract)
		}
		name := stringField(contract, "name")
		tool, ok := visibleByName[name]
		if !ok {
			t.Fatalf("tool contract %q has no provider-visible schema: %#v", name, contract)
		}
		if stringField(contract, "description") != tool.Description {
			t.Fatalf("%s description drift\ncontract=%q\nprovider=%q", name, stringField(contract, "description"), tool.Description)
		}
		if !reflect.DeepEqual(contract["inputSchema"], toolcatalogapp.CanonicalToolParameters(tool.Parameters)) {
			t.Fatalf("%s input schema drift\ncontract=%#v\nprovider=%#v", name, contract["inputSchema"], toolcatalogapp.CanonicalToolParameters(tool.Parameters))
		}
		if stringField(contract, "toolKind") != toolcatalogapp.ToolContractKind(name) {
			t.Fatalf("%s toolKind drift: %#v", name, contract)
		}
		if stringField(contract, "toolPolicy") != toolcatalogapp.ToolContractPolicy(name) {
			t.Fatalf("%s toolPolicy drift: %#v", name, contract)
		}
	}

	mcpContractFound := false
	for _, raw := range contracts {
		contract := raw.(map[string]any)
		if stringField(contract, "name") == toolName {
			mcpContractFound = true
			if stringField(contract, "providerKind") != "mcp" || stringField(contract, "providerId") != "mcp" {
				t.Fatalf("MCP tool contract provider drift: %#v", contract)
			}
		}
	}
	if !mcpContractFound {
		t.Fatalf("expected MCP tool contract for %s in %#v", toolName, contracts)
	}
}

func TestRuntimeToolContractDiagnosticsExcludeCacheOnlyMCPHints(t *testing.T) {
	const cachedTool = "mcp__docs__cached_only"
	handler := &runtimeServerHandler{
		mcp: runtimeToolSchemaMCPStub{
			tools: []string{cachedTool},
			live:  []string{},
			schemas: map[string]json.RawMessage{
				cachedTool: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			},
		},
	}
	for _, raw := range handler.runtimeToolContractDiagnostics() {
		contract, _ := raw.(map[string]any)
		if stringField(contract, "name") == cachedTool {
			t.Fatalf("non-executable cache hint was reported as a live tool contract: %#v", contract)
		}
	}
}

func TestRuntimeDefaultSystemPromptCarriesProviderModelIdentity(t *testing.T) {
	prompt := apploop.DefaultSystemPrompt("deepseek", "deepseek-v4-pro")

	if !strings.Contains(prompt, "provider=deepseek") || !strings.Contains(prompt, "model=deepseek-v4-pro") {
		t.Fatalf("default system prompt must carry provider/model metadata: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not claim to be Claude or Anthropic") {
		t.Fatalf("default system prompt must guard vendor identity drift: %q", prompt)
	}
}

func TestRuntimeDefaultSystemPromptKeepsFilesystemHintByteStable(t *testing.T) {
	prompt := apploop.DefaultRuntimeSystemPrompt("deepseek", "deepseek-v4-pro")
	if !strings.Contains(prompt, "use ~ as the home-directory reference") ||
		!strings.Contains(prompt, "~/Desktop") {
		t.Fatalf("default system prompt lost the portable filesystem hint: %q", prompt)
	}
	for _, forbidden := range []string{"Current local home directory is", "/Users/", "\\Users\\"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("default system prompt embedded host-specific path %q: %q", forbidden, prompt)
		}
	}
}

func TestRuntimeToolSchemasForPromptScopesMCPAutoSafety(t *testing.T) {
	handler := &runtimeServerHandler{
		mcpSearch: runtimeMCPSearchSettings{
			Enabled:                true,
			Mode:                   "auto",
			AutoThresholdToolCount: 2,
			TopKDefault:            1,
			TopKMax:                3,
		},
		mcp: runtimeToolSchemaMCPStub{
			tools: []string{
				"mcp__docs__lookup",
				"mcp__docs__search",
				"mcp__mail__send",
			},
			search: map[string][]string{
				"docs": []string{"mcp__docs__lookup", "mcp__docs__search"},
			},
		},
	}

	for _, prompt := range []string{"你是什么大模型", "你是什么模型", "你好", "这个问题怎么理解", "简单总结一下"} {
		tools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(true, nil, false, false, prompt))
		if len(tools) != 0 {
			t.Fatalf("direct lightweight prompt %q should use the tool-free profile, got %v", prompt, tools)
		}
	}

	docsTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(true, nil, false, false, "docs"))
	if !containsRuntimeString(docsTools, "mcp__docs__lookup") {
		t.Fatalf("matching MCP search result should be advertised: %v", docsTools)
	}
	if containsRuntimeString(docsTools, "mcp__docs__search") || containsRuntimeString(docsTools, "mcp__mail__send") {
		t.Fatalf("MCP auto safety should cap advertised tools to topK: %v", docsTools)
	}
}

func TestRuntimeToolSchemasPinMentionedComputerUseServer(t *testing.T) {
	handler := &runtimeServerHandler{
		mcpSearch: runtimeMCPSearchSettings{
			Enabled:                true,
			Mode:                   "auto",
			AutoThresholdToolCount: 2,
			TopKDefault:            1,
			TopKMax:                1,
		},
		mcp: runtimeToolSchemaMCPStub{
			tools: []string{
				"mcp__analytix-computer-use__get_app_state",
				"mcp__analytix-computer-use__select_text",
				"mcp__analytix_funds__run_case_sql",
			},
		},
	}

	naturalPromptTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(
		true,
		nil,
		false,
		false,
		"使用 Analytix Computer Use 操作 TextEdit，选中第一行",
	))
	if !containsRuntimeString(naturalPromptTools, "mcp__analytix-computer-use__get_app_state") ||
		!containsRuntimeString(naturalPromptTools, "mcp__analytix-computer-use__select_text") {
		t.Fatalf("mentioned Analytix Computer Use server tools should be pinned: %v", naturalPromptTools)
	}
	if containsRuntimeString(naturalPromptTools, "mcp__analytix_funds__run_case_sql") {
		t.Fatalf("pinning a mentioned server should not advertise unrelated MCP servers: %v", naturalPromptTools)
	}

	explicitToolPromptTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(
		true,
		nil,
		false,
		false,
		"必须调用 mcp__analytix-computer-use__select_text",
	))
	if !containsRuntimeString(explicitToolPromptTools, "mcp__analytix-computer-use__select_text") {
		t.Fatalf("explicitly mentioned MCP tool should be pinned: %v", explicitToolPromptTools)
	}
}

func TestRuntimeToolSchemasForPromptKeepsAgentToolsForWorkPrompts(t *testing.T) {
	handler := &runtimeServerHandler{
		subagents: subagentapp.DefaultProfileSettings(),
		skills: runtimeSkillCatalog{
			Enabled: true,
			Skills: []map[string]any{
				{"id": "audit", "name": "audit", "description": "Audit project code"},
			},
		},
		mcpSearch: runtimeMCPSearchSettings{
			Enabled:                true,
			Mode:                   "auto",
			AutoThresholdToolCount: 2,
			TopKDefault:            1,
			TopKMax:                3,
		},
		mcp: runtimeToolSchemaMCPStub{
			tools: []string{
				"mcp__docs__lookup",
				"mcp__docs__search",
				"mcp__mail__send",
			},
			search: map[string][]string{
				"请分析项目代码并使用 docs": []string{"mcp__docs__lookup", "mcp__docs__search"},
			},
		},
	}

	tools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(false, nil, false, false, "请分析项目代码并使用 docs"))
	for _, expected := range []string{"read", "grep", "user_input", "mcp__docs__lookup"} {
		if !containsRuntimeString(tools, expected) {
			t.Fatalf("work prompt should keep agent tool %s in %v", expected, tools)
		}
	}
	for _, hidden := range []string{"task", "parallel_tasks", "delegate_task", "run_skill"} {
		if containsRuntimeString(tools, hidden) {
			t.Fatalf("ordinary work prompt should not advertise heavy %s tool without an explicit cue: %v", hidden, tools)
		}
	}
	if containsRuntimeString(tools, "mcp__mail__send") {
		t.Fatalf("work prompt should still scope unrelated MCP tools: %v", tools)
	}

	subagentTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(false, nil, false, false, "请用子代理分解任务并审查项目代码"))
	for _, expected := range []string{"task", "parallel_tasks", "delegate_task"} {
		if !containsRuntimeString(subagentTools, expected) {
			t.Fatalf("explicit subagent prompt should advertise %s: %v", expected, subagentTools)
		}
	}
	childAgentTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(false, nil, false, false, "请创建 1 个后台子智能体执行只读检查"))
	for _, expected := range []string{"task", "parallel_tasks", "delegate_task"} {
		if !containsRuntimeString(childAgentTools, expected) {
			t.Fatalf("explicit child intelligent agent prompt should advertise %s: %v", expected, childAgentTools)
		}
	}
	backgroundTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(false, nil, false, false, "Inspect background via tools."))
	for _, expected := range []string{"wait", "bash_output", "kill_shell", "restart_job"} {
		if !containsRuntimeString(backgroundTools, expected) {
			t.Fatalf("background inspection prompt should advertise %s: %v", expected, backgroundTools)
		}
	}
	for _, hostOnly := range []string{"steer_job", "pause_job", "resume_job"} {
		if containsRuntimeString(backgroundTools, hostOnly) {
			t.Fatalf("host-only control %s was advertised to the provider: %v", hostOnly, backgroundTools)
		}
	}
	skillTools := runtimeToolSchemaNames(handler.runtimeToolSchemasForPrompt(false, nil, false, false, "Use deep review skill"))
	if !containsRuntimeString(skillTools, "run_skill") {
		t.Fatalf("explicit skill prompt should advertise run_skill: %v", skillTools)
	}
}

func runtimeToolSchemaNames(tools []provider.ToolSchema) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestRuntimeMCPArgumentsNeverInjectAnalytixCaseContextFromProviderData(t *testing.T) {
	handler := &runtimeServerHandler{}
	args := runtimeMCPArgumentsForTest(handler, runtimePendingToolCall{
		ThreadID:  "thr_case",
		TurnID:    "turn_case",
		Workspace: "/Users/sun/Projects/江苏航案件分析",
		Call: provider.ToolCall{
			Name: "mcp__analytix_funds__count_case_rows",
		},
	}, map[string]any{"table_name": "analysis_txn_detail_idx"})

	if _, ok := args["_analytix"]; ok {
		t.Fatalf("provider arguments gained host authority: %#v", args)
	}
	if len(args) != 1 || args["table_name"] != "analysis_txn_detail_idx" {
		t.Fatalf("existing MCP arguments must be preserved: %#v", args)
	}
}

func TestRuntimeMCPArgumentsStripForgedAuthorityForFundTools(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{
		"title":     "case thread",
		"workspace": "/Users/sun/Projects/江苏航案件分析",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store}
	args := runtimeMCPArgumentsForTest(handler, runtimePendingToolCall{
		ThreadID: stringField(thread, "id"),
		TurnID:   "turn_case",
		Call: provider.ToolCall{
			Name: "mcp__analytix_funds__run_full_case_analysis",
		},
	}, map[string]any{"write_report": true, "_analytix": map[string]any{"workspaceRoot": "/forged"}})

	if !reflect.DeepEqual(args, map[string]any{"write_report": true}) {
		t.Fatalf("forged provider authority survived argument sanitization: %#v", args)
	}
}

func TestRuntimeMCPArgumentsStripForgedAuthorityForOtherServers(t *testing.T) {
	args := map[string]any{"query": "hello", "_analytix": map[string]any{"caseId": "forged"}}
	handler := &runtimeServerHandler{}
	result := runtimeMCPArgumentsForTest(handler, runtimePendingToolCall{
		ThreadID:  "thr_case",
		TurnID:    "turn_case",
		Workspace: "/Users/sun/Projects/江苏航案件分析",
		Call: provider.ToolCall{
			Name: "mcp__docs__lookup",
		},
	}, args)

	if !reflect.DeepEqual(result, map[string]any{"query": "hello"}) {
		t.Fatalf("ordinary MCP server retained forged host authority: %#v", result)
	}
}

func runtimeMCPArgumentsForTest(handler *runtimeServerHandler, pending runtimePendingToolCall, args map[string]any) map[string]any {
	_ = handler.runtimePendingToolWorkspace(pending)
	return toolcatalogapp.MCPProviderArguments(args)
}

type providerToolSchemaView struct {
	name        string
	description string
	parameters  json.RawMessage
	source      string
}

type runtimeToolSchemaMCPStub struct {
	tools          []string
	live           []string
	schemas        map[string]json.RawMessage
	search         map[string][]string
	advertisements []toolcatalogapp.MCPToolAdvertisementV1
}

func (m runtimeToolSchemaMCPStub) Connect() {}

func (m runtimeToolSchemaMCPStub) Diagnostics() map[string]any {
	return map[string]any{}
}

func (m runtimeToolSchemaMCPStub) ServerDiagnostics() []any {
	return nil
}

func (m runtimeToolSchemaMCPStub) Search(query string) []string {
	return append([]string(nil), m.search[strings.TrimSpace(query)]...)
}

func (m runtimeToolSchemaMCPStub) CallTool(toolName string, approved bool, arguments ...map[string]any) map[string]any {
	return map[string]any{"executed": false}
}

func (m runtimeToolSchemaMCPStub) RefreshCatalog() map[string]any {
	return map[string]any{}
}

func (m runtimeToolSchemaMCPStub) RestartReconnect() map[string]any {
	return map[string]any{}
}

func (m runtimeToolSchemaMCPStub) Disconnect() {}

func (m runtimeToolSchemaMCPStub) Tools() []string {
	return append([]string(nil), m.tools...)
}

func (m runtimeToolSchemaMCPStub) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []domainmcp.ToolAdvertisementV1 {
	return append([]domainmcp.ToolAdvertisementV1(nil), m.advertisements...)
}

func (m runtimeToolSchemaMCPStub) LiveTools() []string {
	if m.live != nil {
		return append([]string(nil), m.live...)
	}
	return append([]string(nil), m.tools...)
}

func (m runtimeToolSchemaMCPStub) Prompts() []any {
	return nil
}

func (m runtimeToolSchemaMCPStub) Resources() []any {
	return nil
}

func (m runtimeToolSchemaMCPStub) ToolReadOnlyHint(toolName string) bool {
	return false
}

func (m runtimeToolSchemaMCPStub) ToolInputSchema(toolName string) (json.RawMessage, bool) {
	schema, ok := m.schemas[toolName]
	if !ok {
		// This test double represents a successfully refreshed live catalog.
		// Production missing-schema behavior is covered by app/toolcatalog and
		// the MCP protocol/cache fail-closed tests.
		return json.RawMessage(`{"type":"object","additionalProperties":false}`), true
	}
	return append(json.RawMessage(nil), schema...), true
}

func (m runtimeToolSchemaMCPStub) ToolOutputSchema(toolName string) (json.RawMessage, bool) {
	return json.RawMessage(`{"type":"object","additionalProperties":false}`), true
}

func (m runtimeToolSchemaMCPStub) ToolDescription(toolName string) (string, bool) {
	return "", false
}

func jsonRawMessagesEqual(left json.RawMessage, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	leftCanonical, _ := json.Marshal(leftValue)
	rightCanonical, _ := json.Marshal(rightValue)
	return string(leftCanonical) == string(rightCanonical)
}
