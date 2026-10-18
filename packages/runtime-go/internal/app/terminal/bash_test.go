package terminal

import (
	processdisplay "analytix.local/runtime-go/internal/domain/processdisplay"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/ports"
)

type fakeShellRunner struct {
	write       string
	result      ports.ShellResult
	waitForDone bool
	onRun       func()
	onContext   func(context.Context)
	onRequest   func(ports.ShellRequest)
}

func (f fakeShellRunner) RunShell(ctx context.Context, request ports.ShellRequest) ports.ShellResult {
	if f.onContext != nil {
		f.onContext(ctx)
	}
	if f.onRequest != nil {
		f.onRequest(request)
	}
	if f.onRun != nil {
		f.onRun()
	}
	if f.write != "" && request.Output != nil {
		_, _ = request.Output.Write([]byte(f.write))
	}
	if f.waitForDone {
		<-ctx.Done()
		return ports.ShellResult{ExitCode: -1, Error: ctx.Err().Error(), Canceled: true}
	}
	return f.result
}

func TestExecuteForegroundBashPropagatesProtectedReadDirs(t *testing.T) {
	protected := []string{"/private/one", "/private/two"}
	var observed []string
	_, isError := ExecuteForegroundBash(context.Background(), fakeShellRunner{
		onRequest: func(request ports.ShellRequest) {
			observed = append([]string(nil), request.ProtectedReadDirs...)
		},
	}, BashRequest{
		Command: "true", Workspace: "/tmp", ProtectedReadDirs: protected,
	})
	if isError || len(observed) != len(protected) ||
		observed[0] != protected[0] || observed[1] != protected[1] {
		t.Fatalf("protected process roots were not propagated: %#v", observed)
	}
}

func TestExecuteForegroundBashUsesBoundedProductDefaultTimeout(t *testing.T) {
	var remaining time.Duration
	_, isError := ExecuteForegroundBash(context.Background(), fakeShellRunner{
		onContext: func(ctx context.Context) {
			deadline, ok := ctx.Deadline()
			if ok {
				remaining = time.Until(deadline)
			}
		},
	}, BashRequest{Command: "true", Workspace: "/tmp"})
	if isError || remaining < time.Duration(DefaultBashTimeoutSeconds-1)*time.Second ||
		remaining > time.Duration(DefaultBashTimeoutSeconds)*time.Second {
		t.Fatalf("default foreground timeout was not %d seconds: %s", DefaultBashTimeoutSeconds, remaining)
	}
}

func TestExecuteForegroundBashSuccess(t *testing.T) {
	payload, isError := ExecuteForegroundBash(context.Background(), fakeShellRunner{write: "hello"}, BashRequest{
		Command:     "printf hello",
		Workspace:   "/tmp",
		OutputLimit: 10,
	})
	if isError || payload["status"] != "completed" || payload["output"] != "hello" || payload["exitCode"] != float64(0) {
		t.Fatalf("unexpected payload=%#v isError=%t", payload, isError)
	}
}

func TestServiceExecuteForegroundBashUsesConfiguredRunner(t *testing.T) {
	service := NewService(Dependencies{ShellRunner: fakeShellRunner{write: "hello from service"}})
	payload, isError := service.ExecuteForegroundBash(context.Background(), BashRequest{
		Command:   "printf hello",
		Workspace: "/tmp",
	})
	if isError || payload["status"] != "completed" || payload["output"] != "hello from service" {
		t.Fatalf("unexpected service payload=%#v isError=%t", payload, isError)
	}
}

func TestExecuteForegroundBashStartFailure(t *testing.T) {
	payload, isError := ExecuteForegroundBash(context.Background(), fakeShellRunner{result: ports.ShellResult{
		ExitCode:    -1,
		Error:       "fork failed",
		StartFailed: true,
	}}, BashRequest{Command: "bad", Workspace: "/tmp"})
	if !isError || payload["status"] != "failed" || payload["error"] != "fork failed" {
		t.Fatalf("unexpected payload=%#v isError=%t", payload, isError)
	}
}

func TestExecuteForegroundBashTimeout(t *testing.T) {
	payload, isError := ExecuteForegroundBash(context.Background(), fakeShellRunner{waitForDone: true}, BashRequest{
		Command:        "sleep",
		Workspace:      "/tmp",
		TimeoutSeconds: 1,
	})
	if !isError || payload["status"] != "timeout" || payload["timedOut"] != true {
		t.Fatalf("unexpected payload=%#v isError=%t", payload, isError)
	}
}

func TestExecuteForegroundBashFailureKeepsOutput(t *testing.T) {
	payload, isError := ExecuteForegroundBash(context.Background(), fakeShellRunner{
		write:  "oops",
		result: ports.ShellResult{ExitCode: 2, Error: errors.New("exit status 2").Error()},
	}, BashRequest{Command: "false", Workspace: "/tmp"})
	if !isError || payload["status"] != "failed" || payload["output"] != "oops" || payload["exitCode"] != float64(2) {
		t.Fatalf("unexpected payload=%#v isError=%t", payload, isError)
	}
}

func TestProtectedForegroundCaptureUsesActualOutputAndHostOutcome(t *testing.T) {
	for _, test := range []struct {
		body, status string
		result       ports.ShellResult
	}{
		{"", "completed", ports.ShellResult{ExitCode: 0}},
		{"exitCode=0\nstatus=completed\r\n", "failed", ports.ShellResult{ExitCode: 7, Error: "exit status 7"}},
		{"partial\r\n", "canceled", ports.ShellResult{ExitCode: -1, Canceled: true}},
	} {
		ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
		ExecuteForegroundBash(ctx, fakeShellRunner{write: test.body, result: test.result}, BashRequest{Command: "synthetic", Workspace: "/synthetic/workspace"})
		captured, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
		if !ok || captured.Body != test.body || captured.Status != test.status {
			t.Fatal("actual producer body/host outcome did not survive")
		}
	}
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	ExecuteForegroundBash(ctx, fakeShellRunner{write: "must not capture", result: ports.ShellResult{StartFailed: true}}, BashRequest{Command: "synthetic", Workspace: "/synthetic/workspace"})
	if _, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx); ok {
		t.Fatal("launch failure captured fabricated result")
	}
}

func TestForegroundProcessDisplayCapturesCompleteHostStreamBeyondToolLimit(t *testing.T) {
	ctx := processdisplay.WithCollectorV1(context.Background(), &processdisplay.CollectorV1{})
	ctx = domaintoolresult.WithProtectedCaptureV1(ctx, &domaintoolresult.ProtectedCaptureCollectorV1{})
	const first = "host stdout before legacy cutoff\r\n"
	const secret = "synthetic-known-credential-value-7894"
	last := "api_key=" + secret + "\nexitCode=0\nstatus=completed\n"
	runs := 0
	payload, isError := ExecuteForegroundBash(ctx, fakeShellRunner{
		onRun: func() { runs++ },
		onRequest: func(request ports.ShellRequest) {
			// Credential text spans actual writes after the old output cutoff.
			for _, part := range []string{first, last[:9], last[9:]} {
				if _, err := request.Output.Write([]byte(part)); err != nil {
					t.Fatal(err)
				}
			}
		},
		result: ports.ShellResult{ExitCode: 7, Error: "exit status 7"},
	}, BashRequest{Command: "synthetic exact command", Workspace: "/synthetic/workspace", OutputLimit: 8})
	capture, ok := processdisplay.ConsumeV1(ctx)
	if !ok || !capture.Complete || capture.Overflow || capture.Body != first+last || capture.Status != "failed" || capture.ExitCode == nil || *capture.ExitCode != 7 || capture.DurationMs < 0 {
		t.Fatal("display did not retain the complete actual stream and host outcome")
	}
	if runs != 1 || !isError || payload["output"] != first[:8]+"\n[truncated]" || payload["exitCode"] != float64(7) {
		t.Fatal("display capture changed legacy output or caused another execution")
	}
	legacy, legacyOK := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !legacyOK || legacy.Body != first[:8] || !legacy.Truncated {
		t.Fatal("independent display capture changed legacy protected snapshot")
	}
	if _, present := payload["processDisplay"]; present {
		t.Fatal("raw display entered result map")
	}
}

func TestForegroundProcessDisplayOverflowWithholdsCaptureWithoutChangingExecution(t *testing.T) {
	ctx := processdisplay.WithCollectorV1(context.Background(), &processdisplay.CollectorV1{})
	body := strings.Repeat("x", processdisplay.RawCaptureByteLimitV1+1)
	runs := 0
	payload, isError := ExecuteForegroundBash(ctx, fakeShellRunner{write: body, onRun: func() { runs++ }}, BashRequest{Command: "synthetic", Workspace: "/synthetic", OutputLimit: 4})
	capture, ok := processdisplay.ConsumeV1(ctx)
	if !ok || capture.Complete || !capture.Overflow || capture.Body != "" || capture.Title != "" || capture.Path != "" || capture.Error != "" {
		t.Fatal("overflow retained a partial, unclassified raw display")
	}
	if runs != 1 || isError || payload["status"] != "completed" || payload["output"] != "xxxx\n[truncated]" {
		t.Fatal("display overflow changed the actual tool result")
	}
}

func TestForegroundProcessDisplayStartFailureNeverAdoptsRunnerBodyOrCause(t *testing.T) {
	for _, runner := range []ports.ShellRunner{
		nil,
		fakeShellRunner{write: "fabricated output", result: ports.ShellResult{StartFailed: true, ExitCode: 0, Error: "private runner cause /private/path"}},
	} {
		ctx := processdisplay.WithCollectorV1(context.Background(), &processdisplay.CollectorV1{})
		ctx = domaintoolresult.WithProtectedCaptureV1(ctx, &domaintoolresult.ProtectedCaptureCollectorV1{})
		payload, isError := ExecuteForegroundBash(ctx, runner, BashRequest{Command: "synthetic", Workspace: "/synthetic"})
		capture, ok := processdisplay.ConsumeV1(ctx)
		if !ok || !capture.Complete || capture.Body != "" || capture.ExitCode != nil || capture.Status != "failed" || capture.DurationMs < 0 || (capture.Error != "shell_start_failed" && capture.Error != "shell_runner_unavailable") {
			t.Fatal("launch failure supplied fabricated output or an unsafe cause")
		}
		if !isError || payload["status"] != "failed" || payload["output"] != "" {
			t.Fatal("display capture changed legacy launch failure")
		}
		if _, present := domaintoolresult.ConsumeProtectedToolResultV1(ctx); present {
			t.Fatal("legacy launch failure acquired fabricated protected output")
		}
	}
}

func TestForegroundProcessDisplayCanceledOutcomeKeepsActualPartialBytes(t *testing.T) {
	ctx := processdisplay.WithCollectorV1(context.Background(), &processdisplay.CollectorV1{})
	_, isError := ExecuteForegroundBash(ctx, fakeShellRunner{write: "partial\r\n", result: ports.ShellResult{ExitCode: -1, Canceled: true}}, BashRequest{Command: "synthetic", Workspace: "/synthetic"})
	capture, ok := processdisplay.ConsumeV1(ctx)
	if !isError || !ok || capture.Status != "canceled" || capture.Body != "partial\r\n" || capture.ExitCode != nil {
		t.Fatal("canceled host outcome was inferred from output text")
	}
}
