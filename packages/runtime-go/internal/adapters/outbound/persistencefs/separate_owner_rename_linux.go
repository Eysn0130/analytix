//go:build linux

package persistencefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func platformSeparateOwnerRenameNoReplace(sourceFD int, source string, destinationFD int, destination string) error {
	err := unix.Renameat2(sourceFD, source, destinationFD, destination, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EEXIST) {
		return os.ErrExist
	}
	return err
}
