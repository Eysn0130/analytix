//go:build darwin || linux

package rawartifact_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	rawartifactadapter "analytix.local/runtime-go/internal/adapters/outbound/rawartifact"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

func TestLargeOpaqueRecoveryRejectsAbsentRootCreateResidueWithoutDeleting(t *testing.T) {
	_, root, lease := openLargeOpaqueChunkStore(t)
	residue := filepath.Join(filepath.Dir(root), largeOpaqueCreateResidueName(filepath.Base(root)))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	prepared, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
		context.Background(), lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	report, guard, err := prepared.Apply(context.Background())
	if err == nil || guard != nil || report != (finalauthority.LargeOpaqueRecoveryReportV1{}) {
		t.Fatalf("residue recovery report=%+v guard=%v err=%v", report, guard != nil, err)
	}
	if _, err := os.Lstat(residue); err != nil {
		t.Fatalf("fail-closed recovery deleted the prepared root residue: %v", err)
	}
}

func TestLargeOpaqueRecoveryRejectsShardResidueAndObjectTempWithoutDeleting(t *testing.T) {
	store, root, lease := openLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("recovery-committed-evidence"), 4096)
	descriptor := buildLargeOpaqueDescriptor(t, "recovery-committed", body)
	seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
	shard := filepath.Dir(largeOpaqueChunkPath(root, descriptor))
	temp := filepath.Join(shard, largeOpaqueObjectTempName(descriptor.DescriptorDigest, "a"))
	if err := os.WriteFile(temp, []byte("uncommitted"), 0o600); err != nil {
		t.Fatal(err)
	}
	residueShard := "fe"
	if filepath.Base(shard) == residueShard {
		residueShard = "fd"
	}
	shardResidue := filepath.Join(root, largeOpaqueCreateResidueName(residueShard))
	if err := os.Mkdir(shardResidue, 0o700); err != nil {
		t.Fatal(err)
	}

	prepared, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
		context.Background(), root, domainevidence.RawArtifactContentChunkBytesV1,
		testLargeOpaqueRecoveryLimitsV1(), lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	report, guard, err := prepared.Apply(context.Background())
	if err == nil || guard != nil || report != (finalauthority.LargeOpaqueRecoveryReportV1{}) {
		t.Fatalf("present-root residue report=%+v guard=%v err=%v", report, guard != nil, err)
	}
	for _, preserved := range []string{temp, shardResidue} {
		if _, err := os.Lstat(preserved); err != nil {
			t.Fatalf("fail-closed recovery deleted %s: %v", preserved, err)
		}
	}
	assertLargeOpaqueRead(t, store, descriptor, body)
}

func TestLargeOpaqueRecoveryIssuesGuardOnlyForAlreadyCleanTopology(t *testing.T) {
	t.Run("absent root", func(t *testing.T) {
		_, root, lease := openLargeOpaqueChunkStore(t)
		prepared, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
			context.Background(), lease,
		)
		if err != nil {
			t.Fatal(err)
		}
		report, guard, err := prepared.Apply(context.Background())
		if err != nil || guard == nil || report.Present || report.ObjectCount != 0 ||
			report.RemovedTempCount != 0 || report.RemovedCreateResidueCount != 0 {
			t.Fatalf("clean absent-root report=%+v guard=%v err=%v", report, guard != nil, err)
		}
		if err := guard.Revalidate(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := guard.Revalidate(context.Background()); err == nil {
			t.Fatal("clean guard accepted a root created after issuance")
		}
	})

	t.Run("present root", func(t *testing.T) {
		store, root, lease := openLargeOpaqueChunkStore(t)
		body := []byte("clean-recovery-evidence")
		descriptor := buildLargeOpaqueDescriptor(t, "clean-recovery", body)
		seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
		prepared, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
			context.Background(), lease,
		)
		if err != nil {
			t.Fatal(err)
		}
		report, guard, err := prepared.Apply(context.Background())
		if err != nil || guard == nil || !report.Present || report.ObjectCount != 1 ||
			report.RemovedTempCount != 0 || report.RemovedCreateResidueCount != 0 {
			t.Fatalf("clean present-root report=%+v guard=%v err=%v", report, guard != nil, err)
		}
		if err := guard.Revalidate(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestLargeOpaqueChunkOwnerAuthorityCannotSelectDurableDirLookalike(t *testing.T) {
	store, dataRoot, lease := openLargeOpaqueChunkStore(t)
	frozen, ok := lease.FrozenRoots()
	if !ok {
		t.Fatal("persistence lease did not expose frozen roots")
	}
	durableLookalike := filepath.Join(frozen.DurableDir, rawartifactadapter.LargeOpaqueChunkStoreRootNameV1)
	if err := os.MkdirAll(durableLookalike, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(durableLookalike, "must-not-be-owned")
	if err := os.WriteFile(sentinel, []byte("durable-owner-confusion-sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := []byte("data-dir-owner-evidence")
	descriptor := buildLargeOpaqueDescriptor(t, "data-dir-owner", body)
	seedLargeOpaqueCommittedForRecovery(t, store, dataRoot, descriptor, body)
	assertLargeOpaqueRead(t, store, descriptor, body)
	if _, err := os.Lstat(largeOpaqueChunkPath(dataRoot, descriptor)); err != nil {
		t.Fatalf("owner store did not resolve its exact object under frozen DataDir: %v", err)
	}
	if _, err := os.Lstat(largeOpaqueChunkPath(durableLookalike, descriptor)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner store wrote under DurableDir lookalike: %v", err)
	}
	prepared, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
		context.Background(), lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, guard, err := prepared.Apply(context.Background()); err != nil || guard == nil {
		t.Fatalf("DataDir recovery was redirected by DurableDir lookalike: guard=%v err=%v", guard != nil, err)
	}
	if _, err := os.Lstat(sentinel); err != nil {
		t.Fatalf("owner-scoped recovery touched DurableDir lookalike: %v", err)
	}
}

func TestLargeOpaqueRecoveryObservesColdDataDirWithoutPromotion(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir:    filepath.Join(base, "missing", "cold-data"),
		DurableDir: filepath.Join(base, "cold-durable"),
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
	prepared, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
		context.Background(), lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	report, guard, err := prepared.Apply(context.Background())
	if err != nil || guard == nil || report.Present || report.ObjectCount != 0 {
		t.Fatalf("cold DataDir recovery report=%+v guard=%v err=%v", report, guard != nil, err)
	}
	if _, err := os.Lstat(roots.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery promoted cold DataDir: %v", err)
	}
	if err := os.MkdirAll(roots.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := guard.Revalidate(context.Background()); err == nil {
		t.Fatal("cold-root guard survived semantic root promotion")
	}
}

func TestLargeOpaqueRecoveryPreflightRejectsUnsafeTopologyBeforeAnyDeletion(t *testing.T) {
	store, root, lease := openLargeOpaqueChunkStore(t)
	body := []byte("preflight-evidence")
	descriptor := buildLargeOpaqueDescriptor(t, "preflight-zero-delete", body)
	seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
	shard := filepath.Dir(largeOpaqueChunkPath(root, descriptor))
	temp := filepath.Join(shard, largeOpaqueObjectTempName(descriptor.DescriptorDigest, "c"))
	if err := os.WriteFile(temp, []byte("must-survive-failed-preflight"), 0o600); err != nil {
		t.Fatal(err)
	}
	unknownShard := filepath.Join(root, "zz")
	if err := os.Mkdir(unknownShard, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
		context.Background(), root, domainevidence.RawArtifactContentChunkBytesV1,
		testLargeOpaqueRecoveryLimitsV1(), lease,
	); err == nil {
		t.Fatal("unknown later root entry passed complete preflight")
	}
	if _, err := os.Lstat(temp); err != nil {
		t.Fatalf("failed preflight deleted an earlier valid temp: %v", err)
	}
}

func TestLargeOpaqueRecoveryRejectsCreateResidueConflictsAndNonEmptyResidues(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(string) (string, error)
	}{
		{name: "final root and root residue coexist", setup: func(root string) (string, error) {
			if err := os.Mkdir(root, 0o700); err != nil {
				return "", err
			}
			residue := filepath.Join(filepath.Dir(root), largeOpaqueCreateResidueName(filepath.Base(root)))
			return residue, os.Mkdir(residue, 0o700)
		}},
		{name: "non-empty root residue", setup: func(root string) (string, error) {
			residue := filepath.Join(filepath.Dir(root), largeOpaqueCreateResidueName(filepath.Base(root)))
			if err := os.Mkdir(residue, 0o700); err != nil {
				return "", err
			}
			return residue, os.WriteFile(filepath.Join(residue, "payload"), []byte("not empty"), 0o600)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, root, lease := openLargeOpaqueChunkStore(t)
			residue, err := test.setup(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
				context.Background(), root, domainevidence.RawArtifactContentChunkBytesV1,
				testLargeOpaqueRecoveryLimitsV1(), lease,
			); err == nil {
				t.Fatal("unsafe create residue passed recovery preflight")
			}
			if _, err := os.Lstat(residue); err != nil {
				t.Fatalf("failed preflight deleted the create residue: %v", err)
			}
		})
	}
}

func TestLargeOpaqueRecoveryApplyRejectsPostPrepareDriftWithoutDeletingTemps(t *testing.T) {
	store, root, lease := openLargeOpaqueChunkStore(t)
	body := []byte("apply-drift-evidence")
	descriptor := buildLargeOpaqueDescriptor(t, "apply-drift", body)
	seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
	shard := filepath.Dir(largeOpaqueChunkPath(root, descriptor))
	temp := filepath.Join(shard, largeOpaqueObjectTempName(descriptor.DescriptorDigest, "d"))
	if err := os.WriteFile(temp, []byte("prepared-temp"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
		context.Background(), root, domainevidence.RawArtifactContentChunkBytesV1,
		testLargeOpaqueRecoveryLimitsV1(), lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shard, "unknown.after-prepare"), []byte("drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, guard, err := prepared.Apply(context.Background()); err == nil || guard != nil {
		t.Fatalf("drifted recovery apply guard=%v err=%v", guard != nil, err)
	}
	if _, err := os.Lstat(temp); err != nil {
		t.Fatalf("failed apply deleted a prepared temp: %v", err)
	}
}

func TestLargeOpaqueRecoveryApplyRejectsCommittedMetadataTimeDrift(t *testing.T) {
	store, root, lease := openLargeOpaqueChunkStore(t)
	body := []byte("recovery-time-bound-evidence")
	descriptor := buildLargeOpaqueDescriptor(t, "recovery-time-drift", body)
	seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
	committed := largeOpaqueChunkPath(root, descriptor)
	shard := filepath.Dir(committed)
	temp := filepath.Join(shard, largeOpaqueObjectTempName(descriptor.DescriptorDigest, "f"))
	if err := os.WriteFile(temp, []byte("must-survive-time-drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(committed)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
		context.Background(), root, domainevidence.RawArtifactContentChunkBytesV1,
		testLargeOpaqueRecoveryLimitsV1(), lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(committed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(committed, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(committed, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, guard, err := prepared.Apply(context.Background()); err == nil || guard != nil {
		t.Fatalf("time-drifted recovery apply guard=%v err=%v", guard != nil, err)
	}
	if _, err := os.Lstat(temp); err != nil {
		t.Fatalf("time-drifted apply deleted a prepared temp: %v", err)
	}
}

func TestLargeOpaqueRecoverySeparatesCommittedAndTemporaryBounds(t *testing.T) {
	store, root, lease := openLargeOpaqueChunkStore(t)
	body := []byte("combined-bound-evidence")
	descriptor := buildLargeOpaqueDescriptor(t, "combined-bound", body)
	seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
	shard := filepath.Dir(largeOpaqueChunkPath(root, descriptor))
	if err := os.WriteFile(
		filepath.Join(shard, largeOpaqueObjectTempName(descriptor.DescriptorDigest, "e")),
		[]byte("temp"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	prepared, err := finalauthority.PrepareLargeOpaqueStoreRecoveryV1(
		context.Background(), root, domainevidence.RawArtifactContentChunkBytesV1,
		finalauthority.LargeOpaqueRecoveryLimitsV1{
			MaxCommittedObjects: 1,
			MaxTemporaryObjects: 1,
			MaxCreateResidues:   1,
		},
		lease,
	)
	if err != nil {
		t.Fatalf("separate committed and temporary bounds rejected legal topology: %v", err)
	}
	if _, guard, err := prepared.Apply(context.Background()); err == nil || guard != nil {
		t.Fatalf("temporary residue did not fail closed: guard=%v err=%v", guard != nil, err)
	}
}

func testLargeOpaqueRecoveryLimitsV1() finalauthority.LargeOpaqueRecoveryLimitsV1 {
	return finalauthority.LargeOpaqueRecoveryLimitsV1{
		MaxCommittedObjects: 1024,
		MaxTemporaryObjects: 1024,
		MaxCreateResidues:   257,
	}
}

func largeOpaqueCreateResidueName(component string) string {
	digest := sha256.Sum256([]byte(component))
	return ".analytix-cas-create-" + hex.EncodeToString(digest[:8]) + ".tmp"
}

func largeOpaqueObjectTempName(address string, suffix string) string {
	return "." + address + "-" + strings.Repeat(suffix, 24) + ".tmp"
}
