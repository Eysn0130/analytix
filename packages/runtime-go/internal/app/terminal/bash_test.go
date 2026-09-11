package terminal

import (
	"context"
	"errors"
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
