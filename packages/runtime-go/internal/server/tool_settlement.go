package server

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	provider "analytix.local/runtime-go/internal/provider"
)

func (h *runtimeServerHandler) clearRuntimeReadRecords(threadID string) {
	h.runtimeReadRegistry().Clear(threadID)
}

func (h *runtimeServerHandler) recordRuntimeRead(pending runtimePendingToolCall, output any, readContent string, isError bool) {
	if isError {
		return
	}
	record, ok := output.(map[string]any)
	if !ok {
		return
	}
	path, _ := record["path"].(string)
	content, contentOK := record["content"].(string)
	if strings.TrimSpace(path) == "" || !contentOK {
		return
	}
	if readContent != "" {
		content = readContent
	}
	resolved, ok := filestore.ResolveReadPathInfo(pending.Workspace, path, pending.SandboxMode, h.allowWriteRoots)
	if !ok {
		return
	}
	h.runtimeReadRegistry().Record(filetoolsapp.ReadRegistryRecordInput{
		ThreadID: pending.ThreadID,
		Key:      runtimeReadRegistryKey(resolved.Path),
		Record: filetoolsapp.ReadRecord{
			TurnID:       pending.TurnID,
			Path:         resolved.Path,
			RelativePath: workspaceRelativePath(pending.Workspace, resolved.Path),
			Content:      content,
			Truncated:    boolField(record, "truncated"),
		},
	})
}

func (h *runtimeServerHandler) runtimeReadBeforeChangeInput(pending runtimePendingToolCall, resolved string, args map[string]any) filetoolsapp.ReadBeforeChangeInput {
	return h.runtimeReadRegistry().Input(filetoolsapp.ReadRegistryInputQuery{
		ThreadID:     pending.ThreadID,
		TurnID:       pending.TurnID,
		Key:          runtimeReadRegistryKey(resolved),
		Path:         resolved,
		RelativePath: workspaceRelativePath(pending.Workspace, resolved),
		Args:         args,
		IsGoFile:     strings.ToLower(filepath.Ext(resolved)) == ".go",
	})
}

func runtimeReadRegistryKey(path string) string {
	if realPath, err := filestore.WorkspaceRealPath(path); err == nil {
		return realPath
	}
	return path
}

func (h *runtimeServerHandler) runtimeReadRegistry() *filetoolsapp.ReadRegistry {
	if h.readRegistry != nil {
		return h.readRegistry
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.readRegistry == nil {
		h.readRegistry = filetoolsapp.NewReadRegistry()
	}
	return h.readRegistry
}

func (h *runtimeServerHandler) runtimeToolPreflight(pending runtimePendingToolCall) (any, bool, bool) {
	decision := toolcatalogapp.PreflightTool(toolcatalogapp.ToolPreflightInput{
		ToolName:               pending.Call.Name,
		SubagentDepth:          pending.SubagentDepth,
		ApprovalPolicy:         pending.ApprovalPolicy,
		SandboxMode:            pending.SandboxMode,
		BlockedByApprovalNever: h.runtimeToolBlockedByApprovalNever(pending.Call.Name),
		CaseBound:              toolcatalogapp.IsCaseBoundSecurityContext(pending.SecurityContext),
		HostReadOnly:           pending.ExecutionGrant.ReadOnly,
	})
	if decision.Blocked || !decision.RequiresReadGuard {
		return decision.Output, decision.IsError, decision.Blocked
	}
	args, decodeErr := domainsecurity.DecodeCanonicalJSONObject(pending.Call.Arguments)
	if decodeErr != nil {
		return map[string]any{"code": "invalid_arguments", "error": "tool arguments failed strict host decoding", "executed": false}, true, true
	}
	switch decision.ReadGuard {
	case toolcatalogapp.ReadGuardDeleteRange:
		output, blocked := h.validateRuntimeReadBeforeDeleteRange(pending, args)
		if !blocked {
			return nil, false, false
		}
		return output, true, true
	case toolcatalogapp.ReadGuardDeleteSymbol:
		output, blocked := h.validateRuntimeReadBeforeDeleteSymbol(pending, args)
		if !blocked {
			return nil, false, false
		}
		return output, true, true
	case toolcatalogapp.ReadGuardEdit:
		output, blocked := h.validateRuntimeReadBeforeEdit(pending, args)
		if !blocked {
			return nil, false, false
		}
		return output, true, true
	default:
		return nil, false, false
	}
}

func (h *runtimeServerHandler) validateRuntimeReadBeforeDeleteRange(pending runtimePendingToolCall, args map[string]any) (map[string]any, bool) {
	path := strings.TrimSpace(firstNonEmptyAnyString(args["path"], args["filePath"], args["FilePath"]))
	if path == "" {
		return map[string]any{"code": "validation_error", "error": "path is required"}, true
	}
	resolved, ok := filestore.ResolveWritePathForSandbox(pending.Workspace, path, pending.SandboxMode, h.allowWriteRoots)
	if !ok {
		return map[string]any{"code": "workspace_escape", "error": "path must stay inside workspace or configured allow_write root"}, true
	}
	return filetoolsapp.ValidateReadBeforeDeleteRange(h.runtimeReadBeforeChangeInput(pending, resolved, args))
}

func (h *runtimeServerHandler) validateRuntimeReadBeforeDeleteSymbol(pending runtimePendingToolCall, args map[string]any) (map[string]any, bool) {
	path := strings.TrimSpace(firstNonEmptyAnyString(args["path"], args["filePath"], args["FilePath"]))
	if path == "" {
		return map[string]any{"code": "validation_error", "error": "path is required"}, true
	}
	resolved, ok := filestore.ResolveWritePathForSandbox(pending.Workspace, path, pending.SandboxMode, h.allowWriteRoots)
	if !ok {
		return map[string]any{"code": "workspace_escape", "error": "path must stay inside workspace or configured allow_write root"}, true
	}
	return filetoolsapp.ValidateReadBeforeDeleteSymbol(h.runtimeReadBeforeChangeInput(pending, resolved, args))
}

func (h *runtimeServerHandler) validateRuntimeReadBeforeEdit(pending runtimePendingToolCall, args map[string]any) (map[string]any, bool) {
	path := strings.TrimSpace(firstNonEmptyAnyString(args["path"], args["filePath"], args["FilePath"]))
	if path == "" {
		return map[string]any{"code": "validation_error", "error": "path is required"}, true
	}
	resolved, ok := filestore.ResolveWritePathForSandbox(pending.Workspace, path, pending.SandboxMode, h.allowWriteRoots)
	if !ok {
		return map[string]any{"code": "workspace_escape", "error": "path must stay inside workspace or configured allow_write root"}, true
	}
	return filetoolsapp.ValidateReadBeforeEdit(h.runtimeReadBeforeChangeInput(pending, resolved, args))
}

func (h *runtimeServerHandler) persistRuntimeToolResult(pending runtimePendingToolCall, projection domaintoolresult.PublicToolResultProjectionV1, settlement evidenceapp.PreparedToolSettlement) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID:           pending.ThreadID,
		TurnID:             pending.TurnID,
		CreatedAt:          now,
		FinishedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Call:               pending.Call,
		Projection:         projection,
		IsError:            settlement.IsError,
		ContextDigest:      pending.SecurityContext.ContextDigest,
		ContextEpoch:       pending.SecurityContext.ContextEpoch,
		ExecutionGrantID:   pending.ExecutionGrant.GrantID,
		CaseID:             pending.SecurityContext.CaseID,
		DatasetSnapshotID:  pending.SecurityContext.DatasetSnapshotID,
		ServerIdentity:     pending.ExecutionGrant.ServerIdentity,
		PrivateCaseOutcome: privateCaseToolOutcomeForSettlement(projection, settlement.Output),
		EvidenceSettlement: settlement.Marker,
	})
	if err != nil {
		return err
	}
	if pending.PrivateProtocolObserved {
		// This non-secret, host-only provenance bit survives completed-turn
		// restart so a later provider request can close only the tool wire that
		// actually belonged to a private reasoning protocol attempt. Public
		// result and event projections intentionally omit it.
		records.ResultItem["privateProtocolObserved"] = true
	}
	if err := h.store.AppendItemToTurn(pending.ThreadID, pending.TurnID, records.ResultItem); err != nil {
		return err
	}
	_ = h.store.PatchTurnItemStatus(pending.ThreadID, pending.TurnID, pending.ToolCallItemID, records.Status)
	_, _, err = h.store.RecordEvent(records.Event)
	if toolcatalogapp.ShouldRecordGenericToolProgress(pending.Call.Name) {
		progressStatus := "success"
		progressMessage := "tool completed"
		if settlement.IsError {
			progressStatus = "error"
			progressMessage = toolcatalogapp.ToolFailureProgressMessage(records.ResultItem["output"])
		}
		h.recordRuntimeToolProgress(pending, progressStatus, progressMessage)
	}
	return err
}

func privateCaseToolOutcomeForSettlement(projection domaintoolresult.PublicToolResultProjectionV1, output any) *domainevidence.ToolOutcome {
	if projection.ProjectionKind != domaintoolresult.ProjectionCaseSourceStatus {
		return nil
	}
	record, _ := output.(map[string]any)
	outcome, err := domainevidence.ParseToolOutcome(record["toolOutcome"])
	if err != nil {
		return nil
	}
	return &outcome
}

func (h *runtimeServerHandler) persistRuntimeToolResultAndMessage(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) (provider.Message, error) {
	settled, err := h.settleRuntimeToolOutput(ctx, pending, output, isError)
	return settled.Message, err
}

func (h *runtimeServerHandler) settleRuntimeToolOutput(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) (apploop.SettledToolExecution, error) {
	return apploop.SettleToolOutput(
		ctx,
		pending,
		func(effectCtx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return apploop.AcquirePendingToolEffect(
				effectCtx, pending, h.runtimeSubagentState().AcquireContextEffectForAuthority,
			)
		},
		output,
		isError,
		h.settleRuntimeToolResultWithEffectAuthority,
	)
}

func (h *runtimeServerHandler) settleRuntimeToolResultWithEffectAuthority(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) (apploop.SettledToolExecution, error) {
	providerAttempt := apploop.CaptureToolResultProviderAttemptV1(apploop.ToolResultProviderAttemptInputV1{
		Pending: pending, Output: output, IsError: isError, Config: h.visionBridge, Source: h.mcp,
		ProjectExact:       subagentapp.ProjectForegroundParentToolOutputV1,
		ProjectExactPublic: subagentapp.ProjectForegroundParentPublicToolOutputV1,
	})
	defer providerAttempt.DiscardPrivate()
	settlement, err := evidenceapp.PrepareToolSettlement(ctx, h.caseFinalizer, h.turnSecurity, filestore.CaseBindingReader{}, h.store, h.mcp, pending, output, isError)
	if err != nil {
		return apploop.SettledToolExecution{}, err
	}
	prepared, err := providerAttempt.PrepareSettlementV1(settlement.Output, settlement.IsError)
	if err != nil {
		return apploop.SettledToolExecution{}, err
	}
	publicOutput := prepared.PublicOutput
	if err := h.persistRuntimeToolResult(pending, prepared.Projection, settlement); err != nil {
		return apploop.SettledToolExecution{Output: publicOutput, IsError: settlement.IsError}, err
	}
	receipt, receiptPresent, err := evidenceapp.CommitPersistedToolEvidence(ctx, h.caseFinalizer, h.store, pending.SecurityContext, settlement)
	if err != nil {
		return apploop.SettledToolExecution{Output: publicOutput, IsError: settlement.IsError}, err
	}
	if receiptPresent {
		appendErr := caseentityapp.AppendPersistedEvidenceReceiptV1(ctx, h.caseEntities, caseentityapp.AppendPersistedEvidenceReceiptInputV1{
			SecurityContext: pending.SecurityContext, Receipt: receipt,
		})
		if appendErr != nil && (!errors.Is(appendErr, caseentityapp.ErrPrivateStateNotFound) ||
			pending.Call.Name == "mcp__analytix_funds__analyze_account_flows") {
			return apploop.SettledToolExecution{Output: publicOutput, IsError: settlement.IsError}, appendErr
		}
	}
	return apploop.SettledToolExecution{
		Output: publicOutput, IsError: settlement.IsError, Message: prepared.Message,
		ParentContinuation: subagentapp.ResolveParentContinuationAuthorityV1(
			ctx, pending, settlement.Output, h.jobs, h.childCompletions, h.runtimeSkillByName,
		),
	}, nil
}

func (h *runtimeServerHandler) completeRuntimeContinuation(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) (runtimeAgentLoopResult, error) {
	settled, err := h.settleRuntimeToolOutput(ctx, pending, output, isError)
	if err != nil {
		return runtimeAgentLoopResult{}, err
	}
	if code := settled.AuthorityFailureCode(); code != "" {
		return runtimeAgentLoopResult{}, executiongrantapp.ValidationError{Code: code}
	}
	record, _ := settled.Output.(map[string]any)
	code, _ := record["code"].(string)
	if boundary, terminal := appmodel.AnthropicPrivateProtocolTerminalBoundary(appmodel.AnthropicPrivateProtocolTerminalInput{
		Capsule: pending.AnthropicCapsule, ProtocolObserved: pending.PrivateProtocolObserved,
		ReasoningProtocol: pending.ProviderConfig.ReasoningProtocol,
		Effort:            pending.Effort, OutcomeCode: code,
	}); terminal {
		return runtimeAgentLoopResult{AssistantText: boundary}, nil
	}
	return h.completeRuntimeContinuationWithSettled(ctx, pending, settled)
}

func (h *runtimeServerHandler) completeRuntimeContinuationWithSettled(ctx context.Context, pending runtimePendingToolCall, settled apploop.SettledToolExecution) (runtimeAgentLoopResult, error) {
	if decision := apploop.EvaluateProviderContinuation(pending.SecurityContext, pending.ExecutionGrant, pending.Call, settled); decision.Blocked {
		return runtimeAgentLoopResult{AssistantText: decision.Boundary}, nil
	}
	thread, _ := h.store.GetThread(pending.ThreadID)
	execution, err := h.resolveRuntimeTurnIntent(ctx, provider.TurnExecutionInput{
		RequestProviderID: pending.ProviderID, RequestModel: pending.Model, RequestEffort: pending.Effort,
		ThreadProviderID: stringField(thread, "providerId"), ThreadModel: stringField(thread, "model"),
	})
	if err != nil {
		return runtimeAgentLoopResult{}, err
	}
	pending.ProviderConfig = execution.Config
	pending.ProviderID = execution.ProviderID
	pending.Model = execution.Model
	pending.Effort = execution.Effort
	messages := appmodel.CloneProviderMessages(pending.Messages)
	messages = append(messages, settled.Message)
	messages, privateProtocolSession, privateProtocolCapsules, err :=
		runtimePrivateProtocolContinuationState(pending, messages)
	if err != nil {
		return runtimeAgentLoopResult{}, err
	}
	var anthropicCapsule *domainmodel.AnthropicThinkingCapsule
	if len(privateProtocolCapsules) > 0 {
		anthropicCapsule = privateProtocolCapsules[len(privateProtocolCapsules)-1]
	}
	prompt := strings.TrimSpace(pending.Prompt)
	if prompt == "" {
		if turn, ok := appmodel.TurnByID(thread, pending.TurnID); ok {
			prompt = appmodel.UserPromptFromTurn(turn)
		}
	}
	continuationMaxModelSteps := pending.MaxModelSteps
	if continuationMaxModelSteps == nil {
		continuationMaxModelSteps = pending.EffectiveMaxModelSteps
	}
	continuationRefs := append([]domainsecurity.SettledToolReference(nil), pending.PriorSettledToolRefs...)
	continuationRefs = append(continuationRefs, domainsecurity.SettledToolReference{GrantID: pending.ExecutionGrant.GrantID, ResultItemID: toolcatalogapp.ToolResultItemID(pending.TurnID, pending.Call.ID)})
	attachmentPlan, err := h.planRuntimeAttachments(ctx, pending.AttachmentIDs, pending.SecurityContext, pending.ProviderConfig)
	if err != nil {
		return runtimeAgentLoopResult{}, err
	}
	if !attachmentPlan.Empty() {
		if pending.AttachmentPlanDigest != "" && pending.AttachmentPlanDigest != attachmentPlan.PlanDigest {
			return runtimeAgentLoopResult{}, errors.New("pending attachment plan authority changed")
		}
		messages, err = appmodel.BindPrivateAttachmentPlan(messages, attachmentPlan.PlanDigest, prompt)
		if err != nil {
			return runtimeAgentLoopResult{}, err
		}
	}
	input := runtimeAgentLoopInput{
		ThreadID:       pending.ThreadID,
		TurnID:         pending.TurnID,
		Thread:         thread,
		ProviderConfig: pending.ProviderConfig,
		ProviderID:     pending.ProviderID,
		Model:          pending.Model,
		Effort:         pending.Effort,
		Workspace:      pending.Workspace,
		ApprovalPolicy: pending.ApprovalPolicy,
		SandboxMode:    pending.SandboxMode,
		Request: startRuntimeTurnRequest{
			Prompt:                prompt,
			AttachmentIDs:         append([]string(nil), pending.AttachmentIDs...),
			DisableUserInput:      pending.DisableUserInput,
			MaxModelSteps:         continuationMaxModelSteps,
			Mode:                  pending.Mode,
			GUIPlan:               cloneMap(pending.GUIPlan),
			WorkspaceCheckpointID: pending.WorkspaceCheckpointID,
		},
		Attachments:              attachmentPlan,
		ToolScope:                append([]string(nil), pending.ToolScope...),
		SubagentDepth:            pending.SubagentDepth,
		SecurityContext:          pending.SecurityContext,
		ProviderContinuationRefs: continuationRefs,
		PrivateProtocolSession:   privateProtocolSession,
		AnthropicCapsule:         anthropicCapsule,
		PrivateProtocolCapsules:  privateProtocolCapsules,
	}
	input.UsageSource, input.ChildRunID, input.ProviderCallSequence, input.DelegatedToolManifest, input.TerminalRecoveryKind = appmodel.PendingProviderRuntimeValues(pending)
	result, err := h.runRuntimeAgentLoopWithMessages(ctx, input, messages)
	result, err = apploop.RejectCancelledRuntimeResult(ctx, result, err)
	if ctx.Err() != nil {
		err = apploop.RuntimeAgentLoopFailure{Cause: err, Result: result}
	}
	return result, err
}

func runtimePrivateProtocolContinuationState(
	pending runtimePendingToolCall,
	messages []provider.Message,
) (
	[]provider.Message,
	*domainmodel.PrivateProtocolSession,
	[]*domainmodel.AnthropicThinkingCapsule,
	error,
) {
	privateProtocolKind := appmodel.PrivateProtocolReasoningKindForProvider(
		pending.ProviderConfig,
		pending.ProviderConfig.EndpointFormat,
		provider.CustomEndpointRequestShape(pending.ProviderConfig.BaseURL),
	)
	privateProtocolSession := pending.PrivateProtocolSession
	privateProtocolCapsules := append(
		[]*domainmodel.AnthropicThinkingCapsule(nil),
		pending.PrivateProtocolCapsules...,
	)
	if len(privateProtocolCapsules) == 0 && pending.AnthropicCapsule != nil {
		privateProtocolCapsules = append(privateProtocolCapsules, pending.AnthropicCapsule)
	}
	hasPrivateProtocolState := privateProtocolSession != nil || len(privateProtocolCapsules) > 0
	if privateProtocolKind == "" {
		if hasPrivateProtocolState || pending.PrivateProtocolObserved {
			return nil, nil, nil, appmodel.ErrAnthropicPrivateProtocolFlowBinding
		}
		return messages, nil, nil, nil
	}
	privateProtocolSafeHistoryRequired :=
		appmodel.PrivateProtocolRequired(pending.ProviderConfig.ReasoningProtocol, pending.Effort) ||
			pending.PrivateProtocolObserved
	if privateProtocolKind == "anthropic-thinking" &&
		!hasPrivateProtocolState &&
		!privateProtocolSafeHistoryRequired {
		return messages, nil, nil, nil
	}
	// A gate pause ends the physical provider attempt even when volatile bytes
	// still exist in this process. Resume and restart therefore continue only
	// from bounded, provenance-checked semantic history and never replay signed
	// thinking/reasoning or provider-native tool wire across the pause.
	projected, err := privacyprojectionapp.PrivateProtocolSafeHistoryV1(
		pending.SecurityContext,
		messages,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	return projected, nil, nil, nil
}

func runtimeToolCancelledOutput(call provider.ToolCall, cause error) map[string]any {
	return toolcatalogapp.CancelledOutput(call.Name, call.ID, cause)
}
