//go:build !darwin

package fundsquerysource

import (
	"context"
	"errors"
	"os"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

func (*Source) installExact(
	context.Context,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	*os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	return "", errors.Join(
		fundsquerysourceport.ErrUnavailable,
		errors.New("funds immutable snapshot installation is unsupported"),
	)
}
