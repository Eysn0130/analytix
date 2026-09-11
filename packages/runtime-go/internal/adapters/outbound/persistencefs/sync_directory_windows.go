//go:build windows

package persistencefs

import (
	"errors"

	"golang.org/x/sys/windows"
)

func syncDirectory(path string) error {
	handle, err := secureWindowsOpenAbsoluteDirectory(path, false)
	if err != nil {
		return err
	}
	return errors.Join(secureWindowsSyncDirectory(handle), windows.CloseHandle(handle))
}
