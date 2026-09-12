//go:build linux

package securegeneration

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSecureGenerationLinuxAcceptsFreshPrivateStorageMetadata(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "synthetic-payload")
	if err := os.WriteFile(filePath, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, filePath} {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		flags, flagsErr := unix.IoctlGetInt(int(file.Fd()), unix.FS_IOC_GETFLAGS)
		xattrs, xattrsErr := unix.Flistxattr(int(file.Fd()), nil)
		safe := privateFileFlagsSafe(int(file.Fd())) && privateExtendedSecuritySafe(int(file.Fd()))
		closeErr := file.Close()
		if flagsErr != nil || xattrsErr != nil || closeErr != nil || !safe {
			t.Errorf("fresh private %s rejected: inode_flags=%#x xattr_bytes=%d flags_error=%v xattrs_error=%v close_error=%v", filepath.Base(path), flags, xattrs, flagsErr, xattrsErr, closeErr)
		}
	}
}
