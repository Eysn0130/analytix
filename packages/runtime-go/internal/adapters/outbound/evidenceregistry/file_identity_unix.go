//go:build !windows

package evidenceregistry

import (
	"errors"
	"os"
	"syscall"
)

func registryRegularFileLinkCount(_ *os.File, info os.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("evidence registry file identity is unavailable")
	}
	return uint64(stat.Nlink), nil
}
