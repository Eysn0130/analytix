//go:build linux

package secureconfigfs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const maxMountInfoBytes = 1 << 20

func probeSecureFilesystem(fd int) (secureFilesystemIdentity, error) {
	var statfs unix.Statfs_t
	if err := unix.Fstatfs(fd, &statfs); err != nil {
		return secureFilesystemIdentity{}, err
	}
	var statx unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &statx); err != nil ||
		statx.Mask&unix.STATX_MNT_ID == 0 || statx.Mnt_id == 0 {
		return secureFilesystemIdentity{}, errors.New("secure configuration mount identity is unavailable")
	}
	mountInfo, err := readBoundedProcMountInfo()
	if err != nil {
		return secureFilesystemIdentity{}, err
	}
	kind, err := mountFilesystemTypeForID(mountInfo, statx.Mnt_id)
	if err != nil || !linuxFilesystemAllowed(statfs.Type, kind) {
		return secureFilesystemIdentity{}, errors.New("secure configuration requires a managed local ext4 or XFS filesystem")
	}
	return secureFilesystemIdentity{
		kind: kind, fsid: [2]int64{int64(statfs.Fsid.Val[0]), int64(statfs.Fsid.Val[1])}, mountID: statx.Mnt_id,
	}, nil
}

func linuxFilesystemAllowed(magic int64, kind string) bool {
	switch kind {
	case "ext4":
		return magic == unix.EXT4_SUPER_MAGIC
	case "xfs":
		return magic == unix.XFS_SUPER_MAGIC
	default:
		return false
	}
}

func readBoundedProcMountInfo() ([]byte, error) {
	fd, err := unix.Open("/proc/self/mountinfo", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "proc-self-mountinfo")
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("mountinfo handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	var statfs unix.Statfs_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		unix.Fstatfs(fd, &statfs) != nil || statfs.Type != unix.PROC_SUPER_MAGIC {
		return nil, errors.New("mountinfo is not provided by procfs")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxMountInfoBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxMountInfoBytes {
		return nil, errors.New("mountinfo exceeds its bounded contract")
	}
	return body, nil
}

func mountFilesystemTypeForID(body []byte, mountID uint64) (string, error) {
	if mountID == 0 || len(body) == 0 || len(body) > maxMountInfoBytes || bytes.IndexByte(body, 0) >= 0 {
		return "", errors.New("mountinfo input is invalid")
	}
	found := ""
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			return "", errors.New("mountinfo line is malformed")
		}
		parsedID, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil || parsedID == 0 {
			return "", errors.New("mountinfo ID is invalid")
		}
		separator := -1
		for index := 6; index < len(fields); index++ {
			if fields[index] == "-" {
				separator = index
				break
			}
		}
		if separator < 0 || separator+3 >= len(fields) {
			return "", errors.New("mountinfo separator or tail is invalid")
		}
		if parsedID != mountID {
			continue
		}
		if found != "" || fields[separator+1] == "" {
			return "", errors.New("mountinfo mount ID is duplicated or empty")
		}
		found = fields[separator+1]
	}
	if found == "" {
		return "", errors.New("mountinfo mount ID is absent")
	}
	return found, nil
}
