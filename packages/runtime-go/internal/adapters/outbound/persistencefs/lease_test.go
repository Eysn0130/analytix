package persistencefs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompositeLeaseBlocksSecondOwnerAndReleases(t *testing.T) {
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	leaseDir := filepath.Join(base, "leases")
	first, err := acquireCompositeLeaseAt(roots, leaseDir)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if _, err := acquireCompositeLeaseAt(roots, leaseDir); !errors.Is(err, ErrPersistenceInUse) {
		t.Fatalf("second lease should fail closed, got %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first lease: %v", err)
	}
	second, err := acquireCompositeLeaseAt(roots, leaseDir)
	if err != nil {
		t.Fatalf("reacquire released lease: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second lease: %v", err)
	}
}

func TestCompositeLeaseRejectsSameContentManagedRootSwap(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	for _, root := range []string{data, durable} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireCompositeLeaseAt(roots, filepath.Join(base, "leases"))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	held := data + "-held"
	if err := os.Rename(data, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, held := lease.FrozenRoots(); held {
		t.Fatal("same-content managed root replacement retained frozen lease authority")
	}
}

func TestCompositeLeaseRejectsMissingRootAppearanceAndAncestorSwap(t *testing.T) {
	outer := t.TempDir()
	ancestor := filepath.Join(outer, "ancestor")
	if err := os.Mkdir(ancestor, 0o700); err != nil {
		t.Fatal(err)
	}
	roots, err := ResolveRootSet(filepath.Join(ancestor, "data"), filepath.Join(ancestor, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireCompositeLeaseAt(roots, filepath.Join(outer, "leases"))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := os.Mkdir(roots.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, held := lease.FrozenRoots(); held {
		t.Fatal("root created outside the frozen cold-root capability retained lease authority")
	}

	secondAncestor := filepath.Join(outer, "second-ancestor")
	if err := os.Mkdir(secondAncestor, 0o700); err != nil {
		t.Fatal(err)
	}
	secondRoots, err := ResolveRootSet(filepath.Join(secondAncestor, "data"), filepath.Join(secondAncestor, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquireCompositeLeaseAt(secondRoots, filepath.Join(outer, "other-leases"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	heldAncestor := secondAncestor + "-held"
	if err := os.Rename(secondAncestor, heldAncestor); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secondAncestor, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, held := second.FrozenRoots(); held {
		t.Fatal("same-content nearest-ancestor replacement retained lease authority")
	}
}

func TestCompositeLeaseMatchesFilesystemCaseSensitivity(t *testing.T) {
	parent := t.TempDir()
	// t.TempDir's final component can be numeric; uppercasing it would probe
	// the same spelling and misclassify every filesystem as case-insensitive.
	base := filepath.Join(parent, "CaseProbe")
	aliasBase := filepath.Join(parent, "CASEPROBE")
	if err := os.Mkdir(base, 0o700); err != nil {
		t.Fatal(err)
	}
	baseInfo, baseErr := os.Stat(base)
	aliasInfo, aliasErr := os.Stat(aliasBase)
	if baseErr != nil || aliasErr != nil && !errors.Is(aliasErr, os.ErrNotExist) {
		t.Fatalf("case-sensitivity probe failed: base=%v alias=%v", baseErr, aliasErr)
	}
	caseInsensitive := aliasErr == nil && os.SameFile(baseInfo, aliasInfo)
	if !caseInsensitive {
		if err := os.Mkdir(aliasBase, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	firstRoots, err := ResolveRootSet(filepath.Join(base, "CaseData"), filepath.Join(base, "DurableA"))
	if err != nil {
		t.Fatal(err)
	}
	leaseDir := filepath.Join(base, "leases")
	first, err := acquireCompositeLeaseAt(firstRoots, leaseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	aliasRoots, err := ResolveRootSet(filepath.Join(aliasBase, "casedata"), filepath.Join(base, "DurableB"))
	if err != nil {
		t.Fatal(err)
	}
	second, secondErr := acquireCompositeLeaseAt(aliasRoots, leaseDir)
	if second != nil {
		defer second.Close()
	}
	_, overlapErr := ResolveRootSet(filepath.Join(base, "CaseData"), filepath.Join(aliasBase, "casedata", "nested"))
	if caseInsensitive {
		if !errors.Is(secondErr, ErrPersistenceInUse) {
			t.Fatalf("case alias acquired a second writer lease: %v", secondErr)
		}
		if overlapErr == nil {
			t.Fatal("case-aliased ancestor roots were not rejected")
		}
	} else {
		if secondErr != nil || second == nil {
			t.Fatalf("distinct case-sensitive roots could not acquire independent leases: %v", secondErr)
		}
		if overlapErr != nil {
			t.Fatalf("distinct case-sensitive roots were treated as overlapping: %v", overlapErr)
		}
	}
}

func TestCompositeLeaseNamespaceDoesNotDependOnProcessTempEnvironment(t *testing.T) {
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	defer first.Close()
	otherTemp := filepath.Join(base, "other-temp")
	if err := os.MkdirAll(otherTemp, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", otherTemp)
	t.Setenv("TMP", otherTemp)
	t.Setenv("TEMP", otherTemp)
	if _, err := AcquireCompositeLease(roots); !errors.Is(err, ErrPersistenceInUse) {
		t.Fatalf("temp environment changed the lease namespace: %v", err)
	}
}

func TestFrozenRootsRejectsReplacedLeasePath(t *testing.T) {
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireCompositeLeaseAt(roots, filepath.Join(base, "leases"))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if len(lease.paths) == 0 {
		t.Fatal("lease fixture has no lock paths")
	}
	if err := os.Remove(lease.paths[len(lease.paths)-1]); err != nil {
		t.Skipf("platform does not allow unlinking a locked file: %v", err)
	}
	if _, held := lease.FrozenRoots(); held {
		t.Fatal("replaced lease directory entry remained a valid frozen-root authority")
	}
}

func TestCompositeLeaseDirectoryScopeBlocksRecreatedLockNamespace(t *testing.T) {
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	leaseDir := filepath.Join(base, "leases")
	first, err := acquireCompositeLeaseAt(roots, leaseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	held := leaseDir + ".held"
	if err := os.Rename(leaseDir, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(leaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireCompositeLeaseAt(roots, leaseDir); !errors.Is(err, ErrPersistenceInUse) {
		t.Fatalf("recreated lock namespace bypassed the managed-directory inode lease: %v", err)
	}
}

func TestCompositeLeaseKernelAuthoritySurvivesPathReplacement(t *testing.T) {
	if os.Getenv("ANALYTIX_LEASE_REPLACEMENT_CHILD") == "1" {
		roots, err := ResolveRootSet(os.Getenv("ANALYTIX_LEASE_DATA_ROOT"), os.Getenv("ANALYTIX_LEASE_DURABLE_ROOT"))
		if err != nil {
			os.Exit(2)
		}
		lease, err := acquireCompositeLeaseAt(roots, os.Getenv("ANALYTIX_LEASE_DIRECTORY"))
		if errors.Is(err, ErrPersistenceInUse) {
			os.Exit(0)
		}
		if err == nil {
			_ = lease.Close()
		}
		os.Exit(3)
	}
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	for _, root := range []string{data, durable} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	leaseDir := filepath.Join(base, "leases")
	first, err := acquireCompositeLeaseAt(roots, leaseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	for _, root := range []string{data, durable} {
		if err := os.Rename(root, root+".held"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(leaseDir, leaseDir+".held"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(leaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=TestCompositeLeaseKernelAuthoritySurvivesPathReplacement")
	command.Env = append(os.Environ(),
		"ANALYTIX_LEASE_REPLACEMENT_CHILD=1",
		"ANALYTIX_LEASE_DATA_ROOT="+data,
		"ANALYTIX_LEASE_DURABLE_ROOT="+durable,
		"ANALYTIX_LEASE_DIRECTORY="+leaseDir,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("replacement acquired a second cross-process persistence lease: %v: %s", err, output)
	}
}

func TestCompositeLeaseReleasesPartialAcquisition(t *testing.T) {
	base := t.TempDir()
	rootA := filepath.Join(base, "a")
	rootB := filepath.Join(base, "b")
	for _, root := range []string{rootA, rootB} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	leaseDir := filepath.Join(base, "leases")
	held, err := acquireCompositeLeaseAt(RootSet{DataDir: rootB, DurableDir: rootB}, leaseDir)
	if err != nil {
		t.Fatalf("hold second root: %v", err)
	}
	defer held.Close()

	if _, err := acquireCompositeLeaseAt(RootSet{DataDir: rootA, DurableDir: rootB}, leaseDir); !errors.Is(err, ErrPersistenceInUse) {
		t.Fatalf("composite acquisition should fail on held root: %v", err)
	}
	probe, err := acquireCompositeLeaseAt(RootSet{DataDir: rootA, DurableDir: rootA}, leaseDir)
	if err != nil {
		t.Fatalf("partially acquired root was not released: %v", err)
	}
	_ = probe.Close()
}

func TestCanonicalRootsAreSortedAndDeduplicated(t *testing.T) {
	roots := CanonicalRoots(RootSet{DataDir: "/z/data", DurableDir: "/a/durable"})
	if len(roots) != 2 || roots[0] != "/a/durable" || roots[1] != "/z/data" {
		t.Fatalf("canonical lock order mismatch: %#v", roots)
	}
	roots = CanonicalRoots(RootSet{DataDir: "/same", DurableDir: "/same"})
	if len(roots) != 1 || roots[0] != "/same" {
		t.Fatalf("canonical root dedup mismatch: %#v", roots)
	}
}

func TestCompositeLeaseRejectsAncestorOverlapButAllowsSiblings(t *testing.T) {
	base := t.TempDir()
	leaseDir := filepath.Join(base, "leases")
	// Existing sibling roots can share their ancestor lock. Cold roots take a
	// conservative exclusive lock on their nearest existing ancestor until the
	// signed startup promotion has created a stable root inode.
	for _, root := range []string{
		filepath.Join(base, "data"), filepath.Join(base, "durable-a"),
		filepath.Join(base, "sibling"), filepath.Join(base, "durable-b"),
	} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ownerA, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable-a"))
	if err != nil {
		t.Fatal(err)
	}
	leaseA, err := acquireCompositeLeaseAt(ownerA, leaseDir)
	if err != nil {
		t.Fatalf("acquire owner A: %v", err)
	}
	defer leaseA.Close()
	overlapping, err := ResolveRootSet(filepath.Join(base, "data", "private"), filepath.Join(base, "durable-b"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireCompositeLeaseAt(overlapping, leaseDir); !errors.Is(err, ErrPersistenceInUse) {
		t.Fatalf("ancestor/descendant roots should conflict: %v", err)
	}
	sibling, err := ResolveRootSet(filepath.Join(base, "sibling"), filepath.Join(base, "durable-b"))
	if err != nil {
		t.Fatal(err)
	}
	leaseB, err := acquireCompositeLeaseAt(sibling, leaseDir)
	if err != nil {
		t.Fatalf("non-overlapping siblings should coexist: %v", err)
	}
	_ = leaseB.Close()
}

func TestSeparateOwnerRootStaysOutsideRootSetAndSharesOneLease(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	electron := filepath.Join(base, "electron-user-data")
	for _, root := range []string{data, durable, electron} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	owners, err := ResolveSeparateOwnerRoots(roots, electron)
	electronRoots, electronErr := ResolveRootSet(electron, electron)
	if err != nil || electronErr != nil || len(owners) != 1 || owners[0] != electronRoots.DataDir {
		t.Fatalf("resolve separate owner = %#v, %v", owners, err)
	}
	leaseDir := filepath.Join(base, "leases")
	lease, err := acquireCompositeLeaseAtPaths(
		roots, append(CanonicalRoots(roots), owners...), leaseDir,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if frozen, held := lease.FrozenRoots(); !held || frozen != roots {
		t.Fatalf("separate owner contaminated frozen RootSet: %#v held=%v", frozen, held)
	}
	if _, err := acquireCompositeLeaseAt(RootSet{DataDir: electron, DurableDir: electron}, leaseDir); !errors.Is(err, ErrPersistenceInUse) {
		t.Fatalf("separate owner escaped the composite lease: %v", err)
	}
}

func TestSeparateOwnerRootRejectsOverlapBeforeLeaseCreation(t *testing.T) {
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{
		roots.DataDir,
		filepath.Join(roots.DataDir, "electron"),
		filepath.Dir(roots.DataDir),
	} {
		if _, err := ResolveSeparateOwnerRoots(roots, owner); err == nil {
			t.Fatalf("overlapping separate owner was accepted: %s", owner)
		}
	}
}

func TestSeparateOwnerRootRejectsCallerControlledSymlinkWitness(t *testing.T) {
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	realOwner := filepath.Join(base, "real-owner")
	if err := os.Mkdir(realOwner, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedOwner := filepath.Join(base, "linked-owner")
	if err := os.Symlink(realOwner, linkedOwner); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, owner := range []string{linkedOwner, filepath.Join(linkedOwner, "missing-child")} {
		if _, err := ResolveSeparateOwnerRoots(roots, owner); err == nil {
			t.Fatalf("separate-owner symlink witness was accepted: %s", owner)
		}
	}
}
