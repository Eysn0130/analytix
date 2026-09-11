//go:build darwin

package persistencefs

import "golang.org/x/sys/unix"

const separateOwnerDarwinATUnique = 0x8000

func platformSeparateOwnerUnlinkFile(parent int, name string) error {
	return unix.Unlinkat(parent, name, separateOwnerDarwinATUnique)
}
