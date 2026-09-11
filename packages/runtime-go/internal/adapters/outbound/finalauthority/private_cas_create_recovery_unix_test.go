//go:build darwin || linux

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

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
