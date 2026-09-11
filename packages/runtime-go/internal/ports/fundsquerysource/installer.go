package fundsquerysource

import (
	"context"
	"os"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
)

// ImmutableSnapshotInstallDispositionV1 distinguishes a no-replace install
// performed by this call from an exact immutable object already present at the
// same case-scoped address. Neither outcome grants DSV2 use authority.
type ImmutableSnapshotInstallDispositionV1 string

const (
	ImmutableSnapshotInstallCreatedV1       ImmutableSnapshotInstallDispositionV1 = "created"
	ImmutableSnapshotInstallExistingEqualV1 ImmutableSnapshotInstallDispositionV1 = "existing_equal"
)

// ImmutableSnapshotInstaller installs a completed, closed analytical DuckDB
// by exact descriptor. The source must be an immutable host-owned file
// descriptor; implementations commit without replacement and verify the
// durable destination before returning.
type ImmutableSnapshotInstaller interface {
	InstallExact(
		context.Context,
		domainfundsquerysource.ImmutableSnapshotObjectV1,
		*os.File,
	) (ImmutableSnapshotInstallDispositionV1, error)
}

// ImmutableSnapshotPrepublicationSource opens one already-installed immutable
// object by its host-derived case and content identity before a DSV2 selection
// exists. The object remains inert storage: this seam is restricted to the
// synchronous snapshot-admission validator and grants no query or publication
// authority of its own.
type ImmutableSnapshotPrepublicationSource interface {
	WithInstalledExact(
		context.Context,
		domainfundsquerysource.ImmutableSnapshotObjectV1,
		func(context.Context, ExactReadLease) error,
	) error
}
