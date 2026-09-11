//go:build linux

package finalauthority

import "golang.org/x/sys/unix"

func acceptedFinalCASUnixTimesEqual(left, right unix.Stat_t) bool {
	return left.Mtim == right.Mtim && left.Ctim == right.Ctim
}
