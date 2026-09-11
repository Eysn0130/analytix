package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const runtimePendingSemanticRootV1 = "data/private/pending-work"

// The prepared owner is read afresh after authenticated CAS residue recovery.
// Its complete committed graph is verified before projecting all remaining
// signed operations in memory. The signed program is never edited or applied.
func (preserved runtimeReportRestartPreservationV1) validatePendingSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	core := preserved.core
	if ctx == nil || core == nil || core.verification == nil || core.access == nil || core.revalidateKey == nil || preserved.report == nil {
		return errors.New("semantic pending preservation authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return err
	}
	if journal != nil {
		defer func() { resultErr = errors.Join(resultErr, journal.Revalidate(ctx)) }()
	}
	prepared, err := pendingworkstore.PrepareRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "pending-work"), core.access)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Revalidate(ctx), core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	var receipts []domainpendingwork.PendingWorkReceiptV1
	var dispositions []domainpendingwork.PendingWorkDispositionV1
	if journal == nil {
		receipts, dispositions, err = prepared.SnapshotInventory(ctx)
	} else {
		receipts, dispositions, err = prepared.SnapshotCanonicalRecordsV1(ctx)
	}
	if err != nil {
		return err
	}
	var original pendingworkapp.TrustedInventoryV1
	if journal == nil {
		original, err = pendingworkapp.VerifyTrustedSnapshotV1(ctx, receipts, dispositions, core.verification)
	} else {
		var observed *runtimePendingSemanticObservationV1
		observed, err = prepareRuntimePendingSemanticObservationV1(ctx, receipts, dispositions, core.verification, journal)
		if err == nil {
			original = observed.original
		}
	}
	if err != nil {
		return err
	}
	if err := preserved.report.ValidateTrustedPendingInventoryV1(ctx, original); err != nil {
		return err
	}
	receiptByID := map[string]domainpendingwork.PendingWorkReceiptV1{}
	dispositionByID := map[string]domainpendingwork.PendingWorkDispositionV1{}
	// Replay starts from the complete physical cut, not the separately
	// reconstructed Before graph used to bind original report preservation.
	for _, receipt := range receipts {
		receiptByID[receipt.WorkID] = receipt
	}
	for _, disposition := range dispositions {
		dispositionByID[disposition.WorkID] = disposition
	}
	heldPaths := map[string]bool{}
	for _, receipt := range original.Receipts {
		if preserved.report.OwnsThread(receipt.Context.ThreadID) {
			for _, leaf := range []string{"receipts", "dispositions"} {
				heldPaths[runtimePendingSemanticRootV1+"/"+leaf+"/"+receipt.WorkID[:2]+"/"+receipt.WorkID+".json"] = true
			}
		}
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if operation.OperationID == noWriteOperationID {
			continue // The signed executor is bound to an After-only readback.
		}
		for heldPath := range heldPaths {
			if operation.Path == heldPath || strings.HasPrefix(heldPath, operation.Path+"/") &&
				(operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
				return pendingworkapp.ErrRestartPreserved
			}
		}
		if !strings.HasPrefix(operation.Path, runtimePendingSemanticRootV1+"/") {
			continue
		}
		relative := strings.TrimPrefix(operation.Path, runtimePendingSemanticRootV1+"/")
		parts := strings.Split(relative, "/")
		if len(parts) <= 2 && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
			if (parts[0] != "receipts" && parts[0] != "dispositions") || (len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1])) {
				return errors.New("semantic pending directory grammar is invalid")
			}
			continue // Exact directory transitions remain owned by the full plan.
		}
		if len(parts) != 3 || (parts[0] != "receipts" && parts[0] != "dispositions") {
			return errors.New("semantic pending target grammar is invalid")
		}
		if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); ok {
			if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile ||
				operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
				return errors.New("semantic pending residue is outside the authenticated removal cut")
			}
			id := residue.OriginalName[1:65] // The canonical producer grammar proved this range.
			if heldPaths[runtimePendingSemanticRootV1+"/"+parts[0]+"/"+parts[1]+"/"+id+".json"] {
				return pendingworkapp.ErrRestartPreserved
			}
			if _, found := receiptByID[id]; !found {
				return errors.New("semantic pending residue lacks an independent trusted receipt")
			}
			continue
		}
		id := strings.TrimSuffix(parts[2], ".json")
		if !strings.HasSuffix(parts[2], ".json") || !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
			return errors.New("semantic pending record address is invalid")
		}
		switch operation.Kind {
		case domainstartup.SemanticOperationRemoveFile:
			if parts[0] == "receipts" {
				delete(receiptByID, id)
			} else {
				delete(dispositionByID, id)
			}
		case domainstartup.SemanticOperationSetMode:
		case domainstartup.SemanticOperationInstallFile:
			if readAfter == nil {
				return errors.New("semantic pending after bytes are unavailable")
			}
			body, err := readAfter(operation)
			if err != nil {
				return err
			}
			if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
				return errors.New("semantic pending after bytes lost integrity")
			}
			if parts[0] == "receipts" {
				receipt, parseErr := domainpendingwork.ParsePendingWorkReceiptV1(body)
				canonical, canonicalErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
				if parseErr != nil || canonicalErr != nil || receipt.WorkID != id || !bytes.Equal(canonical, body) {
					return errors.New("semantic pending receipt is noncanonical or misaddressed")
				}
				receiptByID[id] = receipt
			} else {
				disposition, parseErr := domainpendingwork.ParsePendingWorkDispositionV1(body)
				canonical, canonicalErr := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
				if parseErr != nil || canonicalErr != nil || disposition.WorkID != id || !bytes.Equal(canonical, body) {
					return errors.New("semantic pending disposition is noncanonical or misaddressed")
				}
				dispositionByID[id] = disposition
			}
		default:
			return errors.New("semantic pending record transition is invalid")
		}
	}
	nextReceipts := make([]domainpendingwork.PendingWorkReceiptV1, 0, len(receiptByID))
	nextDispositions := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(dispositionByID))
	for _, receipt := range receiptByID {
		nextReceipts = append(nextReceipts, receipt)
	}
	for _, disposition := range dispositionByID {
		nextDispositions = append(nextDispositions, disposition)
	}
	candidate, err := pendingworkapp.VerifyTrustedSnapshotV1(ctx, nextReceipts, nextDispositions, core.verification)
	if err != nil {
		return err
	}
	return preserved.report.ValidateTrustedPendingInventoryV1(ctx, candidate)
}
