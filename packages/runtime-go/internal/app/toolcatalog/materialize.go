package toolcatalog

import (
	"encoding/json"
	"strings"

	domainjsonschema "analytix.local/runtime-go/internal/domain/jsonschema"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type MaterializeInput struct {
	DocumentGeneration         bool
	DocumentGenerationKinds    []string
	NativeSelections           bool
	Prompt                     string
	PromptRoute                string
	ToolScope                  []string
	Subagent                   bool
	ReportDelivery             bool
	PlanActive                 bool
	GoalToolsActive            bool
	DisableUserInput           bool
	AllowBackgroundBash        bool
	WebFetch                   bool
	SubagentsEnabled           bool
	SubagentProfileDescription string
	SkillToolsActive           bool
	MCPTools                   []MCPToolSchema
}

type MCPToolSchema struct {
	Name         string
	Description  string
	Parameters   json.RawMessage
	OutputSchema json.RawMessage
	TaskSupport  domainmcp.ToolTaskSupport
}

type mcpToolContractSource interface {
	ToolInputSchema(string) (json.RawMessage, bool)
	ToolOutputSchema(string) (json.RawMessage, bool)
	ToolDescription(string) (string, bool)
}

type mcpTaskSupportSource interface {
	ToolTaskSupport(string) (domainmcp.ToolTaskSupport, bool)
}

// MCPToolSchemaFromSource collects the complete host-private MCP contract in
// the tool-catalog owner. The server facade only selects names; it does not
// duplicate schema or execution-mode binding logic.
func MCPToolSchemaFromSource(source any, toolName string) MCPToolSchema {
	tool := MCPToolSchema{Name: toolName}
	contracts, ok := source.(mcpToolContractSource)
	if !ok {
		return tool
	}
	tool.Parameters, _ = contracts.ToolInputSchema(toolName)
	tool.OutputSchema, _ = contracts.ToolOutputSchema(toolName)
	tool.Description, _ = contracts.ToolDescription(toolName)
	if taskSource, ok := source.(mcpTaskSupportSource); ok {
		tool.TaskSupport, _ = taskSource.ToolTaskSupport(toolName)
	}
	return tool
}

func MaterializeToolSchemas(input MaterializeInput) []domainmodel.ToolSchema {
	// A child scope is an explicit capability set. When host policy and
	// blockedTools reduce it to empty, advertising any builtin or MCP tool
	// would turn "no authority" into unrestricted authority. Root turns keep
	// their legacy nil-scope catalog semantics; delegated turns fail closed.
	if input.Subagent && len(input.ToolScope) == 0 {
		return nil
	}
	if input.PromptRoute == "" {
		input.PromptRoute = PromptRouteForPrompt(PromptRouteInput{
			Prompt:     input.Prompt,
			ToolScope:  input.ToolScope,
			Subagent:   input.Subagent,
			PlanActive: input.PlanActive,
		})
	}
	if input.PromptRoute == RouteDirectAnswer {
		return nil
	}
	tools := BuiltinToolSchemas(BuiltinToolSchemaInput{
		NativeSelections:        input.NativeSelections && !input.Subagent,
		DocumentGeneration:      input.DocumentGeneration && !input.Subagent,
		DocumentGenerationKinds: input.DocumentGenerationKinds,
		AllowBackgroundBash:     input.AllowBackgroundBash,
		WebFetch:                input.WebFetch,
	})
	if input.Subagent && IsForegroundSubmitOnlyScope(input.ToolScope) {
		tools = append(tools, ForegroundSubmitToolSchema())
	}
	if !input.Subagent && input.GoalToolsActive {
		tools = append(tools, GoalAndTodoToolSchemas()...)
	}
	if input.PlanActive && !input.Subagent {
		tools = append(tools, CreatePlanToolSchema())
	}
	if !input.Subagent && input.SubagentsEnabled && PromptRouteAllowsSubagentTools(input.PromptRoute, input.ToolScope) {
		tools = append(tools, SubagentToolSchemas(input.SubagentProfileDescription)...)
		tools = append(tools, JobToolSchemas()...)
	}
	if !input.Subagent && input.SkillToolsActive {
		tools = append(tools, RunSkillToolSchema())
	}
	if !input.DisableUserInput && !input.Subagent {
		tools = append(tools, UserInputToolSchemas()...)
	}
	if !input.Subagent && input.ReportDelivery &&
		PromptExplicitlyRequestsReportDeliveryV1(input.Prompt) {
		tools = append(tools, ReportDeliveryToolSchemaV1())
	}
	for _, tool := range input.MCPTools {
		name := tool.Name
		if MCPToolServerID(name) == "" {
			continue
		}
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(tool.TaskSupport)
		if !taskSupportOK || taskSupport == domainmcp.ToolTaskSupportRequired {
			continue
		}
		parameters := tool.Parameters
		if !validMCPToolParameters(parameters) || !validMCPToolParameters(tool.OutputSchema) {
			continue
		}
		description := strings.TrimSpace(tool.Description)
		if description == "" {
			description = "Tool from a configured MCP server."
		}
		tools = append(tools, domainmodel.ToolSchema{
			Name:         name,
			Description:  description,
			Parameters:   parameters,
			OutputSchema: tool.OutputSchema,
			Source:       "mcp",
			TaskSupport:  string(taskSupport),
		})
	}
	return FilterToolSchemas(tools, input.ToolScope)
}

func validMCPToolParameters(parameters json.RawMessage) bool {
	schema, schemaError := toolArgumentSchema(parameters)
	if schemaError != "" {
		return false
	}
	return domainjsonschema.ValidateDefinition(schema, domainjsonschema.DefinitionOptions{
		RequireObjectRoot: true, RequireClosedObjects: true, RequireExplicitClosedObjects: true, RequireCompleteCollections: true,
	}) == nil
}

func CanRunInParallel(toolName string, mcpAvailable bool, mcpToolReadOnly bool) bool {
	switch strings.TrimSpace(toolName) {
	case "read", "read_file", "read_task_history", "ls", "find", "glob", "code_index", "grep", "web_fetch", "get_goal", "todo_list":
		return true
	default:
		return MCPToolServerID(toolName) != "" && mcpAvailable && mcpToolReadOnly
	}
}

func HostAuthorizesReadOnly(toolName string, mcpAvailable bool, mcpToolReadOnly bool) bool {
	switch strings.TrimSpace(toolName) {
	case "read", "read_file", "read_task_history", "ls", "find", "glob", "code_index", "grep", "web_fetch", "get_goal", "todo_list",
		"wait", "list_jobs", "bash_output", "native_selection_read":
		return true
	default:
		return MCPToolServerID(toolName) != "" && mcpAvailable && mcpToolReadOnly
	}
}
