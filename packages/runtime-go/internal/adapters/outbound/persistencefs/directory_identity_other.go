//go:build !darwin && !linux && !windows

package persistencefs

import (
	"errors"
)

func platformDirectoryIdentity(string) (string, error) {
	return "", errors.New("persistence directory identity is unsupported on this platform")
}
