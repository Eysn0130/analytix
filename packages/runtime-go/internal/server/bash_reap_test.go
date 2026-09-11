//go:build !windows

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	processadapter "analytix.local/runtime-go/internal/adapters/outbound/process"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRuntimeBashToolRequestFromPendingTrimsWorkspaceBeforeValidation(t *testing.T) {
	workspace := t.TempDir()
	pending := runtimeBashPendingForTest(t, workspace, map[string]any{"command": "pwd"})
	pending.Workspace = " " + workspace + " "
	request, failure, failed := runtimeBashToolRequestFromPending(pending, map[string]any{"command": "pwd"})
	if failed || failure != nil {
		t.Fatalf("trimmed absolute workspace should validate: request=%#v failure=%#v failed=%v", request, failure, failed)
	}
	if request.Workspace != workspace || request.TimeoutSeconds != terminalapp.DefaultBashTimeoutSeconds {
		t.Fatalf("workspace should be trimmed: %#v", request)
	}

	pending.Workspace = filepath.Join(workspace, "missing")
	if _, failure, failed := runtimeBashToolRequestFromPending(pending, map[string]any{"command": "pwd"}); !failed || failure["code"] != "invalid_workspace" {
		t.Fatalf("missing workspace should fail validation: %#v failed=%v", failure, failed)
	}
}

func TestForegroundBashReapsBackgroundChildrenOnNormalExit(t *testing.T) {
	workspace := t.TempDir()
	pidFile := filepath.Join(workspace, "child.pid")
	aliveFile := filepath.Join(workspace, "child.alive")
	command := strings.Join([]string{
		"(printf alive > " + strconv.Quote(aliveFile) + "; sleep 60) >/dev/null 2>&1 & child=$!",
		"printf '%s' \"$child\" > " + strconv.Quote(pidFile),
		"while [ ! -f " + strconv.Quote(aliveFile) + " ]; do sleep 0.01; done",
	}, "; ")

	handler := &runtimeServerHandler{shellRunner: processadapter.NewShellRunner()}
	arguments := map[string]any{"command": command, "timeout": float64(5)}
	result, isError := handler.executeBashRuntimeTool(context.Background(), runtimeBashPendingForTest(t, workspace, arguments), arguments)
	if isError {
		t.Fatalf("foreground bash should complete normally before reap: %#v", result)
	}
	if _, err := os.Stat(aliveFile); err != nil {
		t.Fatalf("background child did not start: %v", err)
	}
	rawPID, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", rawPID, err)
	}

	if waitForProcessExit(pid, 2*time.Second) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("foreground bash left background child process %d alive after normal shell exit", pid)
}

func runtimeBashPendingForTest(t *testing.T, workspace string, arguments map[string]any) runtimePendingToolCall {
	t.Helper()
	now := time.Unix(1_700_000_000, 0).UTC()
	threadID := "thread-bash-reap"
	turnID := "turn-bash-reap"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil), ContextEpoch: 1, IssuedAt: now,
	})
	argumentBody, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("bash-reap"), Name: "bash", Arguments: argumentBody}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "test-provider", ServerIdentity: "host:builtin", ToolName: call.Name,
		ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(argumentBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: now,
	})
	return runtimePendingToolCall{
		ThreadID: threadID, TurnID: turnID, Workspace: workspace, SandboxMode: "danger-full-access",
		Call: call, SecurityContext: securityContext, ExecutionGrant: grant,
	}
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) != nil
}
