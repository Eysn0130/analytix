//go:build linux

package secureconfigfs

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxFilesystemAllowlistRequiresMagicAndExactMountType(t *testing.T) {
	for _, test := range []struct {
		magic int64
		kind  string
		want  bool
	}{
		{unix.EXT4_SUPER_MAGIC, "ext4", true},
		{unix.XFS_SUPER_MAGIC, "xfs", true},
		{unix.EXT4_SUPER_MAGIC, "ext2", false},
		{unix.EXT4_SUPER_MAGIC, "ext3", false},
		{unix.EXT4_SUPER_MAGIC, "xfs", false},
		{unix.XFS_SUPER_MAGIC, "ext4", false},
		{0x794c7630, "overlay", false},
		{0x01021994, "tmpfs", false},
		{0x6969, "nfs", false},
		{0, "unknown", false},
	} {
		if got := linuxFilesystemAllowed(test.magic, test.kind); got != test.want {
			t.Fatalf("allowlist(%x,%q)=%v want %v", test.magic, test.kind, got, test.want)
		}
	}
}

func TestMountFilesystemTypeForIDIsExactAndFailClosed(t *testing.T) {
	valid := []byte("36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n37 25 8:2 / /data rw - xfs /dev/sdb1 rw\n")
	if kind, err := mountFilesystemTypeForID(valid, 36); err != nil || kind != "ext4" {
		t.Fatalf("ext4 mount lookup = %q, %v", kind, err)
	}
	if kind, err := mountFilesystemTypeForID(valid, 37); err != nil || kind != "xfs" {
		t.Fatalf("XFS mount lookup = %q, %v", kind, err)
	}
	for name, body := range map[string][]byte{
		"missing":      valid,
		"duplicate":    []byte("36 25 8:1 / / rw - ext4 /dev/a rw\n36 25 8:2 / /x rw - ext4 /dev/b rw\n"),
		"no separator": []byte("36 25 8:1 / / rw ext4 /dev/a rw extra\n"),
		"NUL":          append(append([]byte(nil), valid...), 0),
	} {
		t.Run(name, func(t *testing.T) {
			target := uint64(36)
			if name == "missing" {
				target = 99
			}
			if _, err := mountFilesystemTypeForID(body, target); err == nil {
				t.Fatal("unsafe mountinfo was accepted")
			}
		})
	}
}
