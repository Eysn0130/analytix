//go:build !windows

package persistencefs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type unixScopeLease struct {
	entries []unixScopeLeaseEntry
}

type unixScopeLeaseEntry struct {
	path string
	file *os.File
}

// acquirePlatformScopeLease collapses every not-yet-created path onto its
// nearest existing directory. A cold root therefore takes an exclusive inode
// lock on that ancestor until shutdown; an existing root takes an exclusive
// lock on itself and shared locks on its ancestors. This is conservative for
// cold sibling roots, but preserves deterministic overlap exclusion without a
// replaceable filesystem lock namespace.
func acquirePlatformScopeLease(roots RootSet) (platformScopeLease, error) {
	return acquirePlatformScopeLeaseForSpecs(compositeLeaseSpecs(roots))
}

func acquirePlatformScopeLeaseForSpecs(specs []compositeLeaseSpec) (platformScopeLease, error) {
	modes := map[string]bool{}
	for _, spec := range specs {
		path, err := nearestExistingLeaseDirectory(spec.path)
		if err != nil {
			return nil, err
		}
		modes[path] = modes[path] || spec.exclusive
	}
	paths := make([]string, 0, len(modes))
	for path := range modes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	lease := &unixScopeLease{entries: make([]unixScopeLeaseEntry, 0, len(paths))}
	for _, path := range paths {
		file, err := openLeaseScopeDirectory(path)
		if err != nil {
			_ = lease.Close()
			return nil, err
		}
		locked, err := tryPlatformFileLock(file, modes[path])
		if err != nil || !locked {
			_ = file.Close()
			_ = lease.Close()
			if err != nil {
				return nil, err
			}
			digest := sha256.Sum256([]byte(path))
			return nil, fmt.Errorf("%w: directory scope %s", ErrPersistenceInUse, hex.EncodeToString(digest[:8]))
		}
		lease.entries = append(lease.entries, unixScopeLeaseEntry{path: path, file: file})
	}
	return lease, nil
}

func nearestExistingLeaseDirectory(path string) (string, error) {
	path = filepath.Clean(path)
	for {
		info, err := os.Lstat(path)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", errors.New("persistence lease scope is not a real directory")
			}
			return path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", errors.New("persistence lease scope has no existing ancestor")
		}
		path = parent
	}
}

func openLeaseScopeDirectory(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, errors.New("persistence lease scope directory is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, openedErr := file.Stat()
	after, pathErr := os.Lstat(path)
	if openedErr != nil || pathErr != nil || !opened.IsDir() || after.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = file.Close()
		return nil, errors.New("persistence lease scope directory changed during open")
	}
	return file, nil
}

func (lease *unixScopeLease) Validate() error {
	if lease == nil || len(lease.entries) == 0 {
		return errors.New("persistence directory scope lease is unavailable")
	}
	for _, entry := range lease.entries {
		if entry.file == nil {
			return errors.New("persistence directory scope lease file is unavailable")
		}
		opened, openedErr := entry.file.Stat()
		current, currentErr := os.Lstat(entry.path)
		if openedErr != nil || currentErr != nil || !opened.IsDir() || current.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(opened, current) {
			return errors.New("persistence directory scope identity changed")
		}
	}
	return nil
}

func (lease *unixScopeLease) Close() error {
	if lease == nil {
		return nil
	}
	var closeErr error
	for index := len(lease.entries) - 1; index >= 0; index-- {
		entry := lease.entries[index]
		if entry.file != nil {
			closeErr = errors.Join(closeErr, unlockPlatformFile(entry.file), entry.file.Close())
		}
	}
	lease.entries = nil
	return closeErr
}
