//go:build linux

package persistencefs

import "golang.org/x/sys/unix"

func unixStatModTimeNano(stat unix.Stat_t) int64 {
	return stat.Mtim.Sec*1_000_000_000 + stat.Mtim.Nsec
}

func legacyCheckpointUnixRenameNoReplace(sourceParent int, source string, targetParent int, target string) error {
	return unix.Renameat2(sourceParent, source, targetParent, target, unix.RENAME_NOREPLACE)
}
