//go:build linux

package finalauthority

import "path/filepath"

func privateCASBindingPathEqual(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
