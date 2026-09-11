//go:build !darwin && !linux && !windows

package persistencefs

import "errors"

type startupAuthorityRoot struct{}

func (startupAuthorityRoot) pathValue() string { return "" }

func captureStartupAuthorityRoot(string) (startupAuthorityRoot, error) {
	return startupAuthorityRoot{}, errors.New("startup journal namespace authority is unsupported")
}
func validateStartupAuthorityRoot(startupAuthorityRoot) error {
	return errors.New("startup journal namespace authority is unsupported")
}
func secureStartupAuthorityRead(startupAuthorityRoot, string, int64) ([]byte, string, error) {
	return nil, "", errors.New("startup journal authority storage is unsupported")
}
func secureStartupAuthorityWriteExclusive(startupAuthorityRoot, string, []byte, int64) error {
	return errors.New("startup journal authority storage is unsupported")
}
func secureStartupAuthorityCleanCreateResidue(startupAuthorityRoot, string, int64, func([]byte) error) error {
	return errors.New("startup journal authority storage is unsupported")
}
func startupAuthorityNamedComponent(string) bool { return false }
