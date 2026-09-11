//go:build windows

package evidenceregistry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func acquireRegistryFileLock(ctx context.Context, path string) (func() error, error) {
	directoryHandle, err := openRegistryDirectoryHandle(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(directoryHandle)
	fileHandle := windows.InvalidHandle
	for attempt := 0; attempt < 4; attempt++ {
		fileHandle, err = openRegistryLockRelative(directoryHandle, filepath.Base(path), windows.FILE_OPEN)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
			return nil, err
		}
		fileHandle, err = openRegistryLockRelative(directoryHandle, filepath.Base(path), windows.FILE_CREATE)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
			return nil, err
		}
	}
	if err != nil {
		return nil, errors.New("evidence registry lock creation race did not converge")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(fileHandle, &info); err != nil ||
		info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		_ = windows.CloseHandle(fileHandle)
		return nil, errors.New("evidence registry lock file has unsafe identity")
	}
	file := os.NewFile(uintptr(fileHandle), path)
	if file == nil {
		_ = windows.CloseHandle(fileHandle)
		return nil, errors.New("evidence registry lock file handle is invalid")
	}
	overlapped := &windows.Overlapped{}
	for {
		err = windows.LockFileEx(fileHandle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
		if err == nil {
			return func() error {
				return errors.Join(windows.UnlockFileEx(fileHandle, 0, 1, 0, overlapped), file.Close())
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-contextDone(ctx):
			_ = file.Close()
			return nil, contextError(ctx)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func openRegistryDirectoryHandle(path string) (windows.Handle, error) {
	ntPath, err := registryNTPath(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	objectName, err := windows.NewNTUnicodeString(ntPath)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: 0,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	var allocationSize int64
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, attributes, &status, &allocationSize, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return windows.InvalidHandle, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, errors.New("evidence registry root directory has unsafe identity")
	}
	return handle, nil
}

func openRegistryLockRelative(root windows.Handle, name string, disposition uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: root,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	var allocationSize int64
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE, attributes, &status, &allocationSize,
		0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, disposition,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return handle, nil
}

func registryNTPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, `\\?\`) || strings.HasPrefix(lower, `\\.\`) || strings.HasPrefix(lower, `\??\`) {
		return "", errors.New("evidence registry device namespace paths are not supported")
	}
	if !filepath.IsAbs(cleaned) {
		return "", errors.New("evidence registry path must be absolute")
	}
	if strings.HasPrefix(cleaned, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(cleaned, `\\`), nil
	}
	volume := filepath.VolumeName(cleaned)
	if len(volume) != 2 || volume[1] != ':' {
		return "", errors.New("evidence registry path must be a canonical drive or UNC path")
	}
	return `\??\` + cleaned, nil
}

func contextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return make(chan struct{})
	}
	return ctx.Done()
}
