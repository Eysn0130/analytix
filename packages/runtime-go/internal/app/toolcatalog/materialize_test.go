package toolcatalog

import (
	"encoding/json"
	"reflect"
	"testing"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type mcpToolContractStub struct{}

func (mcpToolContractStub) ToolInputSchema(string) (json.RawMessage, bool) {
	return json.RawMessage(`{"type":"object","additionalProperties":false}`), true
}
func (mcpToolContractStub) ToolOutputSchema(string) (json.RawMessage, bool) {
	return json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`), true
}
func (mcpToolContractStub) ToolDescription(string) (string, bool) { return "Lookup docs", true }
func (mcpToolContractStub) ToolTaskSupport(string) (domainmcp.ToolTaskSupport, bool) {
	return domainmcp.ToolTaskSupportOptional, true
}

func TestMCPToolSchemaFromSourceCollectsHostPrivateContract(t *testing.T) {
	tool := MCPToolSchemaFromSource(mcpToolContractStub{}, "mcp__docs__lookup")
	if tool.Name != "mcp__docs__lookup" || tool.Description != "Lookup docs" || len(tool.Parameters) == 0 || len(tool.OutputSchema) == 0 ||
		tool.TaskSupport != domainmcp.ToolTaskSupportOptional {
		t.Fatalf("complete MCP contract was not collected in toolcatalog: %#v", tool)
	}
}

func TestMaterializeToolSchemasCombinesAppOwnedToolFamilies(t *testing.T) {
	tools := MaterializeToolSchemas(MaterializeInput{
		Prompt:                     "delegate and ask for input",
		PromptRoute:                RouteSubagent,
		PlanActive:                 true,
		GoalToolsActive:            true,
		AllowBackgroundBash:        true,
		WebFetch:                   true,
		SubagentsEnabled:           true,
		SubagentProfileDescription: "worker profile",
		SkillToolsActive:           true,
		MCPTools: []MCPToolSchema{{
			Name:         "mcp__docs__lookup",
			Description:  "Lookup docs",
			Parameters:   json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		}},
	})

	names := toolNames(tools)
	for _, required := range []string{
		"read",
		"web_fetch",
		"get_goal",
		ToolCreatePlanName,
		"parallel_tasks",
		"run_skill",
		"request_user_input",
		"mcp__docs__lookup",
	} {
		if !containsName(names, required) {
			t.Fatalf("materialized tools missing %s from %#v", required, names)
		}
	}
}

func TestEveryAppOwnedProviderToolSchemaIsRecursivelyClosed(t *testing.T) {
	tools := MaterializeToolSchemas(MaterializeInput{
		Prompt:                     "delegate, use every host tool, and ask for input",
		PromptRoute:                RouteSubagent,
		PlanActive:                 true,
		GoalToolsActive:            true,
		AllowBackgroundBash:        true,
		WebFetch:                   true,
		SubagentsEnabled:           true,
		SubagentProfileDescription: "read-only worker",
		SkillToolsActive:           true,
	})
	if len(tools) == 0 {
		t.Fatal("complete app-owned catalog was unexpectedly empty")
	}
	if err := ValidateProviderVisibleToolSchemas(tools); err != nil {
		t.Fatalf("app-owned provider catalog contains an open or incomplete schema: %v", err)
	}
}

func TestValidateProviderVisibleToolSchemasRejectsOpenOrAmbiguousCatalog(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	tests := map[string][]domainmodel.ToolSchema{
		"open root":           {{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","additionalProperties":true}`)}},
		"implicit open root":  {{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
		"open nested object":  {{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"object","properties":{}}},"additionalProperties":false}`)}},
		"array without items": {{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"ids":{"type":"array"}},"additionalProperties":false}`)}},
		"duplicate name": {
			{Name: "lookup", Parameters: closed},
			{Name: "lookup", Parameters: closed},
		},
		"non canonical name": {{Name: " lookup", Parameters: closed}},
	}
	for name, schemas := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProviderVisibleToolSchemas(schemas); err == nil {
				t.Fatal("open, incomplete, or ambiguous provider catalog was accepted")
			}
		})
	}
}

func TestAppOwnedSchemasRejectUnknownProviderAuthorityFields(t *testing.T) {
	tests := []struct {
		name    string
		call    domainmodel.ToolCall
		schemas []domainmodel.ToolSchema
	}{
		{
			name:    "write file unknown field",
			call:    domainmodel.ToolCall{Name: "write_file", Arguments: json.RawMessage(`{"path":"report.md","content":"text","publishWithoutReceipt":true}`)},
			schemas: BuiltinToolSchemas(BuiltinToolSchemaInput{}),
		},
		{
			name:    "user input unknown question field",
			call:    domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(`{"prompt":"continue?","questions":[{"question":"continue?","authority":"host"}]}`)},
			schemas: UserInputToolSchemas(),
		},
		{
			name:    "user input unknown option field",
			call:    domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(`{"prompt":"continue?","questions":[{"question":"continue?","options":[{"label":"yes","grantId":"forged"}]}]}`)},
			schemas: UserInputToolSchemas(),
		},
		{
			name:    "todo provider source authority",
			call:    domainmodel.ToolCall{Name: "todo_write", Arguments: json.RawMessage(`{"todos":[{"content":"work","status":"pending","source":{"kind":"goal","goalId":"forged"}}]}`)},
			schemas: GoalAndTodoToolSchemas(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if details, blocked := ValidateToolCallArguments(test.call, test.schemas); !blocked {
				t.Fatalf("unknown provider authority field was accepted: %#v", details)
			}
		})
	}
}

func TestMaterializeToolSchemasHonorsDirectAnswerSubagentAndScope(t *testing.T) {
	if tools := MaterializeToolSchemas(MaterializeInput{PromptRoute: RouteDirectAnswer}); len(tools) != 0 {
		t.Fatalf("direct answer route should advertise no tools: %#v", tools)
	}
	tools := MaterializeToolSchemas(MaterializeInput{
		Subagent:         true,
		PlanActive:       true,
		GoalToolsActive:  true,
		SubagentsEnabled: true,
		SkillToolsActive: true,
		MCPTools: []MCPToolSchema{{
			Name:         "mcp__docs__lookup",
			Parameters:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
		ToolScope: []string{"read", "mcp__docs__lookup", "request_user_input"},
	})
	if got, want := toolNames(tools), []string{"read", "mcp__docs__lookup"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("subagent/scope materialization got %#v want %#v", got, want)
	}
}

func TestMaterializeToolSchemasEmptySubagentScopeAdvertisesNoBuiltinOrMCP(t *testing.T) {
	tools := MaterializeToolSchemas(MaterializeInput{
		Prompt:      "use docs and read the workspace",
		PromptRoute: RouteToolAgent,
		Subagent:    true,
		MCPTools: []MCPToolSchema{{
			Name:         "mcp__docs__lookup",
			Parameters:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
	})
	if len(tools) != 0 {
		t.Fatalf("empty delegated scope must advertise no builtin or MCP tools: %#v", toolNames(tools))
	}
}

func TestMaterializeToolSchemasRejectsMissingAndOpenEndedMCPSchemas(t *testing.T) {
	closedOutput := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	tools := MaterializeToolSchemas(MaterializeInput{
		PromptRoute: RouteToolAgent,
		MCPTools: []MCPToolSchema{
			{Name: "mcp__docs__missing"},
			{Name: "mcp__docs__open", Parameters: json.RawMessage(`{"type":"object","additionalProperties":true}`), OutputSchema: closedOutput},
			{Name: "mcp__docs__nested_open", Parameters: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"object","properties":{}}},"additionalProperties":false}`), OutputSchema: closedOutput},
			{Name: "mcp__docs__missing_output", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
			{Name: "mcp__docs__open_output", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)},
			{Name: "mcp__docs__closed", Parameters: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"object","properties":{},"additionalProperties":false}},"additionalProperties":false}`), OutputSchema: closedOutput},
		},
	})
	names := toolNames(tools)
	for _, rejected := range []string{"mcp__docs__missing", "mcp__docs__open", "mcp__docs__nested_open", "mcp__docs__missing_output", "mcp__docs__open_output"} {
		if containsName(names, rejected) {
			t.Fatalf("open-ended MCP schema %s must fail closed: %#v", rejected, names)
		}
	}
	if !containsName(names, "mcp__docs__closed") {
		t.Fatalf("recursively closed MCP schema was rejected: %#v", names)
	}
}

func TestCanRunInParallelSeparatesReadOnlyAndWriteTools(t *testing.T) {
	cases := []struct {
		name            string
		mcpAvailable    bool
		mcpToolReadOnly bool
		want            bool
	}{
		{name: "read", want: true},
		{name: "grep", want: true},
		{name: "write", want: false},
		{name: "bash", want: false},
		{name: "mcp__docs__lookup", mcpAvailable: true, mcpToolReadOnly: true, want: true},
		{name: "mcp__shell__run", mcpAvailable: true, mcpToolReadOnly: false, want: false},
		{name: "mcp__missing__tool", mcpAvailable: false, mcpToolReadOnly: true, want: false},
	}
	for _, tc := range cases {
		if got := CanRunInParallel(tc.name, tc.mcpAvailable, tc.mcpToolReadOnly); got != tc.want {
			t.Fatalf("%s parallel got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestJobObservationReadOnlyPolicyDoesNotImplyParallelExecution(t *testing.T) {
	for _, name := range []string{"wait", "list_jobs", "bash_output"} {
		if !HostAuthorizesReadOnly(name, false, false) {
			t.Fatalf("%s observation tool was not host-authorized read-only", name)
		}
		if CanRunInParallel(name, false, false) {
			t.Fatalf("%s blocking/stateful observation was incorrectly made parallel", name)
		}
	}
	for _, name := range []string{"kill_shell", "restart_job"} {
		if HostAuthorizesReadOnly(name, false, false) || CanRunInParallel(name, false, false) {
			t.Fatalf("%s side-effect tool was classified as read-only or parallel", name)
		}
	}
}

func toolNames(tools []domainmodel.ToolSchema) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func containsName(names []string, needle string) bool {
	for _, name := range names {
		if name == needle {
			return true
		}
	}
	return false
}
