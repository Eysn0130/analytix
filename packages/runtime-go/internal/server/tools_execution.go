package server

import (
	"context"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	toolsideeffect "analytix.local/runtime-go/internal/adapters/outbound/toolsideeffect"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	apploop "analytix.local/runtime-go/internal/app/loop"
	mcpapp "analytix.local/runtime-go/internal/app/mcp"
	appplan "analytix.local/runtime-go/internal/app/plan"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	provider "analytix.local/runtime-go/internal/provider"
)

func (h *runtimeServerHandler) authorizeRuntimePending(ctx context.Context, pending runtimePendingToolCall, approvalState string, now time.Time) error {
	if err := h.validateRuntimePendingCatalog(pending); err != nil {
		return err
	}
	return executiongrantapp.AuthorizePending(ctx, h.turnSecurity, filestore.CaseBindingReader{}, h.store, h.mcp, pending, approvalState, now)
}

func (h *runtimeServerHandler) validateRuntimePendingCatalog(pending runtimePendingToolCall) error {
	planActive := appplan.ToolActive(pending.Mode, pending.GUIPlan, pending.Workspace)
	mcpAdvertisements := toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(h.mcp, pending.SecurityContext)
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPAdvertisements(
		pending.DisableUserInput, pending.ToolScope, pending.SubagentDepth > 0, planActive, pending.Prompt,
		h.runtimeGoalToolsActive(pending.ThreadID, pending.Prompt, pending.ToolScope), mcpAdvertisements,
	)
	schemas = toolcatalogapp.SecurityScopedToolSchemas(pending.SecurityContext, pending.SubagentDepth, schemas)
	readOnly, _ := h.runtimeToolPolicy(pending.Call.Name)
	if toolcatalogapp.MCPToolServerID(pending.Call.Name) != "" {
		var found bool
		readOnly, found = toolcatalogapp.MCPReadOnlyPoliciesFromAdvertisementsV1(mcpAdvertisements)[pending.Call.Name]
		if !found {
			return executiongrantapp.ValidationError{Code: "execution_grant_source_unavailable"}
		}
	}
	return executiongrantapp.ValidatePendingCatalog(pending, schemas, readOnly)
}

func (h *runtimeServerHandler) acquireRuntimeToolCallAdmission(ctx context.Context, pending runtimePendingToolCall) (context.Context, func(), error) {
	effectCtx, release, err := h.acquireRuntimePendingToolEffect(ctx, pending)
	if err != nil {
		return ctx, nil, err
	}
	if effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return ctx, nil, executiongrantapp.ValidationError{Code: "execution_grant_authority_unavailable"}
	}
	if err := h.validateRuntimePendingCatalog(pending); err != nil {
		release()
		return ctx, nil, err
	}
	if err := executiongrantapp.AuthorizeUnregisteredPending(
		effectCtx,
		h.turnSecurity,
		filestore.CaseBindingReader{},
		h.store,
		h.mcp,
		pending,
		pending.ExecutionGrant.ApprovalState,
		time.Now().UTC(),
	); err != nil {
		release()
		return ctx, nil, err
	}
	return effectCtx, release, nil
}

func (h *runtimeServerHandler) persistToolCallReady(ctx context.Context, threadID, turnID string, call provider.ToolCall, readyCount int, securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) (string, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return "", executiongrantapp.ValidationError{Code: "execution_grant_authority_unavailable"}
		}
		return "", ctx.Err()
	}
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, call.ID)
	if itemID == "" {
		return "", executiongrantapp.ValidationError{Code: "execution_grant_identity_invalid"}
	}
	return appturn.PersistToolCallReady(h.store, appturn.ToolCallReadyInput{ThreadID: threadID, TurnID: turnID, ItemID: itemID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Call: call, ReadyCount: readyCount, ToolKind: toolcatalogapp.ToolKind(call.Name), Context: securityContext, Grant: grant})
}

func (h *runtimeServerHandler) requestRuntimeApproval(ctx context.Context, pending runtimePendingToolCall) (string, error) {
	return gatecontinuationapp.RequestApproval(ctx, pending, h.runtimeGateContinuationDependencies())
}

func (h *runtimeServerHandler) requestRuntimeUserInput(ctx context.Context, pending runtimePendingToolCall) (string, error) {
	return gatecontinuationapp.RequestUserInput(ctx, pending, h.runtimeGateContinuationDependencies())
}

// executeRuntimeToolWithEffectAuthority requires ctx to carry the effect lease
// acquired by ExecuteAndSettleTool or the durable batch transaction.
func (h *runtimeServerHandler) executeRuntimeToolWithEffectAuthority(ctx context.Context, pending runtimePendingToolCall, override any) (any, bool) {
	if err := h.authorizeRuntimePending(ctx, pending, "", time.Now().UTC()); err != nil {
		return executiongrantapp.ErrorDetails(err, pending.Call.Name), true
	}
	if override != nil {
		return override, false
	}
	h.recordRuntimeToolStarted(pending)
	if toolcatalogapp.ShouldRecordGenericToolProgress(pending.Call.Name) {
		h.recordRuntimeToolProgress(pending, "running", "tool running")
	}
	args, decodeErr := domainsecurity.DecodeCanonicalJSONObject(pending.Call.Arguments)
	if decodeErr != nil {
		return map[string]any{"code": "invalid_arguments", "error": "tool arguments failed strict host decoding", "executed": false}, true
	}
	switch pending.Call.Name {
	case "native_selection_read", "native_selection_propose":
		return h.executeNativeSelectionTool(ctx, pending, args)
	case "read":
		output, readContent, isError := executeReadRuntimeTool(pending, args, h.protectedReadDirs, h.allowWriteRoots, runtimeReadModeWindow)
		h.recordRuntimeRead(pending, output, readContent, isError)
		return output, isError
	case "read_file":
		output, readContent, isError := executeReadRuntimeTool(pending, args, h.protectedReadDirs, h.allowWriteRoots, runtimeReadModeNumbered)
		h.recordRuntimeRead(pending, output, readContent, isError)
		return output, isError
	case "ls":
		return executeListRuntimeTool(pending, args, h.protectedReadDirs, h.allowWriteRoots)
	case "find":
		return executeFindRuntimeTool(pending, args, h.protectedReadDirs, h.allowWriteRoots)
	case "glob":
		return executeGlobRuntimeTool(ctx, pending, args, h.protectedReadDirs, h.allowWriteRoots)
	case "code_index":
		return executeCodeIndexRuntimeTool(ctx, pending, args, h.protectedReadDirs, h.allowWriteRoots)
	case "grep":
		return executeGrepRuntimeTool(ctx, pending, args, h.protectedReadDirs, h.allowWriteRoots)
	case "web_fetch":
		return h.executeWebFetchRuntimeTool(ctx, pending, args)
	case "get_goal":
		return h.executeRuntimeGetGoalTool(pending)
	case "create_goal":
		return h.executeRuntimeCreateGoalTool(pending, args)
	case "complete_step":
		return h.executeRuntimeCompleteStepTool(ctx, pending, args)
	case "update_goal":
		return h.executeRuntimeUpdateGoalTool(ctx, pending, args)
	case "todo_list":
		return h.executeRuntimeTodoListTool(pending)
	case "todo_write":
		return h.executeRuntimeTodoWriteTool(pending, args)
	case "todo_ops", "todo_patch":
		return h.executeRuntimeTodoOpsTool(ctx, pending, args)
	case "bash":
		return h.executeBashRuntimeTool(ctx, pending, args)
	case "edit", "edit_file", "multi_edit":
		return h.executeEditRuntimeTool(ctx, pending, args)
	case "move_file":
		return h.executeMoveRuntimeTool(ctx, pending, args)
	case "notebook_edit":
		return h.executeNotebookEditRuntimeTool(ctx, pending, args)
	case "delete_range":
		return h.executeDeleteRangeRuntimeTool(ctx, pending, args)
	case "delete_symbol":
		return h.executeDeleteSymbolRuntimeTool(ctx, pending, args)
	case "delegate_task", "task":
		return h.executeRuntimeSubagentTask(ctx, pending, args)
	case toolcatalogapp.ForegroundSubmitToolName:
		return h.executeRuntimeForegroundChildSubmit(ctx, pending, args)
	case "parallel_tasks":
		return h.executeRuntimeParallelSubagents(ctx, pending, args)
	case "run_skill":
		return h.executeRuntimeRunSkill(ctx, pending, args)
	case runtimeCreatePlanToolName:
		return executeRuntimeCreatePlanTool(ctx, pending, args)
	case "wait":
		return h.executeRuntimeTaskJobWait(pending, args)
	case "list_jobs":
		return h.executeRuntimeTaskJobList(pending, args)
	case "bash_output":
		return h.executeRuntimeTaskJobOutput(pending, args)
	case "kill_shell":
		return h.executeRuntimeTaskJobKill(pending, args)
	case "restart_job":
		return h.executeRuntimeTaskJobRestart(pending, args)
	case "write", "write_file":
		return h.executeWriteRuntimeTool(ctx, pending, args)
	case "generate_office_document":
		return h.executeGenerateDocumentRuntimeTool(ctx, pending, args)
	default:
		if mcpapp.IsToolName(pending.Call.Name) {
			if h.beginOpaqueEditingExecution(ctx) != nil {
				return map[string]any{"code": "managed_editing_mutation_blocked", "error": "Uncontained MCP execution is unavailable while a controlled editing session is active.", "executed": false}, true
			}
			exactArgs, err := mcpapp.DecodeArguments(pending.Call.Arguments)
			if err != nil {
				return map[string]any{"code": "invalid_arguments", "error": err.Error(), "executed": false}, true
			}
			return mcpapp.ExecuteTool(ctx, h.mcp, mcpapp.ToolRunInputFromPending(pending, exactArgs))
		}
		return map[string]any{"code": "unknown_tool", "error": "unknown tool: " + pending.Call.Name}, true
	}
}

func (h *runtimeServerHandler) executeAndSettleRuntimeTool(ctx context.Context, pending runtimePendingToolCall, override any, transform apploop.ToolOutputTransform) (apploop.SettledToolExecution, error) {
	return apploop.ExecuteAndSettleWithSideEffectIntent(apploop.SideEffectIntentExecutionInput{
		Context: ctx, Pending: pending, Override: override, Transform: transform,
		Acquire: func(effectCtx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return h.acquireRuntimePendingToolEffect(effectCtx, pending)
		},
		Execute: h.executeRuntimeToolWithEffectAuthority, AccountFlowSource: h.mcp,
		Settle: h.settleRuntimeToolResultWithEffectAuthority, PendingWork: h.pendingWork,
		Prepare: h.prepareRuntimeSideEffect, ValidateApproved: h.validateRuntimeApprovedTransition,
		ResolvedEffectFree: h.runtimeRunSkillIsInline, Now: func() time.Time { return time.Now().UTC() },
	})
}

func (h *runtimeServerHandler) acquireRuntimePendingToolEffect(
	ctx context.Context,
	pending runtimePendingToolCall,
) (context.Context, func(), error) {
	if err := executiongrantapp.ValidateExecutionGrantForCall(
		pending.SecurityContext,
		pending.ExecutionGrant,
		pending.Call,
	); err != nil {
		return ctx, nil, err
	}
	caseDataEffect := executiongrantapp.CallUsesCaseDataAuthority(pending.Call) ||
		subagentapp.CaseBoundProviderChildCallRequiresCaseAuthorityV1(pending.SecurityContext, pending.Call.Name)
	return h.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, pending.SecurityContext, caseDataEffect)
}

func (h *runtimeServerHandler) prepareRuntimeSideEffect(ctx context.Context, pending runtimePendingToolCall, issuedAt time.Time) (apploop.PreparedSideEffect, error) {
	children := &runtimeChildProducerPlanV1{owner: h}
	prepared, err := toolsideeffect.Prepare(ctx, pending, issuedAt, toolsideeffect.Dependencies{
		GoalService: h.runtimeGoalToolService(), ResolveSkill: h.runtimeSkillByName, ResolveSkillBody: h.runtimeSkillEntryBody,
		ResolveTask: func(request subagentapp.TaskRequest) (any, error) {
			return children.reserveChildV1(ctx, pending, request)
		},
		MutationInput: func(effectCtx context.Context, pending runtimePendingToolCall, arguments map[string]any) filestore.MutationToolInput {
			return h.mutationToolInput(effectCtx, pending, arguments)
		},
	})
	if err != nil {
		return apploop.PreparedSideEffect{}, err
	}
	return children.bindPreparedSideEffectV1(pending, prepared)
}

func (h *runtimeServerHandler) runtimeRunSkillIsInline(pending runtimePendingToolCall) bool {
	return strings.TrimSpace(pending.ExecutionGrant.ToolName) == "run_skill" &&
		subagentapp.SkillRunIsInline(pending.Call.Arguments, h.runtimeSkillByName)
}

func (h *runtimeServerHandler) validateRuntimeApprovedTransition(pending runtimePendingToolCall) error {
	transition := pending.ApprovalTransition
	if h == nil || h.continuations == nil || transition == nil {
		return executiongrantapp.ValidationError{Code: "execution_grant_approval_transition_missing"}
	}
	thread, err := h.store.GetThread(pending.ThreadID)
	if err != nil {
		return err
	}
	durable, err := executiongrantapp.DurableApprovalTransitionFromThread(
		pending.ThreadID, thread, pending.TurnID, transition.TransitionID,
	)
	if err != nil || durable.Transition != *transition || durable.ApprovedGrant != pending.ExecutionGrant {
		return executiongrantapp.ValidationError{Code: "execution_grant_approval_transition_invalid"}
	}
	original := pending
	original.ExecutionGrant = durable.PendingGrant
	original.ApprovalTransition = nil
	receipt, disposition, approved, err := h.continuations.ResolveApprovedGrantHost(transition.ApprovalID, original)
	if err != nil || approved != pending.ExecutionGrant || receipt.ReceiptID != transition.ContinuationReceiptID ||
		disposition.DispositionID != transition.ContinuationDispositionID {
		return executiongrantapp.ValidationError{Code: "execution_grant_approval_disposition_invalid"}
	}
	return nil
}

func (h *runtimeServerHandler) executeRuntimeAuthorizedReadOnlyToolBatch(ctx context.Context, calls []runtimePendingToolCall) (string, []provider.Message, error) {
	if len(calls) == 0 {
		return "", nil, nil
	}
	var messages []provider.Message
	receipt, err := h.pendingWork.WithOpenToolBatch(ctx, h.runtimeSubagentState().EffectGate(), calls, func(effectCtx context.Context) error {
		results := toolcatalogapp.ExecuteBatch(effectCtx, calls, func(ctx context.Context, pending runtimePendingToolCall) (any, bool) {
			return h.executeRuntimeToolWithEffectAuthority(ctx, pending, nil)
		}, func(pending runtimePendingToolCall, cancelErr error) (any, bool) {
			return runtimeToolCancelledOutput(pending.Call, cancelErr), true
		})
		var executeErr error
		messages, executeErr = apploop.PersistReadOnlyBatchResults(effectCtx, results, func(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) (provider.Message, error) {
			settled, settleErr := apploop.SettleReadOnlyBatchToolOutputV1(ctx, pending,
				func(effectCtx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
					return apploop.AcquirePendingToolEffect(effectCtx, pending, h.runtimeSubagentState().AcquireContextEffectForAuthority)
				}, output, isError, h.settleRuntimeToolResultWithEffectAuthority, h.mcp)
			return settled.Message, settleErr
		})
		return executeErr
	})
	return receipt.WorkID, messages, err
}

func (h *runtimeServerHandler) runtimeToolPolicy(toolName string) (bool, bool) {
	if strings.TrimSpace(toolName) == toolcatalogapp.ForegroundSubmitToolName {
		return true, false
	}
	mcpAvailable := h != nil && h.mcp != nil
	mcpToolReadOnly := mcpAvailable && h.mcp.ToolReadOnlyHint(toolName)
	return toolcatalogapp.HostAuthorizesReadOnly(toolName, mcpAvailable, mcpToolReadOnly), toolcatalogapp.CanRunInParallel(toolName, mcpAvailable, mcpToolReadOnly)
}
