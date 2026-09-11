//go:build windows

package evidenceregistry

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	registryMoveFileReplaceExisting = 0x1
	registryMoveFileWriteThrough    = 0x8
)

var registryMoveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func atomicReplaceRegistryFile(sourcePath string, destinationPath string) error {
	source, err := syscall.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := syscall.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	result, _, callErr := registryMoveFileExW.Call(
		uintptr(unsafe.Pointer(source)), uintptr(unsafe.Pointer(destination)),
		uintptr(registryMoveFileReplaceExisting|registryMoveFileWriteThrough),
	)
	if result != 0 {
		return nil
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return callErr
	}
	return syscall.EINVAL
}

func syncRegistryDirectory(string) error { return nil }
