//go:build darwin

package securegeneration

import (
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSecureGenerationRejectsExtendedACLOnRootAndPayload(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		if output, err := exec.Command("chmod", "+a", "everyone allow list,search,add_file,delete_child", root).CombinedOutput(); err != nil {
			t.Fatalf("root ACL fixture: %v: %s", err, output)
		}
		if _, err := store.Observe(nil); err == nil {
			t.Fatal("root extended ACL was accepted")
		}
	})

	t.Run("payload", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		if result, err := store.Publish(nil, generationFixture(t, "first")); err != nil || result.State != Committed {
			t.Fatal(err)
		}
		payload := filepath.Join(root, "public", "payload.txt")
		if output, err := exec.Command("chmod", "+a", "everyone allow read,write", payload).CombinedOutput(); err != nil {
			t.Fatalf("payload ACL fixture: %v: %s", err, output)
		}
		if _, err := store.Observe(nil); err == nil {
			t.Fatal("payload extended ACL was accepted")
		}
	})
}

func TestSecureGenerationRejectsUnexpectedXattrAndFileFlags(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(t *testing.T, path string)
	}{
		{name: "xattr", apply: func(t *testing.T, path string) {
			if err := unix.Setxattr(path, "com.analytix.audit", []byte("unexpected"), 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Removexattr(path, "com.analytix.audit") })
		}},
		{name: "flags", apply: func(t *testing.T, path string) {
			if err := unix.Chflags(path, unix.UF_NODUMP); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Chflags(path, 0) })
		}},
	} {
		for _, target := range []string{"root", "payload"} {
			t.Run(mutate.name+"-"+target, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "generations")
				store := newGenerationTestStore(t, root)
				if result, err := store.Publish(nil, generationFixture(t, "first")); err != nil || result.State != Committed {
					t.Fatalf("setup publish result=%#v err=%v", result, err)
				}
				path := root
				if target == "payload" {
					path = filepath.Join(root, "public", "payload.txt")
				}
				mutate.apply(t, path)
				if _, err := store.Observe(nil); err == nil {
					t.Fatalf("unexpected %s on %s was accepted", mutate.name, target)
				}
			})
		}
	}
}
