//go:build !windows

package eventlog

import "os"

func atomicReplaceFile(sourcePath string, destinationPath string) error {
	return os.Rename(sourcePath, destinationPath)
}

func syncParentDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errorsJoin(directory.Sync(), directory.Close())
}
