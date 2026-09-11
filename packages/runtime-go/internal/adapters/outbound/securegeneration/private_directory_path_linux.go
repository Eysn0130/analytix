//go:build linux

package securegeneration

import (
	"os"
	"strconv"
	"strings"
)

func privateDirectoryDetached(fd int) (bool, error) {
	if fd < 0 {
		return false, ErrInvalidInput
	}
	target, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(fd))
	if err != nil {
		return false, err
	}
	return strings.HasSuffix(target, " (deleted)"), nil
}
