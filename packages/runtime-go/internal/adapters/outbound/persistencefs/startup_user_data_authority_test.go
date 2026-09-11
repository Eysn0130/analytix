package persistencefs

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCompositeLeaseStartupUserDataIgnoresAmbientConfig(t *testing.T) {
	base := t.TempDir()
	ambientHome := filepath.Join(base, "ambient-home")
	ambientConfig := filepath.Join(base, "ambient-config")
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron-user-data")
	for _, path := range []string{ambientHome, ambientConfig, data, durable, userData} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", ambientHome)
	t.Setenv("XDG_CONFIG_HOME", ambientConfig)
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithStartupUserData(roots, userData)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := lease.ValidateStartupUserData(userData); err != nil {
		t.Fatalf("explicit userData validation: %v", err)
	}
	expected := filepath.Join(userData, "startup-authority-v1", "root-"+rootBindingDigest(roots))
	if info, err := os.Lstat(expected); err != nil || !info.IsDir() {
		t.Fatalf("explicit startup authority missing: info=%v err=%v", info, err)
	}
	ambient := filepath.Join(ambientConfig, "analytix", "startup-authority-v1")
	if _, err := os.Lstat(ambient); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit startup authority mutated ambient config: %v", err)
	}
	if err := lease.ValidateStartupUserData(filepath.Join(base, "other-user-data")); err == nil {
		t.Fatal("changed userData passed the frozen startup authority")
	}
}

func TestCompositeLeaseStartupUserDataRejectsRelativeOverlapAndSymlink(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	for _, path := range []string{data, durable} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireCompositeLeaseWithStartupUserData(roots, "relative-user-data"); err == nil {
		t.Fatal("relative userData passed startup authority admission")
	}
	if _, err := AcquireCompositeLeaseWithStartupUserData(roots, base); err == nil {
		t.Fatal("userData containing managed roots passed startup authority admission")
	}
	realUserData := filepath.Join(base, "real-user-data")
	linkedUserData := filepath.Join(base, "linked-user-data")
	if err := os.Mkdir(realUserData, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realUserData, linkedUserData); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := AcquireCompositeLeaseWithStartupUserData(roots, linkedUserData); err == nil {
		t.Fatal("symlink userData passed startup authority admission")
	}
}

func TestExplicitStartupUserDataDoesNotCopyAmbientJournal(t *testing.T) {
	base := t.TempDir()
	ambientHome := filepath.Join(base, "ambient-home")
	ambientConfig := filepath.Join(base, "ambient-config")
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron-user-data")
	for _, path := range []string{ambientHome, ambientConfig, data, durable, userData} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", ambientHome)
	t.Setenv("XDG_CONFIG_HOME", ambientConfig)
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	ambientLease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := ambientLease.Close(); err != nil {
		t.Fatal(err)
	}
	ambientNamespace, err := persistentStartupNamespacePath(roots, false)
	if err != nil {
		t.Fatal(err)
	}
	ambientBefore, err := treeDigestForStartupUserDataTest(ambientNamespace)
	if err != nil {
		t.Fatal(err)
	}
	explicitLease, err := AcquireCompositeLeaseWithStartupUserData(roots, userData)
	if err != nil {
		t.Fatal(err)
	}
	if err := explicitLease.Close(); err != nil {
		t.Fatal(err)
	}
	ambientAfter, err := treeDigestForStartupUserDataTest(ambientNamespace)
	if err != nil {
		t.Fatal(err)
	}
	if ambientBefore != ambientAfter {
		t.Fatal("explicit startup copied or changed the ambient signed journal")
	}
	explicitNamespace := filepath.Join(userData, "startup-authority-v1", "root-"+rootBindingDigest(roots))
	if canonicalPathKey(explicitNamespace) == canonicalPathKey(ambientNamespace) {
		t.Fatal("explicit and ambient startup namespaces unexpectedly coincide")
	}
	if _, err := os.Lstat(explicitNamespace); err != nil {
		t.Fatalf("explicit namespace missing: %v", err)
	}
}

func treeDigestForStartupUserDataTest(root string) ([32]byte, error) {
	var body []byte
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body = append(body, relative...)
		if entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body = append(body, contents...)
		return nil
	})
	return sha256.Sum256(body), err
}
