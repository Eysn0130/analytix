//go:build windows

package eventlog

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func atomicReplaceFile(sourcePath string, destinationPath string) error {
	source, err := syscall.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := syscall.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	result, _, callErr := moveFileExW.Call(
		uintptr(unsafe.Pointer(source)),
		uintptr(unsafe.Pointer(destination)),
		uintptr(moveFileReplaceExisting|moveFileWriteThrough),
	)
	if result != 0 {
		return nil
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return callErr
	}
	return syscall.EINVAL
}

// MoveFileExW with MOVEFILE_WRITE_THROUGH supplies the target-native durability
// barrier; opening a directory for fsync is not a portable Windows operation.
func syncParentDirectory(string) error { return nil }
