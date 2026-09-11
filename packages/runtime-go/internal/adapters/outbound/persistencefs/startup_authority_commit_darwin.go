//go:build darwin

package persistencefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func secureStartupAuthorityCommitNoReplace(parent int, source, target string) error {
	err := unix.RenameatxNp(parent, source, parent, target, unix.RENAME_EXCL)
	if errors.Is(err, unix.EEXIST) {
		return os.ErrExist
	}
	return err
}
