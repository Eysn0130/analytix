package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// ResolveMutationIdentityPath returns the filesystem-semantic path used only
// for a host side-effect identity. It keeps symlinks distinct by rejecting
// them, canonicalizes the spelling of existing non-symlink components, and
// folds missing suffixes only when the owning filesystem is case-insensitive.
// The returned value is hashed in memory and is never persisted as raw data.
func ResolveMutationIdentityPath(workspace string, requestedPath string) (string, bool) {
	resolved, ok := ResolveAnyPath(workspace, requestedPath)
	if !ok || !filepath.IsAbs(resolved) {
		return "", false
	}
	resolved, err := canonicalMutationIdentitySystemAlias(resolved)
	if err != nil || !filepath.IsAbs(resolved) {
		return "", false
	}
	existing, suffix, ok := nearestExistingMutationAncestor(resolved)
	if !ok || mutationPathContainsSymlink(existing) {
		return "", false
	}
	canonicalExisting, err := filepath.EvalSymlinks(existing)
	if err != nil || !filepath.IsAbs(canonicalExisting) {
		return "", false
	}
	caseSensitive, known := mutationPathCaseSensitive(canonicalExisting)
	if !known {
		return "", false
	}
	identityPath := filepath.Clean(filepath.Join(append([]string{canonicalExisting}, suffix...)...))
	if caseSensitive {
		return identityPath, true
	}
	// Unicode normalization and full case folding are applied only when the
	// filesystem itself treats those spellings as aliases. Applying this on a
	// case-sensitive filesystem would incorrectly merge distinct files.
	return cases.Fold().String(norm.NFC.String(identityPath)), true
}

func nearestExistingMutationAncestor(path string) (string, []string, bool) {
	candidate := filepath.Clean(path)
	suffix := []string{}
	for {
		_, err := os.Lstat(candidate)
		if err == nil {
			return candidate, suffix, true
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", nil, false
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", nil, false
		}
		suffix = append([]string{filepath.Base(candidate)}, suffix...)
		candidate = parent
	}
}

func mutationPathContainsSymlink(path string) bool {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	remainder := strings.TrimPrefix(clean, volume)
	remainder = strings.TrimLeft(remainder, string(filepath.Separator))
	current := volume + string(filepath.Separator)
	if volume == "" {
		current = string(filepath.Separator)
	}
	if remainder == "" {
		return false
	}
	for _, component := range strings.Split(remainder, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}
