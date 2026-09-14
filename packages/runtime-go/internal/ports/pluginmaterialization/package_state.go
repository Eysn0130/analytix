package pluginmaterialization

import (
	"context"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
)

// ExpectedRevision is zero only for a generation without an activation record.
// Generation changes never inherit enabled state or reset an existing revision.
type SetDesiredStateRequestV1 struct {
	GenerationID     string
	ExpectedRevision uint64
	DesiredState     domainplugin.DesiredStateV1
}

// PackageStateStore is scoped to one exact package ID at construction. Missing
// activation returns ErrNotFound only after verifying the current materialization
// and storage container; it is never an enabled state or a synthetic receipt.
// Runtime reconciliation and session capability grants belong to its caller.
type PackageStateStore interface {
	Store
	ReadActivation(context.Context, InstallationAuthority) (domainplugin.ActivationV1, error)
	SetDesiredState(context.Context, SetDesiredStateRequestV1, InstallationAuthority, time.Time) (domainplugin.ActivationV1, error)
}
