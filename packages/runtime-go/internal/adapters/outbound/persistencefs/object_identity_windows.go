//go:build windows

package persistencefs

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

func windowsOpenedObjectFileID(handle windows.Handle, directory bool) (privateCASWindowsFileIDInfo, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		return privateCASWindowsFileIDInfo{}, errors.New("semantic startup Windows object is unsafe")
	}
	var identity privateCASWindowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity)),
	); err != nil || identity.VolumeSerialNumber == 0 || identity.FileID == ([16]byte{}) {
		return privateCASWindowsFileIDInfo{}, errors.New("semantic startup Windows full object identity is unavailable")
	}
	return identity, nil
}

func windowsOpenedDirectoryFileID(handle windows.Handle) (privateCASWindowsFileIDInfo, error) {
	return windowsOpenedObjectFileID(handle, true)
}

func windowsOpenedObjectIdentity(handle windows.Handle, directory bool) (string, error) {
	identity, err := windowsOpenedObjectFileID(handle, directory)
	if err != nil {
		return "", err
	}
	return formatWindowsObjectIdentity(identity.VolumeSerialNumber, identity.FileID), nil
}
