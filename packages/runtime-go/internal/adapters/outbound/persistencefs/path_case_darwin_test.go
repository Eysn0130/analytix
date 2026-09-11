//go:build darwin

package persistencefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinPathCaseSensitivityMatchesFilesystemAliasBehavior(t *testing.T) {
	directory := t.TempDir()
	caseInsensitive, available := platformPathVolumeCaseInsensitive(directory)
	if !available {
		t.Fatal("Darwin path case-sensitivity metadata is unavailable")
	}
	name := filepath.Base(directory)
	alias := filepath.Join(filepath.Dir(directory), swapFirstLetterCase(name))
	originalInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, aliasErr := os.Stat(alias)
	aliasMatches := aliasErr == nil && os.SameFile(originalInfo, aliasInfo)
	if caseInsensitive != aliasMatches {
		t.Fatalf("case-insensitive metadata = %v, alias behavior = %v", caseInsensitive, aliasMatches)
	}
}
