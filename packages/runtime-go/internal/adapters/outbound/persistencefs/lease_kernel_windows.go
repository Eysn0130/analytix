//go:build windows

package persistencefs

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	persistenceCreateMutexW = persistenceKernel32.NewProc("CreateMutexW")
	persistenceCloseHandle  = persistenceKernel32.NewProc("CloseHandle")
)

const persistenceAlreadyExists = syscall.Errno(183)

// Windows conservatively serializes runtime processes across login sessions
// with a kernel-named object. The in-process manager still permits
// non-overlapping roots. Its DACL and multi-session behavior remain subject to
// real Windows host validation before any cross-process concurrency claim.
type windowsKernelArbiter struct {
	handle syscall.Handle
}

func acquirePlatformKernelArbiter() (platformKernelArbiter, error) {
	name, err := syscall.UTF16PtrFromString(`Global\AnalytixPersistenceRuntimeV1`)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := persistenceCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, callErr
	}
	if errors.Is(callErr, persistenceAlreadyExists) {
		_, _, _ = persistenceCloseHandle.Call(handle)
		return nil, ErrPersistenceInUse
	}
	return &windowsKernelArbiter{handle: syscall.Handle(handle)}, nil
}

func (arbiter *windowsKernelArbiter) Acquire([]kernelRangeSpec) error { return nil }
func (arbiter *windowsKernelArbiter) Release(kernelRangeSpec) error   { return nil }

func (arbiter *windowsKernelArbiter) Validate() error {
	if arbiter == nil || arbiter.handle == 0 {
		return errors.New("persistence kernel arbiter is unavailable")
	}
	return nil
}

func (arbiter *windowsKernelArbiter) Close() error {
	if arbiter == nil || arbiter.handle == 0 {
		return nil
	}
	handle := uintptr(arbiter.handle)
	arbiter.handle = 0
	closed, _, closeErr := persistenceCloseHandle.Call(handle)
	if closed == 0 {
		return closeErr
	}
	return nil
}
