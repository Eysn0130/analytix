//go:build darwin

package persistencefs

import (
	"bytes"
	"errors"
	"unsafe"

	"golang.org/x/sys/unix"
)

func platformSecureOpenExistingAbsoluteDirectory(path string) (int, bool, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, true, err
	}
	var resolved [unix.PathMax]byte
	_, _, errno := unix.Syscall(
		unix.SYS_FCNTL,
		uintptr(fd),
		uintptr(unix.F_GETPATH),
		uintptr(unsafe.Pointer(&resolved[0])),
	)
	if errno != 0 {
		_ = unix.Close(fd)
		return -1, true, errno
	}
	end := bytes.IndexByte(resolved[:], 0)
	var stat unix.Stat_t
	if end <= 0 || resolved[0] != '/' || unix.Fstat(fd, &stat) != nil ||
		!canonicalPathsEqualForDevice(string(resolved[:end]), path, uint64(stat.Dev)) {
		_ = unix.Close(fd)
		return -1, true, errors.New("semantic startup secure root resolved through an unsafe ancestor")
	}
	return fd, true, nil
}
