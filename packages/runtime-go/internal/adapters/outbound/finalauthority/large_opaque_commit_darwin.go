//go:build darwin

package finalauthority

import (
	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
)

func largeOpaqueCommitNoReplace(int, string, int, string) (uint64, error) {
	return 0, ErrLargeOpaqueUnsupportedPlatform
}

func largeOpaquePlatformValidateCommitWitness(
	int,
	string,
	largeopaqueport.ExactRef,
	largeOpaqueUnixFileIdentity,
) error {
	return ErrLargeOpaqueUnsupportedPlatform
}
