//go:build !windows

package jobs

import (
	"errors"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

func syncChildRunDirectoryV1(path string, expected ChildRunFilesystemIdentityV1) error {
	handle, err := os.Open(path)
	if err != nil {
		return err
	}
	defer handle.Close()
	identity, err := childRunFilesystemIdentityFromHandleV1(handle, false)
	if err != nil || !sameChildRunFilesystemObjectV1(identity, expected) {
		return errors.New("child-run directory identity changed before sync")
	}
	return handle.Sync()
}

func childRunFilesystemIdentityFromHandleV1(handle *os.File, requireSingleLink bool) (ChildRunFilesystemIdentityV1, error) {
	if handle == nil {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(handle.Fd()), &stat); err != nil {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	linkCount := uint64(stat.Nlink)
	if linkCount == 0 {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	if requireSingleLink && linkCount != 1 {
		return ChildRunFilesystemIdentityV1{}, errors.New("hard-linked child-run objects are forbidden")
	}
	return ChildRunFilesystemIdentityV1{
		Scheme:    childRunFilesystemIdentityUnixV1,
		Device:    strconv.FormatUint(uint64(stat.Dev), 10),
		Inode:     strconv.FormatUint(uint64(stat.Ino), 10),
		LinkCount: linkCount,
	}, nil
}
