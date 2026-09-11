//go:build linux

package securegeneration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSecureGenerationLinuxRejectsUnexpectedXattrAndFileFlags(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(t *testing.T, path string)
	}{
		{name: "xattr", apply: func(t *testing.T, path string) {
			if err := unix.Setxattr(path, "user.analytix_audit", []byte("unexpected"), 0); err != nil {
				if errors.Is(err, unix.ENOTSUP) {
					t.Skip("filesystem does not support user xattrs")
				}
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Removexattr(path, "user.analytix_audit") })
		}},
		{name: "flags", apply: func(t *testing.T, path string) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			const fsNodumpFlag = 0x40
			err = unix.IoctlSetPointerInt(int(file.Fd()), unix.FS_IOC_SETFLAGS, fsNodumpFlag)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				if errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.EOPNOTSUPP) {
					t.Skip("filesystem does not support inode flags")
				}
				t.Fatalf("set inode flags=%v close=%v", err, closeErr)
			}
			t.Cleanup(func() {
				file, openErr := os.Open(path)
				if openErr == nil {
					_ = unix.IoctlSetPointerInt(int(file.Fd()), unix.FS_IOC_SETFLAGS, 0)
					_ = file.Close()
				}
			})
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
