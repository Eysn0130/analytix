//go:build darwin || linux

package rawartifact_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	rawartifactadapter "analytix.local/runtime-go/internal/adapters/outbound/rawartifact"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	"analytix.local/runtime-go/internal/testsupport/rawartifactfixture"
)

func openLargeOpaqueChunkStore(
	t *testing.T,
) (*rawartifactadapter.LargeOpaqueChunkStore, string, *persistencefs.CompositeLease) {
	t.Helper()
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir:    filepath.Join(base, "data"),
		DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	frozen, ok := lease.FrozenRoots()
	if !ok {
		t.Fatal("persistence lease did not freeze roots")
	}
	root := filepath.Join(frozen.DataDir, rawartifactadapter.LargeOpaqueChunkStoreRootNameV1)
	return newLargeOpaqueChunkStoreOnLease(t, lease), root, lease
}

func newLargeOpaqueChunkStoreOnLease(
	t *testing.T,
	lease *persistencefs.CompositeLease,
) *rawartifactadapter.LargeOpaqueChunkStore {
	t.Helper()
	store, err := rawartifactadapter.NewLargeOpaqueChunkStore(lease)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func openProvisionedLargeOpaqueChunkStore(
	t *testing.T,
) (*rawartifactadapter.LargeOpaqueChunkStore, string, *persistencefs.CompositeLease) {
	t.Helper()
	store, root, lease := openLargeOpaqueChunkStore(t)
	provisionLargeOpaqueStoreForTest(t, root)
	return store, root, lease
}

func provisionLargeOpaqueStoreForTest(t *testing.T, root string) {
	t.Helper()
	for index := 0; index < 256; index++ {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("%02x", index)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func provisionLargeOpaqueShardForTest(
	t *testing.T,
	root string,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, descriptor.DescriptorDigest[:2]), 0o700); err != nil {
		t.Fatal(err)
	}
}

func buildLargeOpaqueDescriptor(
	t *testing.T,
	seed string,
	body []byte,
) domainevidence.RawArtifactContentChunkDescriptorV1 {
	t.Helper()
	descriptor, err := rawartifactfixture.BuildChunkDescriptorV1(seed, body)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func seedLargeOpaqueCommittedForRecovery(
	t *testing.T,
	store *rawartifactadapter.LargeOpaqueChunkStore,
	root string,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
	body []byte,
) {
	t.Helper()
	if runtime.GOOS == "linux" {
		provisionLargeOpaqueShardForTest(t, root, descriptor)
		if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
			t.Fatal(err)
		}
		return
	}
	provisionLargeOpaqueShardForTest(t, root, descriptor)
	path := largeOpaqueChunkPath(root, descriptor)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertLargeOpaqueRead(
	t *testing.T,
	store *rawartifactadapter.LargeOpaqueChunkStore,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
	expected []byte,
) {
	t.Helper()
	var output bytes.Buffer
	if err := store.ReadExact(context.Background(), descriptor, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), expected) {
		t.Fatal("large opaque exact read mismatched")
	}
}

func largeOpaqueChunkPath(root string, descriptor domainevidence.RawArtifactContentChunkDescriptorV1) string {
	return filepath.Join(root, descriptor.DescriptorDigest[:2], descriptor.DescriptorDigest+".blob")
}
