//go:build darwin

package persistencefs

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinSecureAbsoluteDirectoryOpenRejectsAncestorSymlink(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real-root")
	nested := filepath.Join(realRoot, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Fatal(err)
	}
	if fd, err := secureOpenAbsoluteDirectory(filepath.Join(alias, "nested"), false); err == nil {
		_ = unix.Close(fd)
		t.Fatal("ancestor symlink was accepted by the Darwin absolute-directory fast path")
	}
	fd, err := secureOpenAbsoluteDirectory(nested, false)
	if err != nil {
		t.Fatalf("real directory was rejected: %v", err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
}
