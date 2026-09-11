//go:build darwin

package finalauthority

import "golang.org/x/sys/unix"

func secureMutableProjectionExchange(parent int, source, target string) error {
	return unix.RenameatxNp(parent, source, parent, target, unix.RENAME_SWAP)
}
