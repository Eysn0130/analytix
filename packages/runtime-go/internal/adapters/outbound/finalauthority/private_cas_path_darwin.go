//go:build darwin

package finalauthority

import (
	"path/filepath"
	"strings"
)

func privateCASBindingPathEqual(left string, right string) bool {
	return privateCASDarwinSystemAlias(filepath.Clean(left)) == privateCASDarwinSystemAlias(filepath.Clean(right))
}

func privateCASDarwinSystemAlias(path string) string {
	for _, alias := range []string{"var", "tmp", "etc"} {
		short := string(filepath.Separator) + alias
		if path == short || strings.HasPrefix(path, short+string(filepath.Separator)) {
			return filepath.Join(string(filepath.Separator), "private", strings.TrimPrefix(path, string(filepath.Separator)))
		}
	}
	return path
}
