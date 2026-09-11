//go:build darwin || linux

package persistencefs

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func securePrepareStartupAuthorityBase(path string) error {
	directory, err := secureOpenAbsoluteDirectory(path, true)
	if err != nil {
		return err
	}
	defer unix.Close(directory)
	if err := unix.Fchmod(directory, 0o700); err != nil {
		return err
	}
	return unix.Fsync(directory)
}

func securePrepareStartupAuthorityNamespace(path string) error {
	parent, err := secureOpenAbsoluteDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	name := filepath.Base(path)
	if !startupAuthorityNamedComponent(name) {
		return errors.New("startup journal namespace name is invalid")
	}
	if err := unix.Mkdirat(parent, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return err
	}
	directory, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer unix.Close(directory)
	var stat unix.Stat_t
	if err := unix.Fstat(directory, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 {
		return errors.New("startup journal namespace is not a private directory")
	}
	return errors.Join(unix.Fsync(directory), unix.Fsync(parent))
}
