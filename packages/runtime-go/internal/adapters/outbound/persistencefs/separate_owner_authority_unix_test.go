//go:build darwin || linux

package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectedProtectedFileStateExactlyMatchesProtectionResult(t *testing.T) {
	_, owner, lease, authority := newSeparateOwnerUnixTestAuthority(t)
	const targetName = "payload"
	target := filepath.Join(owner, targetName)
	if err := os.WriteFile(target, []byte("opaque"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	err := lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		directory, err := access.Directory()
		if err != nil {
			return err
		}
		initial, _, present, err := directory.CaptureFile(context.Background(), targetName, 1024, false, false)
		if err != nil || !present {
			return errors.Join(errors.New("capture source target"), err)
		}
		projected, err := directory.ProjectProtectedFileStateExact(context.Background(), targetName, initial)
		if err != nil {
			return err
		}
		if projected == initial || !sameSeparateOwnerFileMaterialForTest(projected, initial) {
			return errors.New("protected projection did not preserve exact file material while narrowing security")
		}
		protected, err := directory.ProtectFileExact(context.Background(), targetName, initial)
		if err != nil {
			return err
		}
		if protected != projected {
			return errors.New("protected readback differs from the authenticated projection")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sameSeparateOwnerFileMaterialForTest(left, right SeparateOwnerFileState) bool {
	return left.Identity == right.Identity && left.Size == right.Size &&
		left.ModifiedUnixNano == right.ModifiedUnixNano && left.SHA256 == right.SHA256
}

func TestRemoveFileExactRejectsFinalWindowHardlink(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	owner := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, owner} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	targetName := "payload"
	target := filepath.Join(owner, targetName)
	alias := filepath.Join(base, "late-alias")
	if err := os.WriteFile(target, []byte("opaque"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	authority, ok := lease.FrozenSeparateOwnerAuthority(owner)
	if !ok {
		t.Fatal("separate-owner authority is unavailable")
	}
	err = lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		directory, err := access.Directory()
		if err != nil {
			return err
		}
		expected, _, present, err := directory.CaptureFile(context.Background(), targetName, 1024, false, true)
		if err != nil || !present {
			return errors.Join(errors.New("capture protected target"), err)
		}
		err = directory.removeFileExact(context.Background(), targetName, expected, true, func(stage string) error {
			if stage == "before_unlink" {
				return os.Link(target, alias)
			}
			return nil
		})
		if err == nil {
			return errors.New("late hardlink was accepted as durable absence")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("original name was deleted after rejected hardlink: %v", err)
	}
	body, err := os.ReadFile(alias)
	if err != nil || string(body) != "opaque" {
		t.Fatalf("late alias = %q, %v", body, err)
	}
}

func TestCaptureFileRejectsFinalNameReplacement(t *testing.T) {
	base, owner, lease, authority := newSeparateOwnerUnixTestAuthority(t)
	targetName := "payload"
	target := filepath.Join(owner, targetName)
	held := filepath.Join(base, "held-original")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		directory, err := access.Directory()
		if err != nil {
			return err
		}
		state, body, present, err := directory.captureFile(
			context.Background(), targetName, 1024, false, true,
			func(stage string) error {
				if stage != "before_final_name_check" {
					return nil
				}
				if err := os.Rename(target, held); err != nil {
					return err
				}
				return os.WriteFile(target, []byte("replacement"), 0o600)
			},
		)
		if err == nil || present || state.Valid() || body != nil {
			return errors.New("final-name replacement produced a named file capture")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSeparateOwnerUnixBody(t, target, "replacement")
	assertSeparateOwnerUnixBody(t, held, "original")
}

func TestRemoveFileExactPreservesFinalWindowReplacement(t *testing.T) {
	base, owner, lease, authority := newSeparateOwnerUnixTestAuthority(t)
	targetName := "payload"
	target := filepath.Join(owner, targetName)
	held := filepath.Join(base, "held-original")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		directory, err := access.Directory()
		if err != nil {
			return err
		}
		expected, _, present, err := directory.CaptureFile(context.Background(), targetName, 1024, false, true)
		if err != nil || !present {
			return errors.Join(errors.New("capture protected target"), err)
		}
		err = directory.removeFileExact(context.Background(), targetName, expected, true, func(stage string) error {
			if stage != "before_unlink" {
				return nil
			}
			if err := os.Rename(target, held); err != nil {
				return err
			}
			return os.WriteFile(target, []byte("replacement"), 0o600)
		})
		if err == nil {
			return errors.New("final-window replacement was deleted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSeparateOwnerUnixBody(t, target, "replacement")
	assertSeparateOwnerUnixBody(t, held, "original")
}

func TestChildDirectoryBindingRejectsRenameAndReplacement(t *testing.T) {
	base, owner, lease, authority := newSeparateOwnerUnixTestAuthority(t)
	childName := "journal"
	childPath := filepath.Join(owner, childName)
	held := filepath.Join(base, "held-journal")
	if err := os.Mkdir(childPath, 0o700); err != nil {
		t.Fatal(err)
	}
	err := lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		root, err := access.Directory()
		if err != nil {
			return err
		}
		child, present, err := root.OpenDirectory(context.Background(), childName, true)
		if err != nil || !present {
			return errors.Join(errors.New("open child directory"), err)
		}
		defer child.Close()
		if err := os.Rename(childPath, held); err != nil {
			return err
		}
		if err := os.Mkdir(childPath, 0o700); err != nil {
			return err
		}
		if err := child.Validate(); err == nil {
			return errors.New("renamed child retained stale name authority")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(childPath); err != nil || !info.IsDir() {
		t.Fatalf("replacement child was not preserved: %v", err)
	}
	if info, err := os.Stat(held); err != nil || !info.IsDir() {
		t.Fatalf("original child was not preserved: %v", err)
	}
}

func TestRemoveEmptyDirectoryExactPreservesFinalWindowReplacement(t *testing.T) {
	base, owner, lease, authority := newSeparateOwnerUnixTestAuthority(t)
	childName := "journal"
	childPath := filepath.Join(owner, childName)
	held := filepath.Join(base, "held-journal")
	if err := os.Mkdir(childPath, 0o700); err != nil {
		t.Fatal(err)
	}
	err := lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		root, err := access.Directory()
		if err != nil {
			return err
		}
		child, present, err := root.OpenDirectory(context.Background(), childName, true)
		if err != nil || !present {
			return errors.Join(errors.New("open child directory"), err)
		}
		expected := child.State()
		if err := child.Close(); err != nil {
			return err
		}
		err = root.removeEmptyDirectoryExact(context.Background(), childName, expected, func(stage string) error {
			if stage != "before_rmdir" {
				return nil
			}
			if err := os.Rename(childPath, held); err != nil {
				return err
			}
			return os.Mkdir(childPath, 0o700)
		})
		if err == nil {
			return errors.New("final-window child replacement was removed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{childPath, held} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("directory %s was not preserved: %v", path, err)
		}
	}
}

func newSeparateOwnerUnixTestAuthority(
	t *testing.T,
) (string, string, *CompositeLease, *SeparateOwnerRootAuthority) {
	t.Helper()
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	owner := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, owner} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Close() })
	authority, ok := lease.FrozenSeparateOwnerAuthority(owner)
	if !ok {
		t.Fatal("separate-owner authority is unavailable")
	}
	return base, owner, lease, authority
}

func assertSeparateOwnerUnixBody(t *testing.T, path string, expected string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || string(body) != expected {
		t.Fatalf("file %s = %q, %v", path, body, err)
	}
}

func TestSeparateOwnerAuthorityPermissionChangeInvalidatesLease(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	owner := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, owner} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	authority, ok := lease.FrozenSeparateOwnerAuthority(owner)
	if !ok || authority == nil || authority.Digest() == "" {
		t.Fatal("separate-owner authority is unavailable")
	}
	if frozen, held := lease.FrozenRoots(); !held || frozen != roots {
		t.Fatalf("separate-owner authority entered RootSet: %#v held=%v", frozen, held)
	}
	if err := os.Chmod(owner, 0o770); err != nil {
		t.Fatal(err)
	}
	if _, held := lease.FrozenSeparateOwnerAuthority(owner); held {
		t.Fatal("permission-changed separate-owner authority remained live")
	}
	if err := lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(*SeparateOwnerRootAccess) error {
		return nil
	}); err == nil {
		t.Fatal("permission-changed separate-owner root remained accessible")
	}
}

func TestMissingSeparateOwnerAppearanceInvalidatesLease(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	owner := filepath.Join(base, "missing-owner")
	for _, root := range []string{data, durable} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := os.Mkdir(owner, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, held := lease.FrozenSeparateOwnerAuthority(owner); held {
		t.Fatal("appeared separate-owner root retained stale absence authority")
	}
}
