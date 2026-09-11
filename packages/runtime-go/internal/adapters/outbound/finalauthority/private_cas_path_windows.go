//go:build windows

package finalauthority

import (
	"path/filepath"
	"strings"
)

func privateCASBindingPathEqual(left string, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
