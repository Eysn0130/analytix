//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	electronlegacytask "analytix.local/runtime-go/internal/adapters/outbound/electronlegacytask"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
)

const retiredAuditPrimaryParentIDV1 = "thr_retired_audit_parent"

func newRetiredAuditMetadataOnlyPrimaryFixtureV1(t *testing.T, family string) (persistencefs.RootSet, persistencefs.RawSnapshot) {
	t.Helper()
	base := t.TempDir()
	dataRoot := filepath.Join(base, "data")
	durableRoot := filepath.Join(base, "durable")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	parentRoot := filepath.Join(durableRoot, filepath.FromSlash(family), retiredAuditPrimaryParentIDV1)
	if err := os.MkdirAll(parentRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "thread_metadata",
		"thread": map[string]any{
			"id":    retiredAuditPrimaryParentIDV1,
			"turns": []any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentRoot, "metadata.jsonl"), append(metadata, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(dataRoot, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(context.Background(), roots)
	if err != nil {
		t.Fatal(err)
	}
	return roots, snapshot
}

func TestRuntimeOriginalPrimaryInventoryStrictReaderRejectsMetadataOnlyParent(t *testing.T) {
	roots, snapshot := newRetiredAuditMetadataOnlyPrimaryFixtureV1(t, "threads")
	_, _, err := readRuntimeOriginalPrimaryInventoryV1(context.Background(), roots, snapshot)
	if !errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
		t.Fatalf("strict original inventory accepted metadata-only parent: %v", err)
	}
}

func TestRuntimeRetiredAuditPrimaryInventoryOnlyAllowsWitnessedProductParent(t *testing.T) {
	roots, snapshot := newRetiredAuditMetadataOnlyPrimaryFixtureV1(t, "threads")
	if _, _, err := readRuntimeOriginalPrimaryInventoryForRetiredAuditV1(
		context.Background(), roots, snapshot, map[string]struct{}{retiredAuditPrimaryParentIDV1: {}},
	); err != nil {
		t.Fatalf("witnessed product sidecar-only parent was rejected: %v", err)
	}
	if _, _, err := readRuntimeOriginalPrimaryInventoryForRetiredAuditV1(
		context.Background(), roots, snapshot, map[string]struct{}{"thr_unwitnessed_parent": {}},
	); !errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
		t.Fatalf("unwitnessed product sidecar-only parent was accepted: %v", err)
	}
}

func TestRuntimeRetiredAuditPrimaryInventoryRejectsWrongFamilyParent(t *testing.T) {
	roots, snapshot := newRetiredAuditMetadataOnlyPrimaryFixtureV1(t, "runtime-go/threads")
	_, _, err := readRuntimeOriginalPrimaryInventoryForRetiredAuditV1(
		context.Background(), roots, snapshot, map[string]struct{}{retiredAuditPrimaryParentIDV1: {}},
	)
	if !errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
		t.Fatalf("retired audit exception crossed into runtime-go family: %v", err)
	}
}

func TestDesktopPrivateHistoryMigrationV2InvalidRetiredLineagePreservesAllState(t *testing.T) {
	for name, missingParent := range map[string]bool{
		"missing-parent":      true,
		"invalid-parent-tool": false,
	} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			configRoot := isolateDesktopMigrationAuthorityConfig(t, base)
			data := filepath.Join(base, "data")
			durable := filepath.Join(base, "durable")
			userData := filepath.Join(base, "electron")
			childRoot := filepath.Join(data, "child-runs")
			for _, root := range []string{data, durable, userData, childRoot} {
				if err := os.MkdirAll(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			target := filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)
			opaque := []byte("retired Electron state must remain untouched")
			if err := os.WriteFile(target, opaque, 0o600); err != nil {
				t.Fatal(err)
			}
			parentID := "thr_retired_missing_parent"
			if !missingParent {
				parentID = "thr_retired_invalid_parent"
				parentRoot := filepath.Join(durable, "threads", parentID)
				if err := os.MkdirAll(parentRoot, 0o700); err != nil {
					t.Fatal(err)
				}
				metadata := []byte(`{"kind":"thread_metadata","thread":{"id":"thr_retired_invalid_parent","turns":[{"id":"turn-parent","threadId":"thr_retired_invalid_parent","status":"completed","items":[]}]}}` + "\n")
				messages := []byte(`{"id":"item-parent","threadId":"thr_retired_invalid_parent","turnId":"turn-parent","kind":"tool_call","status":"completed","toolName":"read_file","callId":"call-parent"}` + "\n")
				if err := os.WriteFile(filepath.Join(parentRoot, "metadata.jsonl"), metadata, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(parentRoot, "messages.jsonl"), messages, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			child := []byte(`{"id":"child_mr1b6yo0_abc123","parentThreadId":"` + parentID + `","parentTurnId":"turn-parent","parentToolCallId":"call-parent","childThreadId":"child_mr1b6yo0_abc123","prompt":"private","status":"completed","usage":{},"createdAt":"2026-07-01T00:00:00Z","updatedAt":"2026-07-01T00:00:00Z"}`)
			childPath := filepath.Join(childRoot, "child_mr1b6yo0_abc123.json")
			if err := os.WriteFile(childPath, child, 0o600); err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, data, durable, userData)
			if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err == nil {
				t.Fatal("invalid retired lineage reached migration commit")
			}
			if after := startupWholeTreeRecordMapForTest(t, data, durable, userData); !reflect.DeepEqual(before, after) {
				t.Fatal("invalid retired lineage changed Go or Electron state")
			}
			if body, err := os.ReadFile(target); err != nil || !reflect.DeepEqual(body, opaque) {
				t.Fatalf("invalid retired lineage changed Electron bytes: body=%q err=%v", body, err)
			}
			if body, err := os.ReadFile(childPath); err != nil || !reflect.DeepEqual(body, child) {
				t.Fatalf("invalid retired lineage changed child bytes: body=%q err=%v", body, err)
			}
			if _, err := os.Lstat(filepath.Join(userData, journalDirectoryV1ForTest)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid retired lineage created retirement journal: %v", err)
			}
			assertPathAbsentForTest(t, filepath.Join(configRoot, "analytix", "startup-authority-v1"))
		})
	}
}
