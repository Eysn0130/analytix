package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ToolCaller interface {
	CallTool(name string, allowStale bool, args ...map[string]any) map[string]any
}

type ContextToolCaller interface {
	CallToolContext(ctx context.Context, name string, allowStale bool, args ...map[string]any) map[string]any
}

type SecurityBoundContextToolCaller interface {
	CallToolSecurityBoundContext(ctx context.Context, name string, approved bool, envelope domainmcp.HostContextEnvelope, args ...map[string]any) map[string]any
}

type ToolRunInput struct {
	ToolName        string
	Arguments       map[string]any
	Workspace       string
	ThreadID        string
	TurnID          string
	SecurityContext domainsecurity.TurnSecurityContext
	ExecutionGrant  domainsecurity.ExecutionGrant
	Call            domainmodel.ToolCall
}

func ToolRunInputFromPending(pending appmodel.PendingToolCall, arguments map[string]any) ToolRunInput {
	return ToolRunInput{
		ToolName: pending.Call.Name, Arguments: arguments, Workspace: pending.SecurityContext.WorkspaceRealPath,
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, SecurityContext: pending.SecurityContext,
		ExecutionGrant: pending.ExecutionGrant, Call: pending.Call,
	}
}

func IsToolName(name string) bool {
	_, _, ok := domainmcpname.Parse(name)
	return ok
}

func ExecuteTool(ctx context.Context, caller ToolCaller, input ToolRunInput) (map[string]any, bool) {
	call := input.Call
	argumentBytes, argumentErr := json.Marshal(input.Arguments)
	validateContext := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect
	if executiongrantapp.CallUsesCaseDataAuthority(call) {
		validateContext = domainsecurity.ValidateTurnSecurityContextForExecution
	}
	if err := validateContext(input.SecurityContext); err != nil ||
		domainsecurity.ValidateExecutionGrant(input.ExecutionGrant) != nil ||
		!domainmodel.IsHostToolCallIDV1(call.ID) ||
		!domainmodel.IsHostToolCallIDV1(input.ExecutionGrant.ToolCallID) ||
		argumentErr != nil || input.ExecutionGrant.ConnectionEpoch == 0 ||
		input.ExecutionGrant.ContextDigest != input.SecurityContext.ContextDigest || input.ExecutionGrant.TurnID != input.SecurityContext.TurnID ||
		strings.TrimSpace(input.ToolName) != strings.TrimSpace(call.Name) || input.ExecutionGrant.ToolName != strings.TrimSpace(call.Name) ||
		input.ExecutionGrant.ToolCallID != strings.TrimSpace(call.ID) || input.ExecutionGrant.ArgsHash != domainsecurity.CanonicalJSONHash(call.Arguments) ||
		input.ExecutionGrant.ArgsHash != domainsecurity.CanonicalJSONHash(argumentBytes) || strings.TrimSpace(input.Workspace) != input.SecurityContext.WorkspaceRealPath ||
		strings.TrimSpace(input.ThreadID) != input.SecurityContext.ThreadID || strings.TrimSpace(input.TurnID) != input.SecurityContext.TurnID {
		return map[string]any{"code": "mcp_execution_authority_invalid", "error": "MCP execution requires a valid frozen host grant", "executed": false}, true
	}
	if toolcatalogapp.MCPToolNeedsAnalytixCaseContext(call.Name) && !input.ExecutionGrant.ReadOnly {
		return map[string]any{"code": "publication_receipt_required", "error": "Case-data MCP write execution is quarantined until the host publication pipeline issues authority", "executed": false}, true
	}
	if input.ExecutionGrant.ApprovalState != "not_required" && input.ExecutionGrant.ApprovalState != "approved" {
		return map[string]any{"code": "mcp_execution_approval_pending", "error": "MCP execution grant has not been approved", "executed": false}, true
	}
	if toolcatalogapp.MCPToolRequiresHostArtifactAuthority(call.Name) {
		return map[string]any{"code": "publication_receipt_required", "error": "Case artifact execution is quarantined until the host publication pipeline issues authority", "executed": false}, true
	}
	if toolcatalogapp.MCPProviderArgumentsContainHostAuthority(input.Arguments) {
		return map[string]any{"code": "mcp_provider_authority_rejected", "error": "Provider MCP arguments cannot contain reserved host authority fields", "executed": false}, true
	}
	hostContext, err := domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
	if err != nil {
		return map[string]any{"code": "mcp_execution_authority_invalid", "error": "MCP execution requires a valid frozen host grant", "executed": false}, true
	}
	arguments := toolcatalogapp.MCPProviderArguments(input.Arguments)
	result := map[string]any{}
	if caller == nil {
		result = map[string]any{"code": "mcp_unavailable", "error": "MCP manager is unavailable", "executed": false}
	} else {
		boundCaller, ok := caller.(SecurityBoundContextToolCaller)
		if !ok {
			result = map[string]any{"code": "mcp_execution_authority_unavailable", "error": "MCP caller cannot enforce the frozen turn and source authority", "executed": false}
		} else {
			result = boundCaller.CallToolSecurityBoundContext(ctx, input.ToolName, true, hostContext, arguments)
		}
	}
	lossless, hasLossless := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	outcome, err := evidenceapp.NormalizeMCPToolOutcome(evidenceapp.NormalizeMCPToolOutcomeInput{
		Context: input.SecurityContext, Grant: input.ExecutionGrant, Call: call, Raw: result, At: time.Now().UTC(),
	})
	if err != nil {
		if hasLossless && domainmcp.ValidLosslessToolResult(lossless) {
			if disposer, ok := caller.(interface {
				DiscardHostFundsAccountFlowEvidenceV1(domainmcp.LosslessToolResult)
			}); ok {
				disposer.DiscardHostFundsAccountFlowEvidenceV1(lossless)
			}
		}
		return map[string]any{"code": "mcp_tool_outcome_invalid", "error": "MCP result could not be normalized under the frozen grant", "executed": false}, true
	}
	output := cloneResult(result)
	delete(output, domainmcp.HostRawToolResultKey)
	if hasLossless && domainmcp.ValidLosslessToolResult(lossless) {
		output[domainmcp.HostRawToolResultKey] = lossless
	}
	output["toolOutcome"] = domainevidence.ToolOutcomeRecord(outcome)
	return output, outcome.IsError
}

func cloneResult(result map[string]any) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	body, _ := json.Marshal(result)
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}

func DecodeArguments(raw json.RawMessage) (map[string]any, error) {
	return domainjsonstrict.DecodeObject(raw, domainjsonstrict.Options{
		MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
}
