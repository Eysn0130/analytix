//go:build windows

package persistencefs

import (
	"errors"
	"os"
)

func startupAuthorityPathNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist) || secureWindowsNotFound(err)
}
