//go:build darwin || linux

package persistencefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func platformSeparateOwnerDirectoryBinding(path string, requirePrivateRoot bool) (string, string, error) {
	fd, err := secureOpenAbsoluteDirectory(path, false)
	if err != nil {
		return "", "", err
	}
	defer unix.Close(fd)
	beforeIdentity, beforeSecurity, err := captureSeparateOwnerDirectoryBinding(fd, requirePrivateRoot)
	if err != nil {
		return "", "", err
	}
	afterIdentity, afterSecurity, err := captureSeparateOwnerDirectoryBinding(fd, requirePrivateRoot)
	if err != nil || beforeIdentity != afterIdentity || beforeSecurity != afterSecurity {
		return "", "", errors.New("separate-owner Unix directory security changed while binding")
	}
	return afterIdentity, afterSecurity, nil
}

func captureSeparateOwnerDirectoryBinding(fd int, ownerRoot bool) (string, string, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", "", errors.New("separate-owner Unix directory identity is unavailable")
	}
	if ownerRoot && (int(stat.Uid) != os.Geteuid() || stat.Mode&0o022 != 0 || stat.Mode&0o7000 != 0) {
		return "", "", errors.New("separate-owner Unix root owner or write permissions are unsafe")
	}
	scope := separateOwnerAncestorScope
	if ownerRoot {
		scope = separateOwnerObjectScope
	}
	securityDigest, err := separateOwnerUnixSecurityDigest(fd, stat, "directory", scope)
	if err != nil {
		return "", "", err
	}
	return unixDirectoryIdentity(stat), securityDigest, nil
}
