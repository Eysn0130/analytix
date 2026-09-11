package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	startupapp "analytix.local/runtime-go/internal/app/startup"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestLiveCheckpointRecoveryDeltaCannotAbsorbConcurrentManagedMutation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	durableDir := t.TempDir()
	workspace := t.TempDir()
	target := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, target, "before")
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(checkpointRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := checkpointauthority.NewStoreContext(ctx, checkpointRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	authority := checkpointapp.SnapshotAuthority{Store: store}
	service := checkpointapp.OperationService{Authority: authority, Observer: filestore.CheckpointOperationObserver{}}
	now := time.Unix(1_700_910_000, 0).UTC()
	arguments := []byte(`{"path":"a.txt","content":"after"}`)
	securityContext, grant := operationRecoverySecurity(t, now, workspace, "write_file", arguments)
	if _, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("authorized-rebase"), SourceWorkspaceCheckpointID: "authorized-rebase",
		Workspace: workspace, ToolName: "write_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{{
			ResolvedPath: target, ArgumentKey: "path", RequestedPath: "a.txt", Role: "target",
			ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after"),
		}},
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(dataDir, durableDir)
	if err != nil {
		t.Fatal(err)
	}
	reader := persistencefs.NewStartupSnapshotReader(roots)
	before, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prepared, delta, err := recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1(
		ctx, reader, dataDir, access, nil, now.Add(2*time.Second), nil,
	)
	if err != nil || !delta.HasChanges() {
		t.Fatalf("live recovery did not return an exact managed delta: %#v err=%v", delta, err)
	}
	if err := prepared.Verify(ctx, delta); err != nil {
		t.Fatalf("pre-snapshot terminal addition receipt failed: %v", err)
	}
	after, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Verify(ctx, delta); err != nil {
		t.Fatalf("post-snapshot terminal addition receipt failed: %v", err)
	}
	if err := startupapp.ValidateAuthorizedManagedDeltaV1(before, after, delta.Managed); err != nil {
		t.Fatalf("actual terminal CAS delta was not authorized exactly: %v", err)
	}

	// A generic managed snapshot deliberately has no filesystem object ID. A
	// same-body replacement can therefore preserve every snapshot-visible
	// field; the post-bind host receipt verification must still reject it.
	addition := delta.Managed.AddedFiles[0]
	relative := strings.TrimPrefix(addition.Path, "data/")
	recordPath := filepath.Join(dataDir, filepath.FromSlash(relative))
	shardPath := filepath.Dir(recordPath)
	recordInfo, err := os.Stat(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	shardInfo, err := os.Stat(shardPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	replacement := recordPath + ".replacement"
	if err := os.WriteFile(replacement, body, recordInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, recordInfo.ModTime(), recordInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, recordPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(shardPath, shardInfo.ModTime(), shardInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	replaced, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := startupapp.ValidateAuthorizedManagedDeltaV1(before, replaced, delta.Managed); err != nil {
		t.Fatalf("same-body replacement changed generic snapshot fields: %v", err)
	}
	if err := prepared.Verify(ctx, delta); err == nil {
		t.Fatal("same-body replacement retained host-issued terminal addition authority")
	}

	unrelated := filepath.Join(dataDir, "memory", "concurrent.json")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte(`{"trusted":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tampered, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := startupapp.ValidateAuthorizedManagedDeltaV1(before, tampered, delta.Managed); err == nil {
		t.Fatal("concurrent managed mutation was absorbed into the post-recovery baseline")
	}
}
