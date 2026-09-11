//go:build !darwin && !linux && !windows

package persistencefs

import "errors"

func platformPinSeparateOwnerRoot(string, bool, separateOwnerPathBindingV1) (separateOwnerRootPin, error) {
	return nil, errors.New("separate-owner root pin is unsupported on this platform")
}
