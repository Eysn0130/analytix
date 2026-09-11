//go:build linux

package finalauthority

import "golang.org/x/sys/unix"

func largeOpaquePlatformObjectSafe(fd int, _ unix.Stat_t) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size != 0 {
		return false
	}
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	return err == nil && flags == 0
}
