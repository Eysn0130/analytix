package fundsquerysource

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

const (
	dataAnalysisDirectoryV1 = "data-analysis"
	casesDirectoryV1        = "cases"
	snapshotsDirectoryV1    = "snapshots"
)

// Source derives one immutable snapshot location from the protected Electron
// userData root, the current case id and the DSV2-bound DuckDB hash. The root
// is host configuration; no renderer, model, MCP or tool argument can replace
// it. Platform code resolves every component with no-follow handles.
type Source struct {
	userDataRoot string
}

var _ fundsquerysourceport.HostExactSource = (*Source)(nil)
var _ fundsquerysourceport.ImmutableSnapshotPrepublicationSource = (*Source)(nil)

func NewHostExactSource(userDataRoot string) (*Source, error) {
	if userDataRoot == "" || userDataRoot != strings.TrimSpace(userDataRoot) ||
		!filepath.IsAbs(userDataRoot) || filepath.Clean(userDataRoot) != userDataRoot ||
		userDataRoot == string(filepath.Separator) || strings.ContainsRune(userDataRoot, 0) {
		return nil, errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query host source root is invalid"),
		)
	}
	return &Source{userDataRoot: userDataRoot}, nil
}

func (source *Source) WithExact(
	ctx context.Context,
	descriptor domainfundsquerysource.DescriptorV1,
	use func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	if source == nil || ctx == nil || use == nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query host source use is invalid"),
		)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return source.withExact(ctx, descriptor, use)
}

func (source *Source) WithInstalledExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	use func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	if source == nil || ctx == nil || use == nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds prepublication immutable source use is invalid"),
		)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return source.withInstalledExact(ctx, object, use)
}

func immutableSnapshotFileNameV1(descriptor domainfundsquerysource.DescriptorV1) string {
	if domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil {
		return ""
	}
	return descriptor.DuckDBSHA256 + ".duckdb"
}

func immutableSnapshotObjectFileNameV1(object domainfundsquerysource.ImmutableSnapshotObjectV1) string {
	if domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return ""
	}
	return object.DuckDBSHA256 + ".duckdb"
}
