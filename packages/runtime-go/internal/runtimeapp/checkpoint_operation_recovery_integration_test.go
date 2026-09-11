package runtimeapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestOperationServiceRecoveryClassifiesAllBeforeAllAfterAndMixed(t *testing.T) {
	for _, test := range []struct {
		name           string
		toolName       string
		arguments      []byte
		seed           func(t *testing.T, workspace string)
		paths          func(workspace string) []checkpointapp.OperationPathRequest
		crashState     func(t *testing.T, workspace string)
		wantStatus     string
		wantQuarantine bool
	}{
		{
			name: "all before", toolName: "write_file", arguments: []byte(`{"path":"a.txt","content":"after"}`),
			seed: func(t *testing.T, workspace string) {
				writeOperationFixture(t, filepath.Join(workspace, "a.txt"), "before")
			},
			paths: func(workspace string) []checkpointapp.OperationPathRequest {
				return []checkpointapp.OperationPathRequest{{ResolvedPath: filepath.Join(workspace, "a.txt"), ArgumentKey: "path", RequestedPath: "a.txt", Role: "target", ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after")}}
			},
			wantStatus: "no_effect",
		},
		{
			name: "all after", toolName: "write_file", arguments: []byte(`{"path":"a.txt","content":"after"}`),
			seed: func(t *testing.T, workspace string) {
				writeOperationFixture(t, filepath.Join(workspace, "a.txt"), "before")
			},
			paths: func(workspace string) []checkpointapp.OperationPathRequest {
				return []checkpointapp.OperationPathRequest{{ResolvedPath: filepath.Join(workspace, "a.txt"), ArgumentKey: "path", RequestedPath: "a.txt", Role: "target", ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after")}}
			},
			crashState: func(t *testing.T, workspace string) {
				writeOperationFixture(t, filepath.Join(workspace, "a.txt"), "after")
			},
			wantStatus: "completed",
		},
		{
			name: "mixed move", toolName: "move_file", arguments: []byte(`{"source_path":"a.txt","destination_path":"b.txt"}`),
			seed: func(t *testing.T, workspace string) {
				writeOperationFixture(t, filepath.Join(workspace, "a.txt"), "move")
			},
			paths: func(workspace string) []checkpointapp.OperationPathRequest {
				return []checkpointapp.OperationPathRequest{
					{ResolvedPath: filepath.Join(workspace, "a.txt"), ArgumentKey: "source_path", RequestedPath: "a.txt", Role: "source"},
					{ResolvedPath: filepath.Join(workspace, "b.txt"), ArgumentKey: "destination_path", RequestedPath: "b.txt", Role: "destination", ExpectedAfterExisted: true},
				}
			},
			crashState: func(t *testing.T, workspace string) {
				if err := os.Remove(filepath.Join(workspace, "a.txt")); err != nil {
					t.Fatal(err)
				}
			},
			wantStatus: "quarantined", wantQuarantine: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			workspace := t.TempDir()
			test.seed(t, workspace)
			now := time.Unix(1_700_000_000, 0).UTC()
			authority, service := operationRecoveryService(t, ctx)
			securityContext, grant := operationRecoverySecurity(t, now, workspace, test.toolName, test.arguments)
			_, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
				SecurityContext: securityContext, ExecutionGrant: grant,
				CheckpointID: domaincheckpointref.RuntimeID("checkpoint-" + test.name), SourceWorkspaceCheckpointID: "checkpoint-" + test.name,
				Workspace: workspace, ToolName: test.toolName, ArgumentsJSON: test.arguments,
				Paths: test.paths(workspace), CreatedAt: now.Add(time.Second),
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.crashState != nil {
				test.crashState(t, workspace)
			}
			results, err := service.RecoverOpen(ctx, now.Add(2*time.Second))
			if test.wantQuarantine != (err != nil) || len(results) != 1 || results[0].Terminal.Status != test.wantStatus {
				t.Fatalf("recovery mismatch: results=%#v err=%v", results, err)
			}
			open, openErr := authority.OpenOperationGroups(ctx)
			if openErr != nil || len(open) != 0 {
				t.Fatalf("recovered operation remained open: %#v err=%v", open, openErr)
			}
		})
	}
}

func operationRecoveryService(t *testing.T, ctx context.Context) (checkpointapp.SnapshotAuthority, checkpointapp.OperationService) {
	t.Helper()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := checkpointauthority.NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	mutationAuthority, err := filestore.OpenConditionalMutationAuthority(filepath.Join(root, "file-mutation-quarantine-v1"))
	if err != nil {
		t.Fatal(err)
	}
	authority := checkpointapp.SnapshotAuthority{Store: store}
	observer := filestore.CheckpointOperationObserver{MutationAuthority: mutationAuthority}
	return authority, checkpointapp.OperationService{Authority: authority, Observer: observer, Recovery: observer}
}

func operationRecoverySecurity(t *testing.T, now time.Time, workspace, toolName string, arguments []byte) (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) {
	t.Helper()
	workspaceRealPath, err := filestore.WorkspaceRealPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-recovery", TurnID: "turn-recovery", WorkspaceRealPath: workspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{toolName})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: runtimeCheckpointTestHostToolCallID(t, "call-"+toolName),
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scope), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	return securityContext, grant
}

func writeOperationFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
