package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// Physical records and their exact signed provenance remain distinct from
// both the provable transaction-Before graph and the complete final candidate.
// Allocation is a separate canonical union: neither removal nor an unwritten
// signed receipt may release a ChildProducer's reserved identities.
type runtimePendingSemanticObservationV1 struct {
	journal              *persistencefs.AuthenticatedSemanticJournalObservationV1
	physicalReceipts     []domainpendingwork.PendingWorkReceiptV1
	physicalDispositions []domainpendingwork.PendingWorkDispositionV1
	original             pendingworkapp.TrustedInventoryV1
	final                pendingworkapp.TrustedInventoryV1
	allocation           pendingworkapp.TrustedInventoryV1
}

// This check runs in Core before the optional lane gates, including when the
// original inventory has no held thread. Future bytes never satisfy original
// report grant/result witnesses, nor may final close or introduce a hold.
func validateRuntimePendingSemanticScopeV1(ctx context.Context, observed *runtimePendingSemanticObservationV1, authority authorityport.Authority, primaries recoveryport.PrimaryThreadReaderV1, inherited ...pendingworkapp.ReportRestartInheritedHistoryV1) error {
	dispositions := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(observed.original.Dispositions))
	for _, disposition := range observed.original.Dispositions {
		dispositions = append(dispositions, disposition)
	}
	scope, err := pendingworkapp.PlanReportRestartPreservationSnapshotV1(ctx, observed.original.Receipts, dispositions, authority, primaries, inherited...)
	if err != nil {
		return err
	}
	if err := scope.ValidateTrustedPendingInventoryV1(ctx, observed.final); err != nil {
		return err
	}
	return scope.RevalidatePrimary(ctx)
}

func pendingSemanticPathV1(leaf, id string) string {
	return runtimePendingSemanticRootV1 + "/" + leaf + "/" + id[:2] + "/" + id + ".json"
}

func pendingSemanticJournalNeedsObservationV1(journal *persistencefs.AuthenticatedSemanticJournalObservationV1) (bool, error) {
	needed := false
	for _, operation := range journal.OperationsV1() {
		_, _, record, err := pendingSemanticRecordAddressV1(operation)
		if err != nil {
			return false, err
		}
		needed = needed || record
	}
	return needed, nil
}

func pendingSemanticRecordAddressV1(operation domainstartup.SemanticStartupOperationV1) (leaf, id string, record bool, err error) {
	if operation.Path == runtimePendingSemanticRootV1 {
		if operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory {
			return "", "", false, nil
		}
		return "", "", false, errors.New("semantic pending owner transition is invalid")
	}
	if !strings.HasPrefix(operation.Path, runtimePendingSemanticRootV1+"/") {
		return "", "", false, nil
	}
	parts := strings.Split(strings.TrimPrefix(operation.Path, runtimePendingSemanticRootV1+"/"), "/")
	if len(parts) <= 2 && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
		if (parts[0] != "receipts" && parts[0] != "dispositions") || (len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1])) {
			return "", "", false, errors.New("semantic pending observation directory grammar is invalid")
		}
		return "", "", false, nil
	}
	if len(parts) != 3 || (parts[0] != "receipts" && parts[0] != "dispositions") {
		return "", "", false, errors.New("semantic pending observation target grammar is invalid")
	}
	if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); ok {
		if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
			return "", "", false, errors.New("semantic pending observed residue is outside the signed removal cut")
		}
		return "", "", false, nil
	}
	id = strings.TrimSuffix(parts[2], ".json")
	if !strings.HasSuffix(parts[2], ".json") || !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
		return "", "", false, errors.New("semantic pending observation record address is invalid")
	}
	return parts[0], id, true, nil
}

func verifyPendingSemanticRecordV1(ctx context.Context, leaf, id string, body []byte, authority authorityport.Authority) error {
	if ctx == nil || authority == nil {
		return errors.New("semantic pending record authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if leaf == "receipts" {
		receipt, err := domainpendingwork.ParsePendingWorkReceiptV1(body)
		canonical, canonicalErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
		if err != nil || canonicalErr != nil || receipt.WorkID != id || !bytes.Equal(body, canonical) {
			return errors.New("semantic pending observed receipt is noncanonical or misaddressed")
		}
		keyID, publicKey, signature, err := domainpendingwork.PendingWorkReceiptV1AuthorityMaterial(receipt)
		if err != nil {
			return err
		}
		return authority.VerifyTrusted(ctx, keyID, publicKey, domainpendingwork.PendingWorkReceiptV1SigningBytes(receipt), signature)
	}
	disposition, err := domainpendingwork.ParsePendingWorkDispositionV1(body)
	canonical, canonicalErr := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil || canonicalErr != nil || disposition.WorkID != id || !bytes.Equal(body, canonical) {
		return errors.New("semantic pending observed disposition is noncanonical or misaddressed")
	}
	keyID, publicKey, signature, err := domainpendingwork.PendingWorkDispositionV1AuthorityMaterial(disposition)
	if err != nil {
		return err
	}
	return authority.VerifyTrusted(ctx, keyID, publicKey, domainpendingwork.PendingWorkDispositionV1SigningBytes(disposition), signature)
}

func pendingSemanticTrustedBodiesV1(ctx context.Context, bodies map[string][]byte, authority authorityport.Authority) (pendingworkapp.TrustedInventoryV1, error) {
	receipts := []domainpendingwork.PendingWorkReceiptV1{}
	dispositions := []domainpendingwork.PendingWorkDispositionV1{}
	for path, body := range bodies {
		if strings.HasPrefix(path, runtimePendingSemanticRootV1+"/receipts/") {
			receipt, err := domainpendingwork.ParsePendingWorkReceiptV1(body)
			if err != nil {
				return pendingworkapp.TrustedInventoryV1{}, err
			}
			receipts = append(receipts, receipt)
		} else {
			disposition, err := domainpendingwork.ParsePendingWorkDispositionV1(body)
			if err != nil {
				return pendingworkapp.TrustedInventoryV1{}, err
			}
			dispositions = append(dispositions, disposition)
		}
	}
	return pendingworkapp.VerifyTrustedSnapshotV1(ctx, receipts, dispositions, authority)
}

func prepareRuntimePendingSemanticObservationV1(ctx context.Context, receipts []domainpendingwork.PendingWorkReceiptV1, dispositions []domainpendingwork.PendingWorkDispositionV1, authority authorityport.Authority, journal *persistencefs.AuthenticatedSemanticJournalObservationV1) (_ *runtimePendingSemanticObservationV1, resultErr error) {
	if journal == nil {
		return nil, errors.New("pending semantic journal observation is unavailable")
	}
	if err := journal.Revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Revalidate(ctx)) }()
	physical := map[string][]byte{}
	allocation := map[string][]byte{}
	addPhysical := func(leaf, id string, body []byte) error {
		if err := verifyPendingSemanticRecordV1(ctx, leaf, id, body, authority); err != nil {
			return err
		}
		path := pendingSemanticPathV1(leaf, id)
		if _, duplicate := physical[path]; duplicate {
			return errors.New("semantic pending physical record address is duplicated")
		}
		physical[path] = body
		if leaf == "receipts" {
			allocation[path] = body
		}
		return nil
	}
	for _, receipt := range receipts {
		body, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
		if err != nil {
			return nil, err
		}
		if err := addPhysical("receipts", receipt.WorkID, body); err != nil {
			return nil, err
		}
	}
	for _, disposition := range dispositions {
		body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
		if err != nil {
			return nil, err
		}
		if err := addPhysical("dispositions", disposition.WorkID, body); err != nil {
			return nil, err
		}
	}
	originalBodies, finalBodies := map[string][]byte{}, map[string][]byte{}
	for path, body := range physical {
		originalBodies[path], finalBodies[path] = body, body
	}
	for index, operation := range journal.OperationsV1() {
		leaf, id, record, err := pendingSemanticRecordAddressV1(operation)
		if err != nil {
			return nil, err
		}
		if !record {
			continue
		}
		current, present := physical[operation.Path]
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			if present {
				if index > journal.NextOperationV1() || operation.Kind != domainstartup.SemanticOperationInstallFile || int64(len(current)) != operation.After.Size || domainsecurity.SHA256Hex(current) != operation.After.SHA256 {
					return nil, errors.New("pending physical addition is not an applied signed operation")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil {
					return nil, err
				}
				if !after {
					return nil, errors.New("pending physical addition has not reached signed After")
				}
			}
			delete(originalBodies, operation.Path)
		case domainstartup.ManagedEntryTypeFile:
			if !present || int64(len(current)) != operation.Before.Size || domainsecurity.SHA256Hex(current) != operation.Before.SHA256 {
				return nil, errors.New("pending semantic original Before bytes are unavailable")
			}
		default:
			return nil, errors.New("pending semantic original record type is invalid")
		}
		switch operation.Kind {
		case domainstartup.SemanticOperationRemoveFile:
			delete(finalBodies, operation.Path)
		case domainstartup.SemanticOperationSetMode:
		case domainstartup.SemanticOperationInstallFile:
			body, err := journal.ReadAfterV1(ctx, operation)
			if err != nil {
				return nil, err
			}
			if err := verifyPendingSemanticRecordV1(ctx, leaf, id, body, authority); err != nil {
				return nil, err
			}
			finalBodies[operation.Path] = body
			if leaf == "receipts" {
				if old, found := allocation[operation.Path]; found && !bytes.Equal(old, body) {
					return nil, errors.New("pending semantic allocation receipt has conflicting signed versions")
				}
				allocation[operation.Path] = body
			}
		default:
			return nil, errors.New("pending semantic final record transition is invalid")
		}
	}
	original, err := pendingSemanticTrustedBodiesV1(ctx, originalBodies, authority)
	if err != nil {
		return nil, err
	}
	final, err := pendingSemanticTrustedBodiesV1(ctx, finalBodies, authority)
	if err != nil {
		return nil, err
	}
	for _, receipt := range final.Receipts {
		if _, existed := originalBodies[pendingSemanticPathV1("receipts", receipt.WorkID)]; existed {
			continue
		}
		if disposition, closed := final.Dispositions[receipt.WorkID]; !closed || disposition.Status == domainpendingwork.StatusOutcomeUnknown {
			return nil, errors.New("new pending semantic pair is not independently closed")
		}
	}
	allocated, err := pendingSemanticTrustedBodiesV1(ctx, allocation, authority)
	if err != nil {
		return nil, err
	}
	return &runtimePendingSemanticObservationV1{journal: journal, physicalReceipts: append([]domainpendingwork.PendingWorkReceiptV1(nil), receipts...), physicalDispositions: append([]domainpendingwork.PendingWorkDispositionV1(nil), dispositions...), original: original, final: final, allocation: allocated}, nil
}
