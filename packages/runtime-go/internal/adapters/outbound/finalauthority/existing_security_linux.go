//go:build linux

package finalauthority

import (
	"bytes"

	"golang.org/x/sys/unix"
)

func existingPrivateAuthorityExtendedSecuritySafe(fd int) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return false
	}
	if size == 0 {
		return true
	}
	buffer := make([]byte, size)
	read, err := unix.Flistxattr(fd, buffer)
	if err != nil || read != size {
		return false
	}
	for _, name := range bytes.Split(buffer, []byte{0}) {
		if bytes.Equal(name, []byte("system.posix_acl_access")) || bytes.Equal(name, []byte("system.posix_acl_default")) {
			return false
		}
	}
	return true
}
