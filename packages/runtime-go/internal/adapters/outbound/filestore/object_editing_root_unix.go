//go:build darwin || linux

package filestore

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

func objectEditingPrivateRootIdentity(root string) (string, error) {
	fd, _, missing, err := openAtomicUnixParent(filepath.Join(root, ".object-root-probe"), false)
	if err != nil || missing {
		return "", objectediting.ErrForbidden
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o777 != 0o700 {
		return "", objectediting.ErrForbidden
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}

func objectEditingPrivateReceipt(path string) error { return objectEditingPrivateFile(path, 16384) }

func objectEditingPrivateFile(path string, maxBytes int64) error {
	parent, base, missing, err := openAtomicUnixParent(path, false)
	if err != nil || missing {
		return objectediting.ErrForbidden
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return objectediting.ErrPersistence
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o777 != 0o600 || stat.Nlink != 1 || stat.Size < 0 || stat.Size > maxBytes {
		return objectediting.ErrPersistence
	}
	return nil
}

func objectEditingSyncTarget(path string) error {
	parent, base, missing, err := openAtomicUnixParent(path, false)
	if err != nil || missing {
		return objectediting.ErrPersistence
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return objectediting.ErrPersistence
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || unix.Fsync(fd) != nil || unix.Fsync(parent) != nil {
		return objectediting.ErrPersistence
	}
	return nil
}
