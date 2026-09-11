//go:build darwin

package finalauthority

import (
	"bytes"

	"golang.org/x/sys/unix"
)

func largeOpaquePlatformObjectSafe(fd int, stat unix.Stat_t) bool {
	if stat.Flags != 0 {
		return false
	}
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return false
	}
	if size == 0 {
		return true
	}
	names := make([]byte, size)
	read, err := unix.Flistxattr(fd, names)
	if err != nil || read != size {
		return false
	}
	for _, name := range bytes.Split(names, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		// macOS adds this non-security provenance marker to newly created
		// application files and directories. Its kernel record format is
		// validated as well; the name alone is not an authority claim.
		if !largeOpaqueDarwinProvenanceSafe(fd, string(name)) {
			return false
		}
	}
	return true
}

func largeOpaqueDarwinProvenanceSafe(fd int, name string) bool {
	if name != "com.apple.provenance" {
		return false
	}
	size, err := unix.Fgetxattr(fd, name, nil)
	if err != nil || size != 11 {
		return false
	}
	value := make([]byte, size)
	read, err := unix.Fgetxattr(fd, name, value)
	return err == nil && read == len(value) && validLargeOpaqueDarwinProvenance(value)
}

func validLargeOpaqueDarwinProvenance(value []byte) bool {
	if len(value) != 11 || !bytes.Equal(value[:3], []byte{0x01, 0x02, 0x00}) {
		return false
	}
	for _, octet := range value[3:] {
		if octet != 0 {
			return true
		}
	}
	return false
}
