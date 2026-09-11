//go:build darwin || linux

package persistencefs

import (
	"errors"
	"os"
)

func startupAuthorityPathNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
