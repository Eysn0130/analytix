package filestore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestExecuteWriteTextToolWritesAndCapturesCheckpoint(t *testing.T) {
	workspace := t.TempDir()
	prepared := []string{}
	completed := []string{}
	output, isError := ExecuteWriteTextTool(MutationToolInput{
		Workspace: workspace,
		ToolName:  "write_file",
		Args: map[string]any{
			"path":    "nested/a.txt",
			"content": "hello\n",
		},
		Checkpoint: MutationCheckpointHooks{
			Prepare: func(resolvedPath string) (MutationCheckpointDraft, error) {
				prepared = append(prepared, resolvedPath)
				return MutationCheckpointDraft{Enabled: true}, nil
			},
			Complete: func(draft MutationCheckpointDraft, resolvedPath string) error {
				if !draft.Enabled {
					t.Fatalf("checkpoint draft should stay enabled")
				}
				completed = append(completed, resolvedPath)
				return nil
			},
		},
	})
	if isError {
		t.Fatalf("write_file returned error: %#v", output)
	}
	path := filepath.Join(workspace, "nested", "a.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("written content = %q", string(data))
	}
	record, _ := output.(map[string]any)
	if record["relative_path"] != "nested/a.txt" || record["bytes_written"] != float64(6) {
		t.Fatalf("write output mismatch: %#v", output)
	}
	if len(prepared) != 1 || prepared[0] != path || len(completed) != 1 || completed[0] != path {
		t.Fatalf("checkpoint hooks mismatch: prepared=%#v completed=%#v", prepared, completed)
	}
}

func TestExecuteWriteTextToolFailsClosedBeforeMutationWhenCheckpointPrepareFails(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "blocked.txt")
	output, isError := ExecuteWriteTextTool(MutationToolInput{
		Workspace: workspace,
		ToolName:  "write_file",
		Args:      map[string]any{"path": "blocked.txt", "content": "must not be written"},
		Checkpoint: MutationCheckpointHooks{
			Prepare: func(string) (MutationCheckpointDraft, error) {
				return MutationCheckpointDraft{}, errors.New("private checkpoint authority failed")
			},
		},
	})
	record, _ := output.(map[string]any)
	if !isError || record["code"] != "checkpoint_unavailable" {
		t.Fatalf("checkpoint prepare failure did not fail closed: output=%#v error=%v", output, isError)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkpoint prepare failure mutated workspace: %v", err)
	}
}

func TestExecuteWriteTextToolDoesNotReportSuccessWhenCheckpointCompletionFails(t *testing.T) {
	workspace := t.TempDir()
	output, isError := ExecuteWriteTextTool(MutationToolInput{
		Workspace: workspace,
		ToolName:  "write_file",
		Args:      map[string]any{"path": "unsettled.txt", "content": "unsettled"},
		Checkpoint: MutationCheckpointHooks{
			Prepare: func(string) (MutationCheckpointDraft, error) {
				return MutationCheckpointDraft{Enabled: true}, nil
			},
			Complete: func(MutationCheckpointDraft, string) error {
				return errors.New("private checkpoint completion failed")
			},
		},
	})
	record, _ := output.(map[string]any)
	if !isError || record["code"] != "checkpoint_unavailable" {
		t.Fatalf("checkpoint completion failure was reported as success: output=%#v error=%v", output, isError)
	}
}

func TestExecuteWriteTextToolAllowsExternalPathInFullAccess(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "notes.txt")

	output, isError := ExecuteWriteTextTool(MutationToolInput{
		Workspace:   workspace,
		ToolName:    "write_file",
		SandboxMode: "danger-full-access",
		Args: map[string]any{
			"path":    target,
			"content": "external\n",
		},
	})
	if isError {
		t.Fatalf("full access external write returned error: %#v", output)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read external write: %v", err)
	}
	if string(data) != "external\n" {
		t.Fatalf("external write content = %q", string(data))
	}
	record, _ := output.(map[string]any)
	if record["path"] != target {
		t.Fatalf("full access external write output mismatch: %#v", output)
	}

	blocked, blockedErr := ExecuteWriteTextTool(MutationToolInput{
		Workspace:   workspace,
		ToolName:    "write_file",
		SandboxMode: "workspace-write",
		Args: map[string]any{
			"path":    filepath.Join(outside, "blocked.txt"),
			"content": "blocked\n",
		},
	})
	blockedRecord, _ := blocked.(map[string]any)
	if !blockedErr || blockedRecord["code"] != "workspace_escape" {
		t.Fatalf("workspace-write external write should be blocked: %#v isError=%v", blocked, blockedErr)
	}
}

func TestExecuteEditTextToolAppliesExactEdit(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	output, isError := ExecuteEditTextTool(MutationToolInput{
		Workspace: workspace,
		ToolName:  "edit_file",
		Args: map[string]any{
			"path":    "a.txt",
			"oldText": "beta",
			"newText": "omega",
		},
	})
	if isError {
		t.Fatalf("edit_file returned error: %#v", output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if string(data) != "alpha\nomega\n" {
		t.Fatalf("edited content = %q", string(data))
	}
	record, _ := output.(map[string]any)
	if record["replacements"] != float64(1) || record["relative_path"] != "a.txt" {
		t.Fatalf("edit output mismatch: %#v", output)
	}
}

func TestExecuteMoveFileToolMovesWithinWorkspace(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "a.txt")
	destination := filepath.Join(workspace, "nested", "b.txt")
	if err := os.WriteFile(source, []byte("move me"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	moveArguments := []byte(`{"source_path":"a.txt","destination_path":"nested/b.txt"}`)
	output, isError := ExecuteMoveFileTool(MutationToolInput{
		Workspace: workspace, ToolName: "move_file",
		MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(workspace, ".private-mutations")),
		ArgumentsJSON:     moveArguments,
		Args: map[string]any{
			"source_path":      "a.txt",
			"destination_path": "nested/b.txt",
		},
		Checkpoint: moveOperationHooksForTest(t, workspace, moveArguments),
	})
	if isError {
		t.Fatalf("move_file returned error: %#v", output)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source should be moved, stat err=%v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(data) != "move me" {
		t.Fatalf("destination content = %q", string(data))
	}
	record, _ := output.(map[string]any)
	if record["moved"] != true || record["destination_relative_path"] != "nested/b.txt" {
		t.Fatalf("move output mismatch: %#v", output)
	}
}

func moveOperationHooksForTest(t *testing.T, workspace string, arguments []byte) MutationCheckpointHooks {
	t.Helper()
	return MutationCheckpointHooks{
		BeginOperation: func(paths []MutationOperationPath) (MutationOperationDraft, error) {
			now := time.Unix(1_700_400_000, 0).UTC()
			workspaceRealPath, err := WorkspaceRealPath(workspace)
			if err != nil {
				return MutationOperationDraft{}, err
			}
			securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID: "thr-move-tool", TurnID: "turn-move-tool", WorkspaceRealPath: workspaceRealPath,
				TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
			})
			if err != nil {
				return MutationOperationDraft{}, err
			}
			scope, _ := json.Marshal([]string{"move_file"})
			grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
				Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "move_file", ToolCallID: filestoreTestHostToolCallID(t, "call-move-tool"),
				ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
				ScopeHash: domainsecurity.SHA256Hex(scope), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
			})
			observer := CheckpointOperationObserver{}
			domainPaths := make([]domaincheckpoint.OperationPathInputV2, 0, len(paths))
			sourceHash := ""
			for _, item := range paths {
				before, err := observer.CaptureBefore(context.Background(), workspace, item.ResolvedPath)
				if err != nil {
					return MutationOperationDraft{}, err
				}
				if item.Role == "source" {
					sourceHash = before.Hash
				}
				domainPaths = append(domainPaths, domaincheckpoint.OperationPathInputV2{
					ArgumentKey: item.ArgumentKey, RequestedPath: item.RequestedPath,
					PathAuthoritySchemaVersion: before.PathAuthority.SchemaVersion,
					AuthorityKind:              before.PathAuthority.Kind, AuthorityRoot: before.PathAuthority.Root,
					AuthorityRootIdentity: before.PathAuthority.RootIdentity, AuthorityRootHash: before.PathAuthority.RootHash,
					RelativePath: before.PathAuthority.RelativePath, Role: item.Role,
					BeforeExisted: before.Existed, BeforeAvailable: before.ContentAvailable,
					BeforeHash: before.Hash, BeforeSizeBytes: int64(len(before.RawBytes)),
					BeforeSnapshotSchemaVersion: operationBeforeSnapshotVersionForTest(before.Existed),
					BeforeEncoding:              before.Encoding, BeforeBytesBase64: base64.StdEncoding.EncodeToString(before.RawBytes),
					ExpectedAfterExisted: item.ExpectedAfterExisted, ExpectedAfterHash: item.ExpectedAfterHash,
				})
			}
			for index := range domainPaths {
				if domainPaths[index].Role == "destination" {
					domainPaths[index].ExpectedAfterHash = sourceHash
				}
			}
			intent, err := domaincheckpoint.NewOperationGroupIntentV2(domaincheckpoint.OperationGroupIntentInputV2{
				SecurityContext: securityContext, ExecutionGrant: grant,
				CheckpointID: domaincheckpointref.RuntimeID("move-tool"), SourceWorkspaceCheckpointID: "move-tool",
				OperationOrdinal: 1, ToolName: "move_file", ArgumentsJSON: arguments, Paths: domainPaths,
				CreatedAt: now.Add(time.Second),
			})
			if err != nil {
				return MutationOperationDraft{}, err
			}
			return MutationOperationDraft{Enabled: true, Workspace: workspace, AuthorityIntent: intent, Paths: append([]MutationOperationPath(nil), paths...)}, nil
		},
		SettleOperation: func(MutationOperationDraft, bool) (string, error) { return "completed", nil },
	}
}

func operationBeforeSnapshotVersionForTest(existed bool) int {
	if existed {
		return 1
	}
	return 0
}

func TestExecuteMutationToolRejectsReadOnlySandbox(t *testing.T) {
	workspace := t.TempDir()
	output, isError := ExecuteWriteTextTool(MutationToolInput{
		Workspace:   workspace,
		ToolName:    "write_file",
		SandboxMode: "read-only",
		Args:        map[string]any{"path": "a.txt", "content": "blocked"},
	})
	if !isError {
		t.Fatalf("read-only write should be blocked: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["code"] != "sandbox_blocked" {
		t.Fatalf("sandbox output mismatch: %#v", output)
	}
	if _, err := os.Stat(filepath.Join(workspace, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("blocked write should not create file, stat err=%v", err)
	}
}

func TestDangerFullAccessCannotMutateMandatoryRuntimePrivateRoot(t *testing.T) {
	workspace := t.TempDir()
	protected := t.TempDir()
	keyPath := filepath.Join(protected, "authority.json")
	if err := os.WriteFile(keyPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	mandatory := []string{MandatoryProtectedRoot(protected)}
	output, isError := ExecuteWriteTextTool(MutationToolInput{
		Workspace: workspace, ToolName: "write_file", SandboxMode: "danger-full-access", ProtectedReadDirs: mandatory,
		Args: map[string]any{"path": keyPath, "content": "tampered"},
	})
	record, _ := output.(map[string]any)
	if !isError || record["code"] != "protected_dir" {
		t.Fatalf("danger-full-access mutated mandatory runtime private root: output=%#v error=%v", output, isError)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil || string(data) != "original" {
		t.Fatalf("mandatory private key changed: data=%q err=%v", data, err)
	}
	outside := filepath.Join(workspace, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	moveOutput, moveError := ExecuteMoveFileTool(MutationToolInput{
		Workspace: workspace, ToolName: "move_file", SandboxMode: "danger-full-access", ProtectedReadDirs: mandatory,
		Args: map[string]any{"source_path": outside, "destination_path": filepath.Join(protected, "moved.txt")},
	})
	moveRecord, _ := moveOutput.(map[string]any)
	if !moveError || moveRecord["code"] != "protected_dir" {
		t.Fatalf("danger-full-access moved data into mandatory runtime private root: output=%#v error=%v", moveOutput, moveError)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("blocked move changed source: %v", err)
	}
}

func TestProviderMutationToolsCannotChangeWorkspaceHostMetadataEvenWithFullAccess(t *testing.T) {
	workspace := t.TempDir()
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	if err := os.MkdirAll(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	protectedText := filepath.Join(metadata, "protected.txt")
	if err := os.WriteFile(protectedText, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	notebook := filepath.Join(metadata, "protected.ipynb")
	notebookBody := []byte(`{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["old\n"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`)
	if err := os.WriteFile(notebook, notebookBody, 0o600); err != nil {
		t.Fatal(err)
	}
	goFile := filepath.Join(metadata, "protected.go")
	if err := os.WriteFile(goFile, []byte("package hidden\nfunc Keep() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moveSource := filepath.Join(workspace, "move-source.txt")
	if err := os.WriteFile(moveSource, []byte("move source"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := []struct {
		name string
		run  func() (any, bool)
	}{
		{"write", func() (any, bool) {
			return ExecuteWriteTextTool(MutationToolInput{Workspace: workspace, ToolName: "write_file", SandboxMode: "danger-full-access", Args: map[string]any{"path": ".analytix/new.txt", "content": "forged"}})
		}},
		{"edit", func() (any, bool) {
			return ExecuteEditTextTool(MutationToolInput{Workspace: workspace, ToolName: "edit_file", SandboxMode: "danger-full-access", Args: map[string]any{"path": protectedText, "oldText": "beta", "newText": "forged"}})
		}},
		{"move_into", func() (any, bool) {
			return ExecuteMoveFileTool(MutationToolInput{Workspace: workspace, ToolName: "move_file", SandboxMode: "danger-full-access", Args: map[string]any{"source_path": moveSource, "destination_path": ".analytix/moved.txt"}})
		}},
		{"move_out", func() (any, bool) {
			return ExecuteMoveFileTool(MutationToolInput{Workspace: workspace, ToolName: "move_file", SandboxMode: "danger-full-access", Args: map[string]any{"source_path": protectedText, "destination_path": "escaped.txt"}})
		}},
		{"notebook", func() (any, bool) {
			return ExecuteNotebookEditTool(MutationToolInput{Workspace: workspace, ToolName: "notebook_edit", SandboxMode: "danger-full-access", Args: map[string]any{"path": notebook, "cell_id": "intro", "new_source": "forged", "cell_type": "markdown"}})
		}},
		{"delete_range", func() (any, bool) {
			return ExecuteDeleteRangeTool(MutationToolInput{Workspace: workspace, ToolName: "delete_range", SandboxMode: "danger-full-access", Args: map[string]any{"path": protectedText, "start_anchor": "alpha", "end_anchor": "beta"}})
		}},
		{"delete_symbol", func() (any, bool) {
			return ExecuteDeleteSymbolTool(MutationToolInput{Workspace: workspace, ToolName: "delete_symbol", SandboxMode: "danger-full-access", Args: map[string]any{"path": goFile, "name": "Keep"}})
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			output, isError := call.run()
			record, _ := output.(map[string]any)
			if !isError || record["code"] != "host_metadata_protected" {
				body, _ := json.Marshal(output)
				t.Fatalf("metadata mutation escaped: %s error=%v", body, isError)
			}
		})
	}
	if data, err := os.ReadFile(protectedText); err != nil || string(data) != "alpha\nbeta\n" {
		t.Fatalf("protected text changed: %q err=%v", data, err)
	}
	if data, err := os.ReadFile(notebook); err != nil || string(data) != string(notebookBody) {
		t.Fatalf("protected notebook changed: %q err=%v", data, err)
	}
	if data, err := os.ReadFile(goFile); err != nil || string(data) != "package hidden\nfunc Keep() {}\n" {
		t.Fatalf("protected Go file changed: %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(metadata, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("metadata write created a file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(metadata, "moved.txt")); !os.IsNotExist(err) {
		t.Fatalf("metadata move created a file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("metadata move escaped a file: %v", err)
	}
	if data, err := os.ReadFile(moveSource); err != nil || string(data) != "move source" {
		t.Fatalf("blocked move changed source: %q err=%v", data, err)
	}
	lookalikeOutput, lookalikeError := ExecuteWriteTextTool(MutationToolInput{
		Workspace: workspace, ToolName: "write_file", SandboxMode: "danger-full-access",
		Args: map[string]any{"path": ".analytixsdd/plan.md", "content": "allowed"},
	})
	if lookalikeError {
		t.Fatalf("lookalike directory was overblocked: %#v", lookalikeOutput)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, ".analytixsdd", "plan.md")); err != nil || string(data) != "allowed" {
		t.Fatalf("lookalike write failed: %q err=%v", data, err)
	}
}

func TestPreparedNotebookEditResolvesDefaultIndexAndCellIDToSameOwnerEffect(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "one.ipynb")
	body := []byte(`{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["old"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := []map[string]any{
		{"path": "one.ipynb", "new_source": "new"},
		{"path": "one.ipynb", "cell_number": float64(0), "new_source": "new"},
		{"path": "one.ipynb", "cell_id": "intro", "new_source": "new"},
	}
	var baseline string
	for index, args := range inputs {
		prepared, output, failed := PrepareNotebookEditTool(MutationToolInput{Workspace: workspace, ToolName: "notebook_edit", Args: args})
		if failed {
			t.Fatalf("prepare %d failed: %#v", index, output)
		}
		projection, err := PreparedNotebookEditSemanticEffectProjection(prepared)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(projection)
		if index == 0 {
			baseline = string(encoded)
		} else if string(encoded) != baseline {
			t.Fatalf("notebook selector aliases diverged: baseline=%s got=%s", baseline, encoded)
		}
	}
}

func TestExecutePreparedNotebookEditRejectsFileDrift(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "one.ipynb")
	body := []byte(`{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["old"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	input := MutationToolInput{Workspace: workspace, ToolName: "notebook_edit", Args: map[string]any{"path": "one.ipynb", "cell_id": "intro", "new_source": "new"}}
	prepared, output, failed := PrepareNotebookEditTool(input)
	if failed {
		t.Fatalf("prepare failed: %#v", output)
	}
	concurrent := []byte(`{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["concurrent"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`)
	if err := os.WriteFile(path, concurrent, 0o600); err != nil {
		t.Fatal(err)
	}
	result, isError := ExecutePreparedNotebookEditTool(input, prepared)
	if !isError {
		t.Fatalf("stale notebook plan executed: %#v", result)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(concurrent) {
		t.Fatalf("stale plan overwrote concurrent content: got=%q err=%v", got, err)
	}
}

func TestPreparedNotebookDeleteKeepsSameIndexIdentityAfterFirstDeletion(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "two.ipynb")
	body := []byte(`{"cells":[{"cell_type":"markdown","id":"a","metadata":{},"source":["A"]},{"cell_type":"markdown","id":"b","metadata":{},"source":["B"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	input := MutationToolInput{Workspace: workspace, ToolName: "notebook_edit", Args: map[string]any{
		"path": "two.ipynb", "cell_number": float64(0), "edit_mode": "delete",
	}}
	first, output, failed := PrepareNotebookEditTool(input)
	if failed {
		t.Fatalf("first prepare failed: %#v", output)
	}
	firstProjection, _ := PreparedNotebookEditSemanticEffectProjection(first)
	if output, failed := ExecutePreparedNotebookEditTool(input, first); failed {
		t.Fatalf("first delete failed: %#v", output)
	}
	retry, output, failed := PrepareNotebookEditTool(input)
	if failed {
		t.Fatalf("retry prepare failed: %#v", output)
	}
	retryProjection, _ := PreparedNotebookEditSemanticEffectProjection(retry)
	firstBody, _ := json.Marshal(firstProjection)
	retryBody, _ := json.Marshal(retryProjection)
	if string(firstBody) != string(retryBody) {
		t.Fatalf("same index delete minted a second semantic identity: first=%s retry=%s", firstBody, retryBody)
	}
}

func TestPreparedDeleteSymbolResolvesOptionalFiltersToSameOwnerEffect(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\n\nfunc Target() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var projections []string
	for _, args := range []map[string]any{
		{"path": "sample.go", "name": "Target"},
		{"path": "sample.go", "name": "Target", "kind": "func"},
	} {
		prepared, output, failed := PrepareDeleteSymbolTool(MutationToolInput{Workspace: workspace, ToolName: "delete_symbol", Args: args})
		if failed {
			t.Fatalf("prepare failed: %#v", output)
		}
		projection, err := PreparedDeleteSymbolSemanticEffectProjection(prepared)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(projection)
		projections = append(projections, string(encoded))
	}
	if projections[0] != projections[1] {
		t.Fatalf("delete_symbol optional filter aliases diverged: %#v", projections)
	}
}

func TestPreparedDeleteSymbolIdentityDoesNotDependOnLineNumber(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.go")
	input := MutationToolInput{Workspace: workspace, ToolName: "delete_symbol", Args: map[string]any{"path": "sample.go", "name": "Target"}}
	var projections []string
	for _, body := range []string{"package sample\nfunc Target() {}\n", "package sample\n\n\nfunc Target() {}\n"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		prepared, output, failed := PrepareDeleteSymbolTool(input)
		if failed {
			t.Fatalf("prepare failed: %#v", output)
		}
		projection, _ := PreparedDeleteSymbolSemanticEffectProjection(prepared)
		encoded, _ := json.Marshal(projection)
		projections = append(projections, string(encoded))
	}
	if projections[0] != projections[1] {
		t.Fatalf("delete_symbol line drift changed semantic identity: %#v", projections)
	}
}
