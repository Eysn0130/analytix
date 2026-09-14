package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	workspacefs "analytix.local/runtime-go/internal/adapters/outbound/workspacefs"
	controlapp "analytix.local/runtime-go/internal/app/control"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadsummaryapp "analytix.local/runtime-go/internal/app/threadsummary"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobs "analytix.local/runtime-go/internal/jobs"
	provider "analytix.local/runtime-go/internal/provider"
)

type runtimeSubagentTaskRequest = subagentapp.TaskRequest

type runtimeSubagentRunResult struct {
	Output  map[string]any
	Record  jobs.Record
	IsError bool
	Err     error
}

func (h *runtimeServerHandler) executeRuntimeSubagentTask(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	if !h.subagents.Enabled {
		return map[string]any{"code": "subagents_disabled", "error": "subagents are disabled in Analytix runtime settings"}, true
	}
	request, err := subagentapp.TaskRequestFromArgs(pending.Call.Name, args)
	if err != nil {
		return map[string]any{"code": "validation_error", "error": err.Error()}, true
	}
	request, err = subagentapp.BindForegroundHandoffPendingRequest(pending, request)
	if err != nil {
		return map[string]any{"code": "validation_error", "error": err.Error()}, true
	}
	result := h.runRuntimeSubagentTask(ctx, pending, request)
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeForegroundChildSubmit(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	if h == nil || h.jobs == nil {
		return map[string]any{"code": "foreground_handoff_unavailable", "error": "foreground child handoff authority is unavailable"}, true
	}
	return subagentapp.SubmitForegroundChildResult(ctx, h.foregroundHandoffs, h.jobs, pending, args)
}

// resolveRuntimeSubagentSideEffectProjection applies the same profile,
// execution, workspace, and delegated-tool owners that prepare a physical
// child run. Raw aliases, ordering, explicit-default flags, and unused block
// lists are intentionally excluded so they cannot mint a second durable
// semantic intent for the same effective child execution.
func (h *runtimeServerHandler) resolveRuntimeSubagentSideEffectProjection(ctx context.Context, pending runtimePendingToolCall, request runtimeSubagentTaskRequest) (map[string]any, error) {
	var err error
	request, err = subagentapp.BindForegroundHandoffPendingRequest(pending, request)
	if err != nil {
		return nil, err
	}
	securityBinding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		return nil, errors.New("subagent job security binding is unavailable")
	}
	request, err = h.bindRuntimeCaseDelegationRequest(ctx, pending, securityBinding, request)
	if err != nil {
		return nil, err
	}
	request = subagentapp.WithSystemPromptHint(request, apploop.LocalFilesystemHint())
	mcpAdvertisements := toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(h.mcp, pending.SecurityContext)
	return subagentapp.ResolveSideEffectProjection(subagentapp.SideEffectProjectionInput{
		Enabled: h.subagents.Enabled, Settings: h.subagents, Request: request,
		Parent:                  subagentapp.ParentExecution{ProviderID: pending.ProviderID, Model: pending.Model, Effort: pending.Effort, Workspace: pending.Workspace},
		ParentWorkspaceRealPath: pending.SecurityContext.WorkspaceRealPath, CallName: pending.Call.Name,
		MaxModelSteps: pending.MaxModelSteps, EffectiveMaxModelSteps: pending.EffectiveMaxModelSteps, Resolver: h.providerConfig,
		ResolveSource: h.resolveRuntimeSubagentSource,
		ResolveSourceLabel: func(source domainjob.Record) string {
			return threadsummaryapp.ResolveSubagentSourceDisplayLabel(source, h.store)
		},
		CanonicalizeWorkspace: workspacefs.ValidateSubagentWorkspace, MCPAdvertisements: mcpAdvertisements,
		AllowedToolScope: h.runtimeSubagentAllowedToolScope,
		ToolSchemas: func(toolScope []string, prompt string, advertisements []toolcatalogapp.MCPToolAdvertisementV1) []domainmodel.ToolSchema {
			return h.runtimeToolSchemasForPromptWithGoalToolsAndMCPAdvertisements(true, toolScope, true, false, prompt, false, advertisements)
		},
	})
}

func (h *runtimeServerHandler) executeRuntimeParallelSubagents(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	if !h.subagents.Enabled {
		return map[string]any{"code": "subagents_disabled", "error": "subagents are disabled in Analytix runtime settings"}, true
	}
	tasks, err := subagentapp.ParallelTaskRequestsFromArgs(args)
	if err != nil {
		return map[string]any{"code": "validation_error", "error": err.Error()}, true
	}
	securityBinding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		return map[string]any{"code": "validation_error", "error": "subagent job security binding is unavailable"}, true
	}
	for index := range tasks {
		bound, bindErr := h.bindRuntimeCaseDelegationRequest(ctx, pending, securityBinding, tasks[index].Request)
		if bindErr != nil {
			return map[string]any{"code": "validation_error", "error": bindErr.Error()}, true
		}
		tasks[index].Request = bound
	}
	result := subagentapp.RunParallelTasks(ctx, tasks, time.Now().UTC(), func(ctx context.Context, request subagentapp.TaskRequest) subagentapp.RunResult {
		child := h.runRuntimeSubagentTask(ctx, pending, request)
		return subagentapp.RunResult{Output: child.Output, IsError: child.IsError}
	})
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeRunSkill(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	name := subagentapp.SkillNameFromArgs(args)
	skill, ok := h.runtimeSkillByName(name)
	if !ok {
		return map[string]any{"code": "unknown_skill", "error": "unknown skill: " + strings.TrimSpace(name), "available": h.runtimeSkillIDs()}, true
	}
	runAs := subagentapp.SkillRunAs(skill)
	if runAs == "subagent" {
		prompt, promptErr := h.runtimeSkillSubagentPrompt(skill, subagentapp.SkillArguments(args))
		if promptErr != nil {
			return map[string]any{"code": "skill_snapshot_invalid", "error": promptErr.Error(), "skillId": stringField(skill, "id")}, true
		}
		request, err := subagentapp.SkillTaskRequestFromArgs(subagentapp.SkillTaskRequestInput{
			Skill:  skill,
			Args:   args,
			Prompt: prompt,
		})
		if err != nil {
			return map[string]any{"code": "validation_error", "error": err.Error()}, true
		}
		result := h.runRuntimeSubagentTask(ctx, pending, request)
		return subagentapp.RunSkillSubagentOutput(result.Output, skill), result.IsError
	}
	if subagentapp.SkillContinueOrForkRequested(args) {
		return map[string]any{"code": "validation_error", "error": "continue_from/fork_from are only valid for runAs=subagent skills"}, true
	}
	body, err := h.runtimeSkillEntryBody(skill)
	if err != nil {
		return map[string]any{"code": "skill_snapshot_invalid", "error": err.Error(), "skillId": stringField(skill, "id")}, true
	}
	return subagentapp.InlineSkillOutput(skill, args, runAs, body), false
}

func (h *runtimeServerHandler) runRuntimeSubagentTask(ctx context.Context, pending runtimePendingToolCall, request runtimeSubagentTaskRequest) runtimeSubagentRunResult {
	producerRequest := request
	acquireSignal := subagentapp.NewAcquireQueuedSignal(request.AcquireQueued)
	defer acquireSignal.SignalUnlessTransferred()
	if !h.subagents.Enabled {
		return runtimeSubagentFailedOutput(request, "subagents are disabled in Analytix runtime settings")
	}
	if h.beginOpaqueEditingExecution(ctx) != nil {
		return runtimeSubagentFailedOutput(request, "Subagent execution is unavailable while a controlled editing session is active")
	}
	securityBinding, securityErr := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if securityErr != nil {
		return runtimeSubagentFailedOutput(request, "subagent job security binding is unavailable")
	}
	request, err := h.bindRuntimeCaseDelegationRequest(ctx, pending, securityBinding, request)
	if err != nil {
		return runtimeSubagentFailedOutput(request, err.Error())
	}
	preparation, err := h.prepareRuntimeAdmittedChild(request, pending)
	request, source := preparation.Request, preparation.Source
	if err != nil {
		if errors.Is(err, subagentapp.ErrSourceToolScopeMismatch) {
			return runtimeSubagentRunResult{Output: subagentapp.SourceToolScopeMismatchOutput(request), IsError: true}
		}
		if errors.Is(err, subagentapp.ErrSourceNotReplayable) {
			return runtimeSubagentRunResult{Output: subagentapp.SourceNotReplayableOutput(request, source.ID, source.Status), IsError: true}
		}
		return runtimeSubagentFailedOutput(request, err.Error())
	}
	defer preparation.ReleaseSourceLock()
	childProducer, err := h.childProducerForRunV1(ctx, pending, preparation, producerRequest)
	if err != nil {
		return runtimeSubagentFailedOutput(request, err.Error())
	}
	hasSource, execution, workspace, toolScope := preparation.HasSource, preparation.Execution, preparation.Workspace, preparation.ToolScope
	providerID, model, endpointFormat, effort := execution.ProviderID, execution.Model, execution.EndpointFormat, execution.Effort
	isolatedWorktree, childThreadID := request.IsolationMode == string(domainjob.IsolationWorktree), ""
	parentGoal, _ := h.store.GetGoal(pending.ThreadID)
	record, err := subagentapp.StartPreparedTaskRun(subagentapp.PreparedTaskRunStartInput{
		Preparation: preparation, Pending: pending, Security: securityBinding, ParentGoal: parentGoal,
		Settings: h.subagents, StartChildRun: func(start domainjob.StartRequest) (domainjob.Record, error) {
			return h.startPreparedReservedChildV1(ctx, pending, childProducer, start)
		},
	})
	if err != nil {
		return runtimeSubagentFailedOutput(request, err.Error())
	}
	failStartedRecord := func(record jobs.Record, cause error) runtimeSubagentRunResult {
		if cause == nil {
			cause = fmt.Errorf("subagent admission failed")
		}
		if updated, updateErr := h.jobs.UpdateChildRun(record.ID, jobs.UpdateRequest{Status: string(domainjob.StatusFailed), Error: cause.Error()}); updateErr == nil {
			record = updated
		}
		h.recordRuntimeSubagentEvent(pending, record, string(domainjob.StatusFailed), cause.Error())
		return runtimeSubagentRunResult{Output: subagentapp.RunOutput(record, "", nil, cause), Record: record, IsError: true}
	}
	if record.CaseDelegation != nil && request.ForegroundHandoff &&
		request.ForegroundHandoffKind == subagentapp.ForegroundHandoffCaseTypedV1 {
		stageErr := request.UseCaseAnswerSlotBindingV1(func(binding domainjob.CaseDelegatedAnswerSlotBindingV1) error {
			if h.foregroundHandoffs == nil {
				return errors.New("foreground case allowance authority is unavailable")
			}
			return h.foregroundHandoffs.StageCase(record, pending.SecurityContext, binding)
		})
		if stageErr != nil {
			return failStartedRecord(record, stageErr)
		}
		// CompleteTask also deletes after its terminal projection, but failures
		// before CompleteTask (thread creation, admission, or slot acquisition)
		// must burn the process-local typed allowance immediately as well.
		defer h.foregroundHandoffs.Delete(record.ID)
	}
	childControl, executionCtx, err := subagentapp.BeginBoundChildAdmission(ctx, request.RunInBackground, h.runtimeSubagentState(), record.ID, record.SecurityBinding)
	if err != nil {
		return failStartedRecord(record, err)
	}
	defer childControl.Close()
	executionCtx = context.WithValue(executionCtx, runtimeChildProducerSlotKeyV1{}, childProducer)
	if isolatedWorktree {
		prepareThread := func(workspace string) (string, error) {
			return h.prepareRuntimeSubagentThread(executionCtx, pending, request, source, hasSource, providerID, model, endpointFormat, effort, workspace)
		}
		record, workspace, err = subagentapp.PrepareWorktreeIsolation(executionCtx, subagentapp.WorktreeIsolationPrepareInput{
			Manager: h.worktreeManager, Store: h.jobs, Record: record, ParentWorkspace: workspace, ParentThreadID: pending.ThreadID, PrepareThread: prepareThread,
		})
		if err != nil {
			return failStartedRecord(record, err)
		}
	} else {
		childThreadID, err = h.prepareRuntimeSubagentThread(executionCtx, pending, request, source, hasSource, providerID, model, endpointFormat, effort, workspace)
		if err != nil {
			return failStartedRecord(record, err)
		}
		updated, updateErr := h.jobs.UpdateChildRun(record.ID, jobs.UpdateRequest{ChildThreadID: childThreadID})
		if updateErr != nil {
			return failStartedRecord(record, updateErr)
		}
		record = updated
	}
	if err := executionCtx.Err(); err != nil {
		return failStartedRecord(record, err)
	}
	h.recordRuntimeSubagentEvent(pending, record, "queued", "")
	if preparation.SourceLockScope() == "fork" {
		preparation.ReleaseSourceLock()
	}
	if request.RunInBackground {
		backgroundCtx, backgroundCleanup := executionCtx, childControl.TransferCleanup()
		backgroundRelease := preparation.TransferSourceLock()
		backgroundRecord := record
		acquireSignal.Transfer()
		go func() {
			defer backgroundCleanup()
			defer backgroundRelease()
			releaseSlot, slotErr := h.acquireRuntimeSubagentSlot(backgroundCtx)
			if slotErr != nil {
				acquireSignal.Signal()
				updated, updateErr := h.jobs.UpdateChildRun(backgroundRecord.ID, jobs.UpdateRequest{Status: "killed", Error: slotErr.Error()})
				if updateErr == nil {
					backgroundRecord = updated
				}
				h.recordRuntimeSubagentEvent(pending, backgroundRecord, "killed", slotErr.Error())
				return
			}
			defer releaseSlot()
			acquireSignal.Signal()
			if pauseErr := h.pauseRuntimeBackgroundChildIfRequested(backgroundCtx, backgroundRecord.ID); pauseErr != nil {
				updated, updateErr := h.jobs.UpdateChildRun(backgroundRecord.ID, jobs.UpdateRequest{Status: "killed", Error: pauseErr.Error()})
				if updateErr == nil {
					backgroundRecord = updated
				}
				h.recordRuntimeSubagentEvent(pending, backgroundRecord, "killed", pauseErr.Error())
				return
			}
			updated, updateErr := h.jobs.UpdateChildRun(backgroundRecord.ID, jobs.UpdateRequest{Status: "running"})
			if updateErr == nil {
				backgroundRecord = updated
			}
			h.recordRuntimeSubagentEvent(pending, backgroundRecord, "running", "")
			h.completeRuntimeSubagentTask(backgroundCtx, pending, request, backgroundRecord, providerID, model, effort, toolScope, childControl)
		}()
		output := subagentapp.RunOutput(record, "", nil, nil)
		output["status"] = "queued"
		output["background"] = true
		return runtimeSubagentRunResult{Output: output, Record: record}
	}
	releaseSlot, slotErr := h.acquireRuntimeSubagentSlot(executionCtx)
	if slotErr != nil {
		acquireSignal.Signal()
		updated, updateErr := h.jobs.UpdateChildRun(record.ID, jobs.UpdateRequest{Status: "aborted", Error: slotErr.Error()})
		if updateErr == nil {
			record = updated
		}
		h.recordRuntimeSubagentEvent(pending, record, "aborted", slotErr.Error())
		return runtimeSubagentRunResult{Output: subagentapp.RunOutput(record, "", nil, slotErr), Record: record, IsError: true}
	}
	defer releaseSlot()
	acquireSignal.Signal()
	updated, updateErr := h.jobs.UpdateChildRun(record.ID, jobs.UpdateRequest{Status: "running"})
	if updateErr == nil {
		record = updated
	}
	h.recordRuntimeSubagentEvent(pending, record, "running", "")
	return h.completeRuntimeSubagentTask(executionCtx, pending, request, record, providerID, model, effort, toolScope, childControl)
}

func (h *runtimeServerHandler) prepareRuntimeAdmittedChild(request runtimeSubagentTaskRequest, pending runtimePendingToolCall) (subagentapp.TaskRunPreparation, error) {
	return subagentapp.PrepareTaskRun(subagentapp.TaskRunPreparationInput{
		Enabled: h.subagents.Enabled, Settings: h.subagents, Request: request, SystemPromptHint: apploop.LocalFilesystemHint(),
		Parent:                  subagentapp.ParentExecution{ProviderID: pending.ProviderID, Model: pending.Model, Effort: pending.Effort, Workspace: pending.Workspace},
		ParentWorkspaceRealPath: pending.SecurityContext.WorkspaceRealPath, ParentThreadID: pending.ThreadID, SecurityContext: pending.SecurityContext,
		Resolver: h.providerConfig, LockSource: h.jobs.LockChildRun, ResolveSource: h.resolveRuntimeSubagentSource,
		ResolveSourceLabel: func(source domainjob.Record) string {
			return threadsummaryapp.ResolveSubagentSourceDisplayLabel(source, h.store)
		},
		CanonicalizeWorkspace: workspacefs.ValidateSubagentWorkspace,
		ResolveMCPAdvertisements: func() []toolcatalogapp.MCPToolAdvertisementV1 {
			return toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(h.mcp, pending.SecurityContext)
		},
		AllowedToolScope: h.runtimeSubagentAllowedToolScope,
		ToolSchemas: func(scope []string, prompt string, advertisements []toolcatalogapp.MCPToolAdvertisementV1) []domainmodel.ToolSchema {
			return h.runtimeToolSchemasForPromptWithGoalToolsAndMCPAdvertisements(true, scope, true, false, prompt, false, advertisements)
		},
		ThreadIsDescendantOf: h.store.ThreadIsDescendantOf,
	})
}

func (h *runtimeServerHandler) bindRuntimeCaseDelegationRequest(
	ctx context.Context,
	pending runtimePendingToolCall,
	binding *domainjob.SecurityBinding,
	request runtimeSubagentTaskRequest,
) (runtimeSubagentTaskRequest, error) {
	var accountFlowSemantics []domainnative.AccountFlowProviderModelOutputV1
	var accountFlowAnswerSlots []domainjob.CaseDelegatedAnswerSlotBindingV1
	if request.ForegroundHandoff && request.ForegroundHandoffKind == subagentapp.ForegroundHandoffCaseTypedV1 {
		var semanticErr error
		accountFlowSemantics, semanticErr = privacyprojectionapp.AccountFlowProviderSemanticsForCaseDelegationV1(
			pending.SecurityContext, pending.Messages,
		)
		if semanticErr != nil || len(accountFlowSemantics) != 1 {
			return runtimeSubagentTaskRequest{}, errors.New("foreground case child requires one exact host-bound account-flow semantic")
		}
		if h.caseAnswerSlots == nil {
			return runtimeSubagentTaskRequest{}, errors.New("foreground case child answer-slot authority is unavailable")
		}
		accountFlowAnswerSlots, semanticErr = h.caseAnswerSlots(ctx, pending.SecurityContext, accountFlowSemantics)
		if semanticErr != nil || len(accountFlowAnswerSlots) != 1 {
			return runtimeSubagentTaskRequest{}, errors.New("foreground case child requires one exact current claim/evidence group")
		}
	}
	return subagentapp.PrepareCaseDelegationV1(ctx, subagentapp.BindCaseDelegationInputV1{
		SecurityContext:        pending.SecurityContext,
		SecurityBinding:        binding,
		CaseEntities:           h.caseEntities,
		Request:                request,
		AccountFlowSemantics:   accountFlowSemantics,
		AccountFlowAnswerSlots: accountFlowAnswerSlots,
	})
}

func (h *runtimeServerHandler) completeRuntimeSubagentTask(ctx context.Context, pending runtimePendingToolCall, request runtimeSubagentTaskRequest, record jobs.Record, providerID, model, effort string, toolScope []string, startAuthority subagentapp.ChildTurnStartAuthority) runtimeSubagentRunResult {
	result := subagentapp.CompleteTask(ctx, subagentapp.CompleteTaskInput{
		Request:              request,
		Record:               record,
		ProviderID:           providerID,
		Model:                model,
		Effort:               effort,
		ParentMaxModelSteps:  subagentapp.ParentMaxModelSteps(pending.MaxModelSteps, pending.EffectiveMaxModelSteps),
		ParentApprovalPolicy: pending.ApprovalPolicy,
		ParentSandboxMode:    pending.SandboxMode,
		ParentSubagentDepth:  pending.SubagentDepth,
		ToolScope:            toolScope,
		WorktreeManager:      h.worktreeManager,
		Driver:               runtimeSubagentCompletionDriver{handler: h, pending: pending},
		AuthorizeStart:       h.authorizeRuntimeJobStart,
		StartAuthority:       startAuthority,
		CompletionAuthority:  h.childCompletions,
		ForegroundAuthority:  h.foregroundHandoffs,
	})
	return runtimeSubagentRunResult{Output: result.Output, Record: result.Record, IsError: result.IsError, Err: result.Err}
}

func (h *runtimeServerHandler) authorizeRuntimeJobStart(record jobs.Record) (jobs.Record, error) {
	if err := h.beginOpaqueEditingExecution(context.Background()); err != nil {
		return jobs.Record{}, err
	}
	authority := h.runtimeJobSecurityAuthorizer()
	return subagentapp.AuthorizeJobStart(record, h != nil && h.jobs != nil && h.store != nil, h.jobs, authority.Blocker, authority.MarkDeadLetter)
}

func (h *runtimeServerHandler) restartRuntimeSubagentTaskJob(parentThreadID string, jobID string) subagentapp.TaskJobServiceResult {
	record, err := h.jobs.LoadChildRun(jobID)
	if err != nil {
		return subagentapp.TaskJobServiceResult{Response: subagentapp.TaskJobNotFoundResponse(jobID), IsError: true, ErrorCode: subagentapp.TaskJobErrorNotFound, Err: err}
	}
	if result, ok := subagentapp.ValidateRestartTaskJobRecord(record, parentThreadID, jobID); !ok {
		return result
	}
	if reason := h.runtimeJobSecurityAuthorizer().Blocker(record); reason != "" {
		return subagentapp.TaskJobServiceResult{Response: subagentapp.TaskJobNotFoundResponse(jobID), IsError: true, ErrorCode: subagentapp.TaskJobErrorNotFound}
	}
	if record.SecurityBinding != nil {
		return subagentapp.TaskJobServiceResult{Response: subagentapp.ValidationErrorResponse("security-bound task jobs must be restarted by a new authorized task tool call"), Record: record, IsError: true, ErrorCode: subagentapp.TaskJobErrorValidation}
	}
	thread, _ := h.store.GetThread(parentThreadID)
	args := subagentapp.RestartTaskJobArguments(record)
	workspace := firstNonEmptyString(record.Workspace, stringField(thread, "workspace"))
	pending := runtimePendingToolCall{
		ThreadID:               parentThreadID,
		TurnID:                 firstNonEmptyString(record.ParentTurnID, "summary-restart"),
		ProviderID:             record.ProviderID,
		Model:                  record.Model,
		Effort:                 record.Effort,
		Workspace:              workspace,
		ApprovalPolicy:         firstNonEmptyString(stringField(thread, "approvalPolicy"), h.approvalPolicy),
		SandboxMode:            firstNonEmptyString(stringField(thread, "sandboxMode"), h.sandboxMode),
		EffectiveMaxModelSteps: record.MaxModelSteps,
		Call: domainmodel.ToolCall{
			ID:        "restart_" + safeDurableID(jobID),
			Name:      "task",
			Arguments: args,
		},
		ToolCallItemID: firstNonEmptyString(record.ParentToolItemID, "summary_restart_"+safeDurableID(jobID)),
	}
	request := subagentapp.RestartTaskJobRequest(record, workspace)
	result := h.runRuntimeSubagentTask(context.Background(), pending, request)
	return subagentapp.TaskJobServiceResult{
		Response: result.Output,
		Record:   result.Record,
		IsError:  result.IsError,
	}
}

type runtimeSubagentCompletionDriver struct {
	handler *runtimeServerHandler
	pending runtimePendingToolCall
}

func (d runtimeSubagentCompletionDriver) StartTurn(ctx context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
	return d.handler.startRuntimeTurn(ctx, request.ThreadID, request)
}

func (d runtimeSubagentCompletionDriver) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	return d.handler.jobs.UpdateChildRun(id, request)
}

func (d runtimeSubagentCompletionDriver) LoadChildRun(id string) (domainjob.Record, error) {
	return d.handler.jobs.LoadChildRun(id)
}

func (d runtimeSubagentCompletionDriver) TurnCompletion(threadID string, turnID string) (string, int, error) {
	if err := d.handler.store.requireRestartWritableNoLockV1(threadID); err != nil {
		return "", 0, err
	}
	return subagentapp.TurnCompletionFromThreadStoreV1(d.handler.store.GetThread, threadID, turnID)
}

func (d runtimeSubagentCompletionDriver) UsageSnapshot(threadID string) (map[string]any, error) {
	return d.handler.threadUsageSnapshot(threadID)
}

func (d runtimeSubagentCompletionDriver) CacheDiagnostics(threadID string, turnID string) (map[string]any, error) {
	return d.handler.latestRuntimeUsageCacheDiagnostics(threadID, turnID)
}

func (d runtimeSubagentCompletionDriver) RecordProgress(record domainjob.Record, status string, message string) {
	d.handler.recordRuntimeSubagentEvent(d.pending, record, status, message)
}

func (h *runtimeServerHandler) resolveRuntimeSubagentSource(request runtimeSubagentTaskRequest) (jobs.Record, bool, error) {
	ref := strings.TrimSpace(firstNonEmptyAnyString(request.ContinueFrom, request.ForkFrom))
	if ref == "" {
		return jobs.Record{}, false, nil
	}
	record, err := h.jobs.Load(ref)
	if err != nil {
		return jobs.Record{}, false, fmt.Errorf("load subagent reference %q: %w", ref, err)
	}
	return record, true, nil
}

func (h *runtimeServerHandler) prepareRuntimeSubagentThread(ctx context.Context, pending runtimePendingToolCall, request runtimeSubagentTaskRequest, source jobs.Record, hasSource bool, providerID, model, endpointFormat, effort, workspace string) (string, error) {
	slot, ok := ctx.Value(runtimeChildProducerSlotKeyV1{}).(*runtimeChildProducerSlotV1)
	if !ok || slot == nil || slot.owner != h {
		return "", errors.New("signed child thread producer is unavailable")
	}
	threadID, err := subagentapp.PrepareChildThread(subagentapp.PrepareChildThreadInput{
		Store:                 reservedChildThreadStoreV1{ctx: ctx, slot: slot},
		ParentThreadID:        pending.ThreadID,
		PendingCallID:         pending.Call.ID,
		Request:               request,
		Source:                source,
		HasSource:             hasSource,
		DistinctNameCandidate: threadsummaryapp.DistinctSubagentNameCandidate(request.Name, request.Label, request.ProfileName),
		ProviderID:            providerID,
		Model:                 model,
		EndpointFormat:        endpointFormat,
		Effort:                effort,
		Workspace:             workspace,
		ApprovalPolicy:        pending.ApprovalPolicy,
		SandboxMode:           pending.SandboxMode,
	})
	if err != nil {
		return "", err
	}
	if threadID != slot.target.ChildThreadID {
		return "", errors.New("child thread differs from signed target")
	}
	return threadID, nil
}

func (h *runtimeServerHandler) parentGoalForSubagent(threadID string) (string, string) {
	goal, _ := h.store.GetGoal(threadID)
	return subagentapp.ParentGoalIdentity(goal, threadID)
}

func (h *runtimeServerHandler) runtimeSubagentToolScope(request runtimeSubagentTaskRequest) []string {
	return subagentapp.ToolScope(request.Tools, h.runtimeSubagentAllowedToolScope(request, nil))
}

func (h *runtimeServerHandler) runtimeSubagentAllowedToolScope(request runtimeSubagentTaskRequest, mcpTools []string) []string {
	return subagentapp.AllowedToolScopeForPolicy(request, runtimeinfoapp.WebFetchEnabled(h.web), mcpTools)
}

func (h *runtimeServerHandler) recordRuntimeSubagentEvent(pending runtimePendingToolCall, record jobs.Record, status string, message string) {
	if h == nil {
		return
	}
	h.runtimeBackgroundDeliveryService().RecordSubagentLifecycleV1(
		pending.ThreadID, pending.TurnID, pending.ToolCallItemID, pending.Call.ID, pending.Call.Name, record, status, message,
	)
}

// PublishBackgroundCompletionLifecycleExact uses the complete canonical event
// bundle as a durable outbox identity. The bundle is inspected and atomically
// appended under the existing event-store lock, so an admission/event crash
// gap is repairable while exact live, recovery, and restart replays are inert.
func (s *DurableEventSessionStore) PublishBackgroundCompletionLifecycleExact(drafts []map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	loadEvents := func(threadID string) ([]map[string]any, error) {
		replay, err := s.eventLog.LoadSinceContext(context.Background(), threadID, 0)
		if err == nil && len(replay.Diagnostics) != 0 {
			err = errors.New("background completion lifecycle replay requires migration")
		}
		return replay.Events, err
	}
	operations := subagentapp.NewBackgroundCompletionLifecycleExactOperationsV1(
		loadEvents, s.acceptedFinalEvents.PublicationReservedOwnerLocked, s.readThreadNoLock, s.caseThreadView, s.beforeRecordEventHook, s.nextSeqNoLock, s.persistRecordedEventsNoLock, os.ErrNotExist,
		func(threadID string, highest int) { s.highestSeqByThread[threadID] = highest }, s.publishEventBundleNoLock,
	)
	return subagentapp.ReconcileBackgroundCompletionLifecycleExactV1(drafts, operations)
}

func (h *runtimeServerHandler) recordRuntimeToolProgress(pending runtimePendingToolCall, status string, message string) {
	toolcatalogapp.RecordToolProgressEvent(h.recordRuntimeBestEffortEvent, toolcatalogapp.ToolProgressEventInput{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: pending.ToolCallItemID, CallID: pending.Call.ID, ToolName: pending.Call.Name, Status: status, Message: message})
}

func (h *runtimeServerHandler) recordRuntimeTaskJobProgress(pending runtimePendingToolCall, record jobs.Record, status string, message string) {
	subagentapp.RecordTaskJobProgressEvent(h.recordRuntimeBestEffortEvent, subagentapp.TaskJobProgressEventInput{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: pending.ToolCallItemID, CallID: pending.Call.ID, ToolName: pending.Call.Name, Record: record, Status: status, ProgressStatus: toolcatalogapp.ToolProgressStatus(status), Message: message})
}

func (h *runtimeServerHandler) recordRuntimeJobLifecycleEvent(record jobs.Record, status string, message string) {
	if h == nil {
		return
	}
	h.runtimeBackgroundDeliveryService().RecordJobLifecycleV1(record)
}

func (h *runtimeServerHandler) runtimeBackgroundDeliveryService() subagentapp.BackgroundDeliveryService {
	authority := h.runtimeJobSecurityAuthorizer()
	service := subagentapp.NewBackgroundLifecycleDeliveryServiceV1(h.store, h.jobs, authority.Blocker, h.recordRuntimeBestEffortEvent)
	service.SecurityBlockerAt = authority.HistoricalBlockerAt
	service.CompletionAuthority = h.childCompletions
	service.BindLifecycleRuntimeV1(h.store, authority.MarkDeadLetter, runtimeBackgroundLifecycleDiagnosticV1)
	service.AutoContinueStarter = subagentapp.BackgroundAutoContinueRuntimeStarterV1{
		Allocate: h.nextRuntimeTurnIdentity,
		Send:     h.runtimeControl().SendTurn, LoadThread: h.store.GetThread,
	}
	return service
}

func runtimeBackgroundLifecycleDiagnosticV1(owner string) {
	fmt.Fprintf(os.Stderr, "analytix runtime: %s background lifecycle reconciliation failed code=background_lifecycle_reconcile_failed\n", owner)
}

func (h *runtimeServerHandler) nextRuntimeTurnIdentity() (string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	turnID, sequence, _ := h.nextRuntimeTurnIdentityNoLock()
	return turnID, sequence
}

func (h *runtimeServerHandler) pauseRuntimeBackgroundChildIfRequested(ctx context.Context, childRunID string) error {
	return subagentapp.WaitAtBackgroundPauseBoundary(ctx, subagentapp.PauseBoundaryInput{ChildRunID: childRunID, State: h.runtimeSubagentState(), Jobs: h.jobs, RecordEvent: subagentapp.BindChildPauseEventRecorder(h.recordRuntimeBestEffortEvent)})
}

func (h *runtimeServerHandler) recordRuntimeToolStarted(pending runtimePendingToolCall) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event, ok := toolcatalogapp.BuildToolStartedEvent(toolcatalogapp.ToolStartedEventInput{
		ThreadID:  pending.ThreadID,
		TurnID:    pending.TurnID,
		ItemID:    pending.ToolCallItemID,
		CallID:    pending.Call.ID,
		ToolName:  pending.Call.Name,
		CreatedAt: now,
	})
	if ok {
		_ = h.store.PatchTurnItemStatus(pending.ThreadID, pending.TurnID, pending.ToolCallItemID, "running")
		h.recordRuntimeBestEffortEvent(event, "tool started")
	}
}

func (h *runtimeServerHandler) recordRuntimeBestEffortEvent(event map[string]any, _ string) {
	if _, _, err := h.store.RecordEvent(event); err != nil {
		fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_EVENT_RECORD_FAILED")
	}
}

func runtimeSubagentFailedOutput(request runtimeSubagentTaskRequest, message string) runtimeSubagentRunResult {
	return runtimeSubagentRunResult{Output: subagentapp.FailedOutput(request, message), IsError: true}
}

func (h *runtimeServerHandler) runtimeBackgroundJobContextMessages(threadID string, subagentDepth int, securityContext domainsecurity.TurnSecurityContext) []provider.Message {
	if subagentDepth > 0 {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	records, err := h.runtimeSubagentService().ListTaskJobs(threadID)
	if err != nil {
		return nil
	}
	records = subagentapp.SecurityBoundRecords(records, securityContext)
	records = subagentapp.BackgroundJobContextRecords(records, subagentapp.DefaultBackgroundJobContextLimit)
	if len(records) == 0 {
		return nil
	}
	note := subagentapp.BackgroundJobContextNote(records, time.Now().UTC(), subagentapp.DefaultTaskJobStalledAfter, subagentapp.DefaultBackgroundJobContextOutputLimit)
	return []provider.Message{{Role: "user", Content: note}}
}

func (h *runtimeServerHandler) executeRuntimeTaskJobWait(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request := subagentapp.TaskJobWaitRequestFromArgs(args)
	result := h.runtimeSubagentService().WaitTaskJobs(pending.ThreadID, request, true)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobList(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobListRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	result := h.runtimeSubagentService().ListTaskJobViews(pending.ThreadID, request)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobOutput(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobOutputRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	result := h.runtimeSubagentService().OutputTaskJob(pending.ThreadID, request, true)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobKill(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobKillRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	result := h.runtimeSubagentService().KillTaskJob(pending.ThreadID, request)
	if !result.IsError && strings.TrimSpace(result.Record.ID) != "" {
		h.recordRuntimeJobLifecycleEvent(result.Record, result.Record.Status, firstNonEmptyString(result.Record.Error, request.Reason))
	}
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobRestart(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobRestartRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	result := h.runtimeTaskJobHTTPService().RestartTaskJob(pending.ThreadID, request)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobSteer(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobSteerRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	request.SourceTurnID = pending.TurnID
	request.SourceToolCallID = pending.Call.ID
	result := h.runtimeTaskJobHTTPService().SteerTaskJob(ctx, pending.ThreadID, request)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobPause(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobPauseRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	if strings.TrimSpace(request.SourceTurnID) == "" {
		request.SourceTurnID = pending.TurnID
	}
	result := h.runtimeTaskJobHTTPService().PauseTaskJob(pending.ThreadID, request)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTaskJobResume(pending runtimePendingToolCall, args map[string]any) (any, bool) {
	request, validation, invalid := subagentapp.TaskJobResumeRequestFromArgs(args)
	if invalid {
		return validation, true
	}
	if strings.TrimSpace(request.SourceTurnID) == "" {
		request.SourceTurnID = pending.TurnID
	}
	result := h.runtimeTaskJobHTTPService().ResumeTaskJob(pending.ThreadID, request)
	return result.Response, result.IsError
}

func (h *runtimeServerHandler) runtimeTaskJobHTTPService() subagentapp.TaskJobHTTPService {
	return subagentapp.NewTaskJobHTTPService(subagentapp.TaskJobHTTPServiceDeps{
		Service:             h.runtimeSubagentService(),
		Jobs:                h.jobs,
		Turns:               h.store,
		RecordEvent:         h.recordRuntimeBestEffortEvent,
		ParentWorkspace:     subagentapp.ParentWorkspaceResolver(h.store),
		RecordLifecycle:     h.recordRuntimeJobLifecycleEvent,
		RestartJob:          h.restartRuntimeSubagentTaskJob,
		BeginSteerAuthority: h.beginRuntimeTaskJobSteerAuthority,
	})
}

func (h *runtimeServerHandler) beginRuntimeTaskJobSteerAuthority(
	ctx context.Context,
	record domainjob.Record,
	message domainjob.SteerMessage,
) (subagentapp.TaskJobSteerAuthorization, string, error) {
	return subagentapp.BeginCurrentTaskJobSteerAuthorityV1(ctx, subagentapp.CurrentTaskJobSteerAuthorityDepsV1{
		Jobs: h.jobs, ObserveFirstTurn: h.observeRuntimeChildFirstTurnV1, State: h.runtimeSubagentState(), Security: h.turnSecurity,
		Control: h.runtimeControl(), Blocker: h.runtimeJobSecurityAuthorizer().Blocker,
	}, record, message)
}
