//go:build darwin || linux

package persistencefs

import (
	"errors"

	"golang.org/x/sys/unix"
)

type separateOwnerUnixRootPin struct {
	fd         int
	rootExists bool
}

func platformPinSeparateOwnerRoot(
	anchor string,
	rootExists bool,
	expected separateOwnerPathBindingV1,
) (separateOwnerRootPin, error) {
	fd, err := secureOpenAbsoluteDirectory(anchor, false)
	if err != nil {
		return nil, err
	}
	pin := &separateOwnerUnixRootPin{fd: fd, rootExists: rootExists}
	if err := pin.Validate(expected); err != nil {
		_ = pin.Close()
		return nil, err
	}
	return pin, nil
}

func (pin *separateOwnerUnixRootPin) Validate(expected separateOwnerPathBindingV1) error {
	if pin == nil || pin.fd < 0 {
		return errors.New("separate-owner Unix pinned root is unavailable")
	}
	state, err := separateOwnerUnixDirectoryState(pin.fd, false)
	if err != nil || state.Identity != expected.Identity || state.SecurityDigest != expected.SecurityDigest {
		return errors.New("separate-owner Unix pinned root identity or permissions changed")
	}
	return nil
}

func (pin *separateOwnerUnixRootPin) OpenRoot() (*startupPrivateDirectory, error) {
	if pin == nil || pin.fd < 0 || !pin.rootExists {
		return nil, errors.New("separate-owner Unix pinned root is unavailable")
	}
	fd, err := unix.Openat(pin.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	identity, err := unixOpenedDirectoryIdentity(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &startupPrivateDirectory{fd: fd, identity: identity}, nil
}

func (pin *separateOwnerUnixRootPin) Close() error {
	if pin == nil || pin.fd < 0 {
		return nil
	}
	fd := pin.fd
	pin.fd = -1
	return unix.Close(fd)
}
