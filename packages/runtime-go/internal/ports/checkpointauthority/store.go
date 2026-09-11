package checkpointauthority

import (
	"context"
	"errors"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

var (
	ErrNotFound    = errors.New("checkpoint authority record not found")
	ErrConflict    = errors.New("checkpoint authority record conflicts with existing authority")
	ErrCorrupt     = errors.New("checkpoint authority inventory is corrupt")
	ErrIncomplete  = errors.New("checkpoint authority inventory is incomplete")
	ErrQuarantined = errors.New("checkpoint authority operation is quarantined")
)

type Store interface {
	BeginOperationGroup(context.Context, domaincheckpoint.OperationGroupIntentInputV2) (domaincheckpoint.OperationGroupIntentV2, *domaincheckpoint.OperationGroupTerminalV2, bool, error)
	SettleOperationGroup(context.Context, string, string, string, []domaincheckpoint.ObservedOperationPathV2, time.Time) (domaincheckpoint.OperationGroupTerminalV2, error)
	ResolveOperationGroups(context.Context, string, string) ([]domaincheckpoint.MaterializedOperationGroupV2, error)
	OpenOperationGroups(context.Context) ([]domaincheckpoint.OperationGroupIntentV2, error)
	ListOperationGroups(context.Context) ([]domaincheckpoint.OperationGroupStateV2, error)
	BeginSnapshot(context.Context, domaincheckpoint.SnapshotIntentInputV1) (domaincheckpoint.SnapshotIntentV1, error)
	CompleteSnapshot(context.Context, string, bool, string, time.Time) (domaincheckpoint.SnapshotCompletionV1, error)
	AbortSnapshot(context.Context, string, string, time.Time) (domaincheckpoint.SnapshotDispositionV1, error)
	ResolveCheckpoint(context.Context, string, string) ([]domaincheckpoint.MaterializedSnapshotV1, error)
	HasRecords(context.Context) (bool, error)
}

// RecoveryStore is the stricter startup-recovery authority. Its receipt is
// available only when the current call performed the no-replace terminal
// commit; an already-existing equal terminal returns created=false.
type RecoveryStore interface {
	Store
	SettleOperationGroupForRecovery(context.Context, string, string, string, []domaincheckpoint.ObservedOperationPathV2, time.Time) (domaincheckpoint.OperationGroupTerminalV2, privatecasport.AdditionReceiptV2, bool, error)
}
