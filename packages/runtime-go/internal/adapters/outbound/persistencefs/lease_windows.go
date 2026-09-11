//go:build windows

package persistencefs

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	persistenceKernel32     = syscall.NewLazyDLL("kernel32.dll")
	persistenceLockFileEx   = persistenceKernel32.NewProc("LockFileEx")
	persistenceUnlockFileEx = persistenceKernel32.NewProc("UnlockFileEx")
)

const (
	persistenceLockFailImmediately = 0x00000001
	persistenceLockExclusive       = 0x00000002
	persistenceLockViolation       = syscall.Errno(33)
)

func tryPlatformFileLock(file *os.File, exclusive bool) (bool, error) {
	overlapped := &syscall.Overlapped{}
	flags := uintptr(persistenceLockFailImmediately)
	if exclusive {
		flags |= persistenceLockExclusive
	}
	result, _, callErr := persistenceLockFileEx.Call(
		file.Fd(),
		flags,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	if result != 0 {
		return true, nil
	}
	if errors.Is(callErr, persistenceLockViolation) {
		return false, nil
	}
	return false, callErr
}

func unlockPlatformFile(file *os.File) error {
	overlapped := &syscall.Overlapped{}
	result, _, callErr := persistenceUnlockFileEx.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(overlapped)))
	if result == 0 {
		return callErr
	}
	return nil
}
