//go:build darwin

package finalauthority

import (
	"bytes"
	"errors"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

func largeOpaquePlatformExactBasename(_ int, fd int, expected string) error {
	if fd < 0 || filepath.Base(expected) != expected {
		return errors.New("large opaque exact basename input is invalid")
	}
	var path [unix.PathMax]byte
	defer clear(path[:])
	_, _, errno := unix.Syscall(
		unix.SYS_FCNTL,
		uintptr(fd),
		uintptr(unix.F_GETPATH),
		uintptr(unsafe.Pointer(&path[0])),
	)
	if errno != 0 {
		return errno
	}
	end := bytes.IndexByte(path[:], 0)
	if end <= 0 || path[0] != '/' || filepath.Base(string(path[:end])) != expected {
		return errors.New("large opaque kernel basename is not exact")
	}
	return nil
}
