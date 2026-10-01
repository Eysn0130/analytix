//go:build analytix_dev_credentials && (darwin || linux)

package runtimeapp

import (
	"os"
	"syscall"
)

const developmentProviderAuthorityEnabled = true

func developmentProviderDirectorySecure(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint64(stat.Uid) == uint64(os.Getuid()) && info.Mode().Perm() == 0700
}
