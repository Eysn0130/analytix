//go:build !windows

package persistencefs

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

const persistenceKernelDevice = "/dev/zero"

type unixKernelArbiter struct {
	fd       int
	identity unix.Stat_t
}

func acquirePlatformKernelArbiter() (platformKernelArbiter, error) {
	fd, err := unix.Open(persistenceKernelDevice, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	var opened unix.Stat_t
	var current unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if err := unix.Lstat(persistenceKernelDevice, &current); err != nil ||
		opened.Mode&unix.S_IFMT != unix.S_IFCHR || opened.Uid != 0 ||
		opened.Dev != current.Dev || opened.Ino != current.Ino || opened.Rdev != current.Rdev {
		_ = unix.Close(fd)
		return nil, errors.New("persistence kernel arbitration device is not trusted")
	}
	return &unixKernelArbiter{fd: fd, identity: opened}, nil
}

func (arbiter *unixKernelArbiter) Acquire(ranges []kernelRangeSpec) error {
	acquired := make([]kernelRangeSpec, 0, len(ranges))
	for _, spec := range ranges {
		lockType := int16(unix.F_RDLCK)
		if spec.exclusive {
			lockType = unix.F_WRLCK
		}
		lock := unix.Flock_t{Start: spec.offset, Len: 1, Type: lockType, Whence: 0}
		if err := unix.FcntlFlock(uintptr(arbiter.fd), persistenceKernelLockCommand, &lock); err != nil {
			var rollbackErr error
			for _, locked := range acquired {
				rollbackErr = errors.Join(rollbackErr, arbiter.Release(locked))
			}
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EACCES) {
				return errors.Join(ErrPersistenceInUse, rollbackErr)
			}
			return errors.Join(fmt.Errorf("acquire persistence kernel range: %w", err), rollbackErr)
		}
		acquired = append(acquired, spec)
	}
	return nil
}

func (arbiter *unixKernelArbiter) Release(spec kernelRangeSpec) error {
	lock := unix.Flock_t{Start: spec.offset, Len: 1, Type: unix.F_UNLCK, Whence: 0}
	if err := unix.FcntlFlock(uintptr(arbiter.fd), persistenceKernelLockCommand, &lock); err != nil {
		return fmt.Errorf("release persistence kernel range: %w", err)
	}
	return nil
}

func (arbiter *unixKernelArbiter) Validate() error {
	if arbiter == nil || arbiter.fd < 0 {
		return errors.New("persistence kernel arbiter is unavailable")
	}
	var opened unix.Stat_t
	if err := unix.Fstat(arbiter.fd, &opened); err != nil || opened.Mode&unix.S_IFMT != unix.S_IFCHR ||
		opened.Uid != 0 || opened.Dev != arbiter.identity.Dev || opened.Ino != arbiter.identity.Ino || opened.Rdev != arbiter.identity.Rdev {
		return errors.New("persistence kernel arbiter identity changed")
	}
	return nil
}

func (arbiter *unixKernelArbiter) Close() error {
	if arbiter == nil || arbiter.fd < 0 {
		return nil
	}
	fd := arbiter.fd
	arbiter.fd = -1
	return unix.Close(fd)
}
