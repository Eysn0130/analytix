//go:build darwin || linux

package persistencefs

import (
	"errors"

	"golang.org/x/sys/unix"
)

func platformDirectoryIdentity(path string) (string, error) {
	fd, err := secureOpenAbsoluteDirectory(path, false)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", errors.New("persistence directory identity is unavailable")
	}
	return unixDirectoryIdentity(stat), nil
}

func unixDirectoryIdentity(stat unix.Stat_t) string {
	return formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino))
}
