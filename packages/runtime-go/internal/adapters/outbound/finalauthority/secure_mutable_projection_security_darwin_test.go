//go:build darwin

package finalauthority

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSecureMutableProjectionRejectsExtendedACLAfterOpen(t *testing.T) {
	t.Run("target", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		if _, err := store.ReplaceExact(context.Background(), "", []byte("initial")); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, secureMutableProjectionTestName)
		if output, err := exec.Command("chmod", "+a", "everyone allow read,write", target).CombinedOutput(); err != nil {
			t.Fatalf("projection target ACL fixture: %v: %s", err, output)
		}
		if _, err := store.Observe(context.Background()); err == nil {
			t.Fatal("projection target extended ACL was accepted")
		}
	})

	t.Run("root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		if output, err := exec.Command("chmod", "+a", "everyone allow list,search,add_file,delete_child", root).CombinedOutput(); err != nil {
			t.Fatalf("projection root ACL fixture: %v: %s", err, output)
		}
		if _, err := store.Observe(context.Background()); err == nil {
			t.Fatal("projection root extended ACL was accepted")
		}
	})
}

func TestSecureMutableProjectionRejectsRootACLAtMutationCutPoints(t *testing.T) {
	t.Run("before replace", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		store.faults = &secureMutableProjectionFaults{BeforeAtomicReplace: func() {
			if output, err := exec.Command("chmod", "+a", "everyone allow list,search,add_file,delete_child", root).CombinedOutput(); err != nil {
				t.Fatalf("projection root ACL fixture: %v: %s", err, output)
			}
		}}
		result, err := store.ReplaceExact(context.Background(), "", []byte("candidate"))
		if result.State != SecureMutableProjectionNotCommitted || err == nil {
			t.Fatalf("root ACL added before replacement was accepted: result=%#v err=%v", result, err)
		}
	})

	t.Run("after durable replace", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		store.faults = &secureMutableProjectionFaults{AfterDirectorySync: func() error {
			output, err := exec.Command("chmod", "+a", "everyone allow list,search,add_file,delete_child", root).CombinedOutput()
			if err != nil {
				t.Fatalf("projection root ACL fixture: %v: %s", err, output)
			}
			return nil
		}}
		result, err := store.ReplaceExact(context.Background(), "", []byte("candidate"))
		if result.State != SecureMutableProjectionCommitted || err == nil || errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
			t.Fatalf("post-commit root ACL did not fail closed: result=%#v err=%v", result, err)
		}
	})
}
