package toolcatalog

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ReadGuardNone         = ""
	ReadGuardEdit         = "edit"
	ReadGuardDeleteRange  = "delete_range"
	ReadGuardDeleteSymbol = "delete_symbol"
)

type ToolPreflightInput struct {
	ToolName               string
	SubagentDepth          int
	ApprovalPolicy         string
	SandboxMode            string
	BlockedByApprovalNever bool
	CaseBound              bool
	HostReadOnly           bool
}

type ToolPreflightDecision struct {
	Output            map[string]any
	IsError           bool
	Blocked           bool
	RequiresReadGuard bool
	ReadGuard         string
}

func PreflightTool(input ToolPreflightInput) ToolPreflightDecision {
	toolName := strings.TrimSpace(input.ToolName)
	if input.CaseBound && (RequiresCaseArtifactAuthority(toolName) || CaseBoundMCPWriteRequiresPublicationAuthority(toolName, input.HostReadOnly)) {
		return ToolPreflightDecision{
			Output: map[string]any{
				"code": "publication_receipt_required", "error": "case artifact execution requires host publication authority", "executed": false,
			},
			IsError: true,
			Blocked: true,
		}
	}
	if input.SubagentDepth > 0 && (IsSubagentTool(toolName) || IsJobTool(toolName) || IsThreadStateTool(toolName)) {
		return ToolPreflightDecision{
			Output:  map[string]any{"code": "subagent_tool_filtered", "error": toolName + " is not available inside subagents"},
			IsError: true,
			Blocked: true,
		}
	}
	if toolName == "bash" && strings.TrimSpace(input.SandboxMode) != "danger-full-access" {
		return ToolPreflightDecision{
			Output:  map[string]any{"code": "sandbox_blocked", "error": "bash requires danger-full-access"},
			IsError: true,
			Blocked: true,
		}
	}
	if strings.TrimSpace(input.ApprovalPolicy) == "never" && input.BlockedByApprovalNever {
		return ToolPreflightDecision{
			Output:  map[string]any{"code": "approval_policy_blocked", "error": toolName + " is blocked because approvalPolicy is never"},
			IsError: true,
			Blocked: true,
		}
	}
	switch toolName {
	case "delete_range":
		return ToolPreflightDecision{RequiresReadGuard: true, ReadGuard: ReadGuardDeleteRange}
	case "delete_symbol":
		return ToolPreflightDecision{RequiresReadGuard: true, ReadGuard: ReadGuardDeleteSymbol}
	case "edit", "edit_file", "multi_edit":
		return ToolPreflightDecision{RequiresReadGuard: true, ReadGuard: ReadGuardEdit}
	default:
		return ToolPreflightDecision{}
	}
}

func CaseBoundMCPWriteRequiresPublicationAuthority(toolName string, hostReadOnly bool) bool {
	return MCPToolNeedsAnalytixCaseContext(strings.TrimSpace(toolName)) && !hostReadOnly
}

func RequiresCaseArtifactAuthority(toolName string) bool {
	return MCPToolRequiresHostArtifactAuthority(strings.TrimSpace(toolName))
}

func CaseArtifactCallRequiresAuthority(call domainmodel.ToolCall) bool {
	return RequiresCaseArtifactAuthority(call.Name)
}

// HostGeneralOnlyForegroundTaskCall recognizes the optional bounded foreground
// handoff shape. It is a child-result privacy contract, not a global Agent
// catalog or execution ceiling.
func HostGeneralOnlyForegroundTaskCall(call domainmodel.ToolCall) bool {
	if strings.TrimSpace(call.Name) != ForegroundTaskToolName {
		return false
	}
	arguments, err := domainsecurity.DecodeCanonicalJSONObject(call.Arguments)
	if err != nil || len(arguments) != 4 {
		return false
	}
	prompt, promptOK := arguments["prompt"].(string)
	maxSteps, stepsOK := exactJSONInteger(arguments["max_steps"])
	tokenBudget, tokenOK := exactJSONInteger(arguments["token_budget"])
	timeBudgetMS, timeOK := exactJSONInteger(arguments["time_budget_ms"])
	return promptOK && strings.TrimSpace(prompt) != "" && len([]byte(prompt)) <= 8192 &&
		stepsOK && maxSteps >= 1 && maxSteps <= ForegroundMaxSteps &&
		tokenOK && tokenBudget >= 1 && tokenBudget <= ForegroundMaxTokenBudget &&
		timeOK && timeBudgetMS >= 1 && timeBudgetMS <= ForegroundMaxTimeBudgetMS
}

// HostForegroundTaskCallForContextV1 classifies only the top-level bounded
// foreground shape under a host-issued general-only or fact-publication TSC.
// Provider arguments cannot choose the result lane.
func HostForegroundTaskCallForContextV1(
	call domainmodel.ToolCall,
	subagentDepth int,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	if subagentDepth != 0 || !HostGeneralOnlyForegroundTaskCall(call) {
		return false
	}
	return domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(securityContext) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) == nil
}

func exactJSONInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		if int64(int(typed)) != typed {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || int64(int(parsed)) != parsed {
			return 0, false
		}
		return int(parsed), true
	case float64:
		parsed := int(typed)
		return parsed, float64(parsed) == typed
	default:
		return 0, false
	}
}

func IsCaseBoundSecurityContext(context domainsecurity.TurnSecurityContext) bool {
	return domainsecurity.TurnSecurityContextIsCaseSensitive(context)
}
