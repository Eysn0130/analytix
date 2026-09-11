//go:build darwin

package securegeneration

import "golang.org/x/sys/unix"

func renameNoReplace(parent int, source, target string) error {
	return unix.RenameatxNp(parent, source, parent, target, unix.RENAME_EXCL)
}
