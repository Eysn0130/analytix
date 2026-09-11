//go:build !darwin && !linux && !windows

package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func platformInspectLegacyCheckpointSnapshotQuarantine(
	ctx context.Context,
	binding privatecasport.RootBinding,
) (LegacyCheckpointSnapshotQuarantineStateV1, error) {
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotQuarantineStateV1{}, err
	}
	for _, candidate := range []string{
		filepath.Join(binding.RootPath, LegacyCheckpointSnapshotDirectoryV1),
		filepath.Join(binding.RootPath, "private", LegacyCheckpointSnapshotQuarantineV1),
	} {
		if _, err := os.Lstat(candidate); err == nil {
			return LegacyCheckpointSnapshotQuarantineStateV1{}, errors.New("legacy checkpoint quarantine is unsupported on this platform")
		} else if !errors.Is(err, os.ErrNotExist) {
			return LegacyCheckpointSnapshotQuarantineStateV1{}, err
		}
	}
	return LegacyCheckpointSnapshotQuarantineStateV1{}, nil
}

func platformMoveLegacyCheckpointSnapshotsToQuarantine(
	context.Context,
	privatecasport.RootBinding,
	string,
	LegacyCheckpointSnapshotTreeV1,
) (LegacyCheckpointSnapshotTreeV1, error) {
	return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine is unsupported on this platform")
}

func platformValidateColdRootAbsent(frozenRootCapability) error {
	return errors.New("legacy checkpoint cold-root inspection is unsupported on this platform")
}
