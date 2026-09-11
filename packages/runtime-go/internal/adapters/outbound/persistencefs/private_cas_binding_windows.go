//go:build windows

package persistencefs

import (
	"errors"
	"unsafe"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/windows"
)

type privateCASWindowsFileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

func platformStrongDirectoryIdentity(path string) (privatecasport.DirectoryIdentity, string, error) {
	handle, err := secureWindowsOpenAbsoluteDirectory(path, false)
	if err != nil {
		return privatecasport.DirectoryIdentity{}, "", err
	}
	defer windows.CloseHandle(handle)
	var full privateCASWindowsFileIDInfo
	if windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&full)), uint32(unsafe.Sizeof(full)),
	) != nil || full.VolumeSerialNumber == 0 || full.FileID == [16]byte{} {
		return privatecasport.DirectoryIdentity{}, "", errors.New("private CAS Windows persistence root identity is unavailable")
	}
	identity := privatecasport.DirectoryIdentity{
		Kind: privatecasport.DirectoryIdentityWindows, VolumeSerial: full.VolumeSerialNumber, FileID: full.FileID,
	}
	canonical, err := formatStrongDirectoryIdentity(identity)
	return identity, canonical, err
}
