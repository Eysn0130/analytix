//go:build windows

package finalauthority

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type acceptedFinalCASRootAuthority struct {
	path      string
	rootID    privateWindowsFileIDInfo
	threadsID privateWindowsFileIDInfo
}

type acceptedFinalCASWindowsObjectInfo struct {
	id   privateWindowsFileIDInfo
	info windows.ByHandleFileInformation
}

func captureAcceptedFinalCASRoot(path string) (acceptedFinalCASRootAuthority, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return acceptedFinalCASRootAuthority{}, errors.New("accepted final CAS durable root is invalid")
	}
	absolute = filepath.Clean(absolute)
	root, err := acceptedFinalCASWindowsOpenAbsoluteDirectory(absolute)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	defer windows.CloseHandle(root)
	rootInfo, err := acceptedFinalCASWindowsDirectoryInfo(root)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	threads, err := acceptedFinalCASWindowsOpenDirectory(root, "threads")
	if err != nil {
		return acceptedFinalCASRootAuthority{}, errors.New("accepted final CAS threads authority is unavailable")
	}
	defer windows.CloseHandle(threads)
	threadsInfo, err := acceptedFinalCASWindowsDirectoryInfo(threads)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	return acceptedFinalCASRootAuthority{
		path: absolute, rootID: rootInfo.id, threadsID: threadsInfo.id,
	}, nil
}

func (authority acceptedFinalCASRootAuthority) readPrimaryThreadJSON(threadID string) ([]byte, error) {
	return authority.readThreadFile(threadID, "thread.json")
}

func (authority acceptedFinalCASRootAuthority) readThreadFile(threadID, name string) ([]byte, error) {
	if name != "thread.json" && name != "events.jsonl" {
		return nil, errors.New("Core snapshot file identity is invalid")
	}
	if !validCASRecordID(threadID) || !privateWindowsComponent(threadID) {
		return nil, errors.New("accepted final CAS thread identity is invalid")
	}
	root, err := authority.openRoot()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	threads, err := authority.openThreads(root)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(threads)
	thread, err := acceptedFinalCASWindowsOpenDirectory(threads, threadID)
	if err != nil {
		return nil, errors.New("accepted final CAS primary thread directory is unavailable")
	}
	defer windows.CloseHandle(thread)
	threadInfo, err := acceptedFinalCASWindowsDirectoryInfo(thread)
	if err != nil {
		return nil, err
	}
	handle, err := privateWindowsOpenRelative(thread, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return nil, errors.New("accepted final CAS primary thread file is unavailable")
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("accepted final CAS primary thread handle is invalid")
	}
	defer file.Close()
	initial, err := acceptedFinalCASWindowsFileInfo(handle)
	if err != nil {
		return nil, err
	}
	size := int64(uint64(initial.info.FileSizeHigh)<<32 | uint64(initial.info.FileSizeLow))
	body, err := acceptedFinalCASReadStableFile(file, size)
	if err != nil {
		return nil, err
	}
	refreshed, err := acceptedFinalCASWindowsFileInfo(handle)
	if err != nil || !acceptedFinalCASSameWindowsFile(initial, refreshed) {
		return nil, errors.New("accepted final CAS primary thread changed during read")
	}
	if err := authority.verifyCurrentPath(root, threadID, name, threadInfo, initial); err != nil {
		return nil, err
	}
	return body, nil
}

func (authority acceptedFinalCASRootAuthority) openRoot() (windows.Handle, error) {
	root, err := acceptedFinalCASWindowsOpenAbsoluteDirectory(authority.path)
	if err != nil {
		return 0, err
	}
	info, infoErr := acceptedFinalCASWindowsDirectoryInfo(root)
	if infoErr != nil || info.id != authority.rootID {
		_ = windows.CloseHandle(root)
		return 0, errors.New("accepted final CAS durable root identity changed")
	}
	return root, nil
}

func (authority acceptedFinalCASRootAuthority) openThreads(root windows.Handle) (windows.Handle, error) {
	threads, err := acceptedFinalCASWindowsOpenDirectory(root, "threads")
	if err != nil {
		return 0, err
	}
	info, infoErr := acceptedFinalCASWindowsDirectoryInfo(threads)
	if infoErr != nil || info.id != authority.threadsID {
		_ = windows.CloseHandle(threads)
		return 0, errors.New("accepted final CAS threads authority identity changed")
	}
	return threads, nil
}

func (authority acceptedFinalCASRootAuthority) verifyCurrentPath(root windows.Handle, threadID, name string, expectedThreadDir, expectedFile acceptedFinalCASWindowsObjectInfo) error {
	reopenedRoot, err := authority.openRoot()
	if err != nil {
		return err
	}
	_ = windows.CloseHandle(reopenedRoot)
	reopenedThreads, err := authority.openThreads(root)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(reopenedThreads)
	reopenedThread, err := acceptedFinalCASWindowsOpenDirectory(reopenedThreads, threadID)
	if err != nil {
		return errors.New("accepted final CAS primary thread ancestor changed during read")
	}
	defer windows.CloseHandle(reopenedThread)
	threadInfo, err := acceptedFinalCASWindowsDirectoryInfo(reopenedThread)
	if err != nil || !acceptedFinalCASSameWindowsIdentity(expectedThreadDir, threadInfo) {
		return errors.New("accepted final CAS primary thread ancestor changed during read")
	}
	reopenedFile, err := privateWindowsOpenRelative(reopenedThread, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return errors.New("accepted final CAS primary thread identity changed during read")
	}
	defer windows.CloseHandle(reopenedFile)
	fileInfo, err := acceptedFinalCASWindowsFileInfo(reopenedFile)
	if err != nil || !acceptedFinalCASSameWindowsFile(expectedFile, fileInfo) {
		return errors.New("accepted final CAS primary thread identity changed during read")
	}
	return nil
}

func acceptedFinalCASWindowsOpenAbsoluteDirectory(path string) (windows.Handle, error) {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) {
		return 0, errors.New("accepted final CAS Windows root is invalid")
	}
	current, err := privateWindowsOpenAbsolute(volume+string(os.PathSeparator), windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	for _, component := range strings.FieldsFunc(remainder, func(char rune) bool { return char == '\\' || char == '/' }) {
		if !privateWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("accepted final CAS Windows root component is invalid")
		}
		next, openErr := acceptedFinalCASWindowsOpenDirectory(current, component)
		if openErr != nil {
			_ = windows.CloseHandle(current)
			return 0, openErr
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	return current, nil
}

func acceptedFinalCASWindowsOpenDirectory(parent windows.Handle, name string) (windows.Handle, error) {
	return privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
}

func acceptedFinalCASWindowsDirectoryInfo(handle windows.Handle) (acceptedFinalCASWindowsObjectInfo, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return acceptedFinalCASWindowsObjectInfo{}, errors.New("accepted final CAS Windows directory authority is invalid")
	}
	id, err := privateWindowsHandleFileID(handle, true)
	if err != nil {
		return acceptedFinalCASWindowsObjectInfo{}, err
	}
	return acceptedFinalCASWindowsObjectInfo{id: id, info: info}, nil
}

func acceptedFinalCASWindowsFileInfo(handle windows.Handle) (acceptedFinalCASWindowsObjectInfo, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return acceptedFinalCASWindowsObjectInfo{}, errors.New("accepted final CAS primary thread file is unsafe")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size == 0 || size > maxAcceptedFinalCASFileBytes {
		return acceptedFinalCASWindowsObjectInfo{}, errors.New("accepted final CAS primary thread file size is invalid")
	}
	id, err := privateWindowsHandleFileID(handle, false)
	if err != nil {
		return acceptedFinalCASWindowsObjectInfo{}, err
	}
	return acceptedFinalCASWindowsObjectInfo{id: id, info: info}, nil
}

func acceptedFinalCASSameWindowsIdentity(left, right acceptedFinalCASWindowsObjectInfo) bool {
	return left.id == right.id
}

func acceptedFinalCASSameWindowsFile(left, right acceptedFinalCASWindowsObjectInfo) bool {
	return acceptedFinalCASSameWindowsIdentity(left, right) && left.info.FileAttributes == right.info.FileAttributes &&
		left.info.NumberOfLinks == right.info.NumberOfLinks && left.info.FileSizeHigh == right.info.FileSizeHigh && left.info.FileSizeLow == right.info.FileSizeLow &&
		left.info.CreationTime == right.info.CreationTime && left.info.LastWriteTime == right.info.LastWriteTime
}
