//go:build linux

package persistencefs

import "golang.org/x/sys/unix"

func platformSeparateOwnerUnlinkFile(parent int, name string) error {
	return unix.Unlinkat(parent, name, 0)
}
