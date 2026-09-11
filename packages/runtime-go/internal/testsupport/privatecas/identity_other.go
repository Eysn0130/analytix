//go:build !darwin && !linux && !windows

package privatecas

import (
	"errors"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func platformTestDirectoryIdentity(string) (privatecasport.DirectoryIdentity, error) {
	return privatecasport.DirectoryIdentity{}, errors.New("test private CAS directory identity is unsupported")
}
