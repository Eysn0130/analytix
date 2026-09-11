package fundsquerysource

import (
	"context"
	"errors"
	"os"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
)

var (
	ErrUnavailable = errors.New("funds query source is unavailable")
	ErrNotFound    = errors.New("funds query source is not found")
	ErrMismatch    = errors.New("funds query source binding mismatch")
	ErrCorrupt     = errors.New("funds query source is corrupt")
)

// ExactReadLease is callback-scoped inside one DSV2 UseExact invocation. The
// operation context is the current runner/owner lifecycle context; a copy must
// observe it in addition to the lease's DSV2 authority context. The destination
// must be a host-owned, empty, unlinked local file; implementations validate its
// concrete filesystem identity before copying any private byte and reject pipes,
// linked files, second, concurrent and post-callback copies. A concrete *os.File
// keeps arbitrary writers and caller-controlled blocking callbacks out of this
// private data seam.
type ExactReadLease interface {
	CopyExactTo(context.Context, *os.File) error
}

// HostExactSource opens only the immutable DuckDB object derived by trusted
// host policy from a path-free DSV2 descriptor. It exposes no inventory,
// caller-selected path, SQL surface, reusable handle or durable mapping.
type HostExactSource interface {
	WithExact(
		context.Context,
		domainfundsquerysource.DescriptorV1,
		func(context.Context, ExactReadLease) error,
	) error
}
