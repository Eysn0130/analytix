//go:build !darwin && !linux && !windows

package persistencefs

import "errors"

func securePrepareStartupAuthorityNamespace(string) error {
	return errors.New("startup journal persistent namespace is unsupported")
}

func securePrepareStartupAuthorityBase(string) error {
	return errors.New("startup journal persistent authority base is unsupported")
}
