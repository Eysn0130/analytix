//go:build linux

package finalauthority

import (
	"errors"
	"os"
	"strconv"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	"golang.org/x/sys/unix"
)

// /proc/self/fd resolves the held verified descriptor, not its mutable temp
// basename. linkat creates the destination atomically and without replacement.
// Hosts without a trustworthy procfs descriptor projection fail closed.
func largeOpaqueCommitNoReplace(parent int, _ string, sourceFD int, target string) (uint64, error) {
	source := "/proc/self/fd/" + strconv.Itoa(sourceFD)
	err := unix.Linkat(unix.AT_FDCWD, source, parent, target, unix.AT_SYMLINK_FOLLOW)
	if errors.Is(err, unix.EEXIST) {
		return 0, os.ErrExist
	}
	return 1, err
}

func largeOpaquePlatformValidateCommitWitness(
	parent int,
	target string,
	reference largeopaqueport.ExactRef,
	staged largeOpaqueUnixFileIdentity,
) error {
	committed, err := largeOpaqueUnixCommitWitnessIdentity(parent, target, reference, staged.device)
	if err != nil {
		return err
	}
	if committed.device != staged.device || committed.inode != staged.inode {
		return errors.Join(
			ErrLargeOpaqueIntegrity,
			errors.New("large opaque Linux link witness is not the staged inode"),
		)
	}
	return nil
}
