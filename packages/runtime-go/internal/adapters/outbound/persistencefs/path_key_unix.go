//go:build !windows

package persistencefs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func canonicalPathKey(path string) string {
	clean := filepath.Clean(path)
	if pathVolumeCaseInsensitive(clean) {
		return strings.ToLower(clean)
	}
	return clean
}

func canonicalPathsEqual(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || canonicalPathKey(left) == canonicalPathKey(right)
}

func canonicalPathsEqualForDevice(left, right string, device uint64) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if left == right {
		return true
	}
	caseInsensitive, available := platformKnownVolumeCaseInsensitive(right, device)
	return available && caseInsensitive && strings.EqualFold(left, right)
}

func pathVolumeCaseInsensitive(path string) bool {
	if caseInsensitive, available := platformPathVolumeCaseInsensitive(path); available {
		return caseInsensitive
	}
	current := filepath.Clean(path)
	for {
		if info, err := os.Stat(current); err == nil {
			for {
				parent := filepath.Dir(current)
				if parent == current {
					return false
				}
				name := filepath.Base(current)
				alias := swapFirstLetterCase(name)
				if alias != name {
					aliasInfo, aliasErr := os.Stat(filepath.Join(parent, alias))
					return aliasErr == nil && os.SameFile(info, aliasInfo)
				}
				current = parent
				info, err = os.Stat(current)
				if err != nil {
					return false
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func swapFirstLetterCase(value string) string {
	runes := []rune(value)
	for index, current := range runes {
		switch {
		case unicode.IsLower(current):
			runes[index] = unicode.ToUpper(current)
			return string(runes)
		case unicode.IsUpper(current):
			runes[index] = unicode.ToLower(current)
			return string(runes)
		}
	}
	return value
}
