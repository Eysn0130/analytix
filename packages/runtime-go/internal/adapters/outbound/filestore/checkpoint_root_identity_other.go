//go:build !darwin && !linux && !windows

package filestore

import "errors"

func checkpointRootIdentity(string) (string, error) {
	return "", errors.New("checkpoint root identity is unsupported on this platform")
}
