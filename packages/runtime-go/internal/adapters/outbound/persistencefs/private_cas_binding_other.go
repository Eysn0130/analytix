//go:build !darwin && !linux && !windows

package persistencefs

import (
	"errors"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func platformStrongDirectoryIdentity(string) (privatecasport.DirectoryIdentity, string, error) {
	return privatecasport.DirectoryIdentity{}, "", errors.New("private CAS persistence root binding is unsupported on this platform")
}
