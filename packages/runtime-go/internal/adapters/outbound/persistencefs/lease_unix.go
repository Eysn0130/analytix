//go:build !windows

package persistencefs

import (
	"errors"
	"os"
	"syscall"
)

func tryPlatformFileLock(file *os.File, exclusive bool) (bool, error) {
	operation := syscall.LOCK_SH
	if exclusive {
		operation = syscall.LOCK_EX
	}
	err := syscall.Flock(int(file.Fd()), operation|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockPlatformFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
