//go:build !windows

package filestore

import (
	"errors"
	"os"
)

func syncAttachmentDirectoryIfPresent(path string) error {
	dir, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
