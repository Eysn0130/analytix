package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sync"

	attachmentstore "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	attachmentport "analytix.local/runtime-go/internal/ports/attachmentauthority"
)

// Activation remains read-only. A live CAS store is opened only after a
// concrete independent write has passed the complete original graph guard.
type runtimeAttachmentRestartStoreV1 struct {
	mu        sync.Mutex
	preserved runtimeReportRestartPreservationV1
	root      string
	access    finalauthority.SecurePrivateCASRecoveryAccessAuthority
	live      *attachmentstore.Store
}

var _ attachmentport.UploadInventoryStore = (*runtimeAttachmentRestartStoreV1)(nil)
var _ attachmentport.RestartPreservationV1 = (*runtimeAttachmentRestartStoreV1)(nil)

func (preserved runtimeReportRestartPreservationV1) OpenAttachmentRestartStoreV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (attachmentport.UploadInventoryStore, error) {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return attachmentstore.NewStoreContext(ctx, root, access)
	}
	if preserved.core == nil || access == nil || root != filepath.Join(preserved.core.roots.DataDir, "private", "attachment-authority") {
		return nil, errors.New("attachment restart store root binding differs")
	}
	if err := preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return nil, err
	}
	return &runtimeAttachmentRestartStoreV1{preserved: preserved, root: root, access: preserved.core.access}, nil
}

func (store *runtimeAttachmentRestartStoreV1) RevalidateAttachmentRestartV1(ctx context.Context) error {
	return store.preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, "")
}

func (store *runtimeAttachmentRestartStoreV1) RestartPreservesThreadV1(id string) bool {
	return store.preserved.report != nil && store.preserved.report.OwnsThread(id)
}

func (store *runtimeAttachmentRestartStoreV1) observeV1(ctx context.Context) (inventory attachmentstore.OriginalInventoryV1, files runtimeAttachmentFilesV1, resultErr error) {
	preserved := store.preserved
	if preserved.attachments == nil || preserved.core == nil || preserved.report == nil {
		return inventory, nil, errors.New("attachment restart observation is unavailable")
	}
	preserved.attachments.mu.Lock()
	defer preserved.attachments.mu.Unlock()
	original, final, contexts, observation, err := readRuntimeAttachmentSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.attachments.recoveryBefore, preserved.attachments.createResidues)
	if err != nil {
		return inventory, nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
		if resultErr != nil {
			inventory = attachmentstore.OriginalInventoryV1{}
			files = nil
		}
	}()
	// Store operations cannot interleave with an unfinished semantic journal.
	// That journal has a separate original/Final validator and recovery owner.
	if observation.journal != nil {
		return inventory, nil, errors.New("attachment store is unavailable during semantic recovery")
	}
	inventory, err = original.verifyV1(ctx, preserved.core, contexts, preserved.report, observation.prepared)
	if err != nil {
		return inventory, nil, err
	}
	if err := preserved.attachments.validateHeldV1(original, inventory, preserved.report); err != nil {
		return inventory, nil, err
	}
	if err := preserved.attachments.validateHeldV1(final, inventory, preserved.report); err != nil {
		return inventory, nil, err
	}
	return inventory, original, nil
}

func (store *runtimeAttachmentRestartStoreV1) writeV1(ctx context.Context, leaf, digest string, body []byte, threadFor func(attachmentstore.OriginalInventoryV1) (string, error), apply func(*attachmentstore.Store) error) (resultErr error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, files, err := store.observeV1(ctx)
	if err != nil {
		return err
	}
	thread, err := threadFor(inventory)
	if err != nil {
		return err
	}
	if store.RestartPreservesThreadV1(thread) {
		return attachmentport.ErrRestartPreserved
	}
	if !domainsecurity.IsSHA256Hex(digest) {
		return errors.New("attachment restart write identity is invalid")
	}
	name := runtimeAttachmentAuthorityRootV1 + "/" + leaf + "/" + digest[:2] + "/" + digest + ".json"
	operations := []domainstartup.SemanticStartupOperationV1{}
	for _, directory := range []string{runtimeAttachmentAuthorityRootV1, runtimeAttachmentAuthorityRootV1 + "/owners", runtimeAttachmentAuthorityRootV1 + "/upload-intents", runtimeAttachmentAuthorityRootV1 + "/upload-dispositions", runtimeAttachmentAuthorityRootV1 + "/use-receipts", runtimeAttachmentAuthorityRootV1 + "/use-dispositions", path.Dir(name)} {
		before := files.stateV1(directory)
		if before.Type == domainstartup.ManagedEntryTypeAbsent {
			operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationCreateDirectory, Path: directory, Before: before, After: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeDirectory, Mode: uint32(os.ModeDir) | 0o700}})
		}
	}
	operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationInstallFile, Path: name, Before: files.stateV1(name), After: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}})
	if err := store.preserved.validateAttachmentSemanticOperationsV1(ctx, operations, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		if operation.Path != name {
			return nil, errors.New("attachment write requested foreign After bytes")
		}
		return body, nil
	}, ""); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, store.RevalidateAttachmentRestartV1(ctx)) }()
	if store.live == nil {
		for _, name := range []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"} {
			if files.stateV1(runtimeAttachmentAuthorityRootV1+"/"+name).Type == domainstartup.ManagedEntryTypeAbsent {
				if err := store.preserved.attachments.createResidues.InitializeLeafDirectoriesV1(ctx, filepath.Join(store.root, name), store.access); err != nil {
					return err
				}
			}
		}
		var prepared *attachmentstore.PreparedRecoveryV1
		prepared, err = attachmentstore.PrepareRecoveryPreservingOriginalCreateResiduesV1(ctx, store.root, store.access, store.preserved.attachments.createResidues)
		if err == nil {
			err = store.RevalidateAttachmentRestartV1(ctx)
		}
		if err == nil {
			store.live, err = prepared.OpenPreservingOriginalResiduesV1(ctx)
		}
		if err != nil {
			return err
		}
	}
	return apply(store.live)
}

func attachmentWriteThreadV1(id string) func(attachmentstore.OriginalInventoryV1) (string, error) {
	return func(attachmentstore.OriginalInventoryV1) (string, error) { return id, nil }
}

func (store *runtimeAttachmentRestartStoreV1) PutOwnerIfAbsent(ctx context.Context, owner domainattachment.OwnerRecordV1) error {
	body, err := domainattachment.OwnerRecordV1Bytes(owner)
	if err != nil {
		return err
	}
	return store.writeV1(ctx, "owners", owner.OwnerDigest, body, attachmentWriteThreadV1(owner.ThreadID), func(live *attachmentstore.Store) error { return live.PutOwnerIfAbsent(ctx, owner) })
}
func (store *runtimeAttachmentRestartStoreV1) CommitOwnerForOpenUpload(ctx context.Context, intent domainattachment.UploadIntentV1) error {
	if err := domainattachment.ValidateUploadIntentV1(intent); err != nil {
		return err
	}
	body, err := domainattachment.OwnerRecordV1Bytes(intent.Owner)
	if err != nil {
		return err
	}
	return store.writeV1(ctx, "owners", intent.Owner.OwnerDigest, body, attachmentWriteThreadV1(intent.Owner.ThreadID), func(live *attachmentstore.Store) error { return live.CommitOwnerForOpenUpload(ctx, intent) })
}
func (store *runtimeAttachmentRestartStoreV1) PutUploadIntentIfAbsent(ctx context.Context, intent domainattachment.UploadIntentV1) error {
	body, err := domainattachment.UploadIntentV1Bytes(intent)
	if err != nil {
		return err
	}
	return store.writeV1(ctx, "upload-intents", intent.UploadID, body, attachmentWriteThreadV1(intent.Owner.ThreadID), func(live *attachmentstore.Store) error { return live.PutUploadIntentIfAbsent(ctx, intent) })
}
func (store *runtimeAttachmentRestartStoreV1) PutUploadDispositionIfAbsent(ctx context.Context, disposition domainattachment.UploadDispositionV1) error {
	body, err := domainattachment.UploadDispositionV1Bytes(disposition)
	if err != nil {
		return err
	}
	return store.writeV1(ctx, "upload-dispositions", disposition.UploadID, body, func(inventory attachmentstore.OriginalInventoryV1) (string, error) {
		for _, intent := range inventory.UploadIntents {
			if intent.UploadID == disposition.UploadID {
				return intent.Owner.ThreadID, nil
			}
		}
		return "", errors.Join(attachmentport.ErrCorrupt, attachmentport.ErrNotFound)
	}, func(live *attachmentstore.Store) error { return live.PutUploadDispositionIfAbsent(ctx, disposition) })
}
func (store *runtimeAttachmentRestartStoreV1) PutUseReceiptIfAbsent(ctx context.Context, receipt domainattachment.AttachmentUseReceiptV1) error {
	body, err := domainattachment.AttachmentUseReceiptV1Bytes(receipt)
	if err != nil {
		return err
	}
	return store.writeV1(ctx, "use-receipts", receipt.UseID, body, attachmentWriteThreadV1(receipt.Context.ThreadID), func(live *attachmentstore.Store) error { return live.PutUseReceiptIfAbsent(ctx, receipt) })
}
func (store *runtimeAttachmentRestartStoreV1) PutUseDispositionIfAbsent(ctx context.Context, disposition domainattachment.AttachmentUseDispositionV1) error {
	body, err := domainattachment.AttachmentUseDispositionV1Bytes(disposition)
	if err != nil {
		return err
	}
	return store.writeV1(ctx, "use-dispositions", disposition.UseID, body, func(inventory attachmentstore.OriginalInventoryV1) (string, error) {
		for _, receipt := range inventory.UseReceipts {
			if receipt.UseID == disposition.UseID {
				return receipt.Context.ThreadID, nil
			}
		}
		return "", errors.Join(attachmentport.ErrCorrupt, attachmentport.ErrNotFound)
	}, func(live *attachmentstore.Store) error { return live.PutUseDispositionIfAbsent(ctx, disposition) })
}

func (store *runtimeAttachmentRestartStoreV1) ResolveOwner(ctx context.Context, digest string) (domainattachment.OwnerRecordV1, error) {
	if !domainsecurity.IsSHA256Hex(digest) {
		return domainattachment.OwnerRecordV1{}, errors.New("attachment owner lookup identity is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return domainattachment.OwnerRecordV1{}, err
	}
	for _, record := range inventory.Owners {
		if record.OwnerDigest == digest {
			return record, nil
		}
	}
	return domainattachment.OwnerRecordV1{}, attachmentport.ErrNotFound
}
func (store *runtimeAttachmentRestartStoreV1) ResolveUploadIntent(ctx context.Context, id string) (domainattachment.UploadIntentV1, error) {
	if !domainsecurity.IsSHA256Hex(id) {
		return domainattachment.UploadIntentV1{}, errors.New("attachment upload lookup identity is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return domainattachment.UploadIntentV1{}, err
	}
	for _, record := range inventory.UploadIntents {
		if record.UploadID == id {
			return record, nil
		}
	}
	return domainattachment.UploadIntentV1{}, attachmentport.ErrNotFound
}
func (store *runtimeAttachmentRestartStoreV1) ResolveUploadDisposition(ctx context.Context, id string) (domainattachment.UploadDispositionV1, error) {
	if !domainsecurity.IsSHA256Hex(id) {
		return domainattachment.UploadDispositionV1{}, errors.New("attachment upload disposition lookup identity is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return domainattachment.UploadDispositionV1{}, err
	}
	for _, record := range inventory.UploadDispositions {
		if record.UploadID == id {
			return record, nil
		}
	}
	return domainattachment.UploadDispositionV1{}, attachmentport.ErrNotFound
}
func (store *runtimeAttachmentRestartStoreV1) ResolveUseReceipt(ctx context.Context, id string) (domainattachment.AttachmentUseReceiptV1, error) {
	if !domainsecurity.IsSHA256Hex(id) {
		return domainattachment.AttachmentUseReceiptV1{}, errors.New("attachment use lookup identity is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return domainattachment.AttachmentUseReceiptV1{}, err
	}
	for _, record := range inventory.UseReceipts {
		if record.UseID == id {
			return record, nil
		}
	}
	return domainattachment.AttachmentUseReceiptV1{}, attachmentport.ErrNotFound
}
func (store *runtimeAttachmentRestartStoreV1) ResolveUseDisposition(ctx context.Context, id string) (domainattachment.AttachmentUseDispositionV1, error) {
	if !domainsecurity.IsSHA256Hex(id) {
		return domainattachment.AttachmentUseDispositionV1{}, errors.New("attachment use disposition lookup identity is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return domainattachment.AttachmentUseDispositionV1{}, err
	}
	for _, record := range inventory.UseDispositions {
		if record.UseID == id {
			return record, nil
		}
	}
	return domainattachment.AttachmentUseDispositionV1{}, attachmentport.ErrNotFound
}

func (store *runtimeAttachmentRestartStoreV1) visitV1(ctx context.Context, visit func(attachmentstore.OriginalInventoryV1) error) (resultErr error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, store.RevalidateAttachmentRestartV1(ctx)) }()
	return visit(inventory)
}
func (store *runtimeAttachmentRestartStoreV1) VisitOwners(ctx context.Context, visit func(domainattachment.OwnerRecordV1) error) error {
	if visit == nil {
		return errors.New("attachment owner visitor is unavailable")
	}
	return store.visitV1(ctx, func(inventory attachmentstore.OriginalInventoryV1) error {
		for _, record := range inventory.Owners {
			if err := visit(record); err != nil {
				return err
			}
		}
		return nil
	})
}
func (store *runtimeAttachmentRestartStoreV1) VisitUploadIntents(ctx context.Context, visit func(domainattachment.UploadIntentV1) error) error {
	if visit == nil {
		return errors.New("attachment upload visitor is unavailable")
	}
	return store.visitV1(ctx, func(inventory attachmentstore.OriginalInventoryV1) error {
		for _, record := range inventory.UploadIntents {
			if err := visit(record); err != nil {
				return err
			}
		}
		return nil
	})
}
func (store *runtimeAttachmentRestartStoreV1) VisitUploadDispositions(ctx context.Context, visit func(domainattachment.UploadDispositionV1) error) error {
	if visit == nil {
		return errors.New("attachment upload disposition visitor is unavailable")
	}
	return store.visitV1(ctx, func(inventory attachmentstore.OriginalInventoryV1) error {
		for _, record := range inventory.UploadDispositions {
			if err := visit(record); err != nil {
				return err
			}
		}
		return nil
	})
}
func (store *runtimeAttachmentRestartStoreV1) VisitUseReceipts(ctx context.Context, visit func(domainattachment.AttachmentUseReceiptV1) error) error {
	if visit == nil {
		return errors.New("attachment use visitor is unavailable")
	}
	return store.visitV1(ctx, func(inventory attachmentstore.OriginalInventoryV1) error {
		for _, record := range inventory.UseReceipts {
			if err := visit(record); err != nil {
				return err
			}
		}
		return nil
	})
}
func (store *runtimeAttachmentRestartStoreV1) VisitUseDispositions(ctx context.Context, visit func(domainattachment.AttachmentUseDispositionV1) error) error {
	if visit == nil {
		return errors.New("attachment use disposition visitor is unavailable")
	}
	return store.visitV1(ctx, func(inventory attachmentstore.OriginalInventoryV1) error {
		for _, record := range inventory.UseDispositions {
			if err := visit(record); err != nil {
				return err
			}
		}
		return nil
	})
}
func (store *runtimeAttachmentRestartStoreV1) HasRecords(ctx context.Context) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, _, err := store.observeV1(ctx)
	if err != nil {
		return false, err
	}
	return len(inventory.Owners)+len(inventory.UploadIntents)+len(inventory.UploadDispositions)+len(inventory.UseReceipts)+len(inventory.UseDispositions) != 0, nil
}
