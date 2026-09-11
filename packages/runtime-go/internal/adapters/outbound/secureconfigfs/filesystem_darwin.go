//go:build darwin

package secureconfigfs

import (
	"errors"

	"golang.org/x/sys/unix"
)

func probeSecureFilesystem(fd int) (secureFilesystemIdentity, error) {
	var stat unix.Statfs_t
	if err := unix.Fstatfs(fd, &stat); err != nil {
		return secureFilesystemIdentity{}, err
	}
	return validateDarwinSecureFilesystem(stat)
}

func validateDarwinSecureFilesystem(stat unix.Statfs_t) (secureFilesystemIdentity, error) {
	kind, ok := darwinFilesystemName(stat.Fstypename)
	forbidden := uint32(unix.MNT_IGNORE_OWNERSHIP | unix.MNT_AUTOMOUNTED | unix.MNT_REMOVABLE | unix.MNT_UNION | unix.MNT_SNAPSHOT)
	if !ok || kind != "apfs" || stat.Flags&unix.MNT_LOCAL == 0 || stat.Flags&forbidden != 0 {
		return secureFilesystemIdentity{}, errors.New("secure configuration requires a managed local APFS filesystem")
	}
	return secureFilesystemIdentity{
		kind: kind, fsid: [2]int64{int64(stat.Fsid.Val[0]), int64(stat.Fsid.Val[1])},
	}, nil
}

func darwinFilesystemName(raw [16]byte) (string, bool) {
	end := -1
	for index, value := range raw {
		if value == 0 {
			end = index
			break
		}
		if value < 'a' || value > 'z' {
			return "", false
		}
	}
	if end <= 0 {
		return "", false
	}
	for _, value := range raw[end:] {
		if value != 0 {
			return "", false
		}
	}
	return string(raw[:end]), true
}
