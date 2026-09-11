//go:build linux

package persistencefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func secureStartupAuthorityCommitNoReplace(parent int, source, target string) error {
	err := unix.Renameat2(parent, source, parent, target, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EEXIST) {
		return os.ErrExist
	}
	return err
}
