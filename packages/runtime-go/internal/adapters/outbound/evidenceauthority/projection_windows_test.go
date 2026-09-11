//go:build windows

package evidenceauthority

import (
	"errors"
	"path/filepath"
	"testing"

	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
)

func TestProjectionIsTruthfullyUnsupportedOnWindows(t *testing.T) {
	if _, err := NewProjection(filepath.Join(t.TempDir(), "projection")); !errors.Is(err, authorityprojectionport.ErrUnsupported) {
		t.Fatalf("Windows projection did not fail closed as unsupported: %v", err)
	}
}
