//go:build darwin || linux

package datasetsnapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestDatasetSnapshotStoresRejectFilesystemSubstitution(t *testing.T) {
	fixture := newDatasetSnapshotStoreFixture(t)
	ctx := context.Background()

	t.Run("record symlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "records")
		store, err := newTestRecordStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutIfAbsent(ctx, fixture.record); err != nil {
			t.Fatal(err)
		}
		path := datasetSnapshotCASPath(root, fixture.record.RecordDigest)
		body, _ := domainsecurity.DatasetSnapshotAuthorityRecordV1Bytes(fixture.record)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "record.json")
		if err := os.WriteFile(external, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, path); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(ctx, fixture.record.RecordDigest); err == nil {
			t.Fatal("record store followed a symlink substitution")
		}
	})

	t.Run("index hardlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		store, err := newTestIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutIfAbsent(ctx, fixture.index); err != nil {
			t.Fatal(err)
		}
		path := datasetSnapshotCASPath(root, fixture.index.IndexDigest)
		body, _ := os.ReadFile(path)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "index.json")
		if err := os.WriteFile(external, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(external, path); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(ctx, fixture.index.IndexDigest); err == nil {
			t.Fatal("index store accepted a multi-link substitution")
		}
	})

	t.Run("unsafe root mode", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "records")
		store, err := newTestRecordStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutIfAbsent(ctx, fixture.record); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
		if _, err := store.Resolve(ctx, fixture.record.RecordDigest); err == nil {
			t.Fatal("record store accepted an unsafe root mode")
		}
	})

	t.Run("truncated canonical body", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		store, err := newTestIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutIfAbsent(ctx, fixture.index); err != nil {
			t.Fatal(err)
		}
		path := datasetSnapshotCASPath(root, fixture.index.IndexDigest)
		body, _ := os.ReadFile(path)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body[:len(body)/2], 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(ctx, fixture.index.IndexDigest); err == nil {
			t.Fatal("index store accepted a truncated canonical body")
		}
	})
}

func datasetSnapshotCASPath(root, digest string) string {
	return filepath.Join(root, digest[:2], digest+".json")
}
