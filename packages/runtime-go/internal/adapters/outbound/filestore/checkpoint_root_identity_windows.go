//go:build windows

package filestore

import (
	"encoding/hex"
	"fmt"

	"golang.org/x/sys/windows"
)

func checkpointRootIdentity(path string) (string, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	identity, err := atomicWindowsHandleFileID(handle, true)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("windows:%d:%s", identity.VolumeSerialNumber, hex.EncodeToString(identity.FileID[:])), nil
}
