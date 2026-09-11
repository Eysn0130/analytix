//go:build !windows

package evidenceregistry

import (
	"errors"
	"os"
)

func atomicReplaceRegistryFile(sourcePath string, destinationPath string) error {
	return os.Rename(sourcePath, destinationPath)
}

func syncRegistryDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
