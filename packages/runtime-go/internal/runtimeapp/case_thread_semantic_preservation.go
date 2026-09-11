package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const runtimeCaseThreadSemanticRootV1 = "data/private/case-thread-authority"

type runtimeCaseThreadSemanticInventoryV1 map[string]domainsecurity.CaseThreadAuthorityRecord

type runtimeCaseThreadSemanticPreservationV1 struct{ heldDigests map[string]string }

type runtimeCaseThreadSemanticObservationV1 struct {
	original, physical, final       runtimeCaseThreadSemanticInventoryV1
	originalContexts, finalContexts *casethreadapp.VerifiedCommittedContextInventoryV1
	prepared                        *casestore.PreparedRecoveryV1
	journal                         *persistencefs.AuthenticatedSemanticJournalObservationV1
}

func (observation *runtimeCaseThreadSemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil {
		return errors.New("case thread semantic observation is unavailable")
	}
	err := observation.prepared.Revalidate(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return err
}

func caseThreadSemanticPathV1(id string) string {
	return runtimeCaseThreadSemanticRootV1 + "/" + id[:2] + "/" + id + ".json"
}

func caseThreadSemanticAddressV1(operation domainstartup.SemanticStartupOperationV1) (id string, record, residue bool, err error) {
	if operation.Path == runtimeCaseThreadSemanticRootV1 {
		if operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory {
			return "", false, false, nil
		}
		return "", false, false, errors.New("case thread semantic root transition is invalid")
	}
	if !strings.HasPrefix(operation.Path, runtimeCaseThreadSemanticRootV1+"/") {
		return "", false, false, nil
	}
	parts := strings.Split(strings.TrimPrefix(operation.Path, runtimeCaseThreadSemanticRootV1+"/"), "/")
	if len(parts) == 1 && domainprivatecas.ValidShardV1(parts[0]) && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
		return "", false, false, nil
	}
	if len(parts) != 2 || !domainprivatecas.ValidShardV1(parts[0]) {
		return "", false, false, errors.New("case thread semantic target grammar is invalid")
	}
	if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[1], parts[0]); ok {
		if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
			return "", false, false, errors.New("case thread semantic residue is outside the signed removal cut")
		}
		return residue.OriginalName[1:65], false, true, nil
	}
	id = strings.TrimSuffix(parts[1], ".json")
	if !strings.HasSuffix(parts[1], ".json") || !domainsecurity.IsSHA256Hex(id) || id[:2] != parts[0] {
		return "", false, false, errors.New("case thread semantic address is invalid")
	}
	return id, true, false, nil
}

func (inventory runtimeCaseThreadSemanticInventoryV1) cloneV1() runtimeCaseThreadSemanticInventoryV1 {
	copy := runtimeCaseThreadSemanticInventoryV1{}
	for id, record := range inventory {
		copy[id] = record
	}
	return copy
}

func (inventory runtimeCaseThreadSemanticInventoryV1) recordsV1() []domainsecurity.CaseThreadAuthorityRecord {
	records := make([]domainsecurity.CaseThreadAuthorityRecord, 0, len(inventory))
	for _, record := range inventory {
		records = append(records, record)
	}
	return records
}

func caseThreadRecordHeldV1(scope *pendingworkapp.ReportRestartScopeV1, record domainsecurity.CaseThreadAuthorityRecord) bool {
	return scope.OwnsThread(domainsecurity.CaseThreadAuthorityThreadID(record)) || scope.OwnsThread(record.ParentThreadID)
}

func (inventory runtimeCaseThreadSemanticInventoryV1) heldDigestsV1(scope *pendingworkapp.ReportRestartScopeV1) (map[string]string, error) {
	held := map[string]string{}
	for id, record := range inventory {
		if caseThreadRecordHeldV1(scope, record) {
			body, err := domainsecurity.CaseThreadAuthorityRecordBytes(record)
			if err != nil {
				return nil, err
			}
			held[caseThreadSemanticPathV1(id)] = domainsecurity.SHA256Hex(body)
		}
	}
	return held, nil
}

func (inventory runtimeCaseThreadSemanticInventoryV1) applyV1(ctx context.Context, operation domainstartup.SemanticStartupOperationV1, id string, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), authority authorityport.Authority) error {
	switch operation.Kind {
	case domainstartup.SemanticOperationRemoveFile:
		delete(inventory, id)
	case domainstartup.SemanticOperationSetMode:
	case domainstartup.SemanticOperationInstallFile:
		if readAfter == nil {
			return errors.New("case thread semantic After bytes are unavailable")
		}
		body, err := readAfter(operation)
		if err != nil {
			return err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("case thread semantic After bytes lost integrity")
		}
		record, err := domainsecurity.ParseCaseThreadAuthorityRecord(body)
		if err != nil {
			return err
		}
		canonical, err := domainsecurity.CaseThreadAuthorityRecordBytes(record)
		if err != nil || record.RecordDigest != id || !bytes.Equal(body, canonical) {
			return errors.Join(errors.New("case thread semantic After is not canonical or address-bound"), err)
		}
		if err := casethreadapp.VerifyContextRecordSignaturesV1(ctx, []domainsecurity.CaseThreadAuthorityRecord{record}, authority); err != nil {
			return err
		}
		inventory[id] = record
	default:
		return errors.New("case thread semantic record transition is invalid")
	}
	return ctx.Err()
}

func observeRuntimeCaseThreadSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1) (_ *runtimeCaseThreadSemanticObservationV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil {
		return nil, errors.New("case thread original semantic authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, err
	}
	prepared, err := casestore.PrepareRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "case-thread-authority"), core.access)
	if err != nil {
		return nil, err
	}
	observation := &runtimeCaseThreadSemanticObservationV1{prepared: prepared, journal: journal, physical: runtimeCaseThreadSemanticInventoryV1{}}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx))
	}()
	records, err := prepared.SnapshotCanonicalRecordsV1(ctx)
	if err != nil {
		return nil, err
	}
	if err := casethreadapp.VerifyContextRecordSignaturesV1(ctx, records, core.verification); err != nil {
		return nil, err
	}
	for _, record := range records {
		observation.physical[record.RecordDigest] = record
	}
	observation.original, observation.final = observation.physical.cloneV1(), observation.physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		id, isRecord, _, err := caseThreadSemanticAddressV1(operation)
		if err != nil {
			return nil, err
		}
		if !isRecord {
			continue
		}
		record, present := observation.physical[id]
		var body []byte
		if present {
			body, err = domainsecurity.CaseThreadAuthorityRecordBytes(record)
			if err != nil {
				return nil, err
			}
		}
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			if present {
				if index > journal.NextOperationV1() || operation.Kind != domainstartup.SemanticOperationInstallFile || int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
					return nil, errors.New("case thread addition is not an applied signed operation")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil || !after {
					return nil, errors.Join(errors.New("case thread addition has not reached signed After"), err)
				}
			}
			delete(observation.original, id)
		case domainstartup.ManagedEntryTypeFile:
			if !present || int64(len(body)) != operation.Before.Size || domainsecurity.SHA256Hex(body) != operation.Before.SHA256 {
				return nil, errors.New("case thread semantic original Before bytes are unavailable")
			}
		default:
			return nil, errors.New("case thread semantic original record type is invalid")
		}
		if err := observation.final.applyV1(ctx, operation, id, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}, core.verification); err != nil {
			return nil, err
		}
	}
	observation.originalContexts, err = casethreadapp.VerifyCommittedContextInventoryV1(ctx, observation.original.recordsV1(), core.verification)
	if err != nil {
		return nil, err
	}
	observation.finalContexts, err = casethreadapp.VerifyCommittedContextInventoryV1(ctx, observation.final.recordsV1(), core.verification)
	if err != nil {
		return nil, err
	}
	return observation, nil
}

func readRuntimeCaseThreadSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (_ *runtimeCaseThreadSemanticObservationV1, resultErr error) {
	if scope == nil {
		return nil, errors.New("case thread original scope is unavailable")
	}
	observation, err := observeRuntimeCaseThreadSemanticInventoryV1(ctx, core)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx), scope.RevalidatePrimary(ctx)) }()
	originalHeld, err := observation.original.heldDigestsV1(scope)
	if err != nil {
		return nil, err
	}
	finalHeld, err := observation.final.heldDigestsV1(scope)
	if err != nil || !reflect.DeepEqual(originalHeld, finalHeld) {
		return nil, errors.Join(errors.New("case thread semantic candidate changed original hold"), err)
	}
	return observation, nil
}

func prepareRuntimeCaseThreadSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (*runtimeCaseThreadSemanticPreservationV1, error) {
	observation, err := readRuntimeCaseThreadSemanticInventoryV1(ctx, core, scope)
	if err != nil {
		return nil, err
	}
	held, err := observation.original.heldDigestsV1(scope)
	if err != nil {
		return nil, err
	}
	return &runtimeCaseThreadSemanticPreservationV1{heldDigests: held}, nil
}

func (preserved runtimeReportRestartPreservationV1) validateCaseThreadSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.caseThreads == nil {
		return errors.New("original case thread preservation is unavailable")
	}
	observation, err := readRuntimeCaseThreadSemanticInventoryV1(ctx, preserved.core, preserved.report)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	held, err := observation.original.heldDigestsV1(preserved.report)
	if err != nil || !reflect.DeepEqual(held, preserved.caseThreads.heldDigests) {
		return errors.Join(errors.New("original held case thread inventory changed"), err)
	}
	checkHeld := func(operation domainstartup.SemanticStartupOperationV1) error {
		for path := range held {
			if operation.Path == path || strings.HasPrefix(path, operation.Path+"/") && (operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
				return pendingworkapp.ErrRestartPreserved
			}
		}
		id, _, residue, err := caseThreadSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if residue && held[caseThreadSemanticPathV1(id)] != "" {
			return pendingworkapp.ErrRestartPreserved
		}
		return nil
	}
	for _, operation := range observation.journal.OperationsV1() {
		if err := checkHeld(operation); err != nil {
			return err
		}
	}
	candidate := observation.physical.cloneV1()
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		if err := checkHeld(operation); err != nil {
			return err
		}
		id, isRecord, _, err := caseThreadSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if isRecord {
			if err := candidate.applyV1(ctx, operation, id, readAfter, preserved.core.verification); err != nil {
				return err
			}
			if record, exists := candidate[id]; exists && caseThreadRecordHeldV1(preserved.report, record) {
				return pendingworkapp.ErrRestartPreserved
			}
		}
	}
	if _, err := casethreadapp.VerifyCommittedContextInventoryV1(ctx, candidate.recordsV1(), preserved.core.verification); err != nil {
		return err
	}
	return nil
}
