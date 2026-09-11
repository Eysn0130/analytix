package checkpointfile

import (
	"context"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
)

type BeforeState struct {
	PathAuthority    PathAuthority
	Existed          bool
	ContentAvailable bool
	Hash             string
	Encoding         string
	RawBytes         []byte
}

// PathAuthority is the host-issued filesystem root binding for one mutation.
// Root is retained only in the private checkpoint authority; ordinary events
// and provider-visible tool results must project RootHash instead.
type PathAuthority struct {
	SchemaVersion int
	Kind          string
	Root          string
	RootIdentity  string
	RootHash      string
	RelativePath  string
}

type Observer interface {
	CaptureBefore(context.Context, string, string) (BeforeState, error)
	Observe(context.Context, string, PathAuthority, string) domaincheckpoint.ObservedOperationPathV2
	ObserveRelative(context.Context, string, PathAuthority) domaincheckpoint.ObservedOperationPathV2
}

// PreparedOperationRecovery is a read-only semantic decision frozen from one
// durable open intent. Apply must revalidate the exact authority before any
// filesystem mutation.
type PreparedOperationRecovery interface {
	Apply(context.Context) error
}

type OperationRecoveryPlanner interface {
	PrepareOperationRecovery(context.Context, domaincheckpoint.OperationGroupIntentV2) (PreparedOperationRecovery, error)
}
