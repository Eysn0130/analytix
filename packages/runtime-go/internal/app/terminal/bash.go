package terminal

import (
	"context"
	"time"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"analytix.local/runtime-go/internal/ports"
)

const DefaultForegroundOutputLimit = 32 * 1024

type BashRequest struct {
	Command           string
	Workspace         string
	TimeoutSeconds    int
	OutputLimit       int
	KillGrace         time.Duration
	ProtectedReadDirs []string
}

func ExecuteForegroundBash(ctx context.Context, runner ports.ShellRunner, request BashRequest) (map[string]any, bool) {
	timeout := request.TimeoutSeconds
	if timeout <= 0 {
		timeout = DefaultBashTimeoutSeconds
	}
	outputLimit := request.OutputLimit
	if outputLimit <= 0 {
		outputLimit = DefaultForegroundOutputLimit
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	output := NewLimitedOutputBuffer(outputLimit)
	startedAt := time.Now()
	if runner == nil {
		return map[string]any{
			"command":        request.Command,
			"cwd":            request.Workspace,
			"status":         "failed",
			"error":          "shell runner unavailable",
			"exitCode":       float64(-1),
			"durationMs":     float64(time.Since(startedAt).Milliseconds()),
			"timedOut":       false,
			"output":         "",
			"outputBytes":    float64(0),
			"maxOutputBytes": float64(outputLimit),
			"truncated":      false,
		}, true
	}
	result := runner.RunShell(runCtx, ports.ShellRequest{
		Command:           request.Command,
		Dir:               request.Workspace,
		Output:            output,
		KillGrace:         request.KillGrace,
		ProtectedReadDirs: append([]string(nil), request.ProtectedReadDirs...),
	})
	if result.StartFailed {
		return map[string]any{
			"command":        request.Command,
			"cwd":            request.Workspace,
			"status":         "failed",
			"error":          result.Error,
			"exitCode":       float64(-1),
			"durationMs":     float64(time.Since(startedAt).Milliseconds()),
			"timedOut":       false,
			"output":         "",
			"outputBytes":    float64(0),
			"maxOutputBytes": float64(outputLimit),
			"truncated":      false,
		}, true
	}
	status := "completed"
	isError := false
	if runCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
		isError = true
	} else if result.Canceled || runCtx.Err() != nil {
		status = "canceled"
		isError = true
	} else if result.Error != "" {
		status = "failed"
		isError = true
	}
	rawBody, captureCut := output.Snapshot()
	captureBody, byteCut := domaintoolresult.BoundProtectedTextV1(rawBody, domaintoolresult.ProtectedSnapshotBodyLimitV1)
	label, _ := domaintoolresult.BoundProtectedTextV1(request.Command, 1024)
	var exitCode *int
	captureStatus := status
	if result.ExitCode >= 0 {
		code := result.ExitCode
		exitCode = &code
	} else if status == "completed" {
		captureStatus = "unknown"
	}
	domaintoolresult.CaptureProtectedToolResultV1(ctx, domaintoolresult.ProtectedCaptureV1{Kind: "shell", Status: captureStatus, Body: captureBody, Label: label, ExitCode: exitCode, DurationMs: time.Since(startedAt).Milliseconds(), Truncated: captureCut || byteCut})
	outputText := output.String()
	return map[string]any{
		"command":        request.Command,
		"cwd":            request.Workspace,
		"status":         status,
		"exitCode":       float64(result.ExitCode),
		"durationMs":     float64(time.Since(startedAt).Milliseconds()),
		"timedOut":       status == "timeout",
		"output":         outputText,
		"outputBytes":    float64(len([]byte(outputText))),
		"maxOutputBytes": float64(outputLimit),
		"truncated":      output.Truncated(),
	}, isError
}

func (s *Service) ExecuteForegroundBash(ctx context.Context, request BashRequest) (map[string]any, bool) {
	return ExecuteForegroundBash(ctx, s.shellRunner, request)
}
