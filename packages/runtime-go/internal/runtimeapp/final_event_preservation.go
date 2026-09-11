package runtimeapp

import (
	"context"
	"errors"
	"reflect"
	"sort"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

type runtimeFinalEventPreservationV1 struct {
	preserved runtimeReportRestartPreservationV1
	inventory evidenceapp.FinalAuthorityInventory
	terminal  turnterminalapp.RestartRecoveryResultV1
	reader    evidenceapp.AcceptedFinalPublicReader
}

func (preserved runtimeReportRestartPreservationV1) finalEventObserverV1(inventory evidenceapp.FinalAuthorityInventory, terminal turnterminalapp.RestartRecoveryResultV1, reader evidenceapp.AcceptedFinalPublicReader) evidenceapp.FinalEventRestartPreservationV1 {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return nil
	}
	return &runtimeFinalEventPreservationV1{preserved: preserved, inventory: inventory, terminal: terminal, reader: reader}
}

func (observer *runtimeFinalEventPreservationV1) ObserveOriginalFinalEventInventoryV1(ctx context.Context) (_ evidenceapp.PreservedFinalEventInventoryV1, resultErr error) {
	empty := evidenceapp.PreservedFinalEventInventoryV1{}
	preserved := observer.preserved
	if ctx == nil || preserved.report == nil || preserved.core == nil || preserved.acceptedFinal == nil || preserved.durable == nil {
		return empty, errors.New("original final-event preservation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := preserved.core.revalidateKey(ctx); err != nil {
		return empty, err
	}
	original, _, observation, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, preserved.core, preserved.report)
	if err != nil {
		return empty, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx), preserved.durable.Revalidate(ctx, preserved.core.roots.DurableDir))
	}()
	digests, err := original.heldDigestsV1(preserved.report)
	if err != nil || !reflect.DeepEqual(digests, preserved.acceptedFinal.heldDigests) {
		return empty, errors.Join(errors.New("original final-event private graph changed"), err)
	}
	heldRecords := map[string]domainevidence.PrivateAcceptedFinalRecord{}
	for digest, record := range original.records {
		if preserved.report.OwnsThread(record.SecurityContext.ThreadID) {
			heldRecords[digest] = record
		}
	}
	seen := map[string]bool{}
	for _, record := range observer.terminal.Preserved {
		digest := record.AcceptedFinal.RecordDigest
		if seen[digest] || !reflect.DeepEqual(heldRecords[digest], record) {
			return empty, errors.New("original final-event terminal preservation changed")
		}
		seen[digest] = true
	}
	if len(seen) != len(heldRecords) {
		return empty, errors.New("original final-event terminal preservation is incomplete")
	}
	input := evidenceapp.PreservedFinalEventInventoryV1{Classified: observer.inventory, Preserved: observer.terminal.Preserved}
	for _, id := range preserved.report.ThreadIDs() {
		primary, err := preserved.report.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return empty, err
		}
		events, err := preserved.durable.ReadOriginalEventInventoryV1(ctx, id)
		if err != nil {
			return empty, err
		}
		turnSet := map[string]bool{}
		for _, record := range heldRecords {
			if record.SecurityContext.ThreadID == id {
				turnSet[record.SecurityContext.TurnID] = true
			}
		}
		turnIDs := make([]string, 0, len(turnSet))
		for id := range turnSet {
			turnIDs = append(turnIDs, id)
		}
		sort.Strings(turnIDs)
		cas := map[string]domainevidence.AcceptedFinalCASObservationV1{}
		if len(turnIDs) != 0 {
			entry, ok := preserved.core.primaries.entries[id]
			if !ok {
				return empty, errors.New("original final-event primary family is unavailable")
			}
			cas, err = entry.reader.ReadAcceptedFinalCASObservations(ctx, id, turnIDs)
			if err != nil {
				return empty, err
			}
			for _, value := range cas {
				if value.ThreadFileSHA256 != primary.ThreadFileSHA256 {
					return empty, errors.New("original final-event CAS bytes changed")
				}
			}
		}
		input.Threads = append(input.Threads, evidenceapp.PreservedFinalEventThreadV1{Primary: primary, Events: events, CAS: cas})
	}
	return input, nil
}

func (observer *runtimeFinalEventPreservationV1) ValidateCaseCompactionAuthorityTurnV1(id string, thread, turn map[string]any) error {
	reader, ok := observer.reader.(interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	})
	if !ok {
		return errors.New("original final-event compaction observer is unavailable")
	}
	return reader.ValidateCaseCompactionAuthorityTurnV1(id, thread, turn)
}
