//go:build darwin || linux

package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

const b1FormalOwnerHostParent = "/Users/sun/.codex/instruction-maintenance/2026-09-07-resume/own2-b1-host-private-roots"

// Only fresh fixture objects are removed, in reverse creation order. Parent
// identity is checked without freezing its changing child count or inventory.
func pinFormalOwnerFixtureCleanup(t *testing.T, path string) {
	t.Helper()
	pinned, err := os.Lstat(path)
	if err != nil {
		t.Fatal("cannot pin fresh formal owner fixture; retain for inspection")
	}
	stat := pinned.Sys().(*syscall.Stat_t)
	var target string
	if pinned.Mode()&os.ModeSymlink != 0 {
		target, err = os.Readlink(path)
	} else {
		var canonical string
		canonical, err = filepath.EvalSymlinks(path)
		if canonical != path {
			t.Fatal("fresh formal owner fixture is not canonical")
		}
	}
	if err != nil || stat.Uid != uint32(os.Geteuid()) {
		t.Fatal("fresh formal owner fixture identity is invalid")
	}
	t.Cleanup(func() {
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(pinned, current) || current.Mode() != pinned.Mode() {
			t.Errorf("formal owner fixture cleanup retained changed object: %s", path)
			return
		}
		now := current.Sys().(*syscall.Stat_t)
		if now.Dev != stat.Dev || now.Ino != stat.Ino || now.Uid != stat.Uid || now.Gid != stat.Gid || now.Nlink != stat.Nlink {
			t.Errorf("formal owner fixture cleanup retained identity drift: %s", path)
			return
		}
		if target != "" {
			if actual, err := os.Readlink(path); err != nil || actual != target {
				t.Errorf("formal owner fixture cleanup retained changed alias: %s", path)
				return
			}
		} else if canonical, err := filepath.EvalSymlinks(path); err != nil || canonical != path {
			t.Errorf("formal owner fixture cleanup retained noncanonical object: %s", path)
			return
		}
		if current.IsDir() {
			if children, err := os.ReadDir(path); err != nil || len(children) != 0 {
				t.Errorf("formal owner fixture cleanup retained unexpected children: %s", path)
				return
			}
		}
		if os.Remove(path) != nil {
			t.Errorf("formal owner fixture exact cleanup failed: %s", path)
		}
	})
}

func freshFormalOwnerHostFixture(t *testing.T) string {
	t.Helper()
	parent := b1FormalOwnerHostParent
	if runtime.GOOS != "darwin" {
		var err error
		parent, err = filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
	}
	pinned, err := os.Lstat(parent)
	canonical, realErr := filepath.EvalSymlinks(parent)
	if err != nil || realErr != nil || canonical != parent || !pinned.IsDir() ||
		pinned.Mode()&os.ModeSymlink != 0 || pinned.Mode().Perm() != 0o700 ||
		pinned.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		t.Fatal("Owner-provisioned host parent is unavailable; no fallback")
	}
	t.Cleanup(func() {
		current, err := os.Lstat(parent)
		canonical, realErr := filepath.EvalSymlinks(parent)
		if err != nil || realErr != nil || canonical != parent || !os.SameFile(pinned, current) ||
			current.Mode() != pinned.Mode() || current.Sys().(*syscall.Stat_t).Uid != pinned.Sys().(*syscall.Stat_t).Uid {
			t.Error("Owner host parent identity changed")
		}
	})
	var random [3]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "analytix-own2-b1-process-"+hex.EncodeToString(random[:]))
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	pinFormalOwnerFixtureCleanup(t, root)
	return root
}

func TestPrepareFormalOwnerRootRequiresFreshPrivateDedicatedPath(t *testing.T) {
	parent := freshFormalOwnerHostFixture(t)
	root := filepath.Join(parent, "sidecar")
	if err := prepareFormalOwnerRoot(root); err != nil {
		t.Fatal(err)
	}
	pinFormalOwnerFixtureCleanup(t, root)
	state, err := os.Lstat(root)
	if err != nil || !state.IsDir() || state.Mode().Perm() != 0o700 {
		t.Fatal("formal owner root is not private")
	}
	if err := prepareFormalOwnerRoot(root); err == nil {
		t.Fatal("pre-existing formal owner root was accepted")
	}
}

func TestPrepareFormalOwnerRootRejectsProtectedStorageBeforeCreation(t *testing.T) {
	assertRejected := func(root string) {
		t.Helper()
		if err := prepareFormalOwnerRoot(root); err == nil {
			t.Fatal("protected storage became a formal owner root")
		}
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatal("refused formal owner root was created")
		}
	}
	if runtime.GOOS == "darwin" {
		cacheInput := os.Getenv("GOTMPDIR")
		cache, err := filepath.EvalSymlinks(cacheInput)
		if cacheInput == "" || err != nil || !pathContainedByV1("/Volumes/AnalytixCache", cache) {
			t.Fatal("canonical cache test temp is required")
		}
		parent, err := os.MkdirTemp(cache, "analytix-b1-formal-cache-")
		if err != nil {
			t.Fatal(err)
		}
		pinFormalOwnerFixtureCleanup(t, parent)
		assertRejected(filepath.Join(parent, "sidecar"))
	}
	// Exercise the real cwd/source overlap guard using an isolated fixture as
	// cwd. No fixture or changed permission is needed in the source repository.
	parent := freshFormalOwnerHostFixture(t)
	t.Chdir(parent)
	assertRejected(filepath.Join(parent, "sidecar"))
}

func TestPrepareFormalOwnerRootRejectsSymlinkParent(t *testing.T) {
	base := freshFormalOwnerHostFixture(t)
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(base, alias); err != nil {
		t.Fatal(err)
	}
	pinFormalOwnerFixtureCleanup(t, alias)
	if err := prepareFormalOwnerRoot(filepath.Join(alias, "product-owner")); err == nil {
		t.Fatal("symlinked formal owner parent was accepted")
	}
	if _, err := os.Lstat(filepath.Join(base, "product-owner")); !os.IsNotExist(err) {
		t.Fatal("symlink parent refusal created a child")
	}
}

func TestPrepareFormalOwnerRootRejectsAncestorAliasBeforeCreation(t *testing.T) {
	base := freshFormalOwnerHostFixture(t)
	private := filepath.Join(base, "private")
	parent := filepath.Join(private, "parent")
	for _, path := range []string{private, parent} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		pinFormalOwnerFixtureCleanup(t, path)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(private, alias); err != nil {
		t.Fatal(err)
	}
	pinFormalOwnerFixtureCleanup(t, alias)
	if err := prepareFormalOwnerRoot(filepath.Join(alias, "parent", "sidecar")); err == nil {
		t.Fatal("canonical parent alias became a formal owner root")
	}
	if _, err := os.Lstat(filepath.Join(parent, "sidecar")); !os.IsNotExist(err) {
		t.Fatal("ancestor alias refusal created a child")
	}
}

func TestPrepareFormalOwnerRootRejectsWritableParentBeforeCreation(t *testing.T) {
	base := freshFormalOwnerHostFixture(t)
	parent := filepath.Join(base, "writable")
	if os.Mkdir(parent, 0o700) != nil || os.Chmod(parent, 0o770) != nil {
		t.Fatal("prepare task-owned writable parent")
	}
	pinFormalOwnerFixtureCleanup(t, parent)
	if err := prepareFormalOwnerRoot(filepath.Join(parent, "sidecar")); err == nil {
		t.Fatal("writable formal owner parent was accepted")
	}
	if _, err := os.Lstat(filepath.Join(parent, "sidecar")); !os.IsNotExist(err) {
		t.Fatal("writable parent refusal created a child")
	}
}
