//go:build windows

package privatecas

import (
	"errors"
	"unsafe"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/windows"
)

type testWindowsFileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

func platformTestDirectoryIdentity(path string) (privatecasport.DirectoryIdentity, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return privatecasport.DirectoryIdentity{}, err
	}
	handle, err := windows.CreateFile(
		name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0,
	)
	if err != nil {
		return privatecasport.DirectoryIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	var info testWindowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		return privatecasport.DirectoryIdentity{}, errors.New("test private CAS Windows directory identity is unavailable")
	}
	return privatecasport.DirectoryIdentity{
		Kind: privatecasport.DirectoryIdentityWindows, VolumeSerial: info.VolumeSerialNumber, FileID: info.FileID,
	}, nil
}
