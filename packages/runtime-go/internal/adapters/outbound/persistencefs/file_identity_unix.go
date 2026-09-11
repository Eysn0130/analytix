//go:build !windows

package persistencefs

import (
	"fmt"
	"os"
	"syscall"
)

func regularFileIdentity(_ *os.File, info os.FileInfo) (string, uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", 0, fmt.Errorf("file identity unavailable")
	}
	return formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino)), uint64(stat.Nlink), nil
}
