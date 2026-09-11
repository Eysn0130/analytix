//go:build linux

package filestore

import "golang.org/x/sys/unix"

func renameConditionalUnixNoReplace(sourceParent int, source string, destinationParent int, destination string) error {
	return unix.Renameat2(sourceParent, source, destinationParent, destination, unix.RENAME_NOREPLACE)
}

func sameConditionalUnixStableStat(left, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino && left.Mode == right.Mode &&
		left.Nlink == right.Nlink && left.Size == right.Size &&
		left.Mtim == right.Mtim && left.Ctim == right.Ctim
}
