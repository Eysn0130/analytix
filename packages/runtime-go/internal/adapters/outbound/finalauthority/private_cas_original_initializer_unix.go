//go:build darwin || linux

package finalauthority

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

func securePrivateCASInitializeOriginalLeafV1(binding privatecasport.RootBinding, initial privateCASOriginalLeafInitializationV1) error {
	fd, err := privateCASUnixOpenBoundRoot(binding, true, initial)
	if err != nil {
		return err
	}
	return unix.Close(fd)
}
