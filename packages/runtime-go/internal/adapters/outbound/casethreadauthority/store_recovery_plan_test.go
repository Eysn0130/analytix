package casethreadauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPreparedCaseThreadSnapshotIsReadOnlyAndRejectsDrift(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "case-thread-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	if records, err := prepared.SnapshotInventory(ctx); err != nil || len(records) != 0 {
		t.Fatalf("absent original snapshot failed: count=%d err=%v", len(records), err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("absent snapshot created an authority store")
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	record := testCaseThreadAuthorityRecord(t)
	if err := store.PutIfAbsent(ctx, record); err != nil {
		t.Fatal(err)
	}
	prepared, err = PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.RecordDigest[:2], record.RecordDigest+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, canonicalOnly := range []bool{false, true} {
		read := prepared.SnapshotInventory
		if canonicalOnly {
			read = prepared.SnapshotCanonicalRecordsV1
		}
		records, err := read(ctx)
		if err != nil || len(records) != 1 || records[0].RecordDigest != record.RecordDigest {
			t.Fatalf("original snapshot lost canonical record: %v", err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := read(cancelled); !errors.Is(err, context.Canceled) {
			t.Errorf("snapshot lost cancellation: %v", err)
		}
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, current) {
		t.Fatal("snapshot changed original record bytes")
	}
	currentInfo, err := os.Stat(path)
	if err != nil || !os.SameFile(info, currentInfo) || info.Mode() != currentInfo.Mode() {
		t.Fatal("snapshot changed original record identity or mode")
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.SnapshotInventory(ctx); err == nil {
		t.Fatal("prepared snapshot accepted changed physical input")
	}
}
