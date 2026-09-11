//go:build linux

package securegeneration

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

func clearCreatedExtendedSecurity(fd int) error {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return errors.Join(ErrUnsafeRoot, err)
	}
	if size == 0 {
		return nil
	}
	buffer := make([]byte, size)
	read, err := unix.Flistxattr(fd, buffer)
	if err != nil || read != size {
		return errors.Join(ErrUnsafeRoot, err)
	}
	for _, raw := range bytes.Split(buffer, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		if err := unix.Fremovexattr(fd, string(raw)); err != nil {
			return errors.Join(ErrUnsafeRoot, err)
		}
	}
	return nil
}

func privateExtendedSecuritySafe(fd int) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return false
	}
	return size == 0
}

func privateFileFlagsSafe(fd int) bool {
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	return err == nil && flags == 0
}
