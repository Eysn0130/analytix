//go:build darwin

package filestore

import "golang.org/x/sys/unix"

func installAtomicUnixText(parent int, tempName, targetName string, expectedExists bool) error {
	flag := uint32(unix.RENAME_EXCL)
	if expectedExists {
		flag = unix.RENAME_SWAP
	}
	return unix.RenameatxNp(parent, tempName, parent, targetName, flag)
}

func rollbackAtomicUnixTextSwap(parent int, tempName, targetName string) error {
	return unix.RenameatxNp(parent, tempName, parent, targetName, unix.RENAME_SWAP)
}
