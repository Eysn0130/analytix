//go:build linux

package finalauthority

import "golang.org/x/sys/unix"

func secureMutableProjectionExchange(parent int, source, target string) error {
	return unix.Renameat2(parent, source, parent, target, unix.RENAME_EXCHANGE)
}
