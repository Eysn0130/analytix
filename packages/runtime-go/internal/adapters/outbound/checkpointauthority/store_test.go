package checkpointauthority

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestOpenExistingStoreLeavesMissingAuthorityAbsent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, present, err := OpenExistingStoreContext(context.Background(), root, access)
	if err != nil || present || store != nil {
		t.Fatalf("missing checkpoint authority open result: store=%v present=%v err=%v", store, present, err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("opening missing checkpoint authority created its root: %v", err)
	}
}

func TestOpenExistingStoreRejectsPartialLayoutWithoutRepair(t *testing.T) {
	root := filepath.Join(t.TempDir(), "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	presentRoot := filepath.Join(root, "snapshot-intents")
	if _, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(presentRoot, maxCheckpointAuthorityCASBytes, access); err != nil {
		t.Fatal(err)
	}
	store, present, err := OpenExistingStoreContext(context.Background(), root, access)
	if err == nil || !errors.Is(err, checkpointport.ErrCorrupt) || present || store != nil {
		t.Fatalf("partial checkpoint authority failed open: store=%v present=%v err=%v", store, present, err)
	}
	for _, missing := range []string{
		"snapshot-completions", "snapshot-dispositions", "operation-group-intents-v2", "operation-group-terminals-v2",
	} {
		if _, statErr := os.Lstat(filepath.Join(root, missing)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("partial checkpoint authority open repaired %s: %v", missing, statErr)
		}
	}
}

func TestOpenExistingStoreRejectsEmptyAuthorityContainer(t *testing.T) {
	root := filepath.Join(t.TempDir(), "checkpoint-authority")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, present, err := OpenExistingStoreContext(context.Background(), root, access)
	if err == nil || !errors.Is(err, checkpointport.ErrCorrupt) || present || store != nil {
		t.Fatalf("empty checkpoint authority failed open: store=%v present=%v err=%v", store, present, err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("empty checkpoint authority was repaired: entries=%v err=%v", entries, readErr)
	}
}

func TestStoreMaterializesOnlyCompletedOwnerBoundSnapshots(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	securityContext, grant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_1")
	before := "before\n"
	intent, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: "axcp_one", SourceWorkspaceCheckpointID: "gcp_one", RelativePath: "a.txt",
		BeforeExisted: true, BeforeAvailable: true, BeforeHash: domainsecurity.SHA256Hex([]byte(before)),
		BeforeContent: before, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveCheckpoint(ctx, "thr_1", "axcp_one"); err == nil {
		t.Fatal("unsettled snapshot intent must not materialize")
	}
	after := "after\n"
	if _, err := store.CompleteSnapshot(ctx, intent.SnapshotIntentID, true, domainsecurity.SHA256Hex([]byte(after)), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveCheckpoint(ctx, "thr_1", "axcp_one")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Intent.BeforeContent != before || resolved[0].Completion.AfterHash != domainsecurity.SHA256Hex([]byte(after)) {
		t.Fatalf("materialized snapshot mismatch: %#v", resolved)
	}
}

func TestStoreRejectsCheckpointReuseAcrossSecurityContexts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	firstContext, firstGrant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_1")
	if _, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: firstContext, ExecutionGrant: firstGrant, CheckpointID: "axcp_shared",
		SourceWorkspaceCheckpointID: "gcp_shared", RelativePath: "a.txt", CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	secondContext, secondGrant := checkpointAuthoritySecurity(t, now.Add(time.Minute), "thr_1", "turn_2", "/workspace", "call_2")
	if _, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: secondContext, ExecutionGrant: secondGrant, CheckpointID: "axcp_shared",
		SourceWorkspaceCheckpointID: "gcp_shared", RelativePath: "b.txt", CreatedAt: now.Add(time.Minute + time.Second),
	}); err == nil {
		t.Fatal("checkpoint id reuse across turn security contexts must fail closed")
	}
}

func TestStoreAllowsOneExecutionGrantToOwnMultipleMutationPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	securityContext, grant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_move")
	first, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: grant, CheckpointID: "axcp_move",
		SourceWorkspaceCheckpointID: "gcp_move", RelativePath: "old/a.txt", CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: grant, CheckpointID: "axcp_move",
		SourceWorkspaceCheckpointID: "gcp_move", RelativePath: "new/a.txt", CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotIntentID == second.SnapshotIntentID || first.MutationOrdinal != 1 || second.MutationOrdinal != 2 {
		t.Fatalf("multi-path grant did not receive distinct ordered intents: first=%#v second=%#v", first, second)
	}
}

func TestSnapshotAuthorityDerivesNetPathStateAcrossMultipleOperations(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	securityContext, deleteGrant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_delete")
	original := "original"
	deleted, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: deleteGrant, CheckpointID: "axcp_net",
		SourceWorkspaceCheckpointID: "gcp_net", RelativePath: "a.txt", BeforeExisted: true, BeforeAvailable: true,
		BeforeHash: domainsecurity.SHA256Hex([]byte(original)), BeforeContent: original, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteSnapshot(ctx, deleted.SnapshotIntentID, false, "", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	_, recreateGrant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_recreate")
	recreated, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: recreateGrant, CheckpointID: "axcp_net",
		SourceWorkspaceCheckpointID: "gcp_net", RelativePath: "a.txt", CreatedAt: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	replacement := "replacement"
	if _, err := store.CompleteSnapshot(ctx, recreated.SnapshotIntentID, true, domainsecurity.SHA256Hex([]byte(replacement)), now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	records, err := (checkpointapp.SnapshotAuthority{Store: store}).ResolveRecords(ctx, "thr_1", "axcp_net")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0]["changeKind"] != "modified" ||
		records[0]["beforeHash"] != domainsecurity.SHA256Hex([]byte(original)) ||
		records[0]["afterHash"] != domainsecurity.SHA256Hex([]byte(replacement)) {
		t.Fatalf("delete/recreate net state must restore the original file: %#v", records)
	}
}

func TestSnapshotAuthorityDropsCreateThenDeleteNetNoop(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	securityContext, createGrant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_create")
	created, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: createGrant, CheckpointID: "axcp_noop",
		SourceWorkspaceCheckpointID: "gcp_noop", RelativePath: "a.txt", CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	content := "temporary"
	contentHash := domainsecurity.SHA256Hex([]byte(content))
	if _, err := store.CompleteSnapshot(ctx, created.SnapshotIntentID, true, contentHash, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	_, deleteGrant := checkpointAuthoritySecurity(t, now, "thr_1", "turn_1", "/workspace", "call_delete")
	deleted, err := store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: securityContext, ExecutionGrant: deleteGrant, CheckpointID: "axcp_noop",
		SourceWorkspaceCheckpointID: "gcp_noop", RelativePath: "a.txt", BeforeExisted: true, BeforeAvailable: true,
		BeforeHash: contentHash, BeforeContent: content, CreatedAt: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteSnapshot(ctx, deleted.SnapshotIntentID, false, "", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	records, err := (checkpointapp.SnapshotAuthority{Store: store}).ResolveRecords(ctx, "thr_1", "axcp_noop")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("create/delete net no-op must not produce rewind mutation: %#v", records)
	}
}

func checkpointAuthoritySecurity(
	t *testing.T,
	now time.Time,
	threadID string,
	turnID string,
	workspace string,
	callID string,
) (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) {
	t.Helper()
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{"write_file"})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin",
		ToolName: "write_file", ToolCallID: checkpointAuthorityTestHostToolCallID(t, callID), ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"path":"a.txt"}`)),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex(scope),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	return securityContext, grant
}
