//go:build darwin

package formalauthority

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinSecureConfigurationFilesystemBlockerIsExact(t *testing.T) {
	path := "/Volumes/test-authority"
	if blocker := darwinSecureConfigurationFilesystemBlocker(path, 0); blocker != "" {
		t.Fatalf("non-removable mount was blocked: %q", blocker)
	}
	if blocker := darwinSecureConfigurationFilesystemBlocker(path, unix.MNT_LOCAL); blocker != "" {
		t.Fatalf("unrelated mount flag was blocked: %q", blocker)
	}
	blocker := darwinSecureConfigurationFilesystemBlocker(
		path,
		unix.MNT_LOCAL|unix.MNT_REMOVABLE,
	)
	if !strings.Contains(blocker, path) || !strings.Contains(blocker, "MNT_REMOVABLE") {
		t.Fatalf("removable mount blocker = %q", blocker)
	}
}
