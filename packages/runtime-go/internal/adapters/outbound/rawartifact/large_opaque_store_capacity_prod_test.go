//go:build analytix_prod && linux

package rawartifact_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	rawartifactadapter "analytix.local/runtime-go/internal/adapters/outbound/rawartifact"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
)

func TestLargeOpaqueChunkStoreExceedsLegacyMaterializationCapacityAcrossRestart(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	firstLease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	frozen, ok := firstLease.FrozenRoots()
	if !ok {
		_ = firstLease.Close()
		t.Fatal("persistence lease did not freeze roots")
	}
	root := filepath.Join(frozen.DataDir, "raw-artifact-chunks-v1")
	provisionLargeOpaqueStoreForTest(t, root)
	store, err := rawartifactadapter.NewLargeOpaqueChunkStore(firstLease)
	if err != nil {
		_ = firstLease.Close()
		t.Fatal(err)
	}

	// Seventeen full chunks total 136 MiB, exceeding SecurePrivateCAS's legacy
	// 128 MiB aggregate materialization ceiling while retaining one 8 MiB
	// caller buffer and the store's fixed 64 KiB streaming buffer.
	body := make([]byte, domainevidence.RawArtifactContentChunkBytesV1)
	descriptors := make([]domainevidence.RawArtifactContentChunkDescriptorV1, 17)
	for index := range descriptors {
		body[0] = byte(index + 1)
		descriptors[index] = buildLargeOpaqueDescriptor(t, "capacity-"+string(rune('a'+index)), body)
		outcome, err := store.PutExact(context.Background(), descriptors[index], bytes.NewReader(body))
		if err != nil {
			_ = firstLease.Close()
			t.Fatalf("chunk %d: %v", index, err)
		}
		if outcome.Disposition != rawartifactport.LargeOpaqueChunkCreated {
			_ = firstLease.Close()
			t.Fatalf("chunk %d disposition = %q", index, outcome.Disposition)
		}
	}
	if err := firstLease.Close(); err != nil {
		t.Fatal(err)
	}

	secondLease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := secondLease.Close(); err != nil {
			t.Error(err)
		}
	})
	reopened, err := rawartifactadapter.NewLargeOpaqueChunkStore(secondLease)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
		context.Background(),
		root,
		domainevidence.RawArtifactContentChunkBytesV1,
		testLargeOpaqueRecoveryLimitsV1(),
		secondLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	report, guard, err := prepared.Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Present || report.ObjectCount != uint64(len(descriptors)) ||
		report.RemovedTempCount != 0 || report.RemovedCreateResidueCount != 0 || guard == nil {
		t.Fatalf("large recovery report=%+v guard=%v", report, guard != nil)
	}
	if err := guard.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for index, descriptor := range descriptors {
		if err := reopened.ReadExact(context.Background(), descriptor, io.Discard); err != nil {
			t.Fatalf("restart read chunk %d: %v", index, err)
		}
	}
}
