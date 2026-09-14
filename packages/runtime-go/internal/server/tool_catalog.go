package server

import (
	appmodel "analytix.local/runtime-go/internal/app/model"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	provider "analytix.local/runtime-go/internal/provider"
)

const (
	runtimeCreatePlanToolName       = toolcatalogapp.ToolCreatePlanName
	runtimeToolSchemaDiagnosticsKey = "\x00runtime-diagnostics-all-tools"
)

func (h *runtimeServerHandler) runtimeToolCatalog() toolcatalogapp.RuntimeCatalog {
	catalog := toolcatalogapp.RuntimeCatalog{DiagnosticsKey: runtimeToolSchemaDiagnosticsKey}
	if h == nil {
		return catalog
	}
	// Advertise the native family only while Core holds a captured editing
	// baseline. Scope/thread/version validation remains required at execution.
	catalog.NativeSelections = h.officePackageHost != nil && h.managedEditing != nil && h.managedEditing.HasCaptures()
	catalog.Skills = h.currentSkillCatalog()
	_, documentsAvailable := toolcatalogapp.SkillByName(catalog.Skills, toolcatalogapp.DocumentsSkillID)
	catalog.DocumentGeneration = h.documentCodec != nil && h.turnSecurity.Identity != nil && documentsAvailable
	catalog.GoalTodos = h.store
	catalog.MCP = h.mcp
	catalog.MCPSearchResultLimit = runtimeinfoapp.MCPSearchResultLimit(h.mcpSearch)
	catalog.MCPSearchShouldScope = func(toolCount int, hasPrompt bool) bool {
		return runtimeinfoapp.MCPSearchShouldScope(h.mcpSearch, toolCount, hasPrompt)
	}
	catalog.WebFetch = runtimeinfoapp.WebFetchEnabled(h.web)
	catalog.SubagentsEnabled = h.subagents.Enabled
	catalog.SubagentProfileDescription = subagentapp.ProfileDescription(h.subagents)
	return catalog
}

func (h *runtimeServerHandler) runtimeToolSchemas(disableUserInput bool) []provider.ToolSchema {
	return h.runtimeToolSchemasFor(disableUserInput, nil, false, false)
}

func (h *runtimeServerHandler) runtimeToolSchemasFor(disableUserInput bool, toolScope []string, subagent bool, planActive bool) []provider.ToolSchema {
	return h.runtimeToolSchemasForPrompt(disableUserInput, toolScope, subagent, planActive, runtimeToolSchemaDiagnosticsKey)
}

func (h *runtimeServerHandler) runtimeToolSchemasForPrompt(disableUserInput bool, toolScope []string, subagent bool, planActive bool, prompt string) []provider.ToolSchema {
	return h.runtimeToolSchemasForPromptWithGoalTools(disableUserInput, toolScope, subagent, planActive, prompt, h.runtimeGoalToolsPromptActive(prompt, toolScope))
}

func (h *runtimeServerHandler) runtimeToolSchemasForPromptWithGoalTools(disableUserInput bool, toolScope []string, subagent bool, planActive bool, prompt string, goalToolsActive bool) []provider.ToolSchema {
	return h.runtimeToolCatalog().ToolSchemas(disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive)
}

func (h *runtimeServerHandler) runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(disableUserInput bool, toolScope []string, subagent bool, planActive bool, prompt string, goalToolsActive bool, mcpToolNames []string) []provider.ToolSchema {
	return h.runtimeToolCatalog().ToolSchemasWithMCPNames(disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive, mcpToolNames)
}

func (h *runtimeServerHandler) runtimeToolSchemasForPromptWithGoalToolsAndMCPAdvertisements(disableUserInput bool, toolScope []string, subagent bool, planActive bool, prompt string, goalToolsActive bool, advertisements []toolcatalogapp.MCPToolAdvertisementV1) []provider.ToolSchema {
	return h.runtimeToolCatalog().ToolSchemasWithMCPAdvertisements(disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive, advertisements)
}

func (h *runtimeServerHandler) runtimeToolSchemasForPromptWithGoalToolsAndMCPTools(disableUserInput bool, toolScope []string, subagent bool, planActive bool, prompt string, goalToolsActive bool, mcpTools []toolcatalogapp.MCPToolSchema) []provider.ToolSchema {
	return h.runtimeToolCatalog().ToolSchemasWithMCPTools(disableUserInput, toolScope, subagent, planActive, prompt, goalToolsActive, mcpTools)
}

func (h *runtimeServerHandler) runtimeGoalToolsActive(threadID string, prompt string, toolScope []string) bool {
	return h.runtimeToolCatalog().GoalToolsActive(threadID, prompt, toolScope)
}

func (h *runtimeServerHandler) runtimeGoalToolsPromptActive(prompt string, toolScope []string) bool {
	return h.runtimeToolCatalog().GoalToolsPromptActive(prompt, toolScope)
}

func (h *runtimeServerHandler) runtimePromptRouteForPrompt(prompt string, toolScope []string, subagent bool, planActive bool) string {
	return h.runtimeToolCatalog().PromptRoute(prompt, toolScope, subagent, planActive)
}

func (h *runtimeServerHandler) runtimeSkillSummaries() []map[string]any {
	return h.runtimeToolCatalog().SkillSummaries()
}

func (h *runtimeServerHandler) runtimeMCPToolNamesForPrompt(prompt string, toolNames []string, toolScope []string) []string {
	return h.runtimeToolCatalog().MCPToolNamesForPrompt(prompt, toolNames, toolScope)
}

func (h *runtimeServerHandler) runtimeToolBlockedByApprovalNever(toolName string) bool {
	return h.runtimeToolCatalog().ToolBlockedByApprovalNever(toolName)
}

func (h *runtimeServerHandler) runtimePendingToolWorkspace(pending runtimePendingToolCall) string {
	return appmodel.PendingToolWorkspace(pending, h.store)
}
