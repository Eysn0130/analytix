//go:build darwin || linux

package checkpointauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestLegacyCheckpointQuarantinePermissionFailureLeavesSourceInPlace(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	sourceFile := writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeContent":"restricted"}`))
	if err := os.Chmod(sourceFile, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sourceFile, 0o600) })
	if err := PreflightRecovery(context.Background(), checkpointRoot, access); err == nil {
		t.Fatal("unreadable legacy checkpoint record passed startup preflight")
	}
	if _, err := os.Lstat(sourceFile); err != nil {
		t.Fatalf("permission failure moved or deleted legacy source: %v", err)
	}
	quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if _, err := os.Lstat(quarantineRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("permission failure created quarantine state: %v", err)
	}
}

func TestLegacyCheckpointQuarantineCreatesProtectedPrivateParents(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeContent":"protected"}`))
	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err != nil {
		t.Fatal(err)
	}
	quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	auditRoot, _ := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	for _, path := range []string{
		filepath.Join(dataDir, "private"),
		quarantineRoot,
		auditRoot,
		filepath.Join(quarantineRoot, persistencefs.LegacyCheckpointSnapshotPayloadsV1),
	} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("quarantine parent is unavailable: path=%s err=%v", path, err)
		}
		if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("quarantine parent is not protected: path=%s mode=%v", path, info.Mode())
		}
	}
}

func TestLegacyCheckpointQuarantineRejectsPermissiveSourceModesWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		path func(string, string) string
		mode os.FileMode
	}{
		{
			name: "source-directory",
			path: func(dataDir, _ string) string {
				return filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)
			},
			mode: 0o755,
		},
		{
			name: "record-file",
			path: func(_ string, sourceFile string) string { return sourceFile },
			mode: 0o644,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
			access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			sourceFile := writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeContent":"must-stay-private"}`))
			changed := test.path(dataDir, sourceFile)
			if err := os.Chmod(changed, test.mode); err != nil {
				t.Fatal(err)
			}
			if err := PreflightRecovery(context.Background(), checkpointRoot, access); err == nil {
				t.Fatal("permissive legacy checkpoint mode passed startup preflight")
			}
			if body, err := os.ReadFile(sourceFile); err != nil || string(body) != `{"beforeContent":"must-stay-private"}` {
				t.Fatalf("permission rejection changed legacy bytes: body=%q err=%v", body, err)
			}
			if info, err := os.Lstat(changed); err != nil || info.Mode().Perm() != test.mode {
				t.Fatalf("permission rejection changed source mode: info=%v err=%v", info, err)
			}
			quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
			if _, err := os.Lstat(quarantineRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("permission rejection created quarantine state: %v", err)
			}
		})
	}
}

func TestLegacyCheckpointQuarantinePreflightDoesNotConsumeAuditCASResidue(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	auditRoot, err := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	digest := "aa" + strings.Repeat("1", 62)
	residue := filepath.Join(auditRoot, "aa", "."+digest+".json-0123456789abcdef01234567.tmp")
	if err := os.MkdirAll(filepath.Dir(residue), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"interrupted":"before-commit"}`)
	if err := os.WriteFile(residue, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PreflightRecovery(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("valid audit CAS crash residue did not preflight: %v", err)
	}
	if got, err := os.ReadFile(residue); err != nil || string(got) != string(body) {
		t.Fatalf("read-only owner preflight consumed audit residue: body=%q err=%v", got, err)
	}
	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("audit CAS residue did not recover after preflight: %v", err)
	}
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery did not consume validated audit residue: %v", err)
	}
}

func TestLegacyCheckpointPreparedRecoveryRejectsQuarantineOwnerSwapWithOriginalAuditLeaf(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	auditRoot, err := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	digest := "ab" + strings.Repeat("2", 62)
	residueName := "." + digest + ".json-0123456789abcdef01234567.tmp"
	residue := filepath.Join(auditRoot, digest[:2], residueName)
	if err := os.MkdirAll(filepath.Dir(residue), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(residue, []byte("checkpoint-audit-residue"), 0o600); err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareRecoveryV1(context.Background(), checkpointRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	quarantineRoot, err := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	originalOwner := quarantineRoot + ".original"
	if err := os.Rename(quarantineRoot, originalOwner); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(quarantineRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(
		filepath.Join(originalOwner, persistencefs.LegacyCheckpointSnapshotAuditRecordsV1),
		filepath.Join(quarantineRoot, persistencefs.LegacyCheckpointSnapshotAuditRecordsV1),
	); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err == nil {
		t.Fatal("legacy checkpoint recovery accepted a replacement quarantine owner containing the original audit leaf identity")
	}
	if body, err := os.ReadFile(residue); err != nil || string(body) != "checkpoint-audit-residue" {
		t.Fatalf("rejected owner swap changed the frozen residue: body=%q err=%v", body, err)
	}
}
