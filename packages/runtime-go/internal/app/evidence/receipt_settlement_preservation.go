package evidence

import (
	"context"
	"errors"
	"reflect"
	"sort"

	"analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// These are original observations, never execution or issuance capabilities.
// The root must revalidate the complete original physical and signed owner
// inventories on every observation, including absence and shared-index rows.
type PreservedEvidenceSettlementInventoryV1 struct {
	Threads []recoveryport.PrimaryThreadSnapshotV1
	// AdditionalPrimaries completes the original denominator. A primary omitted
	// by the executable reader may be read only if that reader explicitly marks
	// it quarantined; it never joins Threads or acquires held/execution status.
	AdditionalPrimaries  []recoveryport.PrimaryThreadSnapshotV1
	Prepared             []domainevidence.PreparedEvidenceSettlement
	Registries           []registryport.InventoryRecord
	HistoricalRegistryV1 bool
	RegistryHistoryV2    *registryport.OriginalHistoryV2
	RegistryUnavailable  *registryport.UnavailableOriginalInventoryV1
}

type EvidenceSettlementRestartPreservationV1 interface {
	ObserveOriginalEvidenceSettlementInventoryV1(context.Context) (PreservedEvidenceSettlementInventoryV1, error)
}

// This startup-only source observes the complete original registry inventory.
// It supplies historical records, never current execution or commit authority.
type EvidenceSettlementRestartRegistryInventoryV1 interface {
	ObserveRestartEvidenceRegistryInventoryV1(context.Context, []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error)
}

// V2 original history stays separate from the single-registry V1 inventory.
// Neither a local candidate nor this observation authorizes current repair.
type EvidenceSettlementRestartRegistryInventoryV2 interface {
	ObserveRestartEvidenceRegistryInventoryV2(context.Context, []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, *registryport.OriginalHistoryV2, error)
}

type EvidenceSettlementRestartRegistryObservationV1 interface {
	ObserveRestartEvidenceRegistryObservationV1(context.Context, []domainsecurity.TurnSecurityContext) (registryport.OriginalObservationV1, error)
}

func preservedEvidenceSettlementThreadsV1(input *PreservedEvidenceSettlementInventoryV1) map[string]bool {
	held := map[string]bool{}
	if input != nil {
		for _, primary := range input.Threads {
			held[primary.ThreadID] = true
		}
	}
	return held
}

func validatePreservedEvidenceSettlementRecordsV1(input *PreservedEvidenceSettlementInventoryV1, prepared []domainevidence.PreparedEvidenceSettlement, registries []registryport.InventoryRecord) error {
	if input == nil {
		return nil
	}
	held := preservedEvidenceSettlementThreadsV1(input)
	contexts := map[string]domainsecurity.TurnSecurityContext{}
	for _, primary := range input.Threads {
		turns, err := strictSettlementTurns(primary.Thread)
		if err != nil {
			return err
		}
		for _, turn := range turns {
			if value, present := turn["securityContext"]; present && value != nil {
				frozen, err := domainsecurity.ParseTurnSecurityContext(value)
				if err != nil || frozen.ThreadID != primary.ThreadID || frozen.TurnID != authorityString(turn, "id") || contexts[frozen.ContextDigest].ContextDigest != "" {
					return errors.Join(errors.New("original settlement context denominator is invalid"), err)
				}
				contexts[frozen.ContextDigest] = frozen
			}
		}
	}
	expectedPrepared := map[string]domainevidence.PreparedEvidenceSettlement{}
	for _, record := range input.Prepared {
		if !held[record.SecurityContext.ThreadID] || !reflect.DeepEqual(contexts[record.SecurityContext.ContextDigest], record.SecurityContext) || expectedPrepared[record.SettlementID].SettlementID != "" || domainevidence.ValidatePreparedEvidenceSettlement(record) != nil {
			return errors.New("original prepared settlement denominator is invalid")
		}
		expectedPrepared[record.SettlementID] = record
	}
	actualPrepared := map[string]domainevidence.PreparedEvidenceSettlement{}
	for _, record := range prepared {
		if held[record.SecurityContext.ThreadID] {
			if actualPrepared[record.SettlementID].SettlementID != "" {
				return errors.New("held prepared settlement is duplicated")
			}
			actualPrepared[record.SettlementID] = record
		}
	}
	if !reflect.DeepEqual(expectedPrepared, actualPrepared) {
		return errors.New("original prepared settlement denominator changed")
	}
	expectedRegistries := map[string]registryport.InventoryRecord{}
	for _, record := range input.Registries {
		digest := record.Context.ContextDigest
		if !held[record.Context.ThreadID] || !reflect.DeepEqual(contexts[digest], record.Context) || expectedRegistries[digest].Context.ContextDigest != "" || domainevidence.ValidateEvidenceReceiptRegistry(record.Registry) != nil {
			return errors.New("original settlement registry denominator is invalid")
		}
		expectedRegistries[digest] = record
	}
	actualRegistries := map[string]registryport.InventoryRecord{}
	for _, record := range registries {
		if held[record.Context.ThreadID] {
			digest := record.Context.ContextDigest
			if actualRegistries[digest].Context.ContextDigest != "" {
				return errors.New("held settlement registry is duplicated")
			}
			actualRegistries[digest] = record
		}
	}
	if !reflect.DeepEqual(expectedRegistries, actualRegistries) {
		return errors.New("original settlement registry denominator changed")
	}
	return nil
}

type preservedEvidenceSettlementReaderV1 struct {
	AcceptedFinalPublicReader
	ids  []string
	held map[string]map[string]any
}

func (reader preservedEvidenceSettlementReaderV1) AllThreadIDs() ([]string, error) {
	return append([]string{}, reader.ids...), nil
}

func (reader preservedEvidenceSettlementReaderV1) GetThread(id string) (map[string]any, error) {
	if primary, held := reader.held[id]; held {
		return contracts.CloneMap(primary), nil
	}
	return reader.AcceptedFinalPublicReader.GetThread(id)
}

func (reader preservedEvidenceSettlementReaderV1) QuarantinedThread(id string) bool {
	quarantine, ok := reader.AcceptedFinalPublicReader.(interface{ QuarantinedThread(string) bool })
	return ok && quarantine.QuarantinedThread(id)
}

func PreflightEvidenceSettlementInventoryWithPreservationV1(ctx context.Context, reader AcceptedFinalPublicReader, issuer Issuer, observer EvidenceSettlementRestartPreservationV1) (EvidenceSettlementInventory, error) {
	if observer == nil {
		return PreflightEvidenceSettlementInventory(ctx, reader, issuer)
	}
	if ctx == nil || reader == nil {
		return EvidenceSettlementInventory{}, errors.New("original settlement observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return EvidenceSettlementInventory{}, err
	}
	original, err := observer.ObserveOriginalEvidenceSettlementInventoryV1(ctx)
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	original.RegistryHistoryV2, err = cloneOriginalRegistryHistoryV2(original.RegistryHistoryV2)
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	if original.RegistryUnavailable != nil {
		copied := *original.RegistryUnavailable
		original.RegistryUnavailable = &copied
	}
	ids, err := strictEvidenceSettlementThreadIDs(reader)
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	joined := preservedEvidenceSettlementReaderV1{AcceptedFinalPublicReader: reader, ids: ids, held: map[string]map[string]any{}}
	for _, primary := range original.Threads {
		id := primary.ThreadID
		if seen[id] || !domainsecurity.IsSHA256Hex(primary.ThreadFileSHA256) || domainthread.ValidatePrimaryIdentityV1(id, primary.Thread) != nil {
			return EvidenceSettlementInventory{}, errors.New("original settlement thread denominator overlaps or is invalid")
		}
		seen[id] = true
		joined.ids = append(joined.ids, id)
		joined.held[id] = primary.Thread
	}
	originalSeen := preservedEvidenceSettlementThreadsV1(&original)
	quarantine, canClassify := reader.(interface{ QuarantinedThread(string) bool })
	for _, primary := range original.AdditionalPrimaries {
		id := primary.ThreadID
		if originalSeen[id] || !domainsecurity.IsSHA256Hex(primary.ThreadFileSHA256) || domainthread.ValidatePrimaryIdentityV1(id, primary.Thread) != nil {
			return EvidenceSettlementInventory{}, errors.New("additional original settlement primary is duplicated or invalid")
		}
		originalSeen[id] = true
		if seen[id] {
			continue
		}
		if !canClassify || !quarantine.QuarantinedThread(id) {
			return EvidenceSettlementInventory{}, errors.New("additional original settlement primary lacks audit-only classification")
		}
		seen[id] = true
		joined.ids = append(joined.ids, id)
		joined.held[id] = primary.Thread
	}
	sort.Strings(joined.ids)
	var registrySource any
	if source, ok := observer.(EvidenceSettlementRestartRegistryObservationV1); ok {
		registrySource = source
	} else if source, ok := observer.(EvidenceSettlementRestartRegistryInventoryV2); ok {
		registrySource = source
	} else if source, ok := observer.(EvidenceSettlementRestartRegistryInventoryV1); ok {
		registrySource = source
	}
	inventory, err := preflightEvidenceSettlementInventoryV1(ctx, joined, issuer, &original, registrySource)
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	verified, err := observer.ObserveOriginalEvidenceSettlementInventoryV1(ctx)
	if err != nil || !reflect.DeepEqual(original, verified) {
		return EvidenceSettlementInventory{}, errors.Join(errors.New("original settlement observation changed during preflight"), err)
	}
	return inventory, ctx.Err()
}
