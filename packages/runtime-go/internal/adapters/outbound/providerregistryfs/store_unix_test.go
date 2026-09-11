//go:build !windows

package providerregistryfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

func TestStoreUnixPrivateModesAndSymlinkFailuresPreserveTargets(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		setup func(*testing.T, *Store) string
	}{
		{name: "world readable Registry", setup: func(t *testing.T, store *Store) string {
			if err := os.Chmod(store.registryPath, 0o644); err != nil {
				t.Fatalf("Chmod() error = %v", err)
			}
			return store.registryPath
		}},
		{name: "Registry symlink", setup: func(t *testing.T, store *Store) string {
			target := store.registryPath + ".target"
			content, err := os.ReadFile(store.registryPath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if err := os.WriteFile(target, content, 0o600); err != nil {
				t.Fatalf("WriteFile(target) error = %v", err)
			}
			if err := os.Remove(store.registryPath); err != nil {
				t.Fatalf("Remove() error = %v", err)
			}
			if err := os.Symlink(target, store.registryPath); err != nil {
				t.Fatalf("Symlink() error = %v", err)
			}
			return target
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			ctx := context.Background()
			if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			}); err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			protected := testCase.setup(t, store)
			before, err := os.ReadFile(protected)
			if err != nil {
				t.Fatalf("ReadFile(protected) error = %v", err)
			}
			err = store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			})
			if !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("Load(unsafe) error = %v, want persistence failure", err)
			}
			after, err := os.ReadFile(protected)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("unsafe Registry state was overwritten")
			}
		})
	}
}

func TestStoreUnixOSLockBlocksIndependentFileDescription(t *testing.T) {
	t.Parallel()

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := ensureRegistryDirectory(store.dataDir, store.directory); err != nil {
		t.Fatalf("ensureRegistryDirectory() error = %v", err)
	}
	first, err := acquireRegistryLock(context.Background(), store.lockPath)
	if err != nil {
		t.Fatalf("acquireRegistryLock(first) error = %v", err)
	}
	defer first.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	second, err := acquireRegistryLock(ctx, store.lockPath)
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireRegistryLock(competing) error = %v, want deadline", err)
	}
}

func TestStoreUnixCommitJournalRecoversPriorOrFinalizesExactDecision(t *testing.T) {
	t.Parallel()

	for _, committed := range []bool{false, true} {
		name := "rollback unconfirmed"
		if committed {
			name = "finalize committed"
		}
		t.Run(name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			ctx := context.Background()
			if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			}); err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			prior, _ := os.ReadFile(store.registryPath)
			candidateRegistry, _ := domainregistry.Unmarshal(prior)
			candidateRegistry.Revision++
			candidate, _ := domainregistry.Marshal(candidateRegistry)
			journalPath, backupPath, markerPath := recoveryPaths(store.registryPath)
			if err := createExclusivePrivateFile(backupPath, prior); err != nil {
				t.Fatalf("create backup error = %v", err)
			}
			if err := createExclusivePrivateFile(journalPath, journalPriorPresent); err != nil {
				t.Fatalf("create journal error = %v", err)
			}
			if err := os.WriteFile(store.registryPath, candidate, 0o600); err != nil {
				t.Fatalf("write candidate error = %v", err)
			}
			if committed {
				if err := createExclusivePrivateFile(markerPath, commitMarker); err != nil {
					t.Fatalf("create marker error = %v", err)
				}
			}
			if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			}); err != nil {
				t.Fatalf("recovery Load() error = %v", err)
			}
			after, _ := os.ReadFile(store.registryPath)
			want := prior
			if committed {
				want = candidate
			}
			if !bytes.Equal(after, want) {
				t.Fatal("journal recovery selected the wrong durable decision")
			}
			for _, path := range []string{journalPath, backupPath, markerPath} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("recovery retained artifact %s", filepath.Base(path))
				}
			}
		})
	}
}

func TestStoreUnixCommittedFileAndDirectoriesAreRestrictive(t *testing.T) {
	t.Parallel()

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := store.WithExclusive(context.Background(), func(transaction registryport.Transaction) error {
		_, err := transaction.Load(context.Background())
		return err
	}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for path, want := range map[string]os.FileMode{
		store.directory: 0o700, store.registryPath: 0o600, store.lockPath: 0o600,
	} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode().Perm() != want || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s mode=%v error=%v want=%o", filepath.Base(path), info.Mode(), err, want)
		}
	}
}
