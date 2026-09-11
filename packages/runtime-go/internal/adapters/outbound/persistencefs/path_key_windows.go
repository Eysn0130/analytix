//go:build windows

package persistencefs

import (
	"path/filepath"
	"strings"
)

func canonicalPathKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func canonicalPathsEqual(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || strings.EqualFold(left, right)
}

func canonicalPathsEqualForDevice(left, right string, _ uint64) bool {
	return canonicalPathsEqual(left, right)
}
