//go:build windows

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"golang.org/x/sys/windows"
)

func TestPrivateCASWindowsDirectoryRecoveryDeletesOnlyTopologyResidue(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	privateRoot := filepath.Join(dataDir, "private")
	reportRoot := filepath.Join(privateRoot, "report-publication")
	residue := filepath.Join(
		reportRoot,
		domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
	)
	for _, directory := range []string{dataDir, reportRoot, residue} {
		privateCASWindowsCreateProtectedTestDirectory(t, directory)
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
		t.Fatalf("prepared Windows create residues = %d", prepared.ResidueCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Windows topology-bound create residue survived recovery: %v", err)
	}
	for _, retained := range []string{privateRoot, reportRoot} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("Windows create-residue recovery removed canonical topology: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASWindowsDirectoryRecoveryRejectsDeletionWindowReplacement(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	privateRoot := filepath.Join(dataDir, "private")
	reportRoot := filepath.Join(privateRoot, "report-publication")
	residue := filepath.Join(
		reportRoot,
		domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
	)
	for _, directory := range []string{dataDir, reportRoot, residue} {
		privateCASWindowsCreateProtectedTestDirectory(t, directory)
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
		t.Fatalf("prepared Windows create residues = %d", prepared.ResidueCount())
	}
	moved := filepath.Join(reportRoot, "moved-create-residue")
	privateCASCreateResidueRecoveryTestHooks.Lock()
	privateCASCreateResidueRecoveryTestHooks.hook = func(phase string, index int) error {
		if phase != "after_final_validation_before_delete" || index != 0 {
			return nil
		}
		if err := os.Rename(residue, moved); err != nil {
			return err
		}
		handle, err := privateWindowsOpenAbsoluteDirectory(residue, true)
		if err != nil {
			return err
		}
		return windows.CloseHandle(handle)
	}
	privateCASCreateResidueRecoveryTestHooks.Unlock()
	t.Cleanup(func() {
		privateCASCreateResidueRecoveryTestHooks.Lock()
		privateCASCreateResidueRecoveryTestHooks.hook = nil
		privateCASCreateResidueRecoveryTestHooks.Unlock()
	})
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("Windows same-name replacement in the final deletion window was deleted")
	}
	for _, retained := range []string{residue, moved} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("Windows deletion-window rejection lost a directory: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASWindowsOrphanTopologyRollsBackRecursiveEmptyPartialOwner(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	privateRoot := filepath.Join(dataDir, "private")
	owner := filepath.Join(privateRoot, "accepted-finals")
	leaf := filepath.Join(owner, "records")
	shard := filepath.Join(leaf, "ab")
	for _, directory := range []string{dataDir, shard} {
		privateCASWindowsCreateProtectedTestDirectory(t, directory)
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
		t.Fatalf("prepared Windows partial-owner candidates = %d", prepared.CandidateCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(owner); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Windows recursive empty partial owner survived recovery: %v", err)
	}
	if _, err := os.Lstat(privateRoot); err != nil {
		t.Fatalf("Windows partial-owner recovery removed the private root: %v", err)
	}
}

func TestPrivateCASWindowsOrphanTopologyDefersEachPartialEvidenceRegistryLeaf(t *testing.T) {
	for _, presentLeaf := range []string{"indexes", "capsules"} {
		t.Run(presentLeaf, func(t *testing.T) {
			dataDir := filepath.Join(t.TempDir(), "data")
			privateRoot := filepath.Join(dataDir, "private")
			owner := filepath.Join(privateRoot, "evidence-registry")
			present := filepath.Join(owner, presentLeaf)
			shard := filepath.Join(present, "ab")
			for _, directory := range []string{dataDir, shard} {
				privateCASWindowsCreateProtectedTestDirectory(t, directory)
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
				t.Fatalf("Windows evidence registry partial leaf became generic orphan residue: %d", prepared.CandidateCount())
			}
			if err := prepared.Apply(context.Background()); err != nil {
				t.Fatal(err)
			}
			for path, first := range before {
				after, err := os.Stat(path)
				if err != nil || !os.SameFile(first, after) {
					t.Fatalf("Windows evidence registry partial leaf or shard identity changed: path=%s err=%v", path, err)
				}
			}
			missing := "capsules"
			if presentLeaf == "capsules" {
				missing = "indexes"
			}
			if _, err := os.Lstat(filepath.Join(owner, missing)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Windows generic orphan recovery manufactured missing evidence registry leaf: %v", err)
			}
		})
	}
}

func TestPrivateCASWindowsOrphanTopologyDefersCompleteEvidenceRegistryShards(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	privateRoot := filepath.Join(dataDir, "private")
	owner := filepath.Join(privateRoot, "evidence-registry")
	privateCASWindowsCreateProtectedTestDirectory(t, dataDir)
	before := make(map[string]os.FileInfo, 2)
	for _, leaf := range []string{"indexes", "capsules"} {
		shard := filepath.Join(owner, leaf, "ab")
		privateCASWindowsCreateProtectedTestDirectory(t, shard)
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
		t.Fatalf("Windows complete evidence registry shards became generic orphan residue: %d", prepared.CandidateCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for path, first := range before {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(first, after) {
			t.Fatalf("Windows complete evidence registry shard identity changed: path=%s err=%v", path, err)
		}
	}
}

func TestPrivateCASWindowsOrphanTopologyPreservesReservedContinuationMigrationLeaves(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	privateRoot := filepath.Join(dataDir, "private")
	owner := filepath.Join(privateRoot, "gate-continuations")
	legacyReceiptPartition := filepath.Join(owner, "receipts")
	for _, directory := range []string{dataDir, legacyReceiptPartition} {
		privateCASWindowsCreateProtectedTestDirectory(t, directory)
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
		t.Fatalf("reserved Windows migration leaf became orphan cleanup candidate: %d", prepared.CandidateCount())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{owner, legacyReceiptPartition} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("reserved Windows continuation migration topology was removed: path=%s err=%v", retained, err)
		}
	}
}

func TestPrivateCASWindowsDirectoryRecoveryRejectsCaseAliasedTopology(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	privateRoot := filepath.Join(dataDir, "private")
	for _, directory := range []string{
		dataDir,
		filepath.Join(privateRoot, "REPORT-PUBLICATION"),
	} {
		privateCASWindowsCreateProtectedTestDirectory(t, directory)
	}
	authority, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
		context.Background(),
		dataDir,
		authority,
	); err == nil {
		t.Fatal("Windows case-aliased private CAS topology was accepted")
	}
}

func TestPrivateCASWindowsDirectoryRecoveryRejectsBroadenedDACLAndADS(t *testing.T) {
	for _, mutation := range []string{"dacl", "ads"} {
		t.Run(mutation, func(t *testing.T) {
			dataDir := filepath.Join(t.TempDir(), "data")
			privateRoot := filepath.Join(dataDir, "private")
			reportRoot := filepath.Join(privateRoot, "report-publication")
			residue := filepath.Join(
				reportRoot,
				domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
			)
			for _, directory := range []string{dataDir, reportRoot, residue} {
				privateCASWindowsCreateProtectedTestDirectory(t, directory)
			}
			switch mutation {
			case "dacl":
				privateCASWindowsBroadenTestDirectoryDACL(t, residue)
			case "ads":
				if err := os.WriteFile(residue+":attacker", []byte("named-stream"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			authority, err := privatecastest.NewAccessAuthority(privateRoot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(
				context.Background(),
				dataDir,
				authority,
			); err == nil {
				t.Fatalf("Windows create-residue recovery accepted unsafe %s state", mutation)
			}
			if _, err := os.Lstat(residue); err != nil {
				t.Fatalf("Windows unsafe %s rejection mutated the residue: %v", mutation, err)
			}
		})
	}
}

func TestPrivateCASWindowsDirectoryEnumerationReopensIndependentCursor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	child := filepath.Join(root, "child")
	for _, directory := range []string{root, child} {
		privateCASWindowsCreateProtectedTestDirectory(t, directory)
	}
	handle, err := privateWindowsOpenAbsoluteDirectory(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	for pass := 0; pass < 3; pass++ {
		entries, err := privateCASWindowsReadDirBounded(handle, 1)
		if err != nil {
			t.Fatalf("Windows independent directory inventory pass %d failed: %v", pass, err)
		}
		if len(entries) != 1 || entries[0].Name() != "child" {
			t.Fatalf("Windows independent directory inventory pass %d = %#v", pass, entries)
		}
	}
}

func privateCASWindowsBroadenTestDirectoryDACL(t *testing.T, path string) {
	t.Helper()
	handle, err := privateWindowsOpenAbsoluteDirectory(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	user, err := token.GetTokenUser()
	token.Close()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatal("current Windows SID is unavailable")
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FR;;;WD)",
	)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
}

func privateCASWindowsCreateProtectedTestDirectory(t *testing.T, path string) {
	t.Helper()
	handle, err := privateWindowsOpenAbsoluteDirectory(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
}
