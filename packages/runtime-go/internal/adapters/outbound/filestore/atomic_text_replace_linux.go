//go:build linux

package filestore

import "golang.org/x/sys/unix"

func installAtomicUnixText(parent int, tempName, targetName string, expectedExists bool) error {
	flag := uint(unix.RENAME_NOREPLACE)
	if expectedExists {
		flag = unix.RENAME_EXCHANGE
	}
	return unix.Renameat2(parent, tempName, parent, targetName, flag)
}

func rollbackAtomicUnixTextSwap(parent int, tempName, targetName string) error {
	return unix.Renameat2(parent, tempName, parent, targetName, unix.RENAME_EXCHANGE)
}
