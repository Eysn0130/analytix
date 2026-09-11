//go:build !darwin && !linux && !windows

package filestore

import (
	"errors"
	"os"
)

func readRuntimeConfigPathSnapshotV1(path string, _ int64) ([]byte, bool, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	return nil, false, errors.New("secure runtime configuration snapshots are unsupported on this platform")
}
