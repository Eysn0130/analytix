package runtimeapp

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	attachmentstore "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const runtimeAttachmentAuthorityRootV1 = "data/private/attachment-authority"
const runtimeAttachmentFilesRootV1 = "data/attachments"

// Content blobs remain hash/size observations. Only metadata and private
// records need original bytes for semantic parsing and signed Before replay.
type runtimeAttachmentEntryV1 struct {
	state domainstartup.SemanticEntryStateV1
	body  []byte
}
type runtimeAttachmentFilesV1 map[string]runtimeAttachmentEntryV1

type runtimeAttachmentSemanticPreservationV1 struct {
	mu             sync.Mutex
	recoveryBefore runtimeAttachmentFilesV1
	held           map[string]domainstartup.SemanticEntryStateV1
	sharedModes    map[string]uint32
	createResidues *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
}

type runtimeAttachmentSemanticObservationV1 struct {
	prepared           *attachmentstore.PreparedOriginalInventoryV1
	journal            *persistencefs.AuthenticatedSemanticJournalObservationV1
	revalidateContexts func(context.Context) error
}

func (observation *runtimeAttachmentSemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil || observation.revalidateContexts == nil {
		return errors.New("original attachment observation is unavailable")
	}
	err := observation.prepared.Revalidate(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return errors.Join(err, observation.revalidateContexts(ctx))
}

func runtimeAttachmentOwnedPathV1(name string) bool {
	return name == runtimeAttachmentAuthorityRootV1 || strings.HasPrefix(name, runtimeAttachmentAuthorityRootV1+"/") || name == runtimeAttachmentFilesRootV1 || strings.HasPrefix(name, runtimeAttachmentFilesRootV1+"/")
}

func runtimeAttachmentNeedsBodyV1(name string) bool {
	return strings.HasPrefix(name, runtimeAttachmentAuthorityRootV1+"/") || strings.HasPrefix(name, runtimeAttachmentFilesRootV1+"/metadata/")
}

func (files runtimeAttachmentFilesV1) cloneV1() runtimeAttachmentFilesV1 {
	copied := runtimeAttachmentFilesV1{}
	for name, entry := range files {
		entry.body = append([]byte(nil), entry.body...)
		copied[name] = entry
	}
	return copied
}

func (files runtimeAttachmentFilesV1) stateV1(name string) domainstartup.SemanticEntryStateV1 {
	if entry, found := files[name]; found {
		return entry.state
	}
	return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
}

func (files runtimeAttachmentFilesV1) applyV1(operation domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
	if operation.After.Type == domainstartup.ManagedEntryTypeAbsent {
		delete(files, operation.Path)
		return nil
	}
	entry := runtimeAttachmentEntryV1{state: operation.After}
	if operation.After.Type == domainstartup.ManagedEntryTypeFile {
		if operation.Kind == domainstartup.SemanticOperationSetMode {
			old, found := files[operation.Path]
			if !found || old.state.Type != domainstartup.ManagedEntryTypeFile || old.state.Size != operation.After.Size || old.state.SHA256 != operation.After.SHA256 {
				return errors.New("attachment mode operation changed original bytes")
			}
			entry.body = old.body
		} else {
			if readAfter == nil {
				return errors.New("attachment semantic After bytes are unavailable")
			}
			body, err := readAfter(operation)
			if err != nil {
				return err
			}
			if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
				return errors.New("attachment semantic After bytes lost integrity")
			}
			if runtimeAttachmentNeedsBodyV1(operation.Path) {
				entry.body = append([]byte(nil), body...)
			}
		}
	} else if operation.After.Type != domainstartup.ManagedEntryTypeDirectory {
		return errors.New("attachment semantic entry type is invalid")
	}
	files[operation.Path] = entry
	return nil
}

func readRuntimeAttachmentBoundBodyV1(ctx context.Context, name string, state domainstartup.SemanticEntryStateV1) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if state.Type != domainstartup.ManagedEntryTypeFile || state.Size < 0 || state.Size > domainstartup.MaxSemanticManagedFileBytesV1 {
		return nil, errors.New("original attachment body size or kind is invalid")
	}
	before, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || uint32(before.Mode()) != state.Mode || before.Size() != state.Size {
		return nil, errors.New("original attachment metadata observation changed")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, opened) {
		return nil, errors.Join(errors.New("original attachment metadata retargeted"), statErr, file.Close())
	}
	body, readErr := io.ReadAll(io.LimitReader(file, state.Size+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(readErr, statErr, closeErr, ctx.Err()); err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(body)) != state.Size || domainsecurity.SHA256Hex(body) != state.SHA256 {
		return nil, errors.New("original attachment metadata bytes changed")
	}
	return body, nil
}

func (files runtimeAttachmentFilesV1) verifyV1(ctx context.Context, core *runtimeChildIdentityStartupV1, contexts []domainsecurity.TurnSecurityContext, scope *pendingworkapp.ReportRestartScopeV1, physical *attachmentstore.PreparedOriginalInventoryV1) (attachmentstore.OriginalInventoryV1, error) {
	private := map[string]attachmentstore.OriginalEntryV1{}
	for name, entry := range files {
		if err := ctx.Err(); err != nil {
			return attachmentstore.OriginalInventoryV1{}, err
		}
		if !runtimeAttachmentOwnedPathV1(name) || path.Clean(name) != name || strings.Contains(name, "\\") {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment address is invalid")
		}
		if entry.state.Type != domainstartup.ManagedEntryTypeDirectory && entry.state.Type != domainstartup.ManagedEntryTypeFile {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment kind is invalid")
		}
		if entry.state.Mode & ^(uint32(os.ModeDir)|0o777) != 0 || entry.state.Mode&0o777 == 0 || runtime.GOOS != "windows" && entry.state.Mode&0o077 != 0 {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment mode is invalid")
		}
		if name != runtimeAttachmentAuthorityRootV1 && name != runtimeAttachmentFilesRootV1 {
			if parent, found := files[path.Dir(name)]; !found || parent.state.Type != domainstartup.ManagedEntryTypeDirectory {
				return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment parent is absent")
			}
		}
		if name == runtimeAttachmentAuthorityRootV1 || strings.HasPrefix(name, runtimeAttachmentAuthorityRootV1+"/") {
			relative := strings.TrimPrefix(name, runtimeAttachmentAuthorityRootV1+"/")
			if name == runtimeAttachmentAuthorityRootV1 {
				relative = "."
			}
			private[relative] = attachmentstore.OriginalEntryV1{Directory: entry.state.Type == domainstartup.ManagedEntryTypeDirectory, Mode: entry.state.Mode & 0o777, Body: entry.body}
		} else if name != runtimeAttachmentFilesRootV1 {
			relative := strings.TrimPrefix(name, runtimeAttachmentFilesRootV1+"/")
			parts := strings.Split(relative, "/")
			if len(parts) == 1 {
				if (relative != "metadata" && relative != "content") || entry.state.Type != domainstartup.ManagedEntryTypeDirectory {
					return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment ordinary directory is invalid")
				}
			} else if len(parts) != 2 || entry.state.Type != domainstartup.ManagedEntryTypeFile || !filestore.ValidOriginalAttachmentFileV1(parts[0], parts[1]) {
				return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment ordinary file grammar is invalid")
			}
		}
	}
	inventory, err := physical.ParseOriginalEndpointV1(ctx, private, core.verification)
	if err != nil {
		return attachmentstore.OriginalInventoryV1{}, err
	}
	byContext := map[string]domainsecurity.TurnSecurityContext{}
	ownersByDigest := map[string]domainattachment.OwnerRecordV1{}
	for _, owner := range inventory.Owners {
		ownersByDigest[owner.OwnerDigest] = owner
	}
	for _, frozen := range contexts {
		if _, exists := byContext[frozen.ContextDigest]; exists {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment primary context is duplicated")
		}
		byContext[frozen.ContextDigest] = frozen
	}
	for _, receipt := range inventory.UseReceipts {
		frozen, found := byContext[receipt.Context.ContextDigest]
		expected := domainattachment.AttachmentUseContextBindingV1{ContextVersion: frozen.Version, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, WorkspaceRealPath: frozen.WorkspaceRealPath, TenantID: frozen.TenantID, UserID: frozen.UserID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, DatasetSnapshotID: frozen.DatasetSnapshotID, SourceManifestHash: frozen.SourceManifestHash, ContextEpoch: frozen.ContextEpoch, ContextIssuedAt: frozen.IssuedAt, ContextDigest: frozen.ContextDigest}
		if !found || receipt.Context != expected {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment use is detached from its full frozen context")
		}
		if !domainattachment.AttachmentUseOwnerMatchesFrozenContextV1(ownersByDigest[receipt.OwnerDigest], frozen) {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment owner crosses its use frozen context")
		}
	}
	owners := map[string]domainattachment.OwnerRecordV1{}
	for _, owner := range inventory.Owners {
		owners[owner.AttachmentID] = owner
	}
	closed := map[string]bool{}
	for _, disposition := range inventory.UploadDispositions {
		closed[disposition.UploadID] = true
	}
	for _, owner := range inventory.Owners {
		metadata, exists := files[runtimeAttachmentFilesRootV1+"/metadata/"+owner.AttachmentID+".json"]
		content := files.stateV1(runtimeAttachmentFilesRootV1 + "/content/" + owner.AttachmentID + ".bin")
		if !exists || metadata.state.Type != domainstartup.ManagedEntryTypeFile || content.Type != domainstartup.ManagedEntryTypeFile || content.Size != owner.ByteSize || content.SHA256 != owner.BlobSHA256 {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original owner-backed attachment files are incomplete or inconsistent")
		}
		observed, err := filestore.ParseOriginalAttachmentMetadataV1(owner.AttachmentID, metadata.body)
		if err != nil || !reflect.DeepEqual(observed, owner) {
			return attachmentstore.OriginalInventoryV1{}, errors.Join(errors.New("original attachment metadata owner differs"), err)
		}
	}
	for _, intent := range inventory.UploadIntents {
		_, hasOwner := owners[intent.Owner.AttachmentID]
		if !hasOwner && (!scope.OwnsThread(intent.Owner.ThreadID) || closed[intent.UploadID]) {
			continue
		}
		metadata := files.stateV1(runtimeAttachmentFilesRootV1 + "/metadata/" + intent.Owner.AttachmentID + ".json")
		content := files.stateV1(runtimeAttachmentFilesRootV1 + "/content/" + intent.Owner.AttachmentID + ".bin")
		if metadata.Type != domainstartup.ManagedEntryTypeAbsent && (metadata.Type != domainstartup.ManagedEntryTypeFile || metadata.SHA256 != intent.MetadataSHA256) || content.Type != domainstartup.ManagedEntryTypeAbsent && (content.Type != domainstartup.ManagedEntryTypeFile || content.Size != intent.Owner.ByteSize || content.SHA256 != intent.Owner.BlobSHA256) {
			return attachmentstore.OriginalInventoryV1{}, errors.New("original attachment intent files differ")
		}
	}
	return inventory, nil
}

func (files runtimeAttachmentFilesV1) heldV1(inventory attachmentstore.OriginalInventoryV1, scope *pendingworkapp.ReportRestartScopeV1) map[string]domainstartup.SemanticEntryStateV1 {
	held := map[string]domainstartup.SemanticEntryStateV1{}
	independent := map[string]bool{}
	knownPrivate := map[string]bool{}
	mark := func(leaf, digest, thread string) {
		name := runtimeAttachmentAuthorityRootV1 + "/" + leaf + "/" + digest[:2] + "/" + digest + ".json"
		knownPrivate[name] = true
		if scope.OwnsThread(thread) {
			held[name] = files.stateV1(name)
		}
	}
	markOwner := func(owner domainattachment.OwnerRecordV1) {
		independent[owner.AttachmentID] = !scope.OwnsThread(owner.ThreadID)
		if scope.OwnsThread(owner.ThreadID) {
			for _, name := range []string{runtimeAttachmentFilesRootV1 + "/metadata/" + owner.AttachmentID + ".json", runtimeAttachmentFilesRootV1 + "/content/" + owner.AttachmentID + ".bin"} {
				held[name] = files.stateV1(name)
			}
		}
	}
	byUpload := map[string]string{}
	byUse := map[string]string{}
	for _, owner := range inventory.Owners {
		markOwner(owner)
		mark("owners", owner.OwnerDigest, owner.ThreadID)
	}
	for _, intent := range inventory.UploadIntents {
		markOwner(intent.Owner)
		byUpload[intent.UploadID] = intent.Owner.ThreadID
		mark("upload-intents", intent.UploadID, intent.Owner.ThreadID)
	}
	for _, disposition := range inventory.UploadDispositions {
		mark("upload-dispositions", disposition.UploadID, byUpload[disposition.UploadID])
	}
	for _, receipt := range inventory.UseReceipts {
		byUse[receipt.UseID] = receipt.Context.ThreadID
		mark("use-receipts", receipt.UseID, receipt.Context.ThreadID)
	}
	for _, disposition := range inventory.UseDispositions {
		mark("use-dispositions", disposition.UseID, byUse[disposition.UseID])
	}
	for name, entry := range files {
		if entry.state.Type != domainstartup.ManagedEntryTypeFile {
			continue
		}
		if strings.HasPrefix(name, runtimeAttachmentAuthorityRootV1+"/") {
			if !knownPrivate[name] {
				held[name] = entry.state
			}
		} else if !independent[strings.TrimSuffix(path.Base(name), path.Ext(name))] {
			held[name] = entry.state
		}
	}
	return held
}

func (preserved *runtimeAttachmentSemanticPreservationV1) validateHeldV1(files runtimeAttachmentFilesV1, inventory attachmentstore.OriginalInventoryV1, scope *pendingworkapp.ReportRestartScopeV1) error {
	if !reflect.DeepEqual(preserved.held, files.heldV1(inventory, scope)) {
		return errors.New("semantic attachment candidate changed original held inventory")
	}
	for name, mode := range preserved.sharedModes {
		if state := files.stateV1(name); state.Type != domainstartup.ManagedEntryTypeDirectory || state.Mode != mode {
			return errors.New("semantic attachment candidate changed original shared directory")
		}
	}
	return nil
}

func readRuntimeAttachmentSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1, saved runtimeAttachmentFilesV1, createResidues *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (_ runtimeAttachmentFilesV1, _ runtimeAttachmentFilesV1, _ []domainsecurity.TurnSecurityContext, _ *runtimeAttachmentSemanticObservationV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil || scope == nil {
		return nil, nil, nil, nil, errors.New("original attachment authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, nil, nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	prepared, err := attachmentstore.PrepareOriginalInventoryWithCreateResiduesV1(ctx, filepath.Join(core.roots.DataDir, "private", "attachment-authority"), core.access, createResidues)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	private, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	physical := runtimeAttachmentFilesV1{}
	for name, entry := range private {
		label := runtimeAttachmentAuthorityRootV1
		if name != "." {
			label += "/" + name
		}
		state := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: entry.Mode, Size: int64(len(entry.Body)), SHA256: domainsecurity.SHA256Hex(entry.Body)}
		if entry.Directory {
			state = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeDirectory, Mode: uint32(os.ModeDir) | entry.Mode}
		}
		physical[label] = runtimeAttachmentEntryV1{state: state, body: entry.Body}
	}
	snapshot, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, core.roots)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	contexts, revalidateContexts, err := runtimeRegistryContextsV1(ctx, core)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation := &runtimeAttachmentSemanticObservationV1{prepared: prepared, journal: journal, revalidateContexts: func(ctx context.Context) error {
		current, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, core.roots)
		if err != nil || !reflect.DeepEqual(snapshot, current) {
			return errors.Join(errors.New("original attachment managed inventory changed"), err)
		}
		return revalidateContexts(ctx)
	}}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	for _, entry := range snapshot.Entries {
		if entry.Path != runtimeAttachmentFilesRootV1 && !strings.HasPrefix(entry.Path, runtimeAttachmentFilesRootV1+"/") || entry.Type == domainstartup.ManagedEntryTypeAbsent {
			continue
		}
		state := domainstartup.SemanticEntryStateV1{Type: entry.Type, Mode: entry.Mode}
		if entry.Type == domainstartup.ManagedEntryTypeFile {
			state.Size, state.SHA256 = entry.Size, entry.SHA256
		}
		value := runtimeAttachmentEntryV1{state: state}
		if entry.Type == domainstartup.ManagedEntryTypeFile && runtimeAttachmentNeedsBodyV1(entry.Path) {
			value.body, err = readRuntimeAttachmentBoundBodyV1(ctx, filepath.Join(core.roots.DataDir, filepath.FromSlash(strings.TrimPrefix(entry.Path, "data/"))), state)
			if err != nil {
				return nil, nil, nil, nil, err
			}
		}
		physical[entry.Path] = value
	}
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		if err := validateRuntimeAttachmentCreateOperationV1(createResidues, operation); err != nil {
			return nil, nil, nil, nil, err
		}
		for _, root := range []string{runtimeAttachmentAuthorityRootV1, runtimeAttachmentFilesRootV1} {
			if err := validateRuntimeOriginalSemanticAncestorV1(root, operation); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if !runtimeAttachmentOwnedPathV1(operation.Path) {
			continue
		}
		state := physical.stateV1(operation.Path)
		if state != operation.Before {
			if index > journal.NextOperationV1() || state != operation.After {
				return nil, nil, nil, nil, errors.New("attachment physical state is outside authenticated prefix")
			}
			after, err := journal.PhysicallyAfterV1(ctx, operation)
			if err != nil || !after {
				return nil, nil, nil, nil, errors.Join(errors.New("attachment physical state is not signed After"), err)
			}
		}
		if operation.Before.Type == domainstartup.ManagedEntryTypeAbsent {
			delete(original, operation.Path)
		} else {
			entry := physical[operation.Path]
			if operation.Before.Type == domainstartup.ManagedEntryTypeFile && runtimeAttachmentNeedsBodyV1(operation.Path) && (int64(len(entry.body)) != operation.Before.Size || domainsecurity.SHA256Hex(entry.body) != operation.Before.SHA256) {
				previous, found := saved[operation.Path]
				if !found || previous.state != operation.Before {
					return nil, nil, nil, nil, errors.New("attachment original Before bytes are unavailable")
				}
				entry = previous
			}
			entry.state = operation.Before
			if entry.state.Type == domainstartup.ManagedEntryTypeDirectory {
				entry.body = nil
			}
			original[operation.Path] = entry
		}
		if err := final.applyV1(operation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	for _, endpoint := range []runtimeAttachmentFilesV1{original, final} {
		if _, err := endpoint.verifyV1(ctx, core, contexts, scope, prepared); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	return original, physical, contexts, observation, nil
}

func prepareRuntimeAttachmentSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (_ *runtimeAttachmentSemanticPreservationV1, resultErr error) {
	if ctx == nil || core == nil || scope == nil {
		return nil, errors.New("original attachment preservation configuration is unavailable")
	}
	originalCreates := core.originalCreates
	if originalCreates == nil {
		// Standalone observation callers have no root startup session. Actual
		// startup always supplies its pre-bootstrap immutable observation.
		var err error
		originalCreates, err = prepareRuntimeOriginalCreateStartupV1(ctx, core.roots, core.access)
		if err != nil {
			return nil, err
		}
	}
	if err := originalCreates.revalidateV1(ctx, core.roots); err != nil {
		return nil, err
	}
	createResidues := originalCreates.attachment
	if createResidues == nil {
		return nil, errors.New("original attachment creation proof was not retained")
	}
	original, final, contexts, observation, err := readRuntimeAttachmentSemanticInventoryV1(ctx, core, scope, nil, createResidues)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	inventory, err := original.verifyV1(ctx, core, contexts, scope, observation.prepared)
	if err != nil {
		return nil, err
	}
	preserved := &runtimeAttachmentSemanticPreservationV1{recoveryBefore: original.cloneV1(), held: original.heldV1(inventory, scope), sharedModes: map[string]uint32{}, createResidues: createResidues}
	for name, entry := range original {
		if entry.state.Type == domainstartup.ManagedEntryTypeDirectory {
			preserved.sharedModes[name] = entry.state.Mode
		}
	}
	for _, operation := range observation.journal.OperationsV1() {
		if runtimeAttachmentOwnedPathV1(operation.Path) {
			if err := final.applyV1(operation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return observation.journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return nil, err
			}
		}
	}
	inventory, err = final.verifyV1(ctx, core, contexts, scope, observation.prepared)
	if err != nil {
		return nil, err
	}
	if err := preserved.validateHeldV1(final, inventory, scope); err != nil {
		return nil, err
	}
	return preserved, nil
}

func (preserved runtimeReportRestartPreservationV1) validateAttachmentSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.attachments == nil || preserved.core == nil || preserved.report == nil {
		return errors.New("original attachment preservation is unavailable")
	}
	preserved.attachments.mu.Lock()
	defer preserved.attachments.mu.Unlock()
	original, candidate, contexts, observation, err := readRuntimeAttachmentSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.attachments.recoveryBefore, preserved.attachments.createResidues)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
		if resultErr == nil {
			preserved.attachments.recoveryBefore = original.cloneV1()
		}
	}()
	inventory, err := original.verifyV1(ctx, preserved.core, contexts, preserved.report, observation.prepared)
	if err != nil {
		return err
	}
	if err := preserved.attachments.validateHeldV1(original, inventory, preserved.report); err != nil {
		return err
	}
	for _, operation := range observation.journal.OperationsV1() {
		if runtimeAttachmentOwnedPathV1(operation.Path) {
			if err := candidate.applyV1(operation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return observation.journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return err
			}
		}
	}
	for _, operation := range operations {
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		if err := validateRuntimeAttachmentCreateOperationV1(preserved.attachments.createResidues, operation); err != nil {
			return err
		}
		for _, root := range []string{runtimeAttachmentAuthorityRootV1, runtimeAttachmentFilesRootV1} {
			if err := validateRuntimeOriginalSemanticAncestorV1(root, operation); err != nil {
				return err
			}
		}
		if !runtimeAttachmentOwnedPathV1(operation.Path) {
			continue
		}
		if current := candidate.stateV1(operation.Path); current != operation.Before && current != operation.After {
			return errors.New("attachment semantic candidate changed before transition")
		}
		if err := candidate.applyV1(operation, readAfter); err != nil {
			return err
		}
	}
	inventory, err = candidate.verifyV1(ctx, preserved.core, contexts, preserved.report, observation.prepared)
	if err != nil {
		return err
	}
	return preserved.attachments.validateHeldV1(candidate, inventory, preserved.report)
}

// Residues outside the owner remain physical-only evidence. Exact names and
// their ancestors are denied here without injecting a wider logical owner.
func validateRuntimeAttachmentCreateOperationV1(proof *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1, operation domainstartup.SemanticStartupOperationV1) error {
	if proof == nil {
		return errors.New("original attachment creation proof is unavailable")
	}
	for _, relative := range proof.RelativePathsV1() {
		name := "data/" + relative
		if operation.Path == name || strings.HasPrefix(operation.Path, name+"/") {
			return errors.New("semantic program touches an original attachment creation residue")
		}
		if err := validateRuntimeOriginalSemanticAncestorV1(name, operation); err != nil {
			return err
		}
	}
	return nil
}
