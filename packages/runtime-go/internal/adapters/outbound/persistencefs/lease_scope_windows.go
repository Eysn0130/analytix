//go:build windows

package persistencefs

// Windows retains the existing LockFileEx authority until the directory-handle
// variant is validated on an approved Windows host. This no-op scope exists so
// cross-compilation cannot be mistaken for real NT rename/share-mode evidence.
type windowsScopeLease struct{}

func acquirePlatformScopeLease(RootSet) (platformScopeLease, error) {
	return windowsScopeLease{}, nil
}

func acquirePlatformScopeLeaseForSpecs([]compositeLeaseSpec) (platformScopeLease, error) {
	return windowsScopeLease{}, nil
}

func (windowsScopeLease) Validate() error { return nil }
func (windowsScopeLease) Close() error    { return nil }
