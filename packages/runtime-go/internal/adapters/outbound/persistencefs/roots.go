package persistencefs

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RootSet struct {
	DataDir    string
	DurableDir string
}

func ResolveRootSet(dataDir string, durableDir string) (RootSet, error) {
	dataRoot, err := canonicalPathWithoutCreate(dataDir)
	if err != nil {
		return RootSet{}, errors.New("data persistence root resolution failed")
	}
	durableRoot, err := canonicalPathWithoutCreate(durableDir)
	if err != nil {
		return RootSet{}, errors.New("durable persistence root resolution failed")
	}
	dataKey := canonicalPathKey(dataRoot)
	durableKey := canonicalPathKey(durableRoot)
	if dataKey != durableKey && (pathContains(dataKey, durableKey) || pathContains(durableKey, dataKey)) {
		return RootSet{}, errors.New("persistence roots must be equal or non-overlapping")
	}
	return RootSet{DataDir: dataRoot, DurableDir: durableRoot}, nil
}

func CanonicalRoots(roots RootSet) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, 2)
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if root == "" {
			continue
		}
		key := canonicalPathKey(root)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// ResolveSeparateOwnerRoots canonicalizes security roots owned outside the Go
// persistence adapter. They remain deliberately absent from RootSet and its
// semantic journal, while callers may use the returned paths for a shared
// coordination lease and an owner-specific inventory/journal.
func ResolveSeparateOwnerRoots(roots RootSet, ownerRoots ...string) ([]string, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	keys := CanonicalRoots(resolved)
	result := make([]string, 0, len(ownerRoots))
	for _, value := range ownerRoots {
		if err := rejectSeparateOwnerTerminalReparse(value); err != nil {
			return nil, errors.New("separate owner root resolution failed")
		}
		owner, err := canonicalPathWithoutCreate(value)
		if err != nil {
			return nil, errors.New("separate owner root resolution failed")
		}
		ownerKey := canonicalPathKey(owner)
		for _, existing := range keys {
			if ownerKey == existing || pathContains(ownerKey, existing) || pathContains(existing, ownerKey) {
				return nil, errors.New("separate owner roots must be non-overlapping")
			}
		}
		keys = append(keys, ownerKey)
		result = append(result, owner)
	}
	sort.Slice(result, func(left, right int) bool {
		return canonicalPathKey(result[left]) < canonicalPathKey(result[right])
	})
	return result, nil
}

// rejectSeparateOwnerTerminalReparse preserves the caller-controlled root
// witness that canonicalPathWithoutCreate would otherwise erase. Ancestor
// aliases above the nearest existing owner component are normalized once; the
// returned canonical path is the only path subsequently leased or opened.
func rejectSeparateOwnerTerminalReparse(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("separate owner root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return err
	}
	current := filepath.Clean(absolute)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("separate owner root uses a symlink or reparse witness")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
		current = parent
	}
}

func pathContains(parent string, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func canonicalPathWithoutCreate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("persistence root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	existing := absolute
	missing := make([]string, 0, 4)
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", err
		}
		missing = append(missing, filepath.Base(existing))
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
	}
	return filepath.Clean(resolved), nil
}
