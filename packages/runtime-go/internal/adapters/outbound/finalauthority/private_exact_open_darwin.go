//go:build darwin

package finalauthority

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

var privateDarwinCaseInsensitiveDevices sync.Map

// On a case-insensitive Darwin volume, one directory cannot contain two names
// that differ only by case. Openat plus F_GETPATH therefore proves the exact
// stored component without enumerating the parent. Case-sensitive and unknown
// volumes retain the bounded directory scan owned by the portable caller.
func platformPrivateCASUnixOpenExactBoundDirectory(
	parent int,
	component string,
	device uint64,
) (int, bool, bool, error) {
	if parent < 0 || device == 0 || component == "" || component == "." || component == ".." ||
		filepath.Base(component) != component || strings.ContainsRune(component, 0) {
		return -1, false, true, errors.New("private CAS Darwin bound component is invalid")
	}
	caseInsensitive, available := privateDarwinDirectoryCaseInsensitive(parent, device)
	if !available || !caseInsensitive {
		return -1, false, false, nil
	}
	fd, err := unix.Openat(
		parent,
		component,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if errors.Is(err, unix.ENOENT) {
		return -1, false, true, nil
	}
	if err != nil {
		return -1, false, true, err
	}
	var resolved [unix.PathMax]byte
	_, _, errno := unix.Syscall(
		unix.SYS_FCNTL,
		uintptr(fd),
		uintptr(unix.F_GETPATH),
		uintptr(unsafe.Pointer(&resolved[0])),
	)
	end := bytes.IndexByte(resolved[:], 0)
	if errno != 0 || end <= 0 || resolved[0] != '/' || filepath.Base(string(resolved[:end])) != component {
		_ = unix.Close(fd)
		if errno != 0 {
			return -1, false, true, errno
		}
		return -1, false, true, errors.New("private CAS Darwin path aliases a bound component")
	}
	return fd, true, true, nil
}

func privateDarwinDirectoryCaseInsensitive(parent int, device uint64) (bool, bool) {
	if cached, ok := privateDarwinCaseInsensitiveDevices.Load(device); ok {
		return cached.(bool), true
	}
	value, err := unix.Fpathconf(parent, privateDarwinPathconfCaseSensitive)
	if err != nil {
		return false, false
	}
	caseInsensitive := value == 0
	cached, _ := privateDarwinCaseInsensitiveDevices.LoadOrStore(device, caseInsensitive)
	return cached.(bool), true
}
