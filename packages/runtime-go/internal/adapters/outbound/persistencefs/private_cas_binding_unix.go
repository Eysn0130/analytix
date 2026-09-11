//go:build darwin || linux

package persistencefs

import (
	"errors"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

func platformStrongDirectoryIdentity(path string) (privatecasport.DirectoryIdentity, string, error) {
	fd, err := secureOpenAbsoluteDirectory(path, false)
	if err != nil {
		return privatecasport.DirectoryIdentity{}, "", err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Dev == 0 || stat.Ino == 0 {
		return privatecasport.DirectoryIdentity{}, "", errors.New("private CAS persistence root identity is unavailable")
	}
	identity := privatecasport.DirectoryIdentity{
		Kind: privatecasport.DirectoryIdentityUnix, Device: uint64(stat.Dev), Inode: stat.Ino,
	}
	canonical, err := formatStrongDirectoryIdentity(identity)
	return identity, canonical, err
}
