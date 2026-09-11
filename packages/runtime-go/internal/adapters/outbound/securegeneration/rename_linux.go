//go:build linux

package securegeneration

import "golang.org/x/sys/unix"

func renameNoReplace(parent int, source, target string) error {
	return unix.Renameat2(parent, source, parent, target, unix.RENAME_NOREPLACE)
}
