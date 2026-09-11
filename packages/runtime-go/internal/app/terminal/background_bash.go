package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	"analytix.local/runtime-go/internal/ports"
)

const DefaultBackgroundOutputLimit = 1024 * 1024

type BackgroundBashRequest struct {
	Record              domainjob.Record
	Command             string
	Workspace           string
	ProtectedReadDirs   []string
	TimeoutSeconds      int
	OutputLimit         int
	KillGrace           time.Duration
	CancelStatus        string
	IncludeTimedOut     bool
	RespectKilledStatus bool
	StartBarrier        ports.ProcessStartBarrier
}

type BackgroundBashCallbacks struct {
	OnOutput   func(record domainjob.Record, output string, truncated bool)
	OnProgress func(record domainjob.Record, status string, diagnostic string)
}

type StartBackgroundBashJobRequest struct {
	ParentGoalID        string
	ParentGoalObjective string
	ParentThreadID      string
	ParentTurnID        string
	ParentToolItemID    string
	ParentToolCallID    string
	Command             string
	Workspace           string
	LabelPrefix         string
	ToolPolicy          string
	SecurityBinding     *domainjob.SecurityBinding
}

func (s *Service) StartBackgroundBashJob(request StartBackgroundBashJobRequest) (domainjob.Record, error) {
	if domainjob.ValidateSecurityBinding(request.SecurityBinding) != nil ||
		request.SecurityBinding.ParentThreadID != strings.TrimSpace(request.ParentThreadID) ||
		request.SecurityBinding.ParentTurnID != strings.TrimSpace(request.ParentTurnID) ||
		request.SecurityBinding.ParentToolCallID != strings.TrimSpace(request.ParentToolCallID) {
		return domainjob.Record{}, fmt.Errorf("background job security authority is invalid")
	}
	if s.jobs == nil {
		return domainjob.Record{}, fmt.Errorf("job repository unavailable")
	}
	toolPolicy := strings.TrimSpace(request.ToolPolicy)
	if toolPolicy == "" {
		toolPolicy = "workspace-write"
	}
	return s.jobs.StartChildRun(domainjob.StartRequest{
		ParentGoalID:        request.ParentGoalID,
		ParentGoalObjective: request.ParentGoalObjective,
		ParentThreadID:      request.ParentThreadID,
		ParentTurnID:        request.ParentTurnID,
		ParentToolItemID:    request.ParentToolItemID,
		ParentToolCallID:    request.ParentToolCallID,
		SecurityBinding:     domainjob.CloneSecurityBinding(request.SecurityBinding),
		Kind:                "background-shell",
		Name:                "bash",
		Label:               BackgroundBashLabel(request.Command),
		Status:              "running",
		Workspace:           request.Workspace,
		ToolPolicy:          toolPolicy,
		Background:          true,
	})
}

func BackgroundBashLabel(command string) string {
	return "background shell"
}

func CompleteBackgroundBash(ctx context.Context, runner ports.ShellRunner, jobs ports.JobRepository, request BackgroundBashRequest, callbacks BackgroundBashCallbacks) (domainjob.Record, error) {
	record := request.Record
	startedAt := time.Now()
	runCtx := ctx
	cancel := func() {}
	if request.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(request.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	outputLimit := request.OutputLimit
	if outputLimit <= 0 {
		outputLimit = DefaultBackgroundOutputLimit
	}
	updateOutput := func(output string, truncated bool) {
		if jobs == nil {
			return
		}
		projected := projectBackgroundBashText(record, request.Command, output)
		updated, err := jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Output: projected})
		if err == nil {
			record = updated
			if callbacks.OnOutput != nil {
				callbacks.OnOutput(record, projected, truncated)
			}
		}
	}
	output := NewBackgroundOutputBuffer(outputLimit, updateOutput)
	if runner == nil {
		failure := projectBackgroundBashText(record, request.Command, "shell runner unavailable")
		updated, err := updateBackgroundBashFailure(jobs, record, failure)
		if callbacks.OnProgress != nil {
			callbacks.OnProgress(updated, "failed", firstNonEmptyBackgroundString(failure, "background shell failed"))
		}
		return updated, err
	}
	result := runner.RunShell(runCtx, ports.ShellRequest{
		Command:           request.Command,
		Dir:               request.Workspace,
		Output:            output,
		KillGrace:         request.KillGrace,
		StartBarrier:      request.StartBarrier,
		ProtectedReadDirs: append([]string(nil), request.ProtectedReadDirs...),
	})
	if result.StartFailed {
		failure := projectBackgroundBashText(record, request.Command, result.Error)
		updated, err := updateBackgroundBashFailure(jobs, record, failure)
		if callbacks.OnProgress != nil {
			callbacks.OnProgress(updated, "failed", firstNonEmptyBackgroundString(failure, "background shell failed"))
		}
		return updated, err
	}
	status := "completed"
	if runCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
	} else if result.Canceled || runCtx.Err() != nil {
		status = request.CancelStatus
		if status == "" {
			status = "canceled"
		}
	} else if result.Error != "" {
		status = "failed"
	}
	rawOutputText, truncated := output.Snapshot()
	outputText := projectBackgroundBashText(record, request.Command, rawOutputText)
	if request.RespectKilledStatus && jobs != nil {
		current, loadErr := jobs.LoadChildRun(record.ID)
		if loadErr == nil && current.Status == "killed" {
			if callbacks.OnProgress != nil {
				diagnostic := projectBackgroundBashText(current, request.Command, current.Error)
				callbacks.OnProgress(current, "killed", firstNonEmptyBackgroundString(diagnostic, "background shell killed"))
			}
			return current, nil
		}
	}
	errorText := ""
	if result.Error != "" && status != "timeout" && status != "canceled" && status != "killed" {
		errorText = projectBackgroundBashText(record, request.Command, result.Error)
	}
	durationMs := time.Since(startedAt).Milliseconds()
	usage := map[string]any{
		"exitCode":       float64(result.ExitCode),
		"durationMs":     float64(durationMs),
		"outputBytes":    float64(len([]byte(rawOutputText))),
		"maxOutputBytes": float64(outputLimit),
		"truncated":      truncated,
	}
	if request.IncludeTimedOut {
		usage["timedOut"] = status == "timeout"
	}
	updated, updateErr := jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		Status: status,
		Output: outputText,
		Error:  errorText,
		Usage:  usage,
	})
	if updateErr != nil {
		return record, updateErr
	}
	diagnostic := fmt.Sprintf("exitCode=%d durationMs=%d timedOut=%t outputBytes=%d truncated=%t", result.ExitCode, durationMs, status == "timeout", len([]byte(rawOutputText)), truncated)
	if errorText != "" {
		diagnostic = errorText + "; " + diagnostic
	}
	if callbacks.OnProgress != nil {
		callbacks.OnProgress(updated, status, diagnostic)
	}
	return updated, nil
}

func projectBackgroundBashText(record domainjob.Record, command string, value string) string {
	if record.SecurityBinding != nil {
		return ""
	}
	return domainjob.ProjectPersistableUntrustedOutputV1(
		domainsecret.ProjectTextV1(value, domainsecret.ExtractExplicitSecretsV1(command)...),
	)
}

func (s *Service) CompleteBackgroundBash(ctx context.Context, request BackgroundBashRequest, callbacks BackgroundBashCallbacks) (domainjob.Record, error) {
	return CompleteBackgroundBash(ctx, s.shellRunner, s.jobs, request, callbacks)
}

func firstNonEmptyBackgroundString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func updateBackgroundBashFailure(jobs ports.JobRepository, record domainjob.Record, errorText string) (domainjob.Record, error) {
	if jobs == nil {
		return record, nil
	}
	updated, err := jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: "failed", Error: errorText})
	if err != nil {
		return record, err
	}
	return updated, nil
}
