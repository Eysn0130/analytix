//go:build windows

package filestore

import (
	"errors"
	"os"
)

// Windows does not provide the same portable directory-fsync primitive used
// on Unix. File handles are flushed before this barrier; the existence check
// still rejects an unexpected non-directory path.
func syncAttachmentDirectoryIfPresent(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("attachment directory barrier target is not a directory")
	}
	return nil
}
