//go:build windows

package secretstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func readPrivateCommittedFile(path string, maximum int64) ([]byte, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ,
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
	if fileType != windows.FILE_TYPE_DISK || information.FileAttributes&invalidAttributes != 0 || maximum < 0 {
		return errors.New("private file validation failed")
	}
	size := uint64(information.FileSizeHigh)<<32 | uint64(information.FileSizeLow)
	if size > uint64(maximum) {
		return errors.New("private file size invalid")
	}
	return nil
}

func writePrivateFileAtomically(path string, content []byte, hooks *atomicCommitHooks) error {
	directory := filepath.Dir(path)
	if err := ensurePrivateStoreDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
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
	return nil
}

func recoverPrivateFileCommit(string) error { return nil }

func ensurePrivateStoreDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("private directory validation failed")
	}
	return nil
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
