package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	toolsideeffect "analytix.local/runtime-go/internal/adapters/outbound/toolsideeffect"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appplan "analytix.local/runtime-go/internal/app/plan"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

func executeRuntimeCreatePlanTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	if prepared, ok := toolsideeffect.Plan(ctx); ok {
		return filestore.ExecutePreparedCreatePlanTool(prepared)
	}
	return map[string]any{"code": "prepared_plan_invalid", "error": "create_plan prepared owner authority is unavailable"}, true
}

func (h *runtimeServerHandler) materializeRuntimeCreatePlanText(
	ctx context.Context,
	input runtimeAgentLoopInput,
	messages []provider.Message,
	planText string,
	privateProtocolObserved bool,
) (any, bool, []provider.Message, domainsecurity.SettledToolReference, error) {
	args := appplan.FallbackArgs(appplan.FallbackInput{
		Prompt:    input.Request.Prompt,
		GUIPlan:   input.Request.GUIPlan,
		Workspace: input.Workspace,
	}, planText)
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return nil, true, messages, domainsecurity.SettledToolReference{}, err
	}
	call := provider.ToolCall{
		ID:        appplan.MaterializedCallID(input.SecurityContext.ContextDigest),
		Name:      runtimeCreatePlanToolName,
		Arguments: rawArgs,
	}
	exactToolScope := []string{runtimeCreatePlanToolName}
	toolSchemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(input.Request.DisableUserInput, exactToolScope, input.SubagentDepth > 0, true, input.Request.Prompt, false, nil)
	grant, err := executiongrantapp.IssueHost(input.SecurityContext, input.ProviderID, call, toolSchemas, exactToolScope, false, "not_required", 0, "", time.Now().UTC())
	if err != nil {
		return executiongrantapp.ErrorDetails(err, call.Name), true, messages, domainsecurity.SettledToolReference{}, err
	}
	pending := runtimePendingToolCall{
		ThreadID:                input.ThreadID,
		TurnID:                  input.TurnID,
		ProviderConfig:          input.ProviderConfig,
		ProviderID:              input.ProviderID,
		Model:                   input.Model,
		Effort:                  input.Effort,
		Workspace:               input.Workspace,
		Prompt:                  input.Request.Prompt,
		Mode:                    input.Request.Mode,
		GUIPlan:                 cloneMap(input.Request.GUIPlan),
		ApprovalPolicy:          input.ApprovalPolicy,
		SandboxMode:             input.SandboxMode,
		DisableUserInput:        input.Request.DisableUserInput,
		MaxModelSteps:           input.Request.MaxModelSteps,
		Messages:                append([]provider.Message(nil), messages...),
		Call:                    call,
		ToolScope:               append([]string(nil), exactToolScope...),
		SubagentDepth:           input.SubagentDepth,
		SecurityContext:         input.SecurityContext,
		ExecutionGrant:          grant,
		PrivateProtocolObserved: privateProtocolObserved,
	}
	admissionCtx, releaseAdmission, err := h.acquireRuntimeToolCallAdmission(ctx, pending)
	if err != nil {
		return executiongrantapp.ErrorDetails(err, call.Name), true, messages, domainsecurity.SettledToolReference{}, err
	}
	itemID, err := h.persistToolCallReady(admissionCtx, input.ThreadID, input.TurnID, call, 1, input.SecurityContext, grant)
	releaseAdmission()
	if err != nil {
		return nil, true, messages, domainsecurity.SettledToolReference{}, err
	}
	pending.ToolCallItemID = itemID
	settled, err := h.executeAndSettleRuntimeTool(ctx, pending, nil, nil)
	if err != nil {
		return settled.Output, settled.IsError, messages, domainsecurity.SettledToolReference{}, err
	}
	output, isError := settled.Output, settled.IsError
	materializedMessages := append([]provider.Message(nil), messages...)
	materializedMessages = append(materializedMessages, provider.Message{
		Role:      "assistant",
		Content:   strings.TrimSpace(planText),
		ToolCalls: []provider.ToolCall{call},
	})
	return output, isError, materializedMessages, domainsecurity.SettledToolReference{
		GrantID: grant.GrantID, ResultItemID: toolcatalogapp.ToolResultItemID(input.TurnID, call.ID),
	}, nil
}
