//go:build windows

package secretstore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func readPrivateCommittedFile(path string, maximum int64) ([]byte, error) {
	if err := validatePrivateWindowsPath(path); err != nil {
		return nil, err
	}
	if err := ensurePrivateStoreDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	parent, err := openWindowsDirectory(filepath.Dir(path), privateWindowsDirectoryName(filepath.Dir(path)))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)
	if err := validatePrivateWindowsExactName(path); err != nil {
		return nil, err
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), filepath.Base(path))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private file open failed")
	}
	defer file.Close()
	fileType, err := windows.GetFileType(handle)
	if err != nil {
		return nil, err
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return nil, err
	}
	if err := validateWindowsPrivateFileHandle(fileType, information, maximum); err != nil {
		return nil, err
	}
	if err := validatePrivateWindowsSecurity(handle); err != nil {
		if hardenPrivateWindowsSecurity(handle) != nil {
			return nil, err
		}
	}
	content, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		clearBytes(content)
		return nil, err
	}
	if int64(len(content)) > maximum {
		clearBytes(content)
		return nil, errors.New("private file size invalid")
	}
	return content, nil
}

func validateWindowsPrivateFileHandle(fileType uint32, information windows.ByHandleFileInformation, maximum int64) error {
	invalidAttributes := uint32(windows.FILE_ATTRIBUTE_REPARSE_POINT | windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_DEVICE)
	if fileType != windows.FILE_TYPE_DISK || information.FileAttributes&invalidAttributes != 0 || information.NumberOfLinks != 1 || maximum < 0 {
		return errors.New("private file validation failed")
	}
	size := uint64(information.FileSizeHigh)<<32 | uint64(information.FileSizeLow)
	if size > uint64(maximum) {
		return errors.New("private file size invalid")
	}
	return nil
}

func writePrivateFileAtomically(path string, content []byte, hooks *atomicCommitHooks) error {
	if err := validatePrivateWindowsPath(path); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := ensurePrivateStoreDirectory(directory); err != nil {
		return err
	}
	parent, err := openWindowsDirectory(directory, privateWindowsDirectoryName(directory))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	if err := validatePrivateWindowsExactName(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := createPrivateWindowsTempFile(directory, filepath.Base(path))
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := validatePrivateWindowsSecurity(windows.Handle(temporary.Fd())); err != nil {
		return err
	}
	if err := writeAll(temporary, content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if hooks != nil && hooks.beforeReplace != nil {
		if err := hooks.beforeReplace(); err != nil {
			return err
		}
	}
	if hooks != nil && hooks.afterReplaceBeforeDirectorySync != nil {
		if err := hooks.afterReplaceBeforeDirectorySync(); err != nil {
			return err
		}
	}
	source, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return err
	}
	committed = true
	// MoveFileEx promotes the temporary file's security descriptor. Confirm the
	// committed name still has the owner-only DACL before reporting success.
	readback, err := readPrivateCommittedFile(path, int64(len(content)))
	if err == nil && !bytes.Equal(readback, content) {
		err = errors.New("private Windows replacement readback mismatch")
	}
	clearBytes(readback)
	return err
}

func recoverPrivateFileCommit(string) error { return nil }

func ensurePrivateStoreDirectory(directory string) error {
	if err := validatePrivateWindowsPath(directory); err != nil {
		return err
	}
	private := privateWindowsDirectoryName(directory)
	if handle, err := openWindowsDirectory(directory, private); err == nil {
		return windows.CloseHandle(handle)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !private {
		return os.ErrNotExist
	}
	parent := filepath.Dir(directory)
	if parent == directory {
		return errors.New("private Windows directory has no existing parent")
	}
	if err := ensurePrivateStoreDirectory(parent); err != nil {
		return err
	}
	attributes, err := privateWindowsSecurityAttributes()
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	if err := windows.CreateDirectory(name, &attributes); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	handle, err := openWindowsDirectory(directory, true)
	if err != nil {
		return err
	}
	return windows.CloseHandle(handle)
}

func writeAll(file *os.File, content []byte) error {
	for len(content) > 0 {
		written, err := file.Write(content)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}
