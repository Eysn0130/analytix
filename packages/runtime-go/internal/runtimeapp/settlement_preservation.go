package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	evidencesettlement "analytix.local/runtime-go/internal/adapters/outbound/evidencesettlement"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func (preserved runtimeReportRestartPreservationV1) openSettlementStoreV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*evidencesettlement.Store, error) {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return evidencesettlement.NewStore(root)
	}
	if err := preserved.validateSettlementSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return nil, err
	}
	original, err := evidencesettlement.ObservePreparedInventoryV1(ctx, root, access)
	if err != nil {
		return nil, err
	}
	store, err := original.OpenStoreWithRestartPreservationV1(ctx, func(ctx context.Context, record domainevidence.PreparedEvidenceSettlement) error {
		if preserved.report.OwnsThread(record.SecurityContext.ThreadID) {
			return errors.New("original held settlement cannot be prepared during restart preservation")
		}
		return preserved.validateSettlementSemanticOperationsV1(ctx, nil, nil, "")
	})
	if err != nil {
		return nil, err
	}
	if err := preserved.validateSettlementSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return nil, err
	}
	return store, nil
}

func (preserved runtimeReportRestartPreservationV1) settlementObserverV1() evidenceapp.EvidenceSettlementRestartPreservationV1 {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		if preserved.finalHistory != nil && preserved.finalHistory.settlementHistory != nil {
			return runtimeOriginalSettlementObserverV1{frozen: preserved.finalHistory}
		}
		return nil
	}
	return preserved
}

func (preserved runtimeReportRestartPreservationV1) ObserveOriginalEvidenceSettlementInventoryV1(ctx context.Context) (evidenceapp.PreservedEvidenceSettlementInventoryV1, error) {
	original, _, _, err := preserved.observeSettlementInventoryV1(ctx)
	if err != nil {
		return evidenceapp.PreservedEvidenceSettlementInventoryV1{}, err
	}
	return original, nil
}

func (preserved runtimeReportRestartPreservationV1) ObserveRestartEvidenceRegistryInventoryV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	original, records, observed, err := preserved.observeSettlementInventoryV1(ctx)
	if err != nil {
		return nil, err
	}
	if !sameRuntimeSettlementContextsV1(contexts, observed) {
		return nil, errors.New("startup settlement primary context denominator changed")
	}
	if original.RegistryHistoryV2 != nil || original.RegistryUnavailable != nil {
		return nil, errors.New("original V2 registry history requires its complete typed graph")
	}
	return records, nil
}

func (preserved runtimeReportRestartPreservationV1) ObserveRestartEvidenceRegistryInventoryV2(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, *registryport.OriginalHistoryV2, error) {
	original, records, observed, err := preserved.observeSettlementInventoryV1(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !sameRuntimeSettlementContextsV1(contexts, observed) {
		return nil, nil, errors.New("startup V2 settlement primary context denominator changed")
	}
	if original.RegistryUnavailable != nil {
		return nil, nil, errors.New("unavailable original registry requires its typed raw observation")
	}
	return records, original.RegistryHistoryV2, nil
}

func (preserved runtimeReportRestartPreservationV1) ObserveRestartEvidenceRegistryObservationV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) (registryport.OriginalObservationV1, error) {
	original, records, observed, err := preserved.observeSettlementInventoryV1(ctx)
	if err != nil {
		return registryport.OriginalObservationV1{}, err
	}
	if !sameRuntimeSettlementContextsV1(contexts, observed) {
		return registryport.OriginalObservationV1{}, errors.New("startup registry observation primary denominator changed")
	}
	return registryport.OriginalObservationV1{Legacy: records, HistoryV2: original.RegistryHistoryV2, Unavailable: original.RegistryUnavailable}, nil
}

func sameRuntimeSettlementContextsV1(left, right []domainsecurity.TurnSecurityContext) bool {
	if len(left) != len(right) {
		return false
	}
	byDigest := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range left {
		if frozen.ContextDigest == "" || byDigest[frozen.ContextDigest].ContextDigest != "" {
			return false
		}
		byDigest[frozen.ContextDigest] = frozen
	}
	for _, frozen := range right {
		if byDigest[frozen.ContextDigest] != frozen {
			return false
		}
		delete(byDigest, frozen.ContextDigest)
	}
	return len(byDigest) == 0
}

// Reobserve both complete original owners before selecting the held records.
// No Store activation, live registry read, repair, or issuance occurs here.
func (preserved runtimeReportRestartPreservationV1) observeSettlementInventoryV1(ctx context.Context) (_ evidenceapp.PreservedEvidenceSettlementInventoryV1, _ []registryport.InventoryRecord, _ []domainsecurity.TurnSecurityContext, resultErr error) {
	empty := evidenceapp.PreservedEvidenceSettlementInventoryV1{}
	if ctx == nil || preserved.report == nil || preserved.core == nil || preserved.registry == nil || preserved.settlements == nil {
		return empty, nil, nil, errors.New("original settlement startup preservation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return empty, nil, nil, err
	}
	preparedFiles, preparedFinal, contexts, settlementObservation, err := readRuntimeSettlementSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.settlements.recoveryBefore)
	if err != nil {
		return empty, nil, nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, settlementObservation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	if err := preserved.settlements.validateHeldV1(preparedFiles, preserved.report); err != nil {
		return empty, nil, nil, err
	}
	for _, operation := range settlementObservation.journal.OperationsV1() {
		if name, owned := settlementSemanticRelativeV1(operation); owned {
			if err := preparedFinal.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return settlementObservation.journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return empty, nil, nil, err
			}
		}
	}
	if err := preserved.settlements.validateHeldV1(preparedFinal, preserved.report); err != nil {
		return empty, nil, nil, err
	}
	preparedRecords, err := preparedFiles.verifySettlementsV1(ctx, preserved.core, contexts)
	if err != nil {
		return empty, nil, nil, err
	}
	registryFiles, registryFinal, registryContexts, registryObservation, err := readRuntimeRegistrySemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.registry.recoveryBefore)
	if err != nil {
		return empty, nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, registryObservation.Revalidate(ctx)) }()
	if !sameRuntimeSettlementContextsV1(contexts, registryContexts) {
		return empty, nil, nil, errors.New("original settlement owners observed different primary contexts")
	}
	if err := preserved.registry.validateHeldV1(registryFiles, preserved.report, contexts); err != nil {
		return empty, nil, nil, err
	}
	for _, operation := range registryObservation.journal.OperationsV1() {
		if name, owned := registrySemanticRelativeV1(operation); owned {
			if err := registryFinal.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return registryObservation.journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return empty, nil, nil, err
			}
		}
	}
	if err := preserved.registry.validateHeldV1(registryFinal, preserved.report, contexts); err != nil {
		return empty, nil, nil, err
	}
	registryInventory, err := observeRuntimeOriginalRegistryDomainV1(ctx, preserved.core, registryFiles, contexts, true)
	if err != nil {
		return empty, nil, nil, err
	}
	if registryObservation.unavailable != preserved.registry.unavailable || registryInventory.unavailable != preserved.registry.unavailable {
		return empty, nil, nil, errors.New("original settlement registry availability changed")
	}
	registries := registryInventory.legacy
	original := evidenceapp.PreservedEvidenceSettlementInventoryV1{HistoricalRegistryV1: !registryInventory.unavailable && registryInventory.v2 == nil, RegistryHistoryV2: registryInventory.v2}
	if registryInventory.unavailable {
		body, err := json.Marshal(registryFiles)
		if err != nil {
			return empty, nil, nil, err
		}
		original.RegistryUnavailable = &registryport.UnavailableOriginalInventoryV1{InventoryDigest: domainsecurity.SHA256Hex(body)}
	}
	for _, id := range preserved.report.ThreadIDs() {
		primary, err := preserved.report.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return empty, nil, nil, err
		}
		original.Threads = append(original.Threads, primary)
	}
	primarySnapshot, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, preserved.core.roots)
	if err != nil {
		return empty, nil, nil, err
	}
	primaryInventory, _, err := readRuntimeOriginalPrimaryInventoryV1(ctx, preserved.core.roots, primarySnapshot)
	if err != nil {
		return empty, nil, nil, err
	}
	defer func() {
		current, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, preserved.core.roots)
		if err != nil || !reflect.DeepEqual(primarySnapshot, current) {
			resultErr = errors.Join(resultErr, errors.New("original settlement primary inventory changed"), err)
		}
	}()
	for id := range primaryInventory.entries {
		if preserved.report.OwnsThread(id) {
			continue
		}
		primary, err := primaryInventory.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return empty, nil, nil, err
		}
		original.AdditionalPrimaries = append(original.AdditionalPrimaries, primary)
	}
	sort.Slice(original.AdditionalPrimaries, func(i, j int) bool {
		return original.AdditionalPrimaries[i].ThreadID < original.AdditionalPrimaries[j].ThreadID
	})
	for _, record := range preparedRecords {
		if preserved.report.OwnsThread(record.SecurityContext.ThreadID) {
			original.Prepared = append(original.Prepared, record)
		}
	}
	sort.Slice(original.Prepared, func(i, j int) bool { return original.Prepared[i].SettlementID < original.Prepared[j].SettlementID })
	for _, record := range registries {
		if preserved.report.OwnsThread(record.Context.ThreadID) {
			original.Registries = append(original.Registries, record)
		}
	}
	return original, registries, contexts, nil
}
