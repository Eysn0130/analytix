//go:build !darwin

package processsandbox

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPrepareWithDenyRootsFailsClosedOnUnsupportedHost(t *testing.T) {
	_, err := Prepare(
		"/usr/bin/true",
		nil,
		FilesystemPolicy{
			DenyRoots: []string{
				filepath.Join(string(filepath.Separator), "protected"),
			},
		},
	)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Prepare() error = %v, want ErrUnavailable", err)
	}
}
