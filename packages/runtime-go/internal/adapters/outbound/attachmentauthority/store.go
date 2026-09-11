package attachmentauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type Store struct {
	mu                   sync.Mutex
	root                 string
	ownerCAS             *finalauthorityadapter.SecurePrivateCAS
	receiptCAS           *finalauthorityadapter.SecurePrivateCAS
	dispositionCAS       *finalauthorityadapter.SecurePrivateCAS
	uploadIntentCAS      *finalauthorityadapter.SecurePrivateCAS
	uploadDispositionCAS *finalauthorityadapter.SecurePrivateCAS
}

var _ storeport.UploadInventoryStore = (*Store)(nil)

func NewStore(root string, access privatecasport.AccessAuthority) (*Store, error) {
	return NewStoreContext(context.Background(), root, access)
}

func NewStoreContext(ctx context.Context, root string, access privatecasport.AccessAuthority) (*Store, error) {
	absolute, err := attachmentAuthorityRoot(root, access)
	if err != nil {
		return nil, err
	}
	ownerCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "owners"), domainattachment.MaxOwnerRecordBytesV1, access,
	)
	if err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	receiptCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "use-receipts"), domainattachment.MaxAttachmentUseRecordBytesV1, access,
	)
	if err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	dispositionCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "use-dispositions"), domainattachment.MaxAttachmentUseRecordBytesV1, access,
	)
	if err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	uploadIntentCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "upload-intents"), domainattachment.MaxAttachmentUploadRecordBytesV1, access,
	)
	if err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	uploadDispositionCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "upload-dispositions"), domainattachment.MaxAttachmentUploadRecordBytesV1, access,
	)
	if err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	store := &Store{
		root: absolute, ownerCAS: ownerCAS, receiptCAS: receiptCAS, dispositionCAS: dispositionCAS,
		uploadIntentCAS: uploadIntentCAS, uploadDispositionCAS: uploadDispositionCAS,
	}
	if err := store.validateInventory(ctx); err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	return store, nil
}

func PreflightRecovery(ctx context.Context, root string, access privatecasport.RecoveryAccessAuthority) error {
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	return prepared.Revalidate(ctx)
}

func attachmentAuthorityRoot(root string, access privatecasport.AccessAuthority) (string, error) {
	if access == nil || root == "" || root != strings.TrimSpace(root) {
		return "", errors.New("attachment authority root or access authority is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return "", errors.New("attachment authority root is invalid")
	}
	return absolute, nil
}

func (store *Store) PutOwnerIfAbsent(ctx context.Context, owner domainattachment.OwnerRecordV1) error {
	if store == nil || store.ownerCAS == nil || domainattachment.ValidateOwnerRecordV1(owner) != nil {
		return errors.New("attachment owner authority is invalid")
	}
	body, err := domainattachment.OwnerRecordV1Bytes(owner)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, intentErr := store.readUploadIntent(ctx, uploadIDForOwner(owner))
	if intentErr == nil && !uploadIntentMatchesOwner(intent, owner) {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent owner is inconsistent"))
	}
	if intentErr != nil && !errors.Is(intentErr, storeport.ErrNotFound) {
		return intentErr
	}
	if disposition, dispositionErr := store.readUploadDisposition(ctx, uploadIDForOwner(owner)); dispositionErr == nil {
		if disposition.Status != domainattachment.UploadDispositionCommittedV1 {
			return errors.Join(storeport.ErrConflict, errors.New("attachment upload was already closed without owner authority"))
		}
	} else if !errors.Is(dispositionErr, storeport.ErrNotFound) {
		return dispositionErr
	}
	return store.putOwnerExactNoLock(ctx, owner, body)
}

func (store *Store) CommitOwnerForOpenUpload(ctx context.Context, intent domainattachment.UploadIntentV1) error {
	if store == nil || store.ownerCAS == nil || store.uploadIntentCAS == nil || store.uploadDispositionCAS == nil ||
		domainattachment.ValidateUploadIntentV1(intent) != nil {
		return errors.New("attachment open-upload owner commit is invalid")
	}
	body, err := domainattachment.OwnerRecordV1Bytes(intent.Owner)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	storedIntent, err := store.readUploadIntent(ctx, intent.UploadID)
	if err != nil || !sameUploadIntent(storedIntent, intent) {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent readback changed before owner commit"), err)
	}
	if disposition, dispositionErr := store.readUploadDisposition(ctx, intent.UploadID); dispositionErr == nil {
		if disposition.Status != domainattachment.UploadDispositionCommittedV1 {
			return errors.Join(storeport.ErrConflict, errors.New("attachment upload is already closed without owner authority"))
		}
	} else if !errors.Is(dispositionErr, storeport.ErrNotFound) {
		return dispositionErr
	}
	return store.putOwnerExactNoLock(ctx, intent.Owner, body)
}

func (store *Store) putOwnerExactNoLock(ctx context.Context, owner domainattachment.OwnerRecordV1, body []byte) error {
	return putExact(ctx, store.ownerCAS, owner.OwnerDigest, body, func(candidate []byte) error {
		parsed, err := parseCanonicalOwner(candidate)
		if err != nil || parsed.OwnerDigest != owner.OwnerDigest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment owner authority readback is invalid"), err)
		}
		return nil
	})
}

func (store *Store) PutUploadIntentIfAbsent(ctx context.Context, intent domainattachment.UploadIntentV1) error {
	if store == nil || store.uploadIntentCAS == nil || domainattachment.ValidateUploadIntentV1(intent) != nil {
		return errors.New("attachment upload intent authority is invalid")
	}
	body, err := domainattachment.UploadIntentV1Bytes(intent)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if owner, ownerErr := store.readOwner(ctx, intent.Owner.OwnerDigest); ownerErr == nil && !uploadIntentMatchesOwner(intent, owner) {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent conflicts with owner authority"))
	} else if ownerErr != nil && !errors.Is(ownerErr, storeport.ErrNotFound) {
		return ownerErr
	}
	return putExact(ctx, store.uploadIntentCAS, intent.UploadID, body, func(candidate []byte) error {
		parsed, err := parseCanonicalUploadIntent(candidate)
		if err != nil || parsed.UploadID != intent.UploadID {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent readback is invalid"), err)
		}
		return nil
	})
}

func (store *Store) ResolveUploadIntent(ctx context.Context, uploadID string) (domainattachment.UploadIntentV1, error) {
	if store == nil || store.uploadIntentCAS == nil || !canonicalDigest(uploadID) {
		return domainattachment.UploadIntentV1{}, errors.New("attachment upload intent lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readUploadIntent(ctx, uploadID)
}

func (store *Store) PutUploadDispositionIfAbsent(ctx context.Context, disposition domainattachment.UploadDispositionV1) error {
	if store == nil || store.uploadDispositionCAS == nil || domainattachment.ValidateUploadDispositionV1(disposition) != nil {
		return errors.New("attachment upload disposition authority is invalid")
	}
	body, err := domainattachment.UploadDispositionV1Bytes(disposition)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, err := store.readUploadIntent(ctx, disposition.UploadID)
	if err != nil || domainattachment.ValidateUploadDispositionForIntentV1(disposition, intent) != nil {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload disposition has no exact intent authority"), err)
	}
	owner, ownerErr := store.readOwner(ctx, intent.Owner.OwnerDigest)
	switch disposition.Status {
	case domainattachment.UploadDispositionCommittedV1:
		if ownerErr != nil || !uploadIntentMatchesOwner(intent, owner) {
			return errors.Join(storeport.ErrCorrupt, errors.New("committed attachment upload has no exact owner authority"), ownerErr)
		}
	default:
		if ownerErr == nil {
			return errors.Join(storeport.ErrConflict, errors.New("non-committed attachment upload already has owner authority"))
		}
		if !errors.Is(ownerErr, storeport.ErrNotFound) {
			return ownerErr
		}
	}
	return putExact(ctx, store.uploadDispositionCAS, disposition.UploadID, body, func(candidate []byte) error {
		parsed, err := parseCanonicalUploadDisposition(candidate)
		if err != nil || parsed.UploadID != disposition.UploadID {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload disposition readback is invalid"), err)
		}
		return nil
	})
}

func (store *Store) ResolveUploadDisposition(ctx context.Context, uploadID string) (domainattachment.UploadDispositionV1, error) {
	if store == nil || store.uploadDispositionCAS == nil || !canonicalDigest(uploadID) {
		return domainattachment.UploadDispositionV1{}, errors.New("attachment upload disposition lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readUploadDisposition(ctx, uploadID)
}

func (store *Store) ResolveOwner(ctx context.Context, ownerDigest string) (domainattachment.OwnerRecordV1, error) {
	if store == nil || store.ownerCAS == nil || !canonicalDigest(ownerDigest) {
		return domainattachment.OwnerRecordV1{}, errors.New("attachment owner lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readOwner(ctx, ownerDigest)
}

func (store *Store) PutUseReceiptIfAbsent(ctx context.Context, receipt domainattachment.AttachmentUseReceiptV1) error {
	if store == nil || store.receiptCAS == nil || domainattachment.ValidateAttachmentUseReceiptV1(receipt) != nil {
		return errors.New("attachment use receipt is invalid")
	}
	body, err := domainattachment.AttachmentUseReceiptV1Bytes(receipt)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	owner, err := store.readOwner(ctx, receipt.OwnerDigest)
	if err != nil || !receiptMatchesOwner(receipt, owner) {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment use receipt has no exact owner authority"), err)
	}
	return putExact(ctx, store.receiptCAS, receipt.UseID, body, func(candidate []byte) error {
		parsed, err := domainattachment.ParseAttachmentUseReceiptV1(candidate)
		if err != nil || parsed.UseID != receipt.UseID {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment use receipt readback is invalid"), err)
		}
		return nil
	})
}

func (store *Store) ResolveUseReceipt(ctx context.Context, useID string) (domainattachment.AttachmentUseReceiptV1, error) {
	if store == nil || store.receiptCAS == nil || !canonicalDigest(useID) {
		return domainattachment.AttachmentUseReceiptV1{}, errors.New("attachment use receipt lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readReceipt(ctx, useID)
}

func (store *Store) PutUseDispositionIfAbsent(ctx context.Context, disposition domainattachment.AttachmentUseDispositionV1) error {
	if store == nil || store.dispositionCAS == nil || domainattachment.ValidateAttachmentUseDispositionV1(disposition) != nil {
		return errors.New("attachment use disposition is invalid")
	}
	body, err := domainattachment.AttachmentUseDispositionV1Bytes(disposition)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, err := store.readReceipt(ctx, disposition.UseID)
	if err != nil || domainattachment.ValidateAttachmentUseDispositionForReceiptV1(disposition, receipt) != nil {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment use disposition has no exact receipt authority"), err)
	}
	return putExact(ctx, store.dispositionCAS, disposition.UseID, body, func(candidate []byte) error {
		parsed, err := domainattachment.ParseAttachmentUseDispositionV1(candidate)
		if err != nil || parsed.UseID != disposition.UseID {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment use disposition readback is invalid"), err)
		}
		return nil
	})
}

func (store *Store) ResolveUseDisposition(ctx context.Context, useID string) (domainattachment.AttachmentUseDispositionV1, error) {
	if store == nil || store.dispositionCAS == nil || !canonicalDigest(useID) {
		return domainattachment.AttachmentUseDispositionV1{}, errors.New("attachment use disposition lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readDisposition(ctx, useID)
}

func (store *Store) VisitOwners(ctx context.Context, visit func(domainattachment.OwnerRecordV1) error) error {
	if store == nil || visit == nil {
		return errors.New("attachment owner inventory visitor is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.visitOwners(ctx, visit)
}

func (store *Store) VisitUseReceipts(ctx context.Context, visit func(domainattachment.AttachmentUseReceiptV1) error) error {
	if store == nil || visit == nil {
		return errors.New("attachment receipt inventory visitor is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.visitReceipts(ctx, visit)
}

func (store *Store) VisitUseDispositions(ctx context.Context, visit func(domainattachment.AttachmentUseDispositionV1) error) error {
	if store == nil || visit == nil {
		return errors.New("attachment disposition inventory visitor is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.visitDispositions(ctx, visit)
}

func (store *Store) VisitUploadIntents(ctx context.Context, visit func(domainattachment.UploadIntentV1) error) error {
	if store == nil || visit == nil {
		return errors.New("attachment upload intent inventory visitor is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.visitUploadIntents(ctx, visit)
}

func (store *Store) VisitUploadDispositions(ctx context.Context, visit func(domainattachment.UploadDispositionV1) error) error {
	if store == nil || visit == nil {
		return errors.New("attachment upload disposition inventory visitor is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.visitUploadDispositions(ctx, visit)
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, errors.New("attachment authority store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	found := false
	for _, visit := range []func(context.Context, func() error) error{
		func(ctx context.Context, mark func() error) error {
			return store.visitOwners(ctx, func(domainattachment.OwnerRecordV1) error { return mark() })
		},
		func(ctx context.Context, mark func() error) error {
			return store.visitReceipts(ctx, func(domainattachment.AttachmentUseReceiptV1) error { return mark() })
		},
		func(ctx context.Context, mark func() error) error {
			return store.visitDispositions(ctx, func(domainattachment.AttachmentUseDispositionV1) error { return mark() })
		},
		func(ctx context.Context, mark func() error) error {
			return store.visitUploadIntents(ctx, func(domainattachment.UploadIntentV1) error { return mark() })
		},
		func(ctx context.Context, mark func() error) error {
			return store.visitUploadDispositions(ctx, func(domainattachment.UploadDispositionV1) error { return mark() })
		},
	} {
		if err := visit(ctx, func() error { found = true; return nil }); err != nil {
			return false, err
		}
	}
	return found, nil
}

func (store *Store) validateInventory(ctx context.Context) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	owners := map[string]domainattachment.OwnerRecordV1{}
	attachmentOwners := map[string]string{}
	if err := store.visitOwners(ctx, func(owner domainattachment.OwnerRecordV1) error {
		if existing := attachmentOwners[owner.AttachmentID]; existing != "" && existing != owner.OwnerDigest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment owner id collision is ambiguous"))
		}
		owners[owner.OwnerDigest] = owner
		attachmentOwners[owner.AttachmentID] = owner.OwnerDigest
		return nil
	}); err != nil {
		return err
	}
	intents := map[string]domainattachment.UploadIntentV1{}
	if err := store.visitUploadIntents(ctx, func(intent domainattachment.UploadIntentV1) error {
		if existing, ok := intents[intent.UploadID]; ok && !sameUploadIntent(existing, intent) {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent inventory conflicts"))
		}
		if owner, ok := owners[intent.Owner.OwnerDigest]; ok && !uploadIntentMatchesOwner(intent, owner) {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent owner inventory conflicts"))
		}
		intents[intent.UploadID] = intent
		return nil
	}); err != nil {
		return err
	}
	if err := store.visitUploadDispositions(ctx, func(disposition domainattachment.UploadDispositionV1) error {
		intent, ok := intents[disposition.UploadID]
		if !ok || domainattachment.ValidateUploadDispositionForIntentV1(disposition, intent) != nil {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload disposition intent inventory is incomplete"))
		}
		owner, hasOwner := owners[intent.Owner.OwnerDigest]
		if disposition.Status == domainattachment.UploadDispositionCommittedV1 {
			if !hasOwner || !uploadIntentMatchesOwner(intent, owner) {
				return errors.Join(storeport.ErrCorrupt, errors.New("committed attachment upload owner inventory is incomplete"))
			}
		} else if hasOwner {
			return errors.Join(storeport.ErrCorrupt, errors.New("non-committed attachment upload has owner authority"))
		}
		return nil
	}); err != nil {
		return err
	}
	receipts := map[string]domainattachment.AttachmentUseReceiptV1{}
	if err := store.visitReceipts(ctx, func(receipt domainattachment.AttachmentUseReceiptV1) error {
		owner, ok := owners[receipt.OwnerDigest]
		if !ok || !receiptMatchesOwner(receipt, owner) {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment use receipt owner inventory is incomplete"))
		}
		receipts[receipt.UseID] = receipt
		return nil
	}); err != nil {
		return err
	}
	return store.visitDispositions(ctx, func(disposition domainattachment.AttachmentUseDispositionV1) error {
		receipt, ok := receipts[disposition.UseID]
		if !ok || domainattachment.ValidateAttachmentUseDispositionForReceiptV1(disposition, receipt) != nil {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment use disposition receipt inventory is incomplete"))
		}
		return nil
	})
}

func (store *Store) readOwner(ctx context.Context, ownerDigest string) (domainattachment.OwnerRecordV1, error) {
	body, err := store.ownerCAS.Read(ctx, ownerDigest)
	if err != nil {
		return domainattachment.OwnerRecordV1{}, classifyReadError(err)
	}
	owner, err := parseCanonicalOwner(body)
	if err != nil || owner.OwnerDigest != ownerDigest {
		return domainattachment.OwnerRecordV1{}, errors.Join(storeport.ErrCorrupt, errors.New("attachment owner content address is invalid"), err)
	}
	return owner, nil
}

func (store *Store) readReceipt(ctx context.Context, useID string) (domainattachment.AttachmentUseReceiptV1, error) {
	body, err := store.receiptCAS.Read(ctx, useID)
	if err != nil {
		return domainattachment.AttachmentUseReceiptV1{}, classifyReadError(err)
	}
	receipt, err := domainattachment.ParseAttachmentUseReceiptV1(body)
	if err != nil || receipt.UseID != useID {
		return domainattachment.AttachmentUseReceiptV1{}, errors.Join(storeport.ErrCorrupt, errors.New("attachment use receipt content address is invalid"), err)
	}
	return receipt, nil
}

func (store *Store) readDisposition(ctx context.Context, useID string) (domainattachment.AttachmentUseDispositionV1, error) {
	body, err := store.dispositionCAS.Read(ctx, useID)
	if err != nil {
		return domainattachment.AttachmentUseDispositionV1{}, classifyReadError(err)
	}
	disposition, err := domainattachment.ParseAttachmentUseDispositionV1(body)
	if err != nil || disposition.UseID != useID {
		return domainattachment.AttachmentUseDispositionV1{}, errors.Join(storeport.ErrCorrupt, errors.New("attachment use disposition content address is invalid"), err)
	}
	return disposition, nil
}

func (store *Store) readUploadIntent(ctx context.Context, uploadID string) (domainattachment.UploadIntentV1, error) {
	body, err := store.uploadIntentCAS.Read(ctx, uploadID)
	if err != nil {
		return domainattachment.UploadIntentV1{}, classifyReadError(err)
	}
	intent, err := parseCanonicalUploadIntent(body)
	if err != nil || intent.UploadID != uploadID {
		return domainattachment.UploadIntentV1{}, errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent content address is invalid"), err)
	}
	return intent, nil
}

func (store *Store) readUploadDisposition(ctx context.Context, uploadID string) (domainattachment.UploadDispositionV1, error) {
	body, err := store.uploadDispositionCAS.Read(ctx, uploadID)
	if err != nil {
		return domainattachment.UploadDispositionV1{}, classifyReadError(err)
	}
	disposition, err := parseCanonicalUploadDisposition(body)
	if err != nil || disposition.UploadID != uploadID {
		return domainattachment.UploadDispositionV1{}, errors.Join(storeport.ErrCorrupt, errors.New("attachment upload disposition content address is invalid"), err)
	}
	return disposition, nil
}

func (store *Store) visitOwners(ctx context.Context, visit func(domainattachment.OwnerRecordV1) error) error {
	return store.ownerCAS.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		owner, err := parseCanonicalOwner(file.Body)
		if err != nil || owner.OwnerDigest != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment owner filename is not its semantic digest"), err)
		}
		return visit(owner)
	})
}

func (store *Store) visitReceipts(ctx context.Context, visit func(domainattachment.AttachmentUseReceiptV1) error) error {
	return store.receiptCAS.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainattachment.ParseAttachmentUseReceiptV1(file.Body)
		if err != nil || receipt.UseID != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment use receipt filename is not its use identity"), err)
		}
		return visit(receipt)
	})
}

func (store *Store) visitDispositions(ctx context.Context, visit func(domainattachment.AttachmentUseDispositionV1) error) error {
	return store.dispositionCAS.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainattachment.ParseAttachmentUseDispositionV1(file.Body)
		if err != nil || disposition.UseID != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment use disposition filename is not its use identity"), err)
		}
		return visit(disposition)
	})
}

func (store *Store) visitUploadIntents(ctx context.Context, visit func(domainattachment.UploadIntentV1) error) error {
	return store.uploadIntentCAS.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		intent, err := parseCanonicalUploadIntent(file.Body)
		if err != nil || intent.UploadID != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload intent filename is not its semantic identity"), err)
		}
		return visit(intent)
	})
}

func (store *Store) visitUploadDispositions(ctx context.Context, visit func(domainattachment.UploadDispositionV1) error) error {
	return store.uploadDispositionCAS.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := parseCanonicalUploadDisposition(file.Body)
		if err != nil || disposition.UploadID != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("attachment upload disposition filename is not its upload identity"), err)
		}
		return visit(disposition)
	})
}

func putExact(
	ctx context.Context,
	store *finalauthorityadapter.SecurePrivateCAS,
	key string,
	body []byte,
	validate func([]byte) error,
) error {
	if current, err := store.Read(ctx, key); err == nil {
		if validationErr := validate(current); validationErr != nil {
			return validationErr
		}
		if !bytes.Equal(current, body) {
			return errors.Join(storeport.ErrConflict, errors.New("attachment authority key already has different bytes"))
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(storeport.ErrUnavailable, err)
	}
	if err := store.PutIfAbsent(ctx, key, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return errors.Join(storeport.ErrUnavailable, err)
		}
		current, readErr := store.Read(ctx, key)
		if readErr != nil {
			return errors.Join(storeport.ErrUnavailable, readErr)
		}
		if validationErr := validate(current); validationErr != nil {
			return validationErr
		}
		if !bytes.Equal(current, body) {
			return errors.Join(storeport.ErrConflict, errors.New("attachment authority concurrent winner has different bytes"))
		}
		return nil
	}
	written, err := store.Read(ctx, key)
	if err != nil {
		return errors.Join(storeport.ErrUnavailable, err)
	}
	if !bytes.Equal(written, body) {
		return errors.Join(storeport.ErrCorrupt, errors.New("attachment authority write readback changed bytes"))
	}
	return validate(written)
}

func parseCanonicalOwner(body []byte) (domainattachment.OwnerRecordV1, error) {
	owner, err := domainattachment.ParseOwnerRecordV1(body)
	if err != nil {
		return domainattachment.OwnerRecordV1{}, err
	}
	canonical, err := domainattachment.OwnerRecordV1Bytes(owner)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainattachment.OwnerRecordV1{}, errors.New("attachment owner record is not canonically encoded")
	}
	return owner, nil
}

func parseCanonicalUploadIntent(body []byte) (domainattachment.UploadIntentV1, error) {
	intent, err := domainattachment.ParseUploadIntentV1(body)
	if err != nil {
		return domainattachment.UploadIntentV1{}, err
	}
	canonical, err := domainattachment.UploadIntentV1Bytes(intent)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainattachment.UploadIntentV1{}, errors.New("attachment upload intent is not canonically encoded")
	}
	return intent, nil
}

func parseCanonicalUploadDisposition(body []byte) (domainattachment.UploadDispositionV1, error) {
	disposition, err := domainattachment.ParseUploadDispositionV1(body)
	if err != nil {
		return domainattachment.UploadDispositionV1{}, err
	}
	canonical, err := domainattachment.UploadDispositionV1Bytes(disposition)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainattachment.UploadDispositionV1{}, errors.New("attachment upload disposition is not canonically encoded")
	}
	return disposition, nil
}

func uploadIntentMatchesOwner(intent domainattachment.UploadIntentV1, owner domainattachment.OwnerRecordV1) bool {
	if domainattachment.ValidateUploadIntentV1(intent) != nil || domainattachment.ValidateOwnerRecordV1(owner) != nil {
		return false
	}
	left, leftErr := domainattachment.OwnerRecordV1Bytes(intent.Owner)
	right, rightErr := domainattachment.OwnerRecordV1Bytes(owner)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func sameUploadIntent(left domainattachment.UploadIntentV1, right domainattachment.UploadIntentV1) bool {
	leftBody, leftErr := domainattachment.UploadIntentV1Bytes(left)
	rightBody, rightErr := domainattachment.UploadIntentV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func uploadIDForOwner(owner domainattachment.OwnerRecordV1) string {
	uploadID, _ := domainattachment.UploadIDForOwnerV1(owner)
	return uploadID
}

func receiptMatchesOwner(receipt domainattachment.AttachmentUseReceiptV1, owner domainattachment.OwnerRecordV1) bool {
	return domainattachment.ValidateAttachmentUseReceiptV1(receipt) == nil && domainattachment.ValidateOwnerRecordV1(owner) == nil &&
		receipt.OwnerDigest == owner.OwnerDigest && receipt.AttachmentID == owner.AttachmentID &&
		receipt.BlobSHA256 == owner.BlobSHA256 && receipt.BlobByteSize == owner.ByteSize && receipt.MIMEType == owner.MIMEType
}

func classifyReadError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return errors.Join(storeport.ErrNotFound, err)
	}
	if errors.Is(err, finalauthorityadapter.ErrSecurePrivateCASIntegrity) {
		return errors.Join(storeport.ErrCorrupt, err)
	}
	return errors.Join(storeport.ErrUnavailable, err)
}

func classifyAuthorityIntegrityError(err error) error {
	if errors.Is(err, finalauthorityadapter.ErrSecurePrivateCASIntegrity) {
		return errors.Join(storeport.ErrCorrupt, err)
	}
	return err
}

func canonicalDigest(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
