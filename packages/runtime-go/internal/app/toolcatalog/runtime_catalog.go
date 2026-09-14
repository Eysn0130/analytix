package toolcatalog

import (
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type RuntimeGoalTodoReader interface {
	GetGoal(string) (map[string]any, error)
	GetTodos(string) (map[string]any, error)
}

type runtimeMCPSource interface {
	Tools() []string
	Search(string) []string
	ToolReadOnlyHint(string) bool
}

// RuntimeCatalog owns the pure runtime tool-advertisement policy. Adapters
// provide current MCP, goal/todo and capability observations; this service
// never treats a cached catalog as source liveness or execution authority.
type RuntimeCatalog struct {
	NativeSelections           bool
	DiagnosticsKey             string
	GoalTodos                  RuntimeGoalTodoReader
	MCP                        any
	MCPSearchResultLimit       int
	MCPSearchShouldScope       func(toolCount int, hasPrompt bool) bool
	WebFetch                   bool
	ReportDelivery             bool
	SubagentsEnabled           bool
	SubagentProfileDescription string
	Skills                     SkillCatalog
}

func (catalog RuntimeCatalog) ToolSchemas(
	disableUserInput bool,
	toolScope []string,
	subagent bool,
	planActive bool,
	prompt string,
	goalToolsActive bool,
) []domainmodel.ToolSchema {
	var names []string
	if mcp, ok := catalog.MCP.(runtimeMCPSource); ok {
		names = mcp.Tools()
	}
	return catalog.ToolSchemasWithMCPNames(
		disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive, names,
	)
}

func (catalog RuntimeCatalog) ToolSchemasWithMCPNames(
	disableUserInput bool,
	toolScope []string,
	subagent bool,
	planActive bool,
	prompt string,
	goalToolsActive bool,
	mcpToolNames []string,
) []domainmodel.ToolSchema {
	mcpTools := []MCPToolSchema{}
	if catalog.MCP != nil {
		for _, toolName := range catalog.MCPToolNamesForPrompt(prompt, mcpToolNames, toolScope) {
			mcpTools = append(mcpTools, MCPToolSchemaFromSource(catalog.MCP, toolName))
		}
	}
	return catalog.ToolSchemasWithMCPTools(
		disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive, mcpTools,
	)
}

func (catalog RuntimeCatalog) ToolSchemasWithMCPAdvertisements(
	disableUserInput bool,
	toolScope []string,
	subagent bool,
	planActive bool,
	prompt string,
	goalToolsActive bool,
	advertisements []MCPToolAdvertisementV1,
) []domainmodel.ToolSchema {
	selected := MCPToolAdvertisementsByNameV1(
		advertisements,
		catalog.MCPToolNamesForPrompt(prompt, MCPToolNamesFromAdvertisementsV1(advertisements), toolScope),
	)
	return catalog.ToolSchemasWithMCPTools(
		disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive,
		MCPToolSchemasFromAdvertisementsV1(selected),
	)
}

func (catalog RuntimeCatalog) ToolSchemasWithMCPTools(
	disableUserInput bool,
	toolScope []string,
	subagent bool,
	planActive bool,
	prompt string,
	goalToolsActive bool,
	mcpTools []MCPToolSchema,
) []domainmodel.ToolSchema {
	return MaterializeToolSchemas(MaterializeInput{
		NativeSelections: catalog.NativeSelections,
		Prompt:           prompt, PromptRoute: catalog.PromptRoute(prompt, toolScope, subagent, planActive),
		ToolScope: toolScope, Subagent: subagent, PlanActive: planActive,
		ReportDelivery:  catalog.ReportDelivery,
		GoalToolsActive: goalToolsActive, DisableUserInput: disableUserInput,
		AllowBackgroundBash: !subagent && catalog.SubagentsEnabled, WebFetch: catalog.WebFetch,
		SubagentsEnabled: catalog.SubagentsEnabled, SubagentProfileDescription: catalog.SubagentProfileDescription,
		SkillToolsActive: !subagent && catalog.Skills.Enabled && len(catalog.Skills.Skills) > 0 &&
			(prompt == catalog.DiagnosticsKey || PromptMentionsSkillTool(prompt, catalog.Skills.Skills, toolScope)),
		MCPTools: mcpTools,
	})
}

func (catalog RuntimeCatalog) GoalToolsActive(threadID string, prompt string, toolScope []string) bool {
	if catalog.GoalToolsPromptActive(prompt, toolScope) {
		return true
	}
	if catalog.GoalTodos == nil || strings.TrimSpace(threadID) == "" {
		return false
	}
	if goal, err := catalog.GoalTodos.GetGoal(threadID); err == nil && goal != nil {
		return true
	}
	if todos, err := catalog.GoalTodos.GetTodos(threadID); err == nil && todos != nil {
		return true
	}
	return false
}

func (catalog RuntimeCatalog) GoalToolsPromptActive(prompt string, toolScope []string) bool {
	return GoalToolsPromptActive(prompt, toolScope, catalog.DiagnosticsKey)
}

func (catalog RuntimeCatalog) PromptRoute(prompt string, toolScope []string, subagent bool, planActive bool) string {
	return PromptRouteForPrompt(PromptRouteInput{
		Prompt: prompt, ToolScope: toolScope, Subagent: subagent, PlanActive: planActive,
		DiagnosticsKey: catalog.DiagnosticsKey, Skills: catalog.SkillSummaries(),
	})
}

func (catalog RuntimeCatalog) SkillSummaries() []map[string]any {
	if !catalog.Skills.Enabled || len(catalog.Skills.Skills) == 0 {
		return nil
	}
	return catalog.Skills.Skills
}

func (catalog RuntimeCatalog) MCPToolNamesForPrompt(prompt string, toolNames []string, toolScope []string) []string {
	toolNames = UniqueStringsInOrder(toolNames)
	searchEnabled := false
	var search func(string) []string
	if mcp, ok := catalog.MCP.(runtimeMCPSource); ok && len(toolScope) == 0 &&
		catalog.MCPSearchShouldScope != nil && catalog.MCPSearchShouldScope(len(toolNames), strings.TrimSpace(prompt) != "") {
		searchEnabled = true
		search = mcp.Search
	}
	return MCPToolNamesForPrompt(MCPToolNamesForPromptInput{
		Prompt: prompt, ToolNames: toolNames, ToolScope: toolScope,
		SearchEnabled: searchEnabled, ResultLimit: catalog.MCPSearchResultLimit, Search: search,
	})
}

func (catalog RuntimeCatalog) ToolBlockedByApprovalNever(toolName string) bool {
	mcp, available := catalog.MCP.(runtimeMCPSource)
	readOnly := available && mcp.ToolReadOnlyHint(toolName)
	return BlockedByApprovalNever(toolName, available, readOnly)
}
