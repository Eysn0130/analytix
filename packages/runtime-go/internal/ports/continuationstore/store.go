package continuationstore

import (
	"context"
	"errors"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

var ErrNotFound = errors.New("continuation record not found")

type Store interface {
	PutReceiptIfAbsent(context.Context, domaincontinuation.Receipt) error
	ResolveReceipt(context.Context, string) (domaincontinuation.Receipt, error)
	PutDispositionIfAbsent(context.Context, domaincontinuation.Disposition) error
	ResolveDisposition(context.Context, string) (domaincontinuation.Disposition, error)
	HasRecords(context.Context) (bool, error)
}

// InventoryStore exposes the complete private continuation inventory for
// startup trust verification. Visitors must discard accumulated state when a
// visit returns an error; a partial visit is never authority.
type InventoryStore interface {
	VisitReceipts(context.Context, func(domaincontinuation.Receipt) error) error
	VisitDispositions(context.Context, func(domaincontinuation.Disposition) error) error
}
