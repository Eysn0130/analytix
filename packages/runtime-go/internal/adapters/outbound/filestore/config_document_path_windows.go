//go:build windows

package filestore

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func readRuntimeConfigPathSnapshotV1(path string, maxBytes int64) ([]byte, bool, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, errors.New("runtime configuration path is invalid")
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN,
		0,
	)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("runtime configuration path cannot be opened without following links")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, false, errors.New("runtime configuration handle is invalid")
	}
	defer file.Close()
	var beforeHandle windows.ByHandleFileInformation
	before, statErr := file.Stat()
	if statErr != nil || windows.GetFileInformationByHandle(handle, &beforeHandle) != nil ||
		beforeHandle.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 ||
		!before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maxBytes {
		return nil, false, errors.New("runtime configuration path is not a bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(raw)) != before.Size() || int64(len(raw)) > maxBytes {
		return nil, false, errors.New("runtime configuration changed during snapshot read")
	}
	var afterHandle windows.ByHandleFileInformation
	after, statErr := file.Stat()
	if statErr != nil || windows.GetFileInformationByHandle(handle, &afterHandle) != nil ||
		!os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) || !sameRuntimeConfigWindowsGeneration(beforeHandle, afterHandle) {
		return nil, false, errors.New("runtime configuration changed during snapshot read")
	}
	return raw, true, nil
}

func sameRuntimeConfigWindowsGeneration(left, right windows.ByHandleFileInformation) bool {
	return left.FileAttributes == right.FileAttributes &&
		left.CreationTime == right.CreationTime && left.LastWriteTime == right.LastWriteTime &&
		left.VolumeSerialNumber == right.VolumeSerialNumber && left.FileSizeHigh == right.FileSizeHigh &&
		left.FileSizeLow == right.FileSizeLow && left.NumberOfLinks == right.NumberOfLinks &&
		left.FileIndexHigh == right.FileIndexHigh && left.FileIndexLow == right.FileIndexLow
}
