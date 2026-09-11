package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type JobRecord = domainjob.Record

type BashToolContext struct {
	ThreadID            string
	TurnID              string
	ToolCallItemID      string
	ToolCallID          string
	Workspace           string
	WorkspaceAbsolute   bool
	SandboxMode         string
	ProtectedReadDirs   []string
	SubagentDepth       int
	ParentGoalID        string
	ParentGoalObjective string
	SecurityBinding     *domainjob.SecurityBinding
}

type BashToolRunRequest struct {
	Context BashToolContext
	Args    map[string]any
}

type BashToolCallbacks struct {
	RegisterBoundBackgroundJob func(jobID string, binding *domainjob.SecurityBinding, cancel context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool)
	UnregisterBackgroundJob    func(jobID string)
	OnBackgroundStarted        func(record JobRecord)
	OnBackgroundProgress       func(record JobRecord, status string, diagnostic string)
	AuthorizeBackgroundStart   func(record JobRecord) (JobRecord, error)
}

func (s *Service) ExecuteBashTool(ctx context.Context, request BashToolRunRequest, callbacks BashToolCallbacks) (any, bool) {
	pending := request.Context
	args := request.Args
	toolRequest, failure, failed := BashToolRequestFromArgs(
		args,
		pending.Workspace,
		pending.WorkspaceAbsolute,
		pending.SandboxMode,
		DefaultBashTimeoutSeconds,
		MaxBashTimeoutSeconds,
	)
	if failed {
		return failure, true
	}
	if boolToolFlag(args, "run_in_background", "runInBackground") {
		if pending.SubagentDepth > 0 {
			return map[string]any{"code": "background_shell_unavailable", "error": "background bash is not available inside subagents"}, true
		}
		return s.executeBackgroundBashTool(pending, toolRequest, callbacks)
	}
	return s.ExecuteForegroundBash(ctx, BashRequest{
		Command:           toolRequest.Command,
		Workspace:         toolRequest.Workspace,
		TimeoutSeconds:    toolRequest.TimeoutSeconds,
		OutputLimit:       DefaultForegroundOutputLimit,
		KillGrace:         2 * time.Second,
		ProtectedReadDirs: append([]string(nil), pending.ProtectedReadDirs...),
	})
}

func (s *Service) executeBackgroundBashTool(pending BashToolContext, request BashToolRequest, callbacks BashToolCallbacks) (any, bool) {
	if domainjob.ValidateSecurityBinding(pending.SecurityBinding) != nil ||
		pending.SecurityBinding.ParentThreadID != strings.TrimSpace(pending.ThreadID) ||
		pending.SecurityBinding.ParentTurnID != strings.TrimSpace(pending.TurnID) ||
		pending.SecurityBinding.ParentToolCallID != strings.TrimSpace(pending.ToolCallID) ||
		callbacks.RegisterBoundBackgroundJob == nil {
		return map[string]any{
			"code": "background_shell_stale_context", "error": "background job security authority is unavailable",
		}, true
	}
	record, err := s.StartBackgroundBashJob(StartBackgroundBashJobRequest{
		ParentGoalID:        pending.ParentGoalID,
		ParentGoalObjective: pending.ParentGoalObjective,
		ParentThreadID:      pending.ThreadID,
		ParentTurnID:        pending.TurnID,
		ParentToolItemID:    pending.ToolCallItemID,
		ParentToolCallID:    pending.ToolCallID,
		Command:             request.Command,
		Workspace:           request.Workspace,
		SecurityBinding:     pending.SecurityBinding,
	})
	if err != nil {
		return map[string]any{"code": "background_shell_failed", "error": "background shell could not be started"}, true
	}
	backgroundCtx, cancel := context.WithCancel(context.Background())
	var startBarrier *subagentapp.BackgroundJobStartBarrier
	registered := false
	startBarrier, registered = callbacks.RegisterBoundBackgroundJob(record.ID, record.SecurityBinding, cancel)
	if !registered || startBarrier == nil {
		cancel()
		if s.jobs != nil {
			if updated, updateErr := s.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
				Status: string(domainjob.StatusInterrupted), FailureCode: domainjob.FailureChildInterrupted,
			}); updateErr == nil {
				record = updated
			}
		}
		return map[string]any{"code": "background_shell_stale_context", "error": "background job security authority changed before execution"}, true
	}
	if callbacks.OnBackgroundStarted != nil {
		callbacks.OnBackgroundStarted(record)
	}
	go func(record JobRecord) {
		if callbacks.UnregisterBackgroundJob != nil {
			defer callbacks.UnregisterBackgroundJob(record.ID)
		}
		if callbacks.AuthorizeBackgroundStart != nil {
			authorizedRecord, authorizeErr := callbacks.AuthorizeBackgroundStart(record)
			if strings.TrimSpace(authorizedRecord.ID) != "" {
				record = authorizedRecord
			}
			if authorizeErr != nil {
				if s.jobs != nil {
					if updated, updateErr := s.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
						Status: string(domainjob.StatusInterrupted), Error: authorizeErr.Error(),
					}); updateErr == nil {
						record = updated
					}
				}
				if callbacks.OnBackgroundProgress != nil {
					callbacks.OnBackgroundProgress(record, string(domainjob.StatusInterrupted), authorizeErr.Error())
				}
				return
			}
		}
		_, _ = s.CompleteBackgroundBash(backgroundCtx, BackgroundBashRequest{
			Record:              record,
			Command:             request.Command,
			Workspace:           request.Workspace,
			ProtectedReadDirs:   append([]string(nil), pending.ProtectedReadDirs...),
			TimeoutSeconds:      request.TimeoutSeconds,
			OutputLimit:         DefaultBackgroundOutputLimit,
			KillGrace:           2 * time.Second,
			CancelStatus:        string(domainjob.StatusKilled),
			IncludeTimedOut:     true,
			RespectKilledStatus: true,
			StartBarrier:        startBarrier,
		}, BackgroundBashCallbacks{
			OnOutput: func(record JobRecord, output string, truncated bool) {
				if callbacks.OnBackgroundProgress != nil {
					callbacks.OnBackgroundProgress(record, "running", fmt.Sprintf("background bash output: %d bytes", len([]byte(output))))
				}
			},
			OnProgress: callbacks.OnBackgroundProgress,
		})
	}(record)
	return BackgroundBashToolOutput(record, request.Command, request.Workspace, request.TimeoutSeconds, time.Now().UTC()), false
}

func BackgroundBashToolOutput(record JobRecord, command string, workspace string, timeoutSeconds int, now time.Time) map[string]any {
	if subagentapp.SecurityBoundChildOutput(record) {
		return subagentapp.SecurityBoundChildOutputProjection(record)
	}
	out := subagentapp.TaskJobRecordView(record, now, subagentapp.DefaultTaskJobStalledAfter)
	out["kind"] = "background_shell"
	out["jobId"] = record.ID
	out["childRunId"] = record.ID
	return out
}

func boolToolFlag(args map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := args[key].(type) {
		case bool:
			if value {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(value), "true") || strings.TrimSpace(value) == "1" {
				return true
			}
		}
	}
	return false
}
