//go:build darwin

package formalauthority

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// SecureConfigurationFilesystemBlocker reports only a known platform
// prerequisite that makes the production secureconfigfs reader unavailable.
// Other filesystem failures deliberately return no blocker so the integration
// test fails instead of hiding a regression.
func SecureConfigurationFilesystemBlocker(path string) string {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return ""
	}
	return darwinSecureConfigurationFilesystemBlocker(path, uint32(stat.Flags))
}

func darwinSecureConfigurationFilesystemBlocker(path string, flags uint32) string {
	if flags&unix.MNT_REMOVABLE != 0 {
		return fmt.Sprintf(
			"production secure configuration requires non-removable APFS; %s is mounted MNT_REMOVABLE",
			path,
		)
	}
	return ""
}
