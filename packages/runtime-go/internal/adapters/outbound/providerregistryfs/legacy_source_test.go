package providerregistryfs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

func TestLegacySourceReaderPhysicalBoundary(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{
		"exact source and owner", "source inode mismatch", "source device mismatch", "source physical identity mismatch",
		"source symlink", "source hardlink", "source empty", "source oversized",
		"missing lock", "lock symlink", "extra owner", "wrong owner",
		"owner symlink", "owner hardlink", "owner empty", "owner oversized",
	} {
		t.Run(scenario, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(root, "settings.json")
			source := []byte(`{"synthetic":"source"}`)
			owner := []byte(`{"synthetic":"owner"}`)
			write := func(path string, value []byte) {
				t.Helper()
				if err := os.WriteFile(path, value, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(sourcePath, source)
			info, err := os.Lstat(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			device, deviceOK := legacyMigrationFileInfoUintField(info, "Dev")
			inode, inodeOK := legacyMigrationFileInfoUintField(info, "Ino")
			if !deviceOK || !inodeOK || !legacyMigrationRegularSingleLinkFileInfo(info) {
				t.Fatal("fixture physical identity is unavailable")
			}
			request := registryport.LegacySourceRequest{
				SourceLocator:                "synthetic-source",
				SourcePhysicalIdentitySHA256: legacySourcePhysicalIdentity("synthetic-source", sourcePath),
				SourcePath:                   sourcePath, SourceDevice: device, SourceInode: inode,
				LockOwnerToken: "01234567-89ab-4000-8000-0123456789ab",
			}
			lockPath := sourcePath + legacyMigrationSourceLockSuffix
			if err := os.Mkdir(lockPath, 0o700); err != nil {
				t.Fatal(err)
			}
			ownerPath := filepath.Join(lockPath, "owner-"+request.LockOwnerToken+".json")
			write(ownerPath, owner)
			replaceWithSymlink := func(path string) {
				t.Helper()
				target := filepath.Join(root, "moved-"+filepath.Base(path))
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "source physical identity mismatch":
				request.SourcePhysicalIdentitySHA256 = strings.Repeat("a", 64)
			case "source inode mismatch":
				request.SourceInode++
			case "source device mismatch":
				request.SourceDevice++
			case "source symlink":
				replaceWithSymlink(sourcePath)
			case "source hardlink":
				if err := os.Link(sourcePath, filepath.Join(root, "source-link")); err != nil {
					t.Fatal(err)
				}
			case "source empty":
				write(sourcePath, nil)
			case "source oversized":
				write(sourcePath, bytes.Repeat([]byte("s"), registryport.MaxLegacySourceSnapshotBytes+1))
			case "missing lock":
				if err := os.Rename(lockPath, filepath.Join(root, "moved-lock")); err != nil {
					t.Fatal(err)
				}
			case "lock symlink":
				replaceWithSymlink(lockPath)
			case "extra owner":
				write(filepath.Join(lockPath, "extra.json"), owner)
			case "wrong owner":
				if err := os.Rename(ownerPath, filepath.Join(lockPath, "wrong-owner.json")); err != nil {
					t.Fatal(err)
				}
			case "owner symlink":
				replaceWithSymlink(ownerPath)
			case "owner hardlink":
				if err := os.Link(ownerPath, filepath.Join(root, "owner-link")); err != nil {
					t.Fatal(err)
				}
			case "owner empty":
				write(ownerPath, nil)
			case "owner oversized":
				write(ownerPath, bytes.Repeat([]byte("o"), registryport.MaxLegacySourceLockOwnerBytes+1))
			}
			snapshot, err := (LegacySourceReader{}).ReadLegacySource(request)
			if scenario != "exact source and owner" {
				if !errors.Is(err, registryport.ErrVerification) || snapshot.PhysicalPath != "" ||
					len(snapshot.Source) != 0 || len(snapshot.LockOwner) != 0 {
					t.Fatal("invalid physical source returned data or did not fail with safe verification error")
				}
				return
			}
			if err != nil || snapshot.PhysicalPath != sourcePath || !bytes.Equal(snapshot.Source, source) || !bytes.Equal(snapshot.LockOwner, owner) {
				t.Fatal("exact physical source and owner did not round trip")
			}
			ownedSource, ownedOwner := snapshot.Source, snapshot.LockOwner
			snapshot.Clear()
			if !bytes.Equal(ownedSource, make([]byte, len(source))) || !bytes.Equal(ownedOwner, make([]byte, len(owner))) ||
				snapshot.Source != nil || snapshot.LockOwner != nil || snapshot.PhysicalPath != "" {
				t.Fatal("snapshot clear retained sensitive observations")
			}
		})
	}
}

func TestLegacySourceReaderRejectsMalformedPathsBeforeReading(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"", "settings.json", "/settings\x00.json", "/" + strings.Repeat("s", 4096)} {
		snapshot, err := (LegacySourceReader{}).ReadLegacySource(registryport.LegacySourceRequest{
			SourcePath: path, SourceInode: 1, LockOwnerToken: "01234567-89ab-4000-8000-0123456789ab",
		})
		if !errors.Is(err, registryport.ErrInvalidRequest) || len(snapshot.Source) != 0 || len(snapshot.LockOwner) != 0 {
			t.Fatal("malformed source path did not fail closed")
		}
	}
}
