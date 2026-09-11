//go:build windows

package jobs

import (
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

func syncChildRunDirectoryV1(path string, expected ChildRunFilesystemIdentityV1) error {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), "child-run-directory-sync")
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("child-run directory sync handle is invalid")
	}
	defer file.Close()
	identity, err := childRunFilesystemIdentityFromHandleV1(file, false)
	if err != nil || !sameChildRunFilesystemObjectV1(identity, expected) {
		return errors.New("child-run directory identity changed before sync")
	}
	return windows.FlushFileBuffers(handle)
}

type childRunWindowsFileIDInfoV1 struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

func childRunFilesystemIdentityFromHandleV1(handle *os.File, requireSingleLink bool) (ChildRunFilesystemIdentityV1, error) {
	if handle == nil {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	windowsHandle := windows.Handle(handle.Fd())
	var fileID childRunWindowsFileIDInfoV1
	if err := windows.GetFileInformationByHandleEx(
		windowsHandle,
		windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&fileID)),
		uint32(unsafe.Sizeof(fileID)),
	); err != nil {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	var attributes windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windowsHandle, &attributes); err != nil {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	if attributes.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ChildRunFilesystemIdentityV1{}, errors.New("reparse-point child-run objects are forbidden")
	}
	linkCount := uint64(attributes.NumberOfLinks)
	if linkCount == 0 {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	if requireSingleLink && linkCount != 1 {
		return ChildRunFilesystemIdentityV1{}, errors.New("hard-linked child-run objects are forbidden")
	}
	allZero := true
	for _, value := range fileID.FileID {
		if value != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return ChildRunFilesystemIdentityV1{}, ErrChildRunFilesystemIdentityUnavailable
	}
	return ChildRunFilesystemIdentityV1{
		Scheme:    childRunFilesystemIdentityWindowsV1,
		Device:    strconv.FormatUint(fileID.VolumeSerialNumber, 10),
		Inode:     hex.EncodeToString(fileID.FileID[:]),
		LinkCount: linkCount,
	}, nil
}
