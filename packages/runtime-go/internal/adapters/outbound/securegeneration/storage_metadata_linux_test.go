//go:build linux

package securegeneration

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPrivateStorageFlagsOnlyAllowExtentFormat(t *testing.T) {
	const extent = 0x00080000
	for _, format := range []int{0, extent} {
		if !privateStorageFlagsSafe(format) {
			t.Fatalf("ordinary storage format %#x rejected", format)
		}
		for bit := uint(0); bit < 32; bit++ {
			flag := int(uint32(1) << bit)
			if flag != extent && privateStorageFlagsSafe(format|flag) {
				t.Errorf("non-format inode flag %#x accepted with format %#x", flag, format)
			}
		}
	}
	if privateStorageFlagsSafe(-1) || privateFileFlagsSafe(-1) {
		t.Fatal("invalid flags or descriptor accepted")
	}
}

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
