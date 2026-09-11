package runtimeapp

import (
	"context"
	"errors"
	"reflect"

	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// These subsets carry proof responsibilities only. They are never inventory
// inputs or sealed recovery plans: both complete endpoints are audited first.
func runtimeMixedClosedDeferredPlansV1(plan publicationapp.RestartPlanV1) (open, closed publicationapp.RestartPlanV1, ok bool) {
	if len(plan.Attempts) < 2 {
		return open, closed, false
	}
	partial := false
	for _, entry := range plan.Attempts {
		single := publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}
		if runtimeReportPlanIsDeferredV1(single) {
			open.Attempts = append(open.Attempts, entry)
			partial = true
		} else if runtimeReportEntryIsClosedV1(entry) {
			closed.Attempts = append(closed.Attempts, entry)
			partial = partial || entry.DeliveryOutcome == nil
		} else {
			return open, closed, false
		}
	}
	// Entirely delivered histories retain their existing controlled recovery
	// consumer. Every partial inventory uses the same per-entry proof rule.
	return open, closed, partial
}

func runtimeReportEntryIsClosedV1(entry publicationapp.RestartAttemptV1) bool {
	single := publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}
	return runtimeReportPlanIsClosedFailedV1(single) || runtimeReportPlanIsClosedCompletedPrefixV1(single) ||
		(entry.DeliveryOutcome != nil && (entry.State == publicationapp.RestartAttemptDeliveryProjectionV1 || entry.State == publicationapp.RestartAttemptDeliveryRejectionV1))
}

func validateRuntimeMixedClosedCoreV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, closed publicationapp.RestartPlanV1, pending pendingapp.TrustedInventoryV1, primaries recoveryport.PrimaryThreadReaderV1) error {
	for _, entry := range closed.Attempts {
		single := publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}
		if err := validateRuntimeClosedReportPendingV1(single, pending); err != nil {
			return err
		}
		if runtimeReportPlanIsClosedFailedV1(single) {
			if err := validateRuntimeClosedFailedCoreV1(ctx, core, publication, advance, single, pending, primaries); err != nil {
				return err
			}
		} else if runtimeReportPlanIsClosedCompletedPrefixV1(single) {
			if err := validateRuntimeClosedCompletedCoreV1(ctx, core, publication, advance, single, pending, primaries); err != nil {
				return err
			}
		} else if !runtimeReportEntryIsClosedV1(entry) {
			return errRuntimeReportRestartReconciliationRequired
		}
		// Delivery outcomes already passed the complete endpoint historical
		// verifier, including actual Core, grant settlement and witness graph.
	}
	return context.Cause(ctx)
}

func prepareRuntimeMixedReportHistoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, scope *pendingapp.ReportRestartScopeV1, history, finalHistory *runtimeOriginalReportHistoryV1, original, final, associatedOriginal, associatedFinal map[string]runtimeOriginalSemanticFilesV1) error {
	open, closed, ok := runtimeMixedClosedDeferredPlansV1(history.plan)
	if !ok || !reflect.DeepEqual(history.plan, finalHistory.plan) || !reflect.DeepEqual(original, final) {
		return errors.New("mixed report Original/Final history changed")
	}
	if len(runtimeReportCommittedFilesV1(original["controlled-artifact-access"])) != 0 {
		return errRuntimeReportRestartReconciliationRequired
	}
	if len(open.Attempts) != 0 {
		history.deferred = &runtimeDeferredReportHistoryV1{plan: open, advance: advance}
		if err := history.deferred.validateV1(ctx, core, publication, scope, core.pendingInventory); err != nil {
			return err
		}
	}
	if err := validateRuntimeMixedClosedCoreV1(ctx, core, publication, advance, closed, core.pendingInventory, core.primaries); err != nil {
		return err
	}
	history.mixedClosed = &closed
	if err := history.captureClosedResultsV1(ctx, core.primaries); err != nil {
		return err
	}
	var err error
	history.controlled, err = prepareRuntimeCompletedControlledAccessHistoryV2(ctx, core, publication, history, finalHistory, original, final)
	if err != nil {
		return err
	}
	history.semantic, err = prepareRuntimeCompletedReportSemanticPreservationV1(ctx, core, publication, advance, history, original, final, associatedOriginal, associatedFinal)
	return err
}

func (history *runtimeOriginalReportHistoryV1) validateMixedCandidateV1(ctx context.Context, preserved *runtimeCompletedReportSemanticPreservationV1, candidate *runtimeOriginalReportHistoryV1, primaries runtimeReportSemanticPrimariesV1) error {
	open, closed, ok := runtimeMixedClosedDeferredPlansV1(candidate.plan)
	if history.mixedClosed == nil || !ok || !reflect.DeepEqual(history.plan, candidate.plan) ||
		!reflect.DeepEqual(closed, *history.mixedClosed) ||
		(len(open.Attempts) != 0 && (history.deferred == nil || !reflect.DeepEqual(open, history.deferred.plan))) ||
		(len(open.Attempts) == 0 && history.deferred != nil) {
		return errors.New("mixed report semantic candidate changed its complete history")
	}
	for _, entry := range open.Attempts {
		id := entry.Stage.Context.ThreadID
		original, err := preserved.core.primaries.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return err
		}
		if primaries[id].ThreadFileSHA256 != original.ThreadFileSHA256 {
			return errors.New("mixed report semantic candidate changed a held primary")
		}
	}
	if err := validateRuntimeMixedClosedCoreV1(ctx, preserved.core, preserved.publication, preserved.advance, closed, preserved.core.pendingInventory, primaries); err != nil {
		return err
	}
	return history.validateClosedResultsV1(ctx, primaries)
}
