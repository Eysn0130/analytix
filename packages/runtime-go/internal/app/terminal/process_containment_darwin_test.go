//go:build darwin

package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	processcontainment "analytix.local/runtime-go/internal/testsupport/processcontainment"
)

func TestForegroundAndSubagentShellContainProtectedRootsWithoutDisablingOrdinaryWork(t *testing.T) {
	fixture := processcontainment.NewFixture(t)
	runner := processcontainment.NewShellRunner()

	foreground, foregroundError := ExecuteForegroundBash(
		context.Background(),
		runner,
		BashRequest{
			Command:           "/bin/cat " + quoteProcessContainmentPath(fixture.ProtectedFile),
			Workspace:         fixture.OrdinaryRoot,
			ProtectedReadDirs: fixture.ProtectedRoots,
		},
	)
	assertProtectedShellBlocked(t, foreground, foregroundError)

	service := NewService(Dependencies{ShellRunner: runner})
	subagent, subagentError := service.ExecuteBashTool(
		context.Background(),
		BashToolRunRequest{
			Context: BashToolContext{
				Workspace:         fixture.OrdinaryRoot,
				WorkspaceAbsolute: true,
				SandboxMode:       "danger-full-access",
				ProtectedReadDirs: fixture.ProtectedRoots,
				SubagentDepth:     1,
			},
			Args: map[string]any{
				"command": "/bin/cat " + quoteProcessContainmentPath(fixture.ProtectedFile),
			},
		},
		BashToolCallbacks{},
	)
	assertProtectedShellBlocked(t, subagent, subagentError)

	ordinaryOutput := fixture.OrdinaryPath("subagent-ordinary.txt")
	ordinary, ordinaryError := service.ExecuteBashTool(
		context.Background(),
		BashToolRunRequest{
			Context: BashToolContext{
				Workspace:         fixture.OrdinaryRoot,
				WorkspaceAbsolute: true,
				SandboxMode:       "danger-full-access",
				ProtectedReadDirs: fixture.ProtectedRoots,
				SubagentDepth:     1,
			},
			Args: map[string]any{
				"command": "printf ordinary > " + quoteProcessContainmentPath(ordinaryOutput),
			},
		},
		BashToolCallbacks{},
	)
	if ordinaryError {
		t.Fatalf("ordinary subagent shell failed: %#v", ordinary)
	}
	processcontainment.AssertFileContents(t, ordinaryOutput, "ordinary")
}

func TestBackgroundShellContainsProtectedRootsWithoutDisablingOrdinaryWork(t *testing.T) {
	fixture := processcontainment.NewFixture(t)
	runner := processcontainment.NewShellRunner()

	blockedRepo := &fakeJobRepository{}
	blockedDone := make(chan struct{})
	blockedBinding := testBackgroundBashSecurityBinding(t, "thread-protected", "turn-protected", fixture.OrdinaryRoot)
	service := NewService(Dependencies{Jobs: blockedRepo, ShellRunner: runner})
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID: "thread-protected", TurnID: "turn-protected",
			ToolCallID: blockedBinding.ParentToolCallID,
			Workspace:  fixture.OrdinaryRoot, WorkspaceAbsolute: true,
			SandboxMode: "danger-full-access", ProtectedReadDirs: fixture.ProtectedRoots,
			SecurityBinding: blockedBinding,
		},
		Args: map[string]any{
			"command":           "/bin/cat " + quoteProcessContainmentPath(fixture.ProtectedFile),
			"run_in_background": true,
		},
	}, BashToolCallbacks{
		RegisterBoundBackgroundJob: func(string, *domainjob.SecurityBinding, context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
			return subagentapp.NewBackgroundJobStartBarrier(), true
		},
		UnregisterBackgroundJob: func(string) { close(blockedDone) },
	})
	if isError {
		t.Fatalf("protected background shell was not admitted to its contained process: %#v", payload)
	}
	waitForProcessContainmentBackground(t, blockedDone)
	if blockedRepo.record.Status != "failed" ||
		strings.Contains(blockedRepo.record.Output, "must-not-escape") ||
		strings.Contains(blockedRepo.record.Error, "must-not-escape") {
		t.Fatalf("protected background shell escaped containment: %#v", blockedRepo.record)
	}

	ordinaryOutput := fixture.OrdinaryPath("background-ordinary.txt")
	ordinaryRepo := &fakeJobRepository{}
	ordinaryDone := make(chan struct{})
	ordinaryBinding := testBackgroundBashSecurityBinding(t, "thread-ordinary", "turn-ordinary", fixture.OrdinaryRoot)
	service = NewService(Dependencies{Jobs: ordinaryRepo, ShellRunner: runner})
	payload, isError = service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID: "thread-ordinary", TurnID: "turn-ordinary",
			ToolCallID: ordinaryBinding.ParentToolCallID,
			Workspace:  fixture.OrdinaryRoot, WorkspaceAbsolute: true,
			SandboxMode: "danger-full-access", ProtectedReadDirs: fixture.ProtectedRoots,
			SecurityBinding: ordinaryBinding,
		},
		Args: map[string]any{
			"command":           "printf ordinary > " + quoteProcessContainmentPath(ordinaryOutput),
			"run_in_background": true,
		},
	}, BashToolCallbacks{
		RegisterBoundBackgroundJob: func(string, *domainjob.SecurityBinding, context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
			return subagentapp.NewBackgroundJobStartBarrier(), true
		},
		UnregisterBackgroundJob: func(string) { close(ordinaryDone) },
	})
	if isError {
		t.Fatalf("ordinary background shell failed admission: %#v", payload)
	}
	waitForProcessContainmentBackground(t, ordinaryDone)
	if ordinaryRepo.record.Status != "completed" {
		t.Fatalf("ordinary background shell did not complete: %#v", ordinaryRepo.record)
	}
	processcontainment.AssertFileContents(t, ordinaryOutput, "ordinary")
}

func TestForegroundBackgroundAndSubagentShellRejectLocalReadDeputy(t *testing.T) {
	fixture := processcontainment.NewFixture(t)
	runner := processcontainment.NewShellRunner()

	t.Run("foreground", func(t *testing.T) {
		deputy, command := processcontainment.StartReadDeputy(t, fixture.ProtectedFile)
		payload, isError := ExecuteForegroundBash(
			context.Background(),
			runner,
			BashRequest{
				Command: command, Workspace: fixture.OrdinaryRoot,
				ProtectedReadDirs: fixture.ProtectedRoots,
			},
		)
		deputy.AssertNotReached(t)
		assertProtectedShellBlocked(t, payload, isError)
	})

	t.Run("subagent", func(t *testing.T) {
		deputy, command := processcontainment.StartReadDeputy(t, fixture.ProtectedFile)
		service := NewService(Dependencies{ShellRunner: runner})
		payload, isError := service.ExecuteBashTool(
			context.Background(),
			BashToolRunRequest{
				Context: BashToolContext{
					Workspace: fixture.OrdinaryRoot, WorkspaceAbsolute: true,
					SandboxMode: "danger-full-access", ProtectedReadDirs: fixture.ProtectedRoots,
					SubagentDepth: 1,
				},
				Args: map[string]any{"command": command},
			},
			BashToolCallbacks{},
		)
		deputy.AssertNotReached(t)
		assertProtectedShellBlocked(t, payload, isError)
	})

	t.Run("background", func(t *testing.T) {
		deputy, command := processcontainment.StartReadDeputy(t, fixture.ProtectedFile)
		repository := &fakeJobRepository{}
		done := make(chan struct{})
		binding := testBackgroundBashSecurityBinding(
			t,
			"thread-deputy",
			"turn-deputy",
			fixture.OrdinaryRoot,
		)
		service := NewService(Dependencies{Jobs: repository, ShellRunner: runner})
		payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
			Context: BashToolContext{
				ThreadID: "thread-deputy", TurnID: "turn-deputy",
				ToolCallID: binding.ParentToolCallID,
				Workspace:  fixture.OrdinaryRoot, WorkspaceAbsolute: true,
				SandboxMode: "danger-full-access", ProtectedReadDirs: fixture.ProtectedRoots,
				SecurityBinding: binding,
			},
			Args: map[string]any{"command": command, "run_in_background": true},
		}, BashToolCallbacks{
			RegisterBoundBackgroundJob: func(string, *domainjob.SecurityBinding, context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
				return subagentapp.NewBackgroundJobStartBarrier(), true
			},
			UnregisterBackgroundJob: func(string) { close(done) },
		})
		if isError {
			t.Fatalf("delegated-read probe was not admitted to containment: %#v", payload)
		}
		waitForProcessContainmentBackground(t, done)
		deputy.AssertNotReached(t)
		if repository.record.Status != "failed" ||
			strings.Contains(repository.record.Output, "must-not-escape") ||
			strings.Contains(repository.record.Error, "must-not-escape") {
			t.Fatalf("background shell reached a local read deputy: %#v", repository.record)
		}
	})
}

func assertProtectedShellBlocked(t *testing.T, payload any, isError bool) {
	t.Helper()
	body, ok := payload.(map[string]any)
	if !isError || !ok || body["status"] != "failed" ||
		strings.Contains(stringValue(body["output"]), "must-not-escape") ||
		strings.Contains(stringValue(body["error"]), "must-not-escape") {
		t.Fatalf("protected bytes escaped shell containment: payload=%#v isError=%t", payload, isError)
	}
}

func waitForProcessContainmentBackground(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background shell did not settle")
	}
}

func quoteProcessContainmentPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
