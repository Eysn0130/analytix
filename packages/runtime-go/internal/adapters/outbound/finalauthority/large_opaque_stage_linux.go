//go:build linux

package finalauthority

import (
	"context"
	"io"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	"golang.org/x/sys/unix"
)

func largeOpaqueAnonymousPutSupported() bool { return true }

func largeOpaquePutInputSupported(io.Reader) bool { return true }

func largeOpaqueCreateStagingFile(parent int, _ string) (int, bool, uint64, error) {
	fd, err := unix.Openat(
		parent,
		".",
		unix.O_RDWR|unix.O_TMPFILE|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0o600,
	)
	return fd, false, 0, err
}

func largeOpaquePutExactWithoutAnonymousStaging(
	context.Context,
	privateCASRootAuthority,
	largeopaqueport.ExactRef,
	io.Reader,
) (largeopaqueport.PutOutcome, error) {
	return 0, ErrLargeOpaqueUnsupportedPlatform
}
