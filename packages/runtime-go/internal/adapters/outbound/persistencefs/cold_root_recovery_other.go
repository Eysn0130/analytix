//go:build !darwin && !linux && !windows

package persistencefs

import "errors"

func secureColdRootIdentityAndEmpty(frozenRootCapability) (string, error) {
	return "", errors.New("startup journal cold root recovery is unsupported")
}
