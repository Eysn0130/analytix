//go:build darwin

package processauthority

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const darwinSessionCleanupDeadline = 5 * time.Second

func darwinDirectoryAuthorityMatches(opened *pinnedDarwinObject, authority *os.File) bool {
	if opened == nil || opened.file == nil || authority == nil {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(authority.Fd()), &current) == nil && current.Mode&unix.S_IFMT == unix.S_IFDIR &&
		current.Uid == uint32(os.Geteuid()) && current.Mode&0o077 == 0 && current.Dev == opened.stat.Dev &&
		current.Ino == opened.stat.Ino && current.Mode == opened.stat.Mode && current.Uid == opened.stat.Uid
}

func darwinDirectoryPathMatchesAuthority(path string, authority *os.File) bool {
	opened, err := openPinnedDarwinObject(path, true)
	if err != nil {
		return false
	}
	defer opened.file.Close()
	return darwinDirectoryAuthorityMatches(opened, authority)
}

func componentDarwinFileIdentity(stat unix.Stat_t) [8]uint64 {
	return [8]uint64{
		uint64(stat.Dev), stat.Ino, uint64(stat.Size), uint64(stat.Mode),
		uint64(stat.Mtim.Sec), uint64(stat.Mtim.Nsec), uint64(stat.Ctim.Sec), uint64(stat.Ctim.Nsec),
	}
}
