//go:build windows

package persistencefs

import "golang.org/x/sys/windows"

func platformDirectoryIdentity(path string) (string, error) {
	handle, err := secureWindowsOpenAbsoluteDirectory(path, false)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	return windowsOpenedObjectIdentity(handle, true)
}
