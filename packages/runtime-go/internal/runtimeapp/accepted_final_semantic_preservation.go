package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const runtimeAcceptedFinalSemanticRootV1 = "data/private/accepted-finals"

type runtimeAcceptedFinalSemanticPreservationV1 struct {
	heldDigests map[string]string
}

// The key is the accepted-final digest. A context may own multiple candidates.
type runtimeAcceptedFinalSemanticInventoryV1 struct {
	records      map[string]domainevidence.PrivateAcceptedFinalRecord
	dispositions map[string]domainevidence.AcceptedFinalDispositionRecord
}

type runtimeAcceptedFinalSemanticObservationV1 struct {
	prepared *finalauthority.PreparedPrivateStoreRecoveryV1
	journal  *persistencefs.AuthenticatedSemanticJournalObservationV1
}

func (observation *runtimeAcceptedFinalSemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil {
		return errors.New("accepted-final semantic observation is unavailable")
	}
	err := observation.prepared.Revalidate(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return err
}

func acceptedFinalSemanticPathV1(leaf, id string) string {
	return runtimeAcceptedFinalSemanticRootV1 + "/" + leaf + "/" + id[:2] + "/" + id + ".json"
}

func acceptedFinalSemanticAddressV1(operation domainstartup.SemanticStartupOperationV1) (leaf, id string, record, residue bool, err error) {
	if operation.Path == runtimeAcceptedFinalSemanticRootV1 {
		if operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory {
			return "", "", false, false, nil
		}
		return "", "", false, false, errors.New("semantic accepted-final owner transition is invalid")
	}
	if !strings.HasPrefix(operation.Path, runtimeAcceptedFinalSemanticRootV1+"/") {
		return "", "", false, false, nil
	}
	parts := strings.Split(strings.TrimPrefix(operation.Path, runtimeAcceptedFinalSemanticRootV1+"/"), "/")
	if len(parts) <= 2 && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
		if (parts[0] != "records" && parts[0] != "dispositions") || len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1]) {
			return "", "", false, false, errors.New("semantic accepted-final directory grammar is invalid")
		}
		return "", "", false, false, nil
	}
	if len(parts) != 3 || (parts[0] != "records" && parts[0] != "dispositions") {
		return "", "", false, false, errors.New("semantic accepted-final target grammar is invalid")
	}
	if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); ok {
		if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
			return "", "", false, false, errors.New("semantic accepted-final residue is outside the signed removal cut")
		}
		return parts[0], residue.OriginalName[1:65], false, true, nil
	}
	id = strings.TrimSuffix(parts[2], ".json")
	if !strings.HasSuffix(parts[2], ".json") || !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
		return "", "", false, false, errors.New("semantic accepted-final address is invalid")
	}
	return parts[0], id, true, false, nil
}

func (inventory runtimeAcceptedFinalSemanticInventoryV1) cloneV1() runtimeAcceptedFinalSemanticInventoryV1 {
	copy := runtimeAcceptedFinalSemanticInventoryV1{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{}}
	for id, record := range inventory.records {
		copy.records[id] = record
	}
	for id, disposition := range inventory.dispositions {
		copy.dispositions[id] = disposition
	}
	return copy
}

func (inventory runtimeAcceptedFinalSemanticInventoryV1) bodyV1(leaf, id string) ([]byte, bool, error) {
	if leaf == "records" {
		record, exists := inventory.records[id]
		if !exists {
			return nil, false, nil
		}
		body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
		return body, true, err
	}
	disposition, exists := inventory.dispositions[id]
	if !exists {
		return nil, false, nil
	}
	body, err := domainevidence.AcceptedFinalDispositionRecordBytes(disposition)
	return body, true, err
}

func (inventory runtimeAcceptedFinalSemanticInventoryV1) verifyV1(ctx context.Context, authority authorityport.Authority, complete bool) error {
	records := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(inventory.records))
	dispositions := make([]domainevidence.AcceptedFinalDispositionRecord, 0, len(inventory.dispositions))
	for _, record := range inventory.records {
		key, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record.AcceptedFinal)
		if err != nil {
			return err
		}
		if err := authority.VerifyTrusted(ctx, key, publicKey, domainevidence.AcceptedFinalSigningBytes(record.AcceptedFinal), signature); err != nil {
			return err
		}
		records = append(records, record)
	}
	for _, disposition := range inventory.dispositions {
		key, publicKey, signature, err := domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
		if err != nil {
			return err
		}
		if err := authority.VerifyTrusted(ctx, key, publicKey, domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature); err != nil {
			return err
		}
		dispositions = append(dispositions, disposition)
	}
	if complete {
		return finalauthority.ValidatePrivateStoreInventoryV1(records, dispositions)
	}
	return ctx.Err()
}

func (inventory runtimeAcceptedFinalSemanticInventoryV1) heldDigestsV1(scope *pendingworkapp.ReportRestartScopeV1) (map[string]string, error) {
	contexts := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range scope.Contexts() {
		contexts[frozen.ContextDigest] = frozen
	}
	for id, disposition := range inventory.dispositions {
		if scope.OwnsThread(disposition.ThreadID) {
			if _, found := inventory.records[id]; !found {
				return nil, errors.New("original held accepted-final disposition lacks its record")
			}
		}
	}
	digests := map[string]string{}
	for id, record := range inventory.records {
		if !scope.OwnsThread(record.SecurityContext.ThreadID) {
			continue
		}
		frozen, found := contexts[record.SecurityContext.ContextDigest]
		if !found || !reflect.DeepEqual(frozen, record.SecurityContext) {
			return nil, errors.New("held accepted-final context is outside original primary authority")
		}
		for _, leaf := range []string{"records", "dispositions"} {
			body, found, err := inventory.bodyV1(leaf, id)
			if err != nil {
				return nil, err
			}
			if found {
				digests[acceptedFinalSemanticPathV1(leaf, id)] = domainsecurity.SHA256Hex(body)
			}
		}
	}
	return digests, nil
}

func (inventory runtimeAcceptedFinalSemanticInventoryV1) applyV1(operation domainstartup.SemanticStartupOperationV1, leaf, id string, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
	switch operation.Kind {
	case domainstartup.SemanticOperationRemoveFile:
		if leaf == "records" {
			delete(inventory.records, id)
		} else {
			delete(inventory.dispositions, id)
		}
	case domainstartup.SemanticOperationSetMode:
	case domainstartup.SemanticOperationInstallFile:
		if readAfter == nil {
			return errors.New("accepted-final semantic After bytes are unavailable")
		}
		body, err := readAfter(operation)
		if err != nil {
			return err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("accepted-final semantic After bytes lost integrity")
		}
		if leaf == "records" {
			record, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
			canonical, canonicalErr := domainevidence.PrivateAcceptedFinalRecordBytes(record)
			if err != nil || canonicalErr != nil || record.AcceptedFinal.RecordDigest != id || !bytes.Equal(body, canonical) {
				return errors.New("accepted-final semantic record is noncanonical or misaddressed")
			}
			inventory.records[id] = record
		} else {
			disposition, err := domainevidence.ParseAcceptedFinalDispositionRecord(body)
			canonical, canonicalErr := domainevidence.AcceptedFinalDispositionRecordBytes(disposition)
			if err != nil || canonicalErr != nil || disposition.AcceptedFinalDigest != id || !bytes.Equal(body, canonical) {
				return errors.New("accepted-final semantic disposition is noncanonical or misaddressed")
			}
			inventory.dispositions[id] = disposition
		}
	default:
		return errors.New("accepted-final semantic record transition is invalid")
	}
	return nil
}

// Physical records remain physical. Only an exact applied signed addition
// may be withdrawn to prove the transaction-Before inventory. Missing old
// bytes are never reconstructed from a future record or winner candidate.
func readRuntimeAcceptedFinalSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (_ runtimeAcceptedFinalSemanticInventoryV1, _ runtimeAcceptedFinalSemanticInventoryV1, _ *runtimeAcceptedFinalSemanticObservationV1, resultErr error) {
	empty := runtimeAcceptedFinalSemanticInventoryV1{}
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil || scope == nil {
		return empty, empty, nil, errors.New("accepted-final semantic authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return empty, empty, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return empty, empty, nil, err
	}
	prepared, err := finalauthority.PreparePrivateStoreRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "accepted-finals"), core.access)
	if err != nil {
		return empty, empty, nil, err
	}
	observation := &runtimeAcceptedFinalSemanticObservationV1{prepared: prepared, journal: journal}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	records, dispositions, err := prepared.SnapshotCanonicalRecordsV1(ctx)
	if err != nil {
		return empty, empty, nil, err
	}
	physical := runtimeAcceptedFinalSemanticInventoryV1{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{}}
	for _, record := range records {
		physical.records[record.AcceptedFinal.RecordDigest] = record
	}
	for _, disposition := range dispositions {
		physical.dispositions[disposition.AcceptedFinalDigest] = disposition
	}
	if err := physical.verifyV1(ctx, core.verification, false); err != nil {
		return empty, empty, nil, err
	}
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		leaf, id, record, _, err := acceptedFinalSemanticAddressV1(operation)
		if err != nil {
			return empty, empty, nil, err
		}
		if !record {
			continue
		}
		body, present, err := physical.bodyV1(leaf, id)
		if err != nil {
			return empty, empty, nil, err
		}
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			if present {
				if index > journal.NextOperationV1() || operation.Kind != domainstartup.SemanticOperationInstallFile || int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
					return empty, empty, nil, errors.New("accepted-final physical addition is not an applied signed operation")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil || !after {
					return empty, empty, nil, errors.Join(errors.New("accepted-final physical addition has not reached signed After"), err)
				}
			}
			if leaf == "records" {
				delete(original.records, id)
			} else {
				delete(original.dispositions, id)
			}
		case domainstartup.ManagedEntryTypeFile:
			if !present || int64(len(body)) != operation.Before.Size || domainsecurity.SHA256Hex(body) != operation.Before.SHA256 {
				return empty, empty, nil, errors.New("accepted-final semantic original Before bytes are unavailable")
			}
		default:
			return empty, empty, nil, errors.New("accepted-final semantic original record type is invalid")
		}
		if err := final.applyV1(operation, leaf, id, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return empty, empty, nil, err
		}
	}
	for _, inventory := range []runtimeAcceptedFinalSemanticInventoryV1{original, final} {
		if err := inventory.verifyV1(ctx, core.verification, true); err != nil {
			return empty, empty, nil, err
		}
	}
	originalHeld, err := original.heldDigestsV1(scope)
	if err != nil {
		return empty, empty, nil, err
	}
	finalHeld, err := final.heldDigestsV1(scope)
	if err != nil || !reflect.DeepEqual(originalHeld, finalHeld) {
		return empty, empty, nil, errors.Join(errors.New("accepted-final semantic candidate changed original hold"), err)
	}
	return original, physical, observation, nil
}

func prepareRuntimeAcceptedFinalSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (*runtimeAcceptedFinalSemanticPreservationV1, error) {
	original, _, _, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, core, scope)
	if err != nil {
		return nil, err
	}
	digests, err := original.heldDigestsV1(scope)
	if err != nil {
		return nil, err
	}
	return &runtimeAcceptedFinalSemanticPreservationV1{heldDigests: digests}, nil
}

func (preserved runtimeReportRestartPreservationV1) validateAcceptedFinalSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.acceptedFinal == nil || preserved.core == nil || preserved.report == nil {
		return errors.New("original accepted-final preservation is unavailable")
	}
	original, candidate, observation, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, preserved.core, preserved.report)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	heldDigests, err := original.heldDigestsV1(preserved.report)
	if err != nil || !reflect.DeepEqual(heldDigests, preserved.acceptedFinal.heldDigests) {
		return errors.Join(errors.New("accepted-final original held inventory changed"), err)
	}
	heldPaths := map[string]bool{}
	for id, record := range original.records {
		if preserved.report.OwnsThread(record.SecurityContext.ThreadID) {
			for _, leaf := range []string{"records", "dispositions"} {
				heldPaths[acceptedFinalSemanticPathV1(leaf, id)] = true
			}
		}
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if operation.OperationID == noWriteOperationID {
			continue
		}
		for path := range heldPaths {
			if operation.Path == path || strings.HasPrefix(path, operation.Path+"/") && (operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
				return errors.New("semantic operation mutates original held accepted-final authority")
			}
		}
		leaf, id, record, residue, err := acceptedFinalSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if residue {
			originalRecord, found := original.records[id]
			if !found || preserved.report.OwnsThread(originalRecord.SecurityContext.ThreadID) {
				return errors.New("accepted-final residue lacks independent original authority")
			}
		}
		if !record {
			continue
		}
		if err := candidate.applyV1(operation, leaf, id, readAfter); err != nil {
			return err
		}
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			if leaf == "records" && preserved.report.OwnsThread(candidate.records[id].SecurityContext.ThreadID) || leaf == "dispositions" && preserved.report.OwnsThread(candidate.dispositions[id].ThreadID) {
				return errors.New("semantic operation installs new held accepted-final authority")
			}
		}
	}
	if err := candidate.verifyV1(ctx, preserved.core.verification, true); err != nil {
		return err
	}
	finalHeld, err := candidate.heldDigestsV1(preserved.report)
	if err != nil || !reflect.DeepEqual(finalHeld, preserved.acceptedFinal.heldDigests) {
		return errors.Join(errors.New("accepted-final semantic candidate changed original hold"), err)
	}
	return nil
}
