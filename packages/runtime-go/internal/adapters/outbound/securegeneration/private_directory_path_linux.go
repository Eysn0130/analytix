//go:build linux

package securegeneration

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func privateDirectoryDetached(fd int) (bool, error) {
	if fd < 0 {
		return false, ErrInvalidInput
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return false, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return false, ErrUnsafeRoot
	}
	// A live name may itself end in " (deleted)". The procfs suffix alone
	// cannot establish detachment of the held inode.
	if stat.Nlink != 0 {
		return false, nil
	}
	target, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(fd))
	if err != nil {
		return false, err
	}
	return strings.HasSuffix(target, " (deleted)"), nil
}
