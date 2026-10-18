package terminal

import (
	"context"
	"io"
	"sync"
	"time"

	processdisplay "analytix.local/runtime-go/internal/domain/processdisplay"
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
	// The display copy must classify a complete stream independently of the
	// legacy tool output limit. One extra byte detects an incomplete capture.
	displayOutput := NewLimitedOutputBuffer(processdisplay.RawCaptureByteLimitV1 + 1)
	startedAt := time.Now()
	if runner == nil {
		captureForegroundProcessDisplayV1(ctx, request, "", false, "failed", "shell_runner_unavailable", nil, time.Since(startedAt).Milliseconds())
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
		Output:            &processDisplayDualWriterV1{next: io.MultiWriter(output, displayOutput)},
		KillGrace:         request.KillGrace,
		ProtectedReadDirs: append([]string(nil), request.ProtectedReadDirs...),
	})
	if result.StartFailed {
		// A failed launch never supplies output or an exit outcome, even if an
		// invalid runner wrote bytes before reporting StartFailed.
		captureForegroundProcessDisplayV1(ctx, request, "", false, "failed", "shell_start_failed", nil, time.Since(startedAt).Milliseconds())
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
	displayBody, displayCut := displayOutput.Snapshot()
	captureForegroundProcessDisplayV1(ctx, request, displayBody, displayCut, captureStatus, result.Error, exitCode, time.Since(startedAt).Milliseconds())
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

// This volatile capture does not alter the tool result or its legacy protected
// snapshot. The settlement owner projects it only after actual settlement.
func captureForegroundProcessDisplayV1(ctx context.Context, request BashRequest, body string, overflow bool, status, failure string, exitCode *int, durationMs int64) {
	capture := processdisplay.RawCaptureV1{
		Kind: "shell", Complete: true, Overflow: overflow,
		Body: body, Title: request.Command, Path: request.Workspace, Error: failure,
		Status: status, SettlementStatus: "not_required", ExitCode: exitCode, DurationMs: durationMs,
	}
	remaining := processdisplay.RawCaptureByteLimitV1
	for _, value := range []string{capture.Body, capture.Title, capture.Path, capture.Error} {
		if len(value) > remaining {
			capture.Overflow = true
			break
		}
		remaining -= len(value)
	}
	if capture.Overflow {
		capture.Complete = false
		capture.Body, capture.Title, capture.Path, capture.Error = "", "", "", ""
	}
	processdisplay.CaptureV1(ctx, capture)
}

type processDisplayDualWriterV1 struct {
	mu   sync.Mutex
	next io.Writer
}

func (w *processDisplayDualWriterV1) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.next.Write(p)
}
