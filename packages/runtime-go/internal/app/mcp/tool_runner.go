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
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
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
	if ctx == nil {
		return map[string]any{"code": "mcp_execution_authority_invalid", "error": "MCP execution context is unavailable", "executed": false}, true
	}
	// This effect owns its actual MCP request credentials. Reuse the existing
	// bounded registry, including the parent Provider values, then retire it.
	parentSecrets := domainprocess.KnownSecretsV1(ctx)
	textScopeAvailable := len(parentSecrets) <= 64
	ctx = domainprocess.WithKnownSecretsV1(ctx, parentSecrets...)
	defer domainprocess.ClearKnownSecretsV1(ctx)
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
		if hasLossless {
			if disposer, ok := caller.(interface {
				DiscardHostScheduleReceiptV1(domainmcp.LosslessToolResult)
			}); ok {
				disposer.DiscardHostScheduleReceiptV1(lossless)
			}
		}
		if hasLossless && domainmcp.ValidLosslessToolResult(lossless) {
			if disposer, ok := caller.(interface {
				DiscardHostFundsAccountFlowEvidenceV1(domainmcp.LosslessToolResult)
			}); ok {
				disposer.DiscardHostFundsAccountFlowEvidenceV1(lossless)
			}
		}
		return map[string]any{"code": "mcp_tool_outcome_invalid", "error": "MCP result could not be normalized under the frozen grant", "executed": false}, true
	}
	if hasLossless && domaintoolresult.ProtectedScheduleOperationForToolV1(input.ToolName) != "" {
		if reader, ok := caller.(interface {
			ConsumeHostScheduleReceiptV1(domainmcp.LosslessToolResult, domainmcp.HostContextEnvelope, domainevidence.ToolOutcome) (domaintoolresult.ProtectedCaptureV1, bool)
		}); ok {
			if capture, present := reader.ConsumeHostScheduleReceiptV1(lossless, hostContext, outcome); present {
				domaintoolresult.CaptureProtectedToolResultV1(ctx, capture)
			}
		} else if disposer, ok := caller.(interface {
			DiscardHostScheduleReceiptV1(domainmcp.LosslessToolResult)
		}); ok {
			disposer.DiscardHostScheduleReceiptV1(lossless)
		}
		lossless.HostPrivate = nil
	}
	if ctx.Err() == nil && textScopeAvailable && len(domainprocess.KnownSecretsV1(ctx)) <= 64 && hasLossless && domainmcp.ValidToolResultObservation(lossless) && !outcome.IsError && outcome.SemanticStatus == "success" &&
		!toolcatalogapp.MCPToolNeedsAnalytixCaseContext(input.ToolName) &&
		domaintoolresult.ProtectedScheduleOperationForToolV1(input.ToolName) == "" && lossless.HostPrivate == nil {
		domaintoolresult.CaptureProtectedToolTextV1(ctx, protectedMCPContentTextV1(lossless, domainprocess.KnownSecretsV1(ctx)...))
	}
	output := cloneResult(result)
	delete(output, domainmcp.HostRawToolResultKey)
	if hasLossless && domainmcp.ValidLosslessToolResult(lossless) {
		output[domainmcp.HostRawToolResultKey] = lossless
	}
	output["toolOutcome"] = domainevidence.ToolOutcomeRecord(outcome)
	return output, outcome.IsError
}

// The normalized result's .text is not provenance: structuredContent can
// contain that key. Select only literal text blocks in the verified raw result.
func protectedMCPContentTextV1(lossless domainmcp.LosslessToolResult, secrets ...string) *domaintoolresult.ProtectedOutputTextV1 {
	if !domainmcp.ValidToolResultObservation(lossless) {
		return nil
	}
	var payload struct {
		Content []map[string]json.RawMessage `json:"content"`
	}
	if json.Unmarshal(lossless.RawResult, &payload) != nil {
		return nil
	}
	var text strings.Builder
	for _, item := range payload.Content {
		var kind, value string
		if json.Unmarshal(item["type"], &kind) != nil || kind != "text" || json.Unmarshal(item["text"], &value) != nil || value == "" {
			continue
		}
		separator := 0
		if text.Len() > 0 {
			separator = 1
		}
		if len(value) > domaintoolresult.ProtectedOutputTextScanLimitV1-text.Len()-separator {
			return &domaintoolresult.ProtectedOutputTextV1{Availability: "unavailable", Reason: "limit"}
		}
		if separator != 0 {
			text.WriteByte('\n')
		}
		text.WriteString(value)
	}
	return domaintoolresult.ProjectProtectedOutputTextV1(text.String(), secrets...)
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
