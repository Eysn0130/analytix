//go:build darwin

package finalauthority

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

const privateDarwinPathconfCaseSensitive = 11

func platformSecurePrivateOpenExistingAbsoluteDirectory(path string) (int, bool, error) {
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
	if end <= 0 || resolved[0] != '/' || !privateDarwinCanonicalPathsEqual(string(resolved[:end]), path) {
		_ = unix.Close(fd)
		return -1, true, errors.New("private authority secure root resolved through an unsafe ancestor")
	}
	return fd, true, nil
}

func privateDarwinCanonicalPathsEqual(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if left == right {
		return true
	}
	caseSensitive, err := unix.Pathconf(right, privateDarwinPathconfCaseSensitive)
	return err == nil && caseSensitive == 0 && strings.EqualFold(left, right)
}
