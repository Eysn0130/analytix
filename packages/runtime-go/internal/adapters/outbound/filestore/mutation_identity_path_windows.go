//go:build windows

package filestore

import "path/filepath"

// Windows host mutation identity is conservatively case-insensitive. A
// per-directory case-sensitive opt-in can therefore cause a safe false
// duplicate, but cannot permit the same physical effect twice.
func mutationPathCaseSensitive(string) (bool, bool) {
	return false, true
}

func canonicalMutationIdentitySystemAlias(path string) (string, error) {
	return filepath.Clean(path), nil
}
