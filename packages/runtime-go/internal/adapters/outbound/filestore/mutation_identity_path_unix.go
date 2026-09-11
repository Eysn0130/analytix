//go:build !darwin && !windows

package filestore

// Unix filesystems are case-sensitive by default. Existing aliases are still
// canonicalized by ResolveMutationIdentityPath. Case-folded mount support must
// be proven by a platform adapter before it can be claimed in release gates.
func mutationPathCaseSensitive(string) (bool, bool) {
	return true, true
}

func canonicalMutationIdentitySystemAlias(path string) (string, error) {
	return canonicalAtomicUnixSystemAlias(path)
}
