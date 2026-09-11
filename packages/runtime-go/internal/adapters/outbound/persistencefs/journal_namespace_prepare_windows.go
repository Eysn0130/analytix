//go:build windows

package persistencefs

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func securePrepareStartupAuthorityBase(path string) error {
	directory, err := secureWindowsOpenAbsoluteDirectory(path, true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(directory)
	return secureWindowsSyncDirectory(directory)
}

func securePrepareStartupAuthorityNamespace(path string) error {
	parent, err := secureWindowsOpenAbsoluteDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	name := filepath.Base(path)
	if !secureWindowsComponent(name) {
		return errors.New("startup journal Windows namespace name is invalid")
	}
	directory, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, true)
	if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
		directory, err = secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, true)
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(directory)
	return errors.Join(secureWindowsSyncDirectory(directory), secureWindowsSyncDirectory(parent))
}
