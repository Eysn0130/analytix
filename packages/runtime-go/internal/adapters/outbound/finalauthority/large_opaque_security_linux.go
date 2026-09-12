//go:build linux

package finalauthority

import "golang.org/x/sys/unix"

func largeOpaquePlatformObjectSafe(fd int, _ unix.Stat_t) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size != 0 {
		return false
	}
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	return err == nil && largeOpaqueStorageFlagsSafe(flags)
}

func largeOpaqueStorageFlagsSafe(flags int) bool {
	// Linux UAPI FS_EXTENT_FL is ext4's ordinary storage-format marker.
	// Do not admit access/mutation policy flags or any unknown inode flags.
	const fsExtentFlag = 0x00080000
	return flags & ^fsExtentFlag == 0
}
