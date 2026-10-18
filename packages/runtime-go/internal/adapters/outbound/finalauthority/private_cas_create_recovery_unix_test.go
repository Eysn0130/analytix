//go:build darwin || linux

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"golang.org/x/sys/unix"
)

func createRecoveryNameInventoryFixture(t *testing.T) (string, *privateCASCreateRecoveryNameInventory) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "private", "owner", "leaf"), 0o700); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Close(fd); err != nil {
			t.Error(err)
		}
	})
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		t.Fatal(err)
	}
	return root, &privateCASCreateRecoveryNameInventory{
		ctx: context.Background(), dataDir: fd, device: uint64(stat.Dev),
		parents: make(map[string]privateCASCreateRecoveryParentNames),
	}
}

func TestCreateRecoveryNameInventoryReusesOnlyStableObservation(t *testing.T) {
	_, inventory := createRecoveryNameInventoryFixture(t)
	for range 4 {
		fd, present, err := inventory.openParent("private/owner/leaf")
		if err != nil || !present {
			t.Fatal("stable parent failed to open", err)
		}
		if err := unix.Close(fd); err != nil {
			t.Fatal(err)
		}
	}
	if len(inventory.parents) != 3 {
		t.Fatal("shared ancestors did not use one bounded inventory each")
	}
	if err := inventory.revalidate(); err != nil {
		t.Fatal(err)
	}

}

func TestCreateRecoveryNameInventoryReadOnlyRootAndAncestorReopenStayStable(t *testing.T) {
	root, inventory := createRecoveryNameInventoryFixture(t)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal("resolve isolated inventory root")
	}
	fd, present, openErr := inventory.openParent("private/owner/leaf")
	var closeErr error
	if fd >= 0 {
		closeErr = unix.Close(fd)
	}
	observed, cached := inventory.parents["."]
	t.Logf("phase=inventory_root_stable stage=cache open_ok=%t present=%t close_ok=%t root_cached=%t", openErr == nil, present, closeErr == nil, cached)
	if openErr != nil || !present || closeErr != nil || !cached {
		t.Fatal("cache isolated root and ancestors")
	}
	for _, stage := range []string{"root_reopen", "ancestor_reopen", "revalidate"} {
		var stageErr error
		switch stage {
		case "root_reopen":
			fd, stageErr = securePrivateOpenAbsoluteDirectory(root, false)
			if fd >= 0 {
				stageErr = errors.Join(stageErr, unix.Close(fd))
			}
		case "ancestor_reopen":
			fd, present, stageErr = privateCASUnixOpenCreateRecoveryParent(inventory.dataDir, "private/owner/leaf", inventory.device)
			if fd >= 0 {
				stageErr = errors.Join(stageErr, unix.Close(fd))
			}
			if !present {
				stageErr = errors.Join(stageErr, errors.New("isolated ancestor is absent"))
			}
		case "revalidate":
			stageErr = inventory.revalidate()
		}
		before, beforeErr := inventory.identity(inventory.dataDir, ".")
		names, namesErr := inventory.readNames(inventory.dataDir)
		after, afterErr := inventory.identity(inventory.dataDir, ".")
		beforeEqual := beforeErr == nil && before == observed.identity
		namesEqual := namesErr == nil && slices.Equal(names, observed.names)
		afterEqual := afterErr == nil && after == observed.identity
		t.Logf("phase=inventory_root_stable stage=%s action_ok=%t before_ok=%t names_ok=%t after_ok=%t before_equal=%t names_equal=%t after_equal=%t", stage, stageErr == nil, beforeErr == nil, namesErr == nil, afterErr == nil, beforeEqual, namesEqual, afterEqual)
		if stageErr != nil || !beforeEqual || !namesEqual || !afterEqual {
			t.Fatal("read-only stage changed the isolated root observation")
		}
	}
}

func TestCreateRecoveryNameInventoryRejectsTransientRootEntryWithUnchangedNames(t *testing.T) {
	root, inventory := createRecoveryNameInventoryFixture(t)
	// Control mtime before caching, using the existing Chtimes test mechanism.
	// A whole-second value in the past avoids depending on one timestamp tick;
	// the create/remove sequence must itself change it, without sleep or repair.
	baseline := time.Unix(1_600_000_000, 0)
	if err := os.Chtimes(root, baseline, baseline); err != nil {
		t.Fatal("set isolated root baseline mtime")
	}
	info, statErr := os.Stat(root)
	controlled := statErr == nil && info.ModTime().Equal(baseline)
	t.Logf("phase=inventory_root_transient stage=baseline stat_ok=%t mtime_controlled=%t", statErr == nil, controlled)
	if !controlled {
		t.Fatal("filesystem did not retain the controlled whole-second mtime")
	}
	_, cacheErr := inventory.names(inventory.dataDir, ".")
	observed, cached := inventory.parents["."]
	t.Logf("phase=inventory_root_transient stage=cache read_ok=%t root_cached=%t", cacheErr == nil, cached)
	if cacheErr != nil || !cached {
		t.Fatal("cache isolated root names")
	}
	temporary, createErr := os.CreateTemp(root, "inventory-transient-")
	t.Logf("phase=inventory_root_transient stage=create create_ok=%t", createErr == nil)
	if createErr != nil {
		t.Fatal("create isolated transient entry")
	}
	closeErr := temporary.Close()
	removeErr := os.Remove(temporary.Name())
	t.Logf("phase=inventory_root_transient stage=remove close_ok=%t remove_ok=%t", closeErr == nil, removeErr == nil)
	if closeErr != nil || removeErr != nil {
		t.Fatal("close and remove isolated transient entry")
	}
	current, identityErr := inventory.identity(inventory.dataDir, ".")
	names, namesErr := inventory.readNames(inventory.dataDir)
	namesEqual := namesErr == nil && slices.Equal(names, observed.names)
	stableFieldsEqual := identityErr == nil && current.directory == observed.identity.directory && current.links == observed.identity.links && current.size == observed.identity.size
	mtimeChanged := identityErr == nil && [16]byte(current.times[:16]) != [16]byte(observed.identity.times[:16])
	ctimeChanged := identityErr == nil && [16]byte(current.times[16:]) != [16]byte(observed.identity.times[16:])
	t.Logf("phase=inventory_root_transient stage=compare identity_ok=%t names_ok=%t names_equal=%t stable_fields_equal=%t mtime_changed=%t ctime_changed=%t", identityErr == nil, namesErr == nil, namesEqual, stableFieldsEqual, mtimeChanged, ctimeChanged)
	if !namesEqual || !stableFieldsEqual || !mtimeChanged {
		t.Fatal("transient entry did not preserve names and stable fields while changing mtime")
	}
	err := inventory.revalidate()
	rejected := err != nil && strings.Contains(err.Error(), "private CAS create-residue directory inventory changed")
	t.Logf("phase=inventory_root_transient stage=revalidate rejected=%t", rejected)
	if !rejected {
		t.Fatal("unchanged names bypassed the root timestamp revalidation")
	}
}

func TestCreateRecoveryNameInventoryRejectsMutationAfterReuse(t *testing.T) {
	for _, fault := range []string{"parent-replaced", "fifo", "symlink", "case-alias", "missing-appeared", "mode", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			root, inventory := createRecoveryNameInventoryFixture(t)
			target := "private/owner/leaf"
			if fault == "missing-appeared" {
				target = "private/owner/absent"
			}
			fd, present, err := inventory.openParent(target)
			if err != nil || present != (fault != "missing-appeared") {
				t.Fatal("initial observation failed", err)
			}
			if fd >= 0 {
				if err := unix.Close(fd); err != nil {
					t.Fatal(err)
				}
			}
			owner := filepath.Join(root, "private", "owner")
			leaf := filepath.Join(owner, "leaf")
			switch fault {
			case "parent-replaced":
				if err := os.Rename(owner, owner+"-retained"); err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll(leaf, 0o700)
			case "fifo", "symlink", "case-alias":
				if err := os.Rename(leaf, leaf+"-retained"); err != nil {
					t.Fatal(err)
				}
				if fault == "fifo" {
					err = unix.Mkfifo(leaf, 0o600)
				}
				if fault == "symlink" {
					err = os.Symlink(leaf+"-retained", leaf)
				}
				if fault == "case-alias" {
					err = os.Mkdir(filepath.Join(owner, "LEAF"), 0o700)
				}
			case "missing-appeared":
				err = os.Mkdir(filepath.Join(owner, "absent"), 0o700)
			case "mode":
				err = os.Chmod(owner, 0o755)
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				inventory.ctx = ctx
			}
			if err != nil {
				t.Fatal(err)
			}
			fd, _, err = inventory.openParent(target)
			if fd >= 0 {
				_ = unix.Close(fd)
			}
			if err == nil {
				t.Fatal("changed parent reused its cached names")
			}
			if err := inventory.revalidate(); err == nil {
				t.Fatal("final independent observation accepted changed state")
			}
		})
	}
}

func TestCreateRecoveryNameInventoryNeverCachesChildSafety(t *testing.T) {
	root, inventory := createRecoveryNameInventoryFixture(t)
	fd, present, err := inventory.openParent("private/owner/leaf")
	if err != nil || !present {
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	// Changing the child's mode leaves the cached parent name inventory intact.
	// The next fresh child open must nevertheless reject its changed authority.
	if err := os.Chmod(filepath.Join(root, "private", "owner", "leaf"), 0o755); err != nil {
		t.Fatal(err)
	}
	fd, _, err = inventory.openParent("private/owner/leaf")
	if fd >= 0 {
		_ = unix.Close(fd)
	}
	if err == nil {
		t.Fatal("cached parent names bypassed fresh child safety validation")
	}
}

func TestCreateRecoveryNameInventoryReopensAncestorsAtFinalBarrier(t *testing.T) {
	root, inventory := createRecoveryNameInventoryFixture(t)
	fd, present, err := inventory.openParent("private/owner/leaf")
	if err != nil || !present {
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	// The old directories remain valid openable objects, but their path no
	// longer belongs to the observed tree. A retained FD alone is insufficient.
	if err := os.Rename(filepath.Join(root, "private"), filepath.Join(root, "old-private")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "private", "owner", "leaf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := inventory.revalidate(); err == nil {
		t.Fatal("moved original ancestor satisfied final path revalidation")
	}
}

func TestPrivateCASCreateResidueRecoveryDeletesMissingPrivateResidueWithoutCreatingPrivate(t *testing.T) {
	dataDir := t.TempDir()
	residue := filepath.Join(dataDir, domainprivatecas.CreateDirectoryResidueNameV1("private"))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.ResidueCount() != 1 {
		t.Fatalf("prepared create residues = %d", prepared.ResidueCount())
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-private create residue survived recovery: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "private")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("create-residue recovery promoted the missing private root: %v", err)
	}
}

func TestPrivateCASCreateResidueRecoveryUsesTopologyAndDeletesEmptyShard(t *testing.T) {
	dataDir := t.TempDir()
	privateRoot := filepath.Join(dataDir, "private")
	caseThreadRoot := filepath.Join(privateRoot, "case-thread-authority")
	reportRoot := filepath.Join(privateRoot, "report-publication")
	unrelatedRoot := filepath.Join(privateRoot, "authority")
	for _, directory := range []string{caseThreadRoot, reportRoot, unrelatedRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(unrelatedRoot, "sentinel")
	if err := os.WriteFile(unrelated, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixedResidue := filepath.Join(
		reportRoot,
		domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
	)
	emptyShard := filepath.Join(caseThreadRoot, "ab")
	for _, directory := range []string{fixedResidue, emptyShard} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	authority, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.ResidueCount() != 1 {
		t.Fatalf("prepared create residues = %d", prepared.ResidueCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(fixedResidue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("topology-bound create residue survived recovery: %v", err)
	}
	if _, err := os.Lstat(emptyShard); err != nil {
		t.Fatalf("pre-journal create-residue recovery deleted canonical final topology: %v", err)
	}
	orphaned, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if orphaned.CandidateCount() != 2 {
		t.Fatalf("prepared orphan topology candidates = %d", orphaned.CandidateCount())
	}
	if err := orphaned.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{emptyShard, reportRoot} {
		if _, err := os.Lstat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("topology-bound empty directory survived recovery: path=%s err=%v", removed, err)
		}
	}
	for _, retained := range []string{privateRoot, caseThreadRoot, unrelatedRoot, unrelated} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("create-residue recovery touched unrelated or canonical state: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASCreateResidueRecoveryGlobalPreflightDeletesNothingOnUnsafeLaterCandidate(t *testing.T) {
	dataDir := t.TempDir()
	privateRoot := filepath.Join(dataDir, "private")
	acceptedFinals := filepath.Join(privateRoot, "accepted-finals")
	reportRoot := filepath.Join(privateRoot, "report-publication")
	for _, directory := range []string{acceptedFinals, reportRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	early := filepath.Join(
		acceptedFinals,
		domainprivatecas.CreateDirectoryResidueNameV1("records"),
	)
	late := filepath.Join(
		reportRoot,
		domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
	)
	for _, directory := range []string{early, late} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	authority, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(late, "unsafe"), []byte("unsafe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("unsafe later create residue passed the global revalidation barrier")
	}
	for _, retained := range []string{early, late, filepath.Join(late, "unsafe")} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("failed global preflight partially deleted state: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASCreateResidueRecoveryRejectsReplacementAndAliases(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "same-name replacement",
			mutate: func(t *testing.T, residue string) {
				t.Helper()
				moved := filepath.Join(t.TempDir(), "original-residue")
				if err := os.Rename(residue, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(residue, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "case alias",
			mutate: func(t *testing.T, residue string) {
				t.Helper()
				alias := filepath.Join(filepath.Dir(residue), "Private")
				if err := os.Mkdir(alias, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			residue := filepath.Join(dataDir, domainprivatecas.CreateDirectoryResidueNameV1("private"))
			if err := os.Mkdir(residue, 0o700); err != nil {
				t.Fatal(err)
			}
			authority, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
				context.Background(),
				dataDir,
				authority,
			)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, residue)
			if err := prepared.Revalidate(context.Background()); err == nil {
				t.Fatal("create-residue replacement or alias survived revalidation")
			}
			if _, err := os.Lstat(residue); err != nil {
				t.Fatalf("failed revalidation mutated the residue: %v", err)
			}
		})
	}
}

func TestPrivateCASCreateResidueRecoveryRejectsDeletionWindowReplacement(t *testing.T) {
	dataDir := t.TempDir()
	residue := filepath.Join(dataDir, domainprivatecas.CreateDirectoryResidueNameV1("private"))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "original-residue")
	privateCASCreateResidueRecoveryTestHooks.Lock()
	privateCASCreateResidueRecoveryTestHooks.hook = func(phase string, index int) error {
		if phase != "after_final_validation_before_delete" || index != 0 {
			return nil
		}
		if err := os.Rename(residue, moved); err != nil {
			return err
		}
		return os.Mkdir(residue, 0o700)
	}
	privateCASCreateResidueRecoveryTestHooks.Unlock()
	t.Cleanup(func() {
		privateCASCreateResidueRecoveryTestHooks.Lock()
		privateCASCreateResidueRecoveryTestHooks.hook = nil
		privateCASCreateResidueRecoveryTestHooks.Unlock()
	})
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("same-name replacement in the final deletion window was deleted")
	}
	for _, retained := range []string{residue, moved} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("deletion-window rejection lost a directory: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASOrphanTopologyRecoveryRollsBackOnlyRecursiveEmptyPartialOwner(t *testing.T) {
	dataDir := t.TempDir()
	privateRoot := filepath.Join(dataDir, "private")
	owner := filepath.Join(privateRoot, "accepted-finals")
	leaf := filepath.Join(owner, "records")
	emptyShard := filepath.Join(leaf, "ab")
	if err := os.MkdirAll(emptyShard, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.CandidateCount() != 3 {
		t.Fatalf("partial owner recovery candidates = %d", prepared.CandidateCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{emptyShard, leaf, owner} {
		if _, err := os.Lstat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("recursive-empty partial owner survived recovery: path=%s err=%v", removed, err)
		}
	}
	if _, err := os.Lstat(privateRoot); err != nil {
		t.Fatalf("partial owner rollback removed the private root: %v", err)
	}
}

func TestPrivateCASOrphanTopologyRecoveryDefersEachPartialEvidenceRegistryLeaf(t *testing.T) {
	for _, presentLeaf := range []string{"indexes", "capsules"} {
		t.Run(presentLeaf, func(t *testing.T) {
			dataDir := t.TempDir()
			privateRoot := filepath.Join(dataDir, "private")
			owner := filepath.Join(privateRoot, "evidence-registry")
			present := filepath.Join(owner, presentLeaf)
			shard := filepath.Join(present, "ab")
			if err := os.MkdirAll(shard, 0o700); err != nil {
				t.Fatal(err)
			}
			before := make(map[string]os.FileInfo, 2)
			for _, path := range []string{present, shard} {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = info
			}
			authority, err := privatecastest.NewAccessAuthority(privateRoot)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(context.Background(), dataDir, authority)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.CandidateCount() != 0 {
				t.Fatalf("evidence registry partial leaf became generic orphan residue: %d", prepared.CandidateCount())
			}
			if err := prepared.Apply(context.Background()); err != nil {
				t.Fatal(err)
			}
			for path, first := range before {
				after, err := os.Stat(path)
				if err != nil || !os.SameFile(first, after) {
					t.Fatalf("evidence registry partial leaf or shard identity changed: path=%s err=%v", path, err)
				}
			}
			missing := "capsules"
			if presentLeaf == "capsules" {
				missing = "indexes"
			}
			if _, err := os.Lstat(filepath.Join(owner, missing)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("generic orphan recovery manufactured missing evidence registry leaf: %v", err)
			}
		})
	}
}

func TestPrivateCASOrphanTopologyRecoveryDefersCompleteEvidenceRegistryShards(t *testing.T) {
	dataDir := t.TempDir()
	privateRoot := filepath.Join(dataDir, "private")
	owner := filepath.Join(privateRoot, "evidence-registry")
	before := make(map[string]os.FileInfo, 2)
	for _, leaf := range []string{"indexes", "capsules"} {
		shard := filepath.Join(owner, leaf, "ab")
		if err := os.MkdirAll(shard, 0o700); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(shard)
		if err != nil {
			t.Fatal(err)
		}
		before[shard] = info
	}
	authority, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(context.Background(), dataDir, authority)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.CandidateCount() != 0 {
		t.Fatalf("complete evidence registry shards became generic orphan residue: %d", prepared.CandidateCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for path, first := range before {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(first, after) {
			t.Fatalf("complete evidence registry shard identity changed: path=%s err=%v", path, err)
		}
	}
}

func TestPrivateCASOrphanTopologyRecoveryPreservesReservedContinuationMigrationLeaves(t *testing.T) {
	dataDir := t.TempDir()
	privateRoot := filepath.Join(dataDir, "private")
	owner := filepath.Join(privateRoot, "gate-continuations")
	legacyReceiptPartition := filepath.Join(owner, "receipts")
	if err := os.MkdirAll(legacyReceiptPartition, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.CandidateCount() != 0 {
		t.Fatalf("reserved migration leaf became orphan cleanup candidate: %d", prepared.CandidateCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{owner, legacyReceiptPartition} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("reserved continuation migration topology was removed: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASOrphanTopologyRecoveryPreservesCompleteOwnerAndRejectsPopulatedPartialOwner(t *testing.T) {
	t.Run("complete empty owner", func(t *testing.T) {
		dataDir := t.TempDir()
		privateRoot := filepath.Join(dataDir, "private")
		owner := filepath.Join(privateRoot, "accepted-finals")
		for _, leaf := range []string{"records", "dispositions"} {
			if err := os.MkdirAll(filepath.Join(owner, leaf), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		authority, err := privatecastest.NewAccessAuthority(privateRoot)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(
			context.Background(),
			dataDir,
			authority,
		)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.CandidateCount() != 0 {
			t.Fatalf("complete empty owner became rollback candidate: %d", prepared.CandidateCount())
		}
		if err := prepared.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, retained := range []string{
			owner,
			filepath.Join(owner, "records"),
			filepath.Join(owner, "dispositions"),
		} {
			if _, err := os.Lstat(retained); err != nil {
				t.Fatalf("complete owner was removed: path=%s err=%v", retained, err)
			}
		}
	})

	t.Run("populated partial owner", func(t *testing.T) {
		dataDir := t.TempDir()
		privateRoot := filepath.Join(dataDir, "private")
		owner := filepath.Join(privateRoot, "accepted-finals")
		leaf := filepath.Join(owner, "records")
		shard := filepath.Join(leaf, "ab")
		if err := os.MkdirAll(shard, 0o700); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(shard, "ab"+strings.Repeat("0", 62)+".json")
		if err := os.WriteFile(record, []byte(`{"record":"present"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		authority, err := privatecastest.NewAccessAuthority(privateRoot)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(
			context.Background(),
			dataDir,
			authority,
		); err == nil {
			t.Fatal("populated partial owner received rollback authority")
		}
		for _, retained := range []string{owner, leaf, shard, record} {
			if _, err := os.Lstat(retained); err != nil {
				t.Fatalf("failed partial-owner preflight mutated state: path=%s err=%v", retained, err)
			}
		}
	})
}
