//go:build !windows

package providerregistryfs

import (
	"os"
	"path/filepath"

	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	"golang.org/x/sys/unix"
)

func openLegacySourceFile(path string) (*os.File, error) {
	// NONBLOCK prevents a replaced FIFO from waiting for a writer before the
	// common reader can validate the opened file's type and physical identity.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	file := os.NewFile(uintptr(fd), filepath.Base(path))
	if file == nil {
		_ = unix.Close(fd)
		return nil, registryport.ErrVerification
	}
	return file, nil
}
