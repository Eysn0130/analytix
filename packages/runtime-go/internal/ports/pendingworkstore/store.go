package pendingworkstore

import (
	"context"
	"errors"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

var ErrNotFound = errors.New("pending work record not found")

type Store interface {
	PutReceiptIfAbsent(context.Context, domainpendingwork.PendingWorkReceiptV1) error
	// CreateReceiptExclusive reports whether this caller performed the unique
	// content-addressed create. Exact replay is valid storage state but returns
	// created=false so it can never mint a second executable write lease.
	CreateReceiptExclusive(context.Context, domainpendingwork.PendingWorkReceiptV1) (created bool, err error)
	ReadReceipt(context.Context, string) (domainpendingwork.PendingWorkReceiptV1, error)
	ListReceipts(context.Context) ([]domainpendingwork.PendingWorkReceiptV1, error)
	PutDispositionIfAbsent(context.Context, domainpendingwork.PendingWorkDispositionV1) error
	ReadDisposition(context.Context, string) (domainpendingwork.PendingWorkDispositionV1, error)
	ListDispositions(context.Context) ([]domainpendingwork.PendingWorkDispositionV1, error)
	// SnapshotInventory reads both roots under the process-wide ownership gate
	// shared by every store for the same persistence root. Production also holds
	// the cross-process data-root lease for the store lifetime; adapters must
	// additionally reject a non-stable append-only observation.
	SnapshotInventory(context.Context) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error)
	HasRecords(context.Context) (bool, error)
}
