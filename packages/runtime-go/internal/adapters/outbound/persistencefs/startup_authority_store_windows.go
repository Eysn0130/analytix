//go:build windows

package persistencefs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

type startupAuthorityRoot struct {
	path string
	id   privateCASWindowsFileIDInfo
}

func (root startupAuthorityRoot) pathValue() string { return root.path }

func captureStartupAuthorityRoot(path string) (startupAuthorityRoot, error) {
	handle, err := secureWindowsOpenAbsoluteDirectory(path, false)
	if err != nil {
		return startupAuthorityRoot{}, err
	}
	defer windows.CloseHandle(handle)
	identity, err := windowsOpenedDirectoryFileID(handle)
	if err != nil {
		return startupAuthorityRoot{}, errors.New("startup journal Windows namespace is unsafe")
	}
	if err := secureWindowsSyncDirectory(handle); err != nil {
		return startupAuthorityRoot{}, err
	}
	return startupAuthorityRoot{path: filepath.Clean(path), id: identity}, nil
}

func (root startupAuthorityRoot) open() (windows.Handle, error) {
	handle, err := secureWindowsOpenAbsoluteDirectory(root.path, false)
	if err != nil {
		return 0, err
	}
	identity, identityErr := windowsOpenedDirectoryFileID(handle)
	if identityErr != nil || identity != root.id {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("startup journal Windows namespace identity changed")
	}
	return handle, nil
}

func validateStartupAuthorityRoot(root startupAuthorityRoot) error {
	handle, err := root.open()
	if err != nil {
		return err
	}
	return windows.CloseHandle(handle)
}

func secureStartupAuthorityRead(root startupAuthorityRoot, name string, maxBytes int64) ([]byte, string, error) {
	if !secureWindowsComponent(name) || maxBytes <= 0 {
		return nil, "", errors.New("startup authority Windows named read input is invalid")
	}
	directory, err := root.open()
	if err != nil {
		return nil, "", err
	}
	defer windows.CloseHandle(directory)
	return secureStartupAuthorityReadAt(directory, name, maxBytes)
}

func secureStartupAuthorityReadAt(directory windows.Handle, name string, maxBytes int64) ([]byte, string, error) {
	handle, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if secureWindowsNotFound(err) {
		return nil, "", os.ErrNotExist
	}
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, "", errors.New("startup authority Windows file handle is invalid")
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, "", errors.New("startup authority Windows file is unsafe")
	}
	size := int64(uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow))
	if size <= 0 || size > maxBytes {
		return nil, "", errors.New("startup authority Windows file size is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != size {
		return nil, "", errors.New("startup authority Windows file read failed")
	}
	identity, err := windowsOpenedObjectIdentity(handle, false)
	if err != nil {
		return nil, "", errors.New("startup authority Windows file identity is unavailable")
	}
	return body, identity, nil
}

func secureStartupAuthorityWriteExclusive(root startupAuthorityRoot, name string, body []byte, maxBytes int64) error {
	if !secureWindowsComponent(name) || len(body) == 0 || int64(len(body)) > maxBytes {
		return errors.New("startup authority Windows named write input is invalid")
	}
	directory, err := root.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(directory)
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary, err := startupAuthorityCreateTempName(name, body, hex.EncodeToString(suffix))
	if err != nil {
		return err
	}
	handle, err := secureWindowsOpenRelative(directory, temporary, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("startup authority Windows temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = secureWindowsDeleteRelative(directory, temporary, "file")
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("startup authority Windows write was incomplete")
		}
		written += count
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("startup authority Windows staged write verification failed")
	}
	if err := startupWindowsRenameNoReplace(handle, directory, name); err != nil {
		if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
			return os.ErrExist
		}
		return err
	}
	tempExists = false
	if err := file.Sync(); err != nil {
		return err
	}
	if err := secureWindowsSyncDirectory(directory); err != nil {
		return err
	}
	written, _, err := secureStartupAuthorityReadAt(directory, name, maxBytes)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup authority Windows committed write verification failed")
	}
	return nil
}

func startupWindowsRenameNoReplace(handle, parent windows.Handle, target string) error {
	return startupWindowsRename(handle, parent, target, false)
}

func startupWindowsRename(handle, parent windows.Handle, target string, replace bool) error {
	targetName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	nameLength := (len(targetName) - 1) * 2
	dummy := windowsFileRenameInformation{}
	buffer := make([]byte, int(unsafe.Offsetof(dummy.FileName))+nameLength)
	info := (*windowsFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	if replace {
		info.ReplaceIfExists = 1
	}
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:nameLength/2:nameLength/2], targetName[:len(targetName)-1])
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

func secureStartupAuthorityCleanCreateResidue(root startupAuthorityRoot, name string, maxBytes int64, validate func([]byte) error) error {
	return cleanStartupAuthorityCreateResidue(root, name, maxBytes, validate)
}

func secureStartupAuthorityReadTempAt(directory windows.Handle, name string, maxBytes int64) ([]byte, error) {
	handle, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("startup authority Windows temp handle is invalid")
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, errors.New("startup authority Windows temp is unsafe")
	}
	size := int64(uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow))
	if size < 0 || size > maxBytes {
		return nil, errors.New("startup authority Windows temp size is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != size {
		return nil, errors.New("startup authority Windows temp read failed")
	}
	return body, nil
}

func startupAuthorityNamedComponent(name string) bool { return secureWindowsComponent(name) }
