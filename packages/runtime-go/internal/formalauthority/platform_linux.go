//go:build linux

package formalauthority

// Linux filesystem support is validated by the production secureconfigfs
// reader itself. Do not skip unexpected ext4/XFS, mount-identity, permission,
// ACL, or inventory failures.
func SecureConfigurationFilesystemBlocker(string) string {
	return ""
}
