//go:build !darwin

package fundsquerysource

import (
	"context"
	"errors"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

func (*Source) withExact(
	context.Context,
	domainfundsquerysource.DescriptorV1,
	func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	return errors.Join(
		fundsquerysourceport.ErrUnavailable,
		errors.New("funds query immutable host source is unsupported"),
	)
}

func (*Source) withInstalledExact(
	context.Context,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	return errors.Join(
		fundsquerysourceport.ErrUnavailable,
		errors.New("funds query prepublication immutable host source is unsupported"),
	)
}
