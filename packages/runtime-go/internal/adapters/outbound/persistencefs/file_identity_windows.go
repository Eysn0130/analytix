//go:build windows

package persistencefs

import (
	"os"

	"golang.org/x/sys/windows"
)

func regularFileIdentity(file *os.File, _ os.FileInfo) (string, uint64, error) {
	handle := windows.Handle(file.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return "", 0, err
	}
	identity, err := windowsOpenedObjectIdentity(handle, false)
	if err != nil {
		return "", 0, err
	}
	return identity, uint64(info.NumberOfLinks), nil
}
