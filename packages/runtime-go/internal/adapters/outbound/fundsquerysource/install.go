package fundsquerysource

import (
	"context"
	"errors"
	"os"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

var _ fundsquerysourceport.ImmutableSnapshotInstaller = (*Source)(nil)

// InstallExact commits one completed analytical DuckDB into the same private,
// case-scoped immutable namespace consumed by WithExact. Publication of a DSV2
// selection remains a separate later step.
func (source *Source) InstallExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	input *os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	if source == nil || ctx == nil || input == nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return "", errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds immutable snapshot install input is invalid"),
		)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return source.installExact(ctx, object, input)
}
