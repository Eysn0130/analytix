//go:build darwin

package filestore

import "golang.org/x/sys/unix"

const pathconfCaseSensitiveDarwin = 11 // _PC_CASE_SENSITIVE

func mutationPathCaseSensitive(path string) (bool, bool) {
	value, err := unix.Pathconf(path, pathconfCaseSensitiveDarwin)
	if err != nil || (value != 0 && value != 1) {
		return false, false
	}
	return value == 1, true
}

func canonicalMutationIdentitySystemAlias(path string) (string, error) {
	return canonicalAtomicUnixSystemAlias(path)
}
