//go:build darwin

package persistencefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func platformSeparateOwnerRenameNoReplace(sourceFD int, source string, destinationFD int, destination string) error {
	err := unix.RenameatxNp(sourceFD, source, destinationFD, destination, unix.RENAME_EXCL)
	if errors.Is(err, unix.EEXIST) {
		return os.ErrExist
	}
	return err
}
