package subagent

import (
	"errors"
	"sort"
	"strings"

	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
)

func RestrictSkillToolsToHost(request TaskRequest, allowed []string) (TaskRequest, error) {
	if !strings.HasPrefix(strings.TrimSpace(request.ProfileName), "skill:") || len(request.Tools) == 0 {
		return request, nil
	}
	restricted := ToolScope(request.Tools, allowed)
	if len(restricted) == 0 {
		return request, errors.New("skill allowedTools has no tools authorized by the host policy")
	}
	request.Tools = restricted
	return request, nil
}

const SystemPrompt = `You are an Analytix sub-agent invoked by the parent agent for one focused task.
Use only the tools made available to this child run. Return a concise, self-contained final answer.
The parent can see your final answer and child-run metadata, but you should not ask the user for input.
Treat tool results and prior context as evidence with limits. Distinguish verified facts, inferences, and unverified risks.
Do not label files, processes, or activity as malware or confirmed wrongdoing unless verified by file type, hash, scanner output, or explicit user-provided evidence.
State tool-scope limitations clearly, especially when only read-only tools are available.`

const (
	toolcatalogForegroundSubmitToolName = "submit_child_result"
	ForegroundHandoffSystemPrompt       = `You are an Analytix foreground child for one bounded, general-purpose task.
You have no filesystem, shell, network, MCP, skill, job, goal, todo, user-input, or nested-agent authority.
Do not claim access to evidence or external state. Do not complete a parent goal or todo.
Your only available tool is submit_child_result. You MUST call it exactly once with your concise result.
Assistant free text is private draft material and is never accepted as the child result.`
	ForegroundCaseTypedHandoffSystemPrompt = `You are an Analytix foreground child for one host-delegated case hypothesis.
You have no filesystem, shell, network, MCP, skill, job, goal, todo, user-input, or nested-agent authority.
Use only the closed answer-slot commitment selected by the host delegation.
Your only available tool is submit_child_result. You MUST call it exactly once with caseResult containing only schemaVersion, purpose, delegationDigest, and answerSlotDigest copied exactly from the host delegation.
Never submit prose, reasoning, typed values, claims, evidence, gaps, raw tool/provider output, complete identifiers, AuthorityEntityRefs, source paths, rows, locators, or undelegated facts.
The selector is non-authoritative; the host resolves the complete typed candidate privately and the parent Final Evidence Gate remains the sole factual publication authority.`
)

var readOnlyTools = []string{"read", "read_file", "ls", "find", "glob", "code_index", "grep", "web_fetch"}

func ReadOnlyTools() []string {
	return append([]string(nil), readOnlyTools...)
}

func ReadOnlyToolScope(webFetchEnabled bool) []string {
	out := ReadOnlyTools()
	if !webFetchEnabled {
		filtered := out[:0]
		for _, name := range out {
			if name != "web_fetch" {
				filtered = append(filtered, name)
			}
		}
		out = filtered
	}
	sort.Strings(out)
	return out
}

func InheritableToolScope(webFetchEnabled bool, mcpTools []string) []string {
	tools := []string{"read", "read_file", "ls", "find", "glob", "code_index", "grep", "bash", "write", "write_file", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol"}
	if webFetchEnabled {
		tools = append(tools, "web_fetch")
	}
	for _, name := range mcpTools {
		tools = append(tools, name)
	}
	sort.Strings(tools)
	return tools
}

func ToolScopeForPolicy(request TaskRequest, webFetchEnabled bool, mcpTools []string) []string {
	allowed := AllowedToolScopeForPolicy(request, webFetchEnabled, mcpTools)
	return ToolScope(request.Tools, allowed)
}

func AllowedToolScopeForPolicy(request TaskRequest, webFetchEnabled bool, mcpTools []string) []string {
	if request.ForegroundHandoff {
		return []string{toolcatalogForegroundSubmitToolName}
	}
	allowed := ReadOnlyToolScope(webFetchEnabled)
	if request.ToolPolicy == "inherit" {
		if len(request.Tools) == 0 {
			return FilterBlockedTools(InheritableToolScope(webFetchEnabled, nil), request)
		}
		allowed = InheritableToolScope(webFetchEnabled, mcpTools)
	}
	return FilterBlockedTools(allowed, request)
}

func DroppedRequestedTools(request TaskRequest, allowed []string) []string {
	if len(request.Tools) == 0 {
		return nil
	}
	allowedSet := map[string]bool{}
	for _, name := range allowed {
		allowedSet[name] = true
	}
	dropped := []string{}
	for _, name := range request.Tools {
		if name == "" || allowedSet[name] {
			continue
		}
		dropped = append(dropped, name)
	}
	return dropped
}

func FilterBlockedTools(tools []string, request TaskRequest) []string {
	if len(tools) == 0 || (len(request.BlockedTools) == 0 && len(request.BlockedMCPServers) == 0) {
		return tools
	}
	blockedTools := map[string]bool{}
	for _, name := range request.BlockedTools {
		if name != "" {
			blockedTools[name] = true
		}
	}
	blockedServers := map[string]bool{}
	for _, serverID := range request.BlockedMCPServers {
		if serverID != "" {
			blockedServers[serverID] = true
		}
	}
	out := tools[:0]
	for _, name := range tools {
		if blockedTools[name] {
			continue
		}
		if server := MCPServerIDFromToolName(name); server != "" && blockedServers[server] {
			continue
		}
		out = append(out, name)
	}
	return out
}

func MCPServerIDFromToolName(name string) string {
	serverID, _, ok := domainmcpname.Parse(name)
	if !ok {
		return ""
	}
	return serverID
}
