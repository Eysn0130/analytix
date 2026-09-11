//go:build darwin || linux

package persistencefs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
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
	parent, err := secureOpenAbsoluteDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	base := filepath.Base(path)
	child, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	identity, err := unixOpenedDirectoryIdentity(child)
	if err != nil || identity != expectedIdentity {
		_ = unix.Close(child)
		return errors.New("private tree identity changed before removal")
	}
	if err := secureRemoveUnixDirectoryContents(ctx, child); err != nil {
		_ = unix.Close(child)
		return err
	}
	if err := unix.Close(child); err != nil {
		return err
	}
	var current unix.Stat_t
	if err := unix.Fstatat(parent, base, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil || unixDirectoryIdentity(current) != expectedIdentity {
		return errors.New("private tree directory entry changed before removal")
	}
	if err := unix.Unlinkat(parent, base, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(parent)
}

func secureRemoveUnixDirectoryContents(ctx context.Context, directory int) error {
	count := 0
	if err := preflightUnixPrivateTree(ctx, directory, 0, &count); err != nil {
		return err
	}
	removed := 0
	return removeUnixPrivateTreeContents(ctx, directory, 0, &removed)
}

func preflightUnixPrivateTree(ctx context.Context, directory int, depth int, count *int) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if depth > maxPrivateTreeDepth {
		return StartupResourceLimitError{Code: "private_tree_depth", Limit: maxPrivateTreeDepth}
	}
	identity, err := unixOpenedDirectoryIdentity(directory)
	if err != nil {
		return err
	}
	duplicate, err := unix.Openat(directory, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(duplicate), "private-tree")
	if file == nil {
		_ = unix.Close(duplicate)
		return errors.New("private tree directory handle is invalid")
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
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
				_ = file.Close()
				return errors.New("private tree contains an unsafe entry")
			}
			var stat unix.Stat_t
			if err := unix.Fstatat(directory, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				_ = file.Close()
				return err
			}
			switch stat.Mode & unix.S_IFMT {
			case unix.S_IFDIR:
				child, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
				if err != nil {
					_ = file.Close()
					return err
				}
				childIdentity := unixDirectoryIdentity(stat)
				if openedIdentity, identityErr := unixOpenedDirectoryIdentity(child); identityErr != nil || openedIdentity != childIdentity {
					_ = unix.Close(child)
					_ = file.Close()
					return errors.New("private tree directory identity changed during preflight")
				}
				childErr := preflightUnixPrivateTree(ctx, child, depth+1, count)
				currentIdentity, identityErr := unixOpenedDirectoryIdentity(child)
				closeErr := unix.Close(child)
				if childErr != nil || identityErr != nil || closeErr != nil || currentIdentity != childIdentity {
					_ = file.Close()
					return errors.Join(childErr, identityErr, closeErr)
				}
			case unix.S_IFREG:
				if stat.Nlink != 1 {
					_ = file.Close()
					return errors.New("private tree contains a hard-linked file")
				}
			default:
				_ = file.Close()
				return errors.New("private tree contains a symlink or special file")
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
	currentIdentity, err := unixOpenedDirectoryIdentity(directory)
	if err != nil || currentIdentity != identity {
		return errors.New("private tree directory identity changed after preflight")
	}
	return nil
}

func removeUnixPrivateTreeContents(ctx context.Context, directory int, depth int, removed *int) error {
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
		entries, err := readUnixPrivateTreeBatch(directory)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return unix.Fsync(directory)
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
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
				return errors.New("private tree contains an unsafe entry")
			}
			var stat unix.Stat_t
			if err := unix.Fstatat(directory, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return err
			}
			switch stat.Mode & unix.S_IFMT {
			case unix.S_IFDIR:
				child, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
				if err != nil {
					return err
				}
				childIdentity := unixDirectoryIdentity(stat)
				if openedIdentity, identityErr := unixOpenedDirectoryIdentity(child); identityErr != nil || openedIdentity != childIdentity {
					_ = unix.Close(child)
					return errors.New("private tree directory identity changed after preflight")
				}
				if err := removeUnixPrivateTreeContents(ctx, child, depth+1, removed); err != nil {
					_ = unix.Close(child)
					return err
				}
				if err := unix.Close(child); err != nil {
					return err
				}
				var current unix.Stat_t
				if err := unix.Fstatat(directory, name, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
					current.Mode&unix.S_IFMT != unix.S_IFDIR || unixDirectoryIdentity(current) != childIdentity {
					return errors.New("private tree directory entry changed after preflight")
				}
				if err := unix.Unlinkat(directory, name, unix.AT_REMOVEDIR); err != nil {
					return err
				}
			case unix.S_IFREG:
				if stat.Nlink != 1 {
					return errors.New("private tree contains a hard-linked file")
				}
				if err := unix.Unlinkat(directory, name, 0); err != nil {
					return err
				}
			default:
				return errors.New("private tree contains a symlink or special file")
			}
		}
	}
}

func readUnixPrivateTreeBatch(directory int) ([]os.DirEntry, error) {
	duplicate, err := unix.Openat(directory, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "private-tree-delete")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, errors.New("private tree directory handle is invalid")
	}
	entries, readErr := file.ReadDir(startupPrivateEntryPage)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errors.Join(readErr, closeErr)
	}
	return entries, closeErr
}
