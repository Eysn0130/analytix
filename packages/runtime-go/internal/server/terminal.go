package server

import (
	"context"
	"errors"
	"path/filepath"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	workspacefs "analytix.local/runtime-go/internal/adapters/outbound/workspacefs"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type runtimeBashToolRequest = terminalapp.BashToolRequest

func runtimeBashToolRequestFromPending(pending runtimePendingToolCall, args map[string]any) (runtimeBashToolRequest, map[string]any, bool) {
	toolContext, err := runtimeBashToolContext(pending)
	if err != nil {
		return runtimeBashToolRequest{}, runtimeBashSecurityAuthorityFailure(), true
	}
	toolRequest, failure, failed := terminalapp.BashToolRequestFromArgs(
		args,
		toolContext.Workspace,
		toolContext.WorkspaceAbsolute,
		toolContext.SandboxMode,
		terminalapp.DefaultBashTimeoutSeconds,
		terminalapp.MaxBashTimeoutSeconds,
	)
	if failed {
		return runtimeBashToolRequest{}, failure, true
	}
	workspace, failure, failed := workspacefs.ValidateBashWorkspace(toolRequest.Workspace, true)
	if failed {
		return runtimeBashToolRequest{}, failure, true
	}
	toolRequest.Workspace = workspace
	return toolRequest, nil, false
}

func (h *runtimeServerHandler) executeBashRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	toolContext, err := runtimeBashToolContext(pending)
	if err != nil {
		return runtimeBashSecurityAuthorityFailure(), true
	}
	toolRequest, failure, failed := terminalapp.BashToolRequestFromArgs(
		args,
		toolContext.Workspace,
		toolContext.WorkspaceAbsolute,
		toolContext.SandboxMode,
		terminalapp.DefaultBashTimeoutSeconds,
		terminalapp.MaxBashTimeoutSeconds,
	)
	if failed {
		return failure, true
	}
	workspace, failure, failed := workspacefs.ValidateBashWorkspace(toolRequest.Workspace, true)
	if failed {
		return failure, true
	}
	toolContext.Workspace = workspace
	toolContext.WorkspaceAbsolute = true
	// Preserve the requested filesystem policy on every host. Unsupported native
	// adapters must reject it before exec, never receive an empty policy merely
	// because this platform has no containment implementation yet.
	toolContext.ProtectedReadDirs = filestore.EffectiveProcessProtectedRoots(h.protectedReadDirs, toolContext.SandboxMode)
	if boolField(args, "run_in_background") || boolField(args, "runInBackground") {
		toolContext.ParentGoalID, toolContext.ParentGoalObjective = h.parentGoalForSubagent(pending.ThreadID)
	}
	return h.runtimeTerminalService().ExecuteBashTool(ctx, terminalapp.BashToolRunRequest{
		Context: toolContext,
		Args:    args,
	}, h.runtimeBashToolCallbacks(pending))
}

func runtimeBashToolContext(pending runtimePendingToolCall) (terminalapp.BashToolContext, error) {
	workspace := firstNonEmptyString(pending.Workspace)
	securityBinding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		return terminalapp.BashToolContext{}, errors.New("runtime bash security authority is invalid")
	}
	return terminalapp.BashToolContext{
		ThreadID:          pending.ThreadID,
		TurnID:            pending.TurnID,
		ToolCallItemID:    pending.ToolCallItemID,
		ToolCallID:        pending.Call.ID,
		Workspace:         workspace,
		WorkspaceAbsolute: filepath.IsAbs(workspace),
		SandboxMode:       pending.SandboxMode,
		SubagentDepth:     pending.SubagentDepth,
		SecurityBinding:   securityBinding,
	}, nil
}

func runtimeBashSecurityAuthorityFailure() map[string]any {
	return map[string]any{
		"code": "tool_security_binding_invalid", "error": "runtime tool security authority is unavailable",
	}
}

func (h *runtimeServerHandler) runtimeTerminalService() *terminalapp.Service {
	return terminalapp.NewService(terminalapp.Dependencies{
		Jobs:        h.jobs,
		ShellRunner: h.shellRunner,
	})
}

func (h *runtimeServerHandler) runtimeBashToolCallbacks(pending runtimePendingToolCall) terminalapp.BashToolCallbacks {
	return terminalapp.BashToolCallbacks{
		AuthorizeBackgroundStart: h.authorizeRuntimeJobStart,
		RegisterBoundBackgroundJob: func(jobID string, binding *domainjob.SecurityBinding, cancel context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
			return h.runtimeSubagentState().RegisterBoundBackgroundJobWithStartBarrier(jobID, binding, cancel)
		},
		UnregisterBackgroundJob: func(jobID string) {
			h.runtimeSubagentState().UnregisterBackgroundJob(jobID)
		},
		OnBackgroundStarted: func(record terminalapp.JobRecord) {
			h.recordRuntimeTaskJobProgress(pending, record, "running", "background bash started")
		},
		OnBackgroundProgress: func(record terminalapp.JobRecord, status string, diagnostic string) {
			if domainjob.TerminalStatusV1(status) || subagentapp.TaskJobTerminal(record) {
				// A standalone terminal progress event would reserve one member of
				// the exact completion bundle and block live/restart reconciliation.
				h.recordRuntimeJobLifecycleEvent(record, status, diagnostic)
				return
			}
			h.recordRuntimeTaskJobProgress(pending, record, status, diagnostic)
		},
	}
}
