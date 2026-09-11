package attachmentauthority

import (
	"context"
	"errors"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
)

var (
	ErrNotFound         = errors.New("attachment authority record is not found")
	ErrConflict         = errors.New("attachment authority record conflicts")
	ErrCorrupt          = errors.New("attachment authority record is corrupt")
	ErrUnavailable      = errors.New("attachment authority store is unavailable")
	ErrRestartPreserved = errors.New("attachment authority is preserved after restart")
)

// RestartPreservationV1 exposes only the startup owner's original, verified
// denial scope. Revalidation covers the complete original attachment graph;
// it never supplies current upload, use or publication authority.
type RestartPreservationV1 interface {
	RevalidateAttachmentRestartV1(context.Context) error
	RestartPreservesThreadV1(string) bool
}

type Store interface {
	PutOwnerIfAbsent(context.Context, domainattachment.OwnerRecordV1) error
	ResolveOwner(context.Context, string) (domainattachment.OwnerRecordV1, error)
	PutUseReceiptIfAbsent(context.Context, domainattachment.AttachmentUseReceiptV1) error
	ResolveUseReceipt(context.Context, string) (domainattachment.AttachmentUseReceiptV1, error)
	PutUseDispositionIfAbsent(context.Context, domainattachment.AttachmentUseDispositionV1) error
	ResolveUseDisposition(context.Context, string) (domainattachment.AttachmentUseDispositionV1, error)
}

type InventoryStore interface {
	Store
	VisitOwners(context.Context, func(domainattachment.OwnerRecordV1) error) error
	VisitUseReceipts(context.Context, func(domainattachment.AttachmentUseReceiptV1) error) error
	VisitUseDispositions(context.Context, func(domainattachment.AttachmentUseDispositionV1) error) error
	HasRecords(context.Context) (bool, error)
}

// UploadTransactionStore owns the private, immutable transaction records that
// make an attachment upload recoverable across the ordinary-file/private-CAS
// commit boundary. UploadID is the mutually exclusive key for dispositions.
type UploadTransactionStore interface {
	PutUploadIntentIfAbsent(context.Context, domainattachment.UploadIntentV1) error
	ResolveUploadIntent(context.Context, string) (domainattachment.UploadIntentV1, error)
	CommitOwnerForOpenUpload(context.Context, domainattachment.UploadIntentV1) error
	PutUploadDispositionIfAbsent(context.Context, domainattachment.UploadDispositionV1) error
	ResolveUploadDisposition(context.Context, string) (domainattachment.UploadDispositionV1, error)
}

type UploadInventoryStore interface {
	InventoryStore
	UploadTransactionStore
	VisitUploadIntents(context.Context, func(domainattachment.UploadIntentV1) error) error
	VisitUploadDispositions(context.Context, func(domainattachment.UploadDispositionV1) error) error
}
