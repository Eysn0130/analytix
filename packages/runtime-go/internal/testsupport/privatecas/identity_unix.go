//go:build darwin || linux

package privatecas

import (
	"errors"
	"os"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

func platformTestDirectoryIdentity(path string) (privatecasport.DirectoryIdentity, error) {
	file, err := os.Open(path)
	if err != nil {
		return privatecasport.DirectoryIdentity{}, err
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return privatecasport.DirectoryIdentity{}, errors.New("test private CAS directory identity is unavailable")
	}
	return privatecasport.DirectoryIdentity{
		Kind: privatecasport.DirectoryIdentityUnix, Device: uint64(stat.Dev), Inode: stat.Ino,
	}, nil
}
