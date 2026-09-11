//go:build linux

package finalauthority

import (
	"errors"
	"strings"

	"golang.org/x/sys/unix"
)

func largeOpaquePlatformExactBasename(parent int, _ int, expected string) error {
	alias := strings.ToUpper(expected)
	if alias == expected {
		return errors.New("large opaque exact basename has no independent case probe")
	}
	aliasFD, err := unix.Openat(
		parent,
		alias,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return errors.Join(errors.New("large opaque case-fold probe failed closed"), err)
	}
	return errors.Join(errors.New("large opaque case-fold alias is present"), unix.Close(aliasFD))
}
