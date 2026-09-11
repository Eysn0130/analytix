//go:build darwin || linux

package persistencefs

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func secureColdRootIdentityAndEmpty(capability frozenRootCapability) (string, error) {
	fd, err := secureOpenAbsoluteDirectory(capability.Root, false)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	identity, err := unixOpenedDirectoryIdentity(fd)
	if err != nil {
		return "", err
	}
	duplicate, err := unix.Dup(fd)
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(duplicate), "cold-root-recovery")
	if file == nil {
		_ = unix.Close(duplicate)
		return "", errors.New("startup journal cold root handle is invalid")
	}
	entries, readErr := file.ReadDir(1)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil {
		return "", errors.Join(readErr, closeErr)
	}
	if len(entries) != 0 {
		return "", errors.New("startup journal cold root is not empty at signed promotion recovery")
	}
	return identity, nil
}
