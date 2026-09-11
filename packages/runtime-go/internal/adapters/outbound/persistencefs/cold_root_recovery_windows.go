//go:build windows

package persistencefs

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func secureColdRootIdentityAndEmpty(capability frozenRootCapability) (string, error) {
	handle, err := secureWindowsOpenAbsoluteDirectory(capability.Root, false)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	identity, err := windowsOpenedDirectoryIdentity(handle)
	if err != nil {
		return "", err
	}
	duplicate, err := duplicateWindowsHandle(handle)
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(duplicate), "cold-root-recovery")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return "", errors.New("startup journal cold Windows root handle is invalid")
	}
	entries, readErr := file.ReadDir(1)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil {
		return "", errors.Join(readErr, closeErr)
	}
	if len(entries) != 0 {
		return "", errors.New("startup journal cold Windows root is not empty at signed promotion recovery")
	}
	return identity, nil
}
