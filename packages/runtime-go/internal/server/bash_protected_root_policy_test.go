package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

type protectedRootObservingShell struct {
	roots []string
	calls int
}

func (runner *protectedRootObservingShell) RunShell(_ context.Context, request ports.ShellRequest) ports.ShellResult {
	runner.roots = append([]string(nil), request.ProtectedReadDirs...)
	runner.calls++
	return ports.ShellResult{}
}

func TestBashServerPropagatesProtectedPolicyToShellOnEveryHost(t *testing.T) {
	workspace := t.TempDir()
	mandatory := filepath.Join(t.TempDir(), "private")
	optional := filepath.Join(t.TempDir(), "optional")
	for _, mode := range []string{"workspace-write", "danger-full-access"} {
		t.Run(mode, func(t *testing.T) {
			runner := &protectedRootObservingShell{}
			roots := []string{optional, filestore.MandatoryProtectedRoot(mandatory)}
			handler := &runtimeServerHandler{shellRunner: runner, protectedReadDirs: roots}
			args := map[string]any{"command": "echo bounded"}
			pending := runtimeBashPendingForTest(t, workspace, args)
			pending.SandboxMode = mode
			result, isError := handler.executeBashRuntimeTool(context.Background(), pending, args)
			if mode == "workspace-write" {
				failure, ok := result.(map[string]any)
				if !isError || !ok || failure["code"] != "sandbox_blocked" || runner.calls != 0 {
					t.Fatal("workspace-write bypassed the existing host-shell approval boundary")
				}
				return
			}
			if isError || runner.calls != 1 {
				t.Fatal("valid synthetic shell request rejected before policy observation")
			}
			if want := filestore.EffectiveProcessProtectedRoots(roots, mode); !reflect.DeepEqual(runner.roots, want) || len(runner.roots) == 0 {
				t.Fatal("server discarded protected process policy")
			}
		})
	}
}

func TestBashServerDoesNotDropMalformedMandatoryPolicy(t *testing.T) {
	runner := &protectedRootObservingShell{}
	handler := &runtimeServerHandler{shellRunner: runner, protectedReadDirs: []string{"analytix-mandatory-protected:relative"}}
	args := map[string]any{"command": "echo bounded"}
	_, _ = handler.executeBashRuntimeTool(context.Background(), runtimeBashPendingForTest(t, t.TempDir(), args), args)
	if !reflect.DeepEqual(runner.roots, []string{"relative"}) {
		t.Fatal("malformed mandatory policy was discarded before the native boundary could reject it")
	}
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
