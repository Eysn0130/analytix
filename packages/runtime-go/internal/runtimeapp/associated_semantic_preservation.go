package runtimeapp

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	datasetstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidencestore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	datasetapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	datasetport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

var runtimeAssociatedOwnersV1 = [...]string{"dataset-snapshot-authority", "evidence-authority"}

type runtimeAssociatedOwnerPreservationV1 struct {
	original       runtimeOriginalSemanticFilesV1
	recoveryBefore runtimeOriginalSemanticFilesV1
	unavailable    bool
}

type runtimeAssociatedSemanticPreservationV1 struct {
	owners map[string]*runtimeAssociatedOwnerPreservationV1
}

type runtimeAssociatedPhysicalV1 interface {
	RevalidatePhysicalV1(context.Context) error
	SnapshotOriginalFilesV1(context.Context) (map[string]finalauthority.SecurePrivateCASOriginalEntryV1, error)
}

type runtimeAssociatedObservationV1 struct {
	owners             map[string]runtimeAssociatedPhysicalV1
	journal            *persistencefs.AuthenticatedSemanticJournalObservationV1
	revalidateContexts func(context.Context) error
}

func (observation *runtimeAssociatedObservationV1) Revalidate(ctx context.Context) error {
	var err error
	for _, owner := range observation.owners {
		err = errors.Join(err, owner.RevalidatePhysicalV1(ctx))
	}
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return errors.Join(err, observation.revalidateContexts(ctx))
}

func runtimeAssociatedRelativeV1(owner string, operation domainstartup.SemanticStartupOperationV1) (string, bool) {
	root := "data/private/" + owner
	if operation.Path == root {
		return ".", true
	}
	if strings.HasPrefix(operation.Path, root+"/") {
		return strings.TrimPrefix(operation.Path, root+"/"), true
	}
	return "", false
}

// Observe all raw entries before classifying domain failures. Journal Before
// must be backed by actual bytes, or the same previously verified transaction;
// signed hashes alone cannot reconstruct a deleted original.
func readRuntimeAssociatedSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingapp.ReportRestartScopeV1, saved *runtimeAssociatedSemanticPreservationV1) (_ map[string]runtimeOriginalSemanticFilesV1, _ map[string]runtimeOriginalSemanticFilesV1, _ []domainsecurity.TurnSecurityContext, _ *runtimeAssociatedObservationV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil || scope == nil {
		return nil, nil, nil, nil, errors.New("original associated authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, nil, nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	contexts, revalidateContexts, err := runtimeRegistryContextsV1(ctx, core)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation := &runtimeAssociatedObservationV1{owners: map[string]runtimeAssociatedPhysicalV1{}, journal: journal, revalidateContexts: revalidateContexts}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	originals, finals := map[string]runtimeOriginalSemanticFilesV1{}, map[string]runtimeOriginalSemanticFilesV1{}
	for _, owner := range runtimeAssociatedOwnersV1 {
		root := filepath.Join(core.roots.DataDir, "private", owner)
		var prepared runtimeAssociatedPhysicalV1
		if owner == "dataset-snapshot-authority" {
			prepared, err = datasetstore.PrepareOriginalObservationV1(ctx, root, core.access)
		} else {
			prepared, err = evidencestore.PrepareOriginalObservationV1(ctx, root, core.access)
		}
		if err != nil {
			return nil, nil, nil, nil, err
		}
		observation.owners[owner] = prepared
		raw, err := prepared.SnapshotOriginalFilesV1(ctx)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		physical := runtimeOriginalSemanticFilesV1(raw)
		original, final := physical.cloneV1(), physical.cloneV1()
		for index, operation := range journal.OperationsV1() {
			if err := validateRuntimeOriginalSemanticAncestorV1("data/private/"+owner, operation); err != nil {
				return nil, nil, nil, nil, err
			}
			name, owned := runtimeAssociatedRelativeV1(owner, operation)
			if !owned {
				continue
			}
			entry, found := physical[name]
			state := runtimeOriginalSemanticStateV1(entry, found)
			if state != operation.Before {
				if index > journal.NextOperationV1() || state != operation.After {
					return nil, nil, nil, nil, errors.New("original associated physical state is outside signed prefix")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil || !after {
					return nil, nil, nil, nil, errors.Join(errors.New("original associated physical state has not reached signed After"), err)
				}
			}
			switch operation.Before.Type {
			case domainstartup.ManagedEntryTypeAbsent:
				delete(original, name)
			case domainstartup.ManagedEntryTypeDirectory:
				original[name] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: operation.Before.Mode & 0o777}
			case domainstartup.ManagedEntryTypeFile:
				if !found || entry.Directory || int64(len(entry.Body)) != operation.Before.Size || domainsecurity.SHA256Hex(entry.Body) != operation.Before.SHA256 {
					if saved == nil || saved.owners[owner] == nil {
						return nil, nil, nil, nil, errors.New("original associated Before bytes are unavailable")
					}
					previous, exists := saved.owners[owner].recoveryBefore[name]
					if !exists || runtimeOriginalSemanticStateV1(previous, true) != operation.Before {
						return nil, nil, nil, nil, errors.New("original associated Before bytes are unavailable")
					}
					entry = previous
				}
				entry.Mode = operation.Before.Mode
				original[name] = entry
			default:
				return nil, nil, nil, nil, errors.New("original associated Before type is invalid")
			}
			if err := final.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		originals[owner], finals[owner] = original, final
	}
	return originals, finals, contexts, observation, nil
}

type runtimeOriginalDatasetMaterialsV1 runtimeOriginalSemanticFilesV1

func (files runtimeOriginalDatasetMaterialsV1) ResolveExact(ctx context.Context, _ datasetport.MaterialKindV2, reference datasetport.ExactMaterialReferenceV2) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !domainsecurity.IsSHA256Hex(reference.SHA256) || reference.Address != reference.SHA256 {
		return nil, datasetport.ErrMismatch
	}
	entry, found := files["materials/"+reference.SHA256[:2]+"/"+reference.SHA256+".json"]
	if !found || entry.Directory {
		return nil, datasetport.ErrNotFound
	}
	if uint64(len(entry.Body)) != reference.ByteLength || domainsecurity.SHA256Hex(entry.Body) != reference.SHA256 {
		return nil, datasetport.ErrCorrupt
	}
	return append([]byte(nil), entry.Body...), nil
}

// Errors returned in unavailable are domain-only. Native physical, key,
// independent enrollment, cancellation and primary failures stay hard errors.
func observeRuntimeAssociatedDomainV1(ctx context.Context, core *runtimeChildIdentityStartupV1, owner string, files runtimeOriginalSemanticFilesV1) (inventory datasetstore.OriginalInventoryV1, unavailable bool, resultErr error) {
	var hasRecords bool
	var err error
	if owner == "evidence-authority" {
		hasRecords, err = evidencestore.OriginalFilesHaveRecordsV1(ctx, files)
	} else {
		hasRecords, err = datasetstore.OriginalFilesHaveRecordsV1(ctx, files)
	}
	if err != nil {
		return inventory, false, err
	}
	if !hasRecords {
		return inventory, false, ctx.Err()
	}
	if err := errors.Join(core.revalidateKey(ctx), core.originalRegistryTrust.Revalidate(ctx), ctx.Err()); err != nil {
		return inventory, false, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, core.revalidateKey(ctx), core.originalRegistryTrust.Revalidate(ctx), ctx.Err())
		if resultErr != nil {
			inventory = datasetstore.OriginalInventoryV1{}
			unavailable = false
		}
	}()
	trust := core.originalRegistryTrust.projection
	verifier := runtimeOriginalRegistryVerifierV1{keyID: core.verification.KeyID(), publicKey: append([]byte(nil), core.verification.PublicKey()...)}
	var domainErr error
	if owner == "evidence-authority" {
		_, domainErr = evidencestore.ParseOriginalGraphV1(ctx, files, trust.InstallationID, trust.Enrollment.EnrollmentID, trust.Enrollment.WitnessKeyID, core.originalRegistryTrust.witnessKey, verifier)
	} else {
		inventory, domainErr = datasetstore.ParseOriginalInventoryV1(ctx, files, trust.InstallationID, trust.Enrollment.EnrollmentID, verifier.KeyID(), verifier.PublicKey())
		if domainErr == nil {
			for _, bundle := range inventory.Bundles {
				if domainErr = datasetapp.ValidateOriginalMaterialGraphV2(ctx, bundle, runtimeOriginalDatasetMaterialsV1(files)); domainErr != nil {
					break
				}
			}
		}
	}
	if errors.Is(domainErr, context.Canceled) || errors.Is(domainErr, context.DeadlineExceeded) {
		return inventory, false, domainErr
	}
	if domainErr != nil {
		return datasetstore.OriginalInventoryV1{}, true, nil
	}
	return inventory, false, nil
}

func runtimeDatasetRecordHeldV1(record domainsecurity.VersionedDatasetSnapshotAuthorityRecord, scope *pendingapp.ReportRestartScopeV1, contexts []domainsecurity.TurnSecurityContext) bool {
	var binding domainsecurity.DatasetSnapshotBindingKeyV1
	var snapshot string
	if record.V1 != nil {
		binding, _ = domainsecurity.DatasetSnapshotBindingKeyFromRecordV1(*record.V1)
		snapshot = record.V1.DatasetSnapshotID
	} else if record.V2 != nil {
		binding, snapshot = record.V2.Binding, record.V2.DatasetSnapshotID
	} else {
		return false
	}
	for _, frozen := range contexts {
		if scope.OwnsThread(frozen.ThreadID) && snapshot == frozen.DatasetSnapshotID &&
			binding.TenantID == frozen.TenantID && binding.UserID == frozen.UserID && binding.WorkspaceRealPath == frozen.WorkspaceRealPath &&
			binding.CaseID == frozen.CaseID && binding.CaseBindingHash == frozen.CaseBindingHash && binding.BindingObservationDigest == frozen.PublicationPolicy.BindingObservationDigest {
			return true
		}
	}
	return false
}

func (preserved *runtimeAssociatedOwnerPreservationV1) validateV1(ctx context.Context, core *runtimeChildIdentityStartupV1, owner string, files runtimeOriginalSemanticFilesV1, scope *pendingapp.ReportRestartScopeV1, contexts []domainsecurity.TurnSecurityContext) error {
	for name, original := range preserved.original {
		entry, found := files[name]
		if !found || !reflect.DeepEqual(entry, original) {
			return errors.New("original associated bytes, mode or presence changed")
		}
	}
	if reflect.DeepEqual(files, preserved.original) {
		return nil
	}
	if preserved.unavailable {
		return errors.New("original associated unavailable owner changed")
	}
	for name, entry := range files {
		if _, found := preserved.original[name]; found || entry.Directory {
			continue
		}
		base := path.Base(name)
		digest := strings.TrimSuffix(base, ".json")
		if !domainsecurity.IsSHA256Hex(digest) || base != digest+".json" {
			return errors.New("original associated candidate adds an unbound residue")
		}
	}
	inventory, unavailable, err := observeRuntimeAssociatedDomainV1(ctx, core, owner, files)
	if err != nil || unavailable {
		return errors.Join(errors.New("original associated candidate graph is unavailable"), err)
	}
	for digest, record := range inventory.Records {
		leaf := "legacy-records"
		if record.V2 != nil {
			leaf = "authority-bundles-v2"
		}
		if _, found := preserved.original[leaf+"/"+digest[:2]+"/"+digest+".json"]; !found && runtimeDatasetRecordHeldV1(record, scope, contexts) {
			return errors.New("original associated candidate adds a held dataset record")
		}
	}
	for digest, index := range inventory.Indexes {
		if _, found := preserved.original["indexes/"+digest[:2]+"/"+digest+".json"]; !found && runtimeDatasetRecordHeldV1(inventory.Records[index.SnapshotRecordDigest], scope, contexts) {
			return errors.New("original associated candidate adds an index for a held dataset record")
		}
	}
	return nil
}

func prepareRuntimeAssociatedSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingapp.ReportRestartScopeV1) (_ *runtimeAssociatedSemanticPreservationV1, resultErr error) {
	originals, finals, contexts, observation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, core, scope, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	preserved := &runtimeAssociatedSemanticPreservationV1{owners: map[string]*runtimeAssociatedOwnerPreservationV1{}}
	for _, owner := range runtimeAssociatedOwnersV1 {
		original := originals[owner]
		_, unavailable, err := observeRuntimeAssociatedDomainV1(ctx, core, owner, original)
		if err != nil {
			return nil, err
		}
		guard := &runtimeAssociatedOwnerPreservationV1{original: original.cloneV1(), recoveryBefore: original.cloneV1(), unavailable: unavailable}
		if err := guard.validateV1(ctx, core, owner, finals[owner], scope, contexts); err != nil {
			return nil, err
		}
		preserved.owners[owner] = guard
	}
	return preserved, nil
}

func (preserved runtimeReportRestartPreservationV1) validateAssociatedSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.associated == nil || preserved.core == nil || preserved.report == nil {
		return errors.New("original associated preservation is unavailable")
	}
	originals, candidates, contexts, observation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.associated)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
		if resultErr == nil {
			for _, owner := range runtimeAssociatedOwnersV1 {
				preserved.associated.owners[owner].recoveryBefore = originals[owner].cloneV1()
			}
		}
	}()
	for _, owner := range runtimeAssociatedOwnersV1 {
		guard := preserved.associated.owners[owner]
		if guard == nil {
			return errors.New("original associated owner preservation is unavailable")
		}
		if err := guard.validateV1(ctx, preserved.core, owner, originals[owner], preserved.report, contexts); err != nil {
			return err
		}
		// Check the complete journal Final before considering the next program.
		if err := guard.validateV1(ctx, preserved.core, owner, candidates[owner], preserved.report, contexts); err != nil {
			return err
		}
		for _, operation := range operations {
			if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
				continue
			}
			if err := validateRuntimeOriginalSemanticAncestorV1("data/private/"+owner, operation); err != nil {
				return err
			}
			name, owned := runtimeAssociatedRelativeV1(owner, operation)
			if !owned {
				continue
			}
			if path.Clean(name) != name {
				return errors.New("original associated address is invalid")
			}
			if err := candidates[owner].applyV1(operation, name, readAfter); err != nil {
				return err
			}
		}
		if err := guard.validateV1(ctx, preserved.core, owner, candidates[owner], preserved.report, contexts); err != nil {
			return err
		}
	}
	return nil
}
