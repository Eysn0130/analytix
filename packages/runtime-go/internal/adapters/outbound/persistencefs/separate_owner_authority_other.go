//go:build !darwin && !linux && !windows

package persistencefs

import "errors"

func platformSeparateOwnerDirectoryBinding(string, bool) (string, string, error) {
	return "", "", errors.New("separate-owner root authority is unsupported on this platform")
}
