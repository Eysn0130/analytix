//go:build darwin

package securegeneration

import (
	"bytes"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

func privateDirectoryDetached(fd int) (bool, error) {
	if fd < 0 {
		return false, ErrInvalidInput
	}
	var path [unix.PathMax]byte
	_, _, errno := unix.Syscall(
		unix.SYS_FCNTL,
		uintptr(fd),
		uintptr(unix.F_GETPATH),
		uintptr(unsafe.Pointer(&path[0])),
	)
	if errno != 0 {
		if errors.Is(errno, unix.ENOENT) {
			return true, nil
		}
		return false, errno
	}
	end := bytes.IndexByte(path[:], 0)
	if end <= 0 || path[0] != '/' {
		return false, ErrUnsafeRoot
	}
	_, err := os.Lstat(string(path[:end]))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, err
}
