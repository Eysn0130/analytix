//go:build darwin || linux

package filestore

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func checkpointRootIdentity(path string) (string, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return "", err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Dev == 0 || stat.Ino == 0 {
		return "", errors.New("checkpoint root directory identity is invalid")
	}
	return fmt.Sprintf("unix:%d:%d", uint64(stat.Dev), uint64(stat.Ino)), nil
}
