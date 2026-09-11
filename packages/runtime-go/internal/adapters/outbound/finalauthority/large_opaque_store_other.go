//go:build !darwin && !linux

package finalauthority

import (
	"context"
	"io"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
)

func largeOpaqueAnonymousPutSupported() bool { return false }

func largeOpaquePutInputSupported(io.Reader) bool { return false }

func largeOpaquePutExactPlatform(
	context.Context,
	privateCASRootAuthority,
	largeopaqueport.ExactRef,
	io.Reader,
) (largeopaqueport.PutOutcome, error) {
	return 0, ErrLargeOpaqueUnsupportedPlatform
}

func largeOpaqueReadExactPlatform(
	context.Context,
	privateCASRootAuthority,
	largeopaqueport.ExactRef,
	io.Writer,
) error {
	return ErrLargeOpaqueUnsupportedPlatform
}
