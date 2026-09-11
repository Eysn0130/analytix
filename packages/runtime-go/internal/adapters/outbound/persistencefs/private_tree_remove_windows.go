//go:build windows

package persistencefs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"golang.org/x/sys/windows"
)

func secureRemovePrivateTree(path, expectedIdentity string) error {
	return secureRemovePrivateTreeContext(context.Background(), path, expectedIdentity)
}

func secureRemovePrivateTreeContext(ctx context.Context, path, expectedIdentity string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if expectedIdentity == "" || filepath.Base(path) == "." || filepath.Base(path) == string(filepath.Separator) {
		return errors.New("private tree removal identity is invalid")
	}
	parent, err := secureWindowsOpenAbsoluteDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	base := filepath.Base(path)
	child, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, true)
	if secureWindowsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	identity, err := windowsOpenedDirectoryIdentity(child)
	if err != nil || identity != expectedIdentity {
		_ = windows.CloseHandle(child)
		return errors.New("private tree identity changed before removal")
	}
	if err := secureRemoveWindowsDirectoryContents(ctx, child); err != nil {
		_ = windows.CloseHandle(child)
		return err
	}
	if err := windows.CloseHandle(child); err != nil {
		return err
	}
	current, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if err != nil {
		return err
	}
	currentIdentity, identityErr := windowsOpenedDirectoryIdentity(current)
	closeErr := windows.CloseHandle(current)
	if identityErr != nil || closeErr != nil || currentIdentity != expectedIdentity {
		return errors.New("private tree directory entry changed before removal")
	}
	if err := secureWindowsDeleteRelative(parent, base, domainstartup.ManagedEntryTypeDirectory); err != nil {
		return err
	}
	return secureWindowsSyncDirectory(parent)
}

func secureRemoveWindowsDirectoryContents(ctx context.Context, directory windows.Handle) error {
	count := 0
	if err := preflightWindowsPrivateTree(ctx, directory, 0, &count); err != nil {
		return err
	}
	removed := 0
	return removeWindowsPrivateTreeContents(ctx, directory, 0, &removed)
}

func preflightWindowsPrivateTree(ctx context.Context, directory windows.Handle, depth int, count *int) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if depth > maxPrivateTreeDepth {
		return StartupResourceLimitError{Code: "private_tree_depth", Limit: maxPrivateTreeDepth}
	}
	identity, err := windowsOpenedDirectoryIdentity(directory)
	if err != nil {
		return err
	}
	duplicate, err := secureWindowsReopenDirectory(directory)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(duplicate), "private-tree")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return errors.New("private tree Windows directory handle is invalid")
	}
	for {
		if err := contextError(ctx); err != nil {
			_ = file.Close()
			return err
		}
		entries, readErr := file.ReadDir(startupPrivateEntryPage)
		for _, entry := range entries {
			if err := contextError(ctx); err != nil {
				_ = file.Close()
				return err
			}
			*count++
			if *count > maxStartupPrivateEntries {
				_ = file.Close()
				return StartupResourceLimitError{Code: "private_tree_entries", Limit: maxStartupPrivateEntries}
			}
			name := entry.Name()
			if !secureWindowsComponent(name) {
				_ = file.Close()
				return errors.New("private tree contains an unsafe Windows entry")
			}
			if entry.IsDir() {
				child, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, true)
				if err != nil {
					_ = file.Close()
					return err
				}
				childIdentity, identityErr := windowsOpenedDirectoryIdentity(child)
				childErr := preflightWindowsPrivateTree(ctx, child, depth+1, count)
				currentIdentity, currentErr := windowsOpenedDirectoryIdentity(child)
				closeErr := windows.CloseHandle(child)
				if identityErr != nil || childErr != nil || currentErr != nil || closeErr != nil || childIdentity != currentIdentity {
					_ = file.Close()
					return errors.Join(identityErr, childErr, currentErr, closeErr)
				}
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				_ = file.Close()
				return errors.New("private tree contains a Windows reparse or special entry")
			}
			handle, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
			if err != nil {
				_ = file.Close()
				return err
			}
			var info windows.ByHandleFileInformation
			infoErr := windows.GetFileInformationByHandle(handle, &info)
			closeErr := windows.CloseHandle(handle)
			if infoErr != nil || closeErr != nil || info.NumberOfLinks != 1 ||
				info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
				_ = file.Close()
				return errors.New("private tree contains an unsafe Windows file")
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return readErr
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	currentIdentity, err := windowsOpenedDirectoryIdentity(directory)
	if err != nil || currentIdentity != identity {
		return errors.New("private tree Windows directory identity changed after preflight")
	}
	return nil
}

func removeWindowsPrivateTreeContents(ctx context.Context, directory windows.Handle, depth int, removed *int) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if depth > maxPrivateTreeDepth {
		return StartupResourceLimitError{Code: "private_tree_depth", Limit: maxPrivateTreeDepth}
	}
	for {
		if err := contextError(ctx); err != nil {
			return err
		}
		entries, err := readWindowsPrivateTreeBatch(directory)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return secureWindowsSyncDirectory(directory)
		}
		for _, entry := range entries {
			if err := contextError(ctx); err != nil {
				return err
			}
			*removed++
			if *removed > maxStartupPrivateEntries {
				return StartupResourceLimitError{Code: "private_tree_entries", Limit: maxStartupPrivateEntries}
			}
			name := entry.Name()
			if !secureWindowsComponent(name) {
				return errors.New("private tree contains an unsafe Windows entry")
			}
			if entry.IsDir() {
				child, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, true)
				if err != nil {
					return err
				}
				childIdentity, identityErr := windowsOpenedDirectoryIdentity(child)
				if identityErr != nil {
					_ = windows.CloseHandle(child)
					return identityErr
				}
				if err := removeWindowsPrivateTreeContents(ctx, child, depth+1, removed); err != nil {
					_ = windows.CloseHandle(child)
					return err
				}
				if err := windows.CloseHandle(child); err != nil {
					return err
				}
				current, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
				if err != nil {
					return err
				}
				currentIdentity, currentErr := windowsOpenedDirectoryIdentity(current)
				closeErr := windows.CloseHandle(current)
				if currentErr != nil || closeErr != nil || currentIdentity != childIdentity {
					return errors.New("private tree Windows directory entry changed after preflight")
				}
				if err := secureWindowsDeleteRelative(directory, name, domainstartup.ManagedEntryTypeDirectory); err != nil {
					return err
				}
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return errors.New("private tree contains a Windows reparse or special entry")
			}
			handle, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
			if err != nil {
				return err
			}
			var info windows.ByHandleFileInformation
			infoErr := windows.GetFileInformationByHandle(handle, &info)
			closeErr := windows.CloseHandle(handle)
			if infoErr != nil || closeErr != nil || info.NumberOfLinks != 1 ||
				info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
				return errors.New("private tree Windows file changed after preflight")
			}
			if err := secureWindowsDeleteRelative(directory, name, domainstartup.ManagedEntryTypeFile); err != nil {
				return err
			}
		}
	}
}

func readWindowsPrivateTreeBatch(directory windows.Handle) ([]os.DirEntry, error) {
	duplicate, err := secureWindowsReopenDirectory(directory)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "private-tree-delete")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return nil, errors.New("private tree Windows directory handle is invalid")
	}
	entries, readErr := file.ReadDir(startupPrivateEntryPage)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errors.Join(readErr, closeErr)
	}
	return entries, closeErr
}

func duplicateWindowsHandle(handle windows.Handle) (windows.Handle, error) {
	process := windows.CurrentProcess()
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(process, handle, process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0, err
	}
	return duplicate, nil
}
