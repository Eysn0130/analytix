package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	apploop "analytix.local/runtime-go/internal/app/loop"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

func (h *runtimeServerHandler) restoreRuntimeState() error {
	if err := h.pendingWork.RevalidateRestartPreservationV1(context.Background()); err != nil {
		return err
	}
	if _, err := h.pendingWork.CloseAllOpenOnRestart(context.Background(), time.Now().UTC()); err != nil {
		return err
	}
	pendingWorkInventory, err := h.pendingWork.TrustedInventoryV1(context.Background())
	if err != nil {
		return err
	}
	unknownDispatches, err := pendingworkapp.RestartOutcomeUnknownGrantsV1(pendingWorkInventory)
	if err != nil {
		return err
	}
	restartGrantOutcomes, err := pendingworkapp.RestartGrantOutcomesByTurnV1(unknownDispatches)
	if err != nil {
		return err
	}
	if err := h.pendingWork.AddClosedReportRestartOutcomesV1(context.Background(), pendingWorkInventory, restartGrantOutcomes); err != nil {
		return err
	}
	// Validate the complete signed outcome inventory before separating records
	// that must never enter grant cancellation or stale-turn terminalization.
	for _, dispatch := range unknownDispatches {
		if h.pendingWork.OwnsRestartTurnV1(dispatch.ThreadID, dispatch.TurnID) {
			delete(restartGrantOutcomes, controlapp.TurnKey(dispatch.ThreadID, dispatch.TurnID))
		}
	}
	continuationInventory := continuationapp.TrustedInventoryV1{Dispositions: map[string]domaincontinuation.Disposition{}}
	if h.continuations != nil && h.continuations.Available() {
		var err error
		continuationInventory, err = h.continuations.TrustedInventoryV1(context.Background())
		if err != nil {
			return err
		}
	}
	restartGateInventory, err := gatecontinuationapp.NewRestartInventory(h.runtimeRestartGateInventoryDependencies())
	if err != nil {
		return err
	}
	gateReceiptsByThread := gatecontinuationapp.TrustedGateReceiptsByThreadV1(continuationInventory)
	threadIDs, err := h.store.AllThreadIDs()
	if err != nil {
		return err
	}
	if err := restartGateInventory.CloseOpenWithoutOwningThreadExceptOwnedV1(threadIDs, continuationInventory, h.pendingWork); err != nil {
		return err
	}
	maxTurnSeq := h.turnSeq
	for _, turnID := range h.pendingWork.RestartPreservedTurnIDsV1() {
		if sequence, ok := appturn.SequenceID(turnID); ok && sequence > maxTurnSeq {
			maxTurnSeq = sequence
		}
	}
	pendingApprovals := map[string]controlapp.PendingGateState[runtimePendingToolCall]{}
	pendingInputs := map[string]controlapp.PendingGateState[runtimePendingToolCall]{}
	restorableThreadIDs := make([]string, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		if h.pendingWork.OwnsRestartTurnV1(threadID, "") {
			continue
		}
		thread, err := h.store.GetThread(threadID)
		if err != nil {
			return err
		}
		if h.caseThreads != nil && h.caseThreads.IsCaseThread(threadID) && !h.caseThreads.CanExecute(threadID) {
			if err := restartGateInventory.CloseOpenForIsolatedThreadExceptOwnedV1(gateReceiptsByThread[threadID], continuationInventory.Dispositions, h.pendingWork); err != nil {
				return err
			}
			if appturn.ThreadHasActiveTurn(thread) {
				fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_QUARANTINED_ACTIVE_CASE_THREAD")
			}
			continue
		}
		restorableThreadIDs = append(restorableThreadIDs, threadID)
		epochState, persistEpochState, err := contextepochapp.RestoreThreadState(context.Background(), thread, threadID, nil, time.Now().UTC())
		if err != nil {
			return err
		}
		if persistEpochState {
			if _, err := h.store.PatchThread(threadID, map[string]any{"contextEpochState": contextepochapp.PublicState(epochState)}); err != nil {
				return err
			}
			thread, err = h.store.GetThread(threadID)
			if err != nil {
				return err
			}
		}
		if err := restartGateInventory.ReconcileThreadExceptOwnedV1(
			threadID, thread, gateReceiptsByThread[threadID], continuationInventory.Dispositions, h.pendingWork,
		); err != nil {
			return err
		}
		thread, err = h.store.GetThread(threadID)
		if err != nil {
			return err
		}
		result, err := h.store.LoadEventsSince(threadID, 0)
		if err != nil {
			return err
		}
		maxTurnSeq = appturn.RestoredRuntimeSequenceV1(maxTurnSeq, result.Events, nil)
		replay := controlapp.ReplayPendingGates(threadID, result.Events)
		for id, record := range replay.Approvals {
			state := controlapp.PendingGateState[runtimePendingToolCall]{Record: record}
			if err := threadapp.RevalidateAndCloseRestartedGateV1(
				domaincontinuation.KindApproval, id, thread, state, h.runtimeRestartGateReconciliationDependencies(),
			); err != nil {
				return fmt.Errorf("reconcile restarted approval %s: %w", id, err)
			}
			pendingApprovals[id] = state
		}
		for id, record := range replay.Inputs {
			state := controlapp.PendingGateState[runtimePendingToolCall]{Record: record}
			if err := threadapp.RevalidateAndCloseRestartedGateV1(
				domaincontinuation.KindUserInput, id, thread, state, h.runtimeRestartGateReconciliationDependencies(),
			); err != nil {
				return fmt.Errorf("reconcile restarted user input %s: %w", id, err)
			}
			pendingInputs[id] = state
		}
		for id, resolution := range replay.ClosedApprovalResolutions {
			if err := threadapp.ValidateClosedRestartedGateResolutionV1(
				domaincontinuation.KindApproval, id, thread, resolution, h.runtimeRestartGateReconciliationDependencies(),
			); err != nil {
				return fmt.Errorf("validate closed restarted approval %s: %w", id, err)
			}
		}
		for id, resolution := range replay.ClosedInputResolutions {
			if err := threadapp.ValidateClosedRestartedGateResolutionV1(
				domaincontinuation.KindUserInput, id, thread, resolution, h.runtimeRestartGateReconciliationDependencies(),
			); err != nil {
				return fmt.Errorf("validate closed restarted user input %s: %w", id, err)
			}
		}
		controlapp.AddClosedRestartGateTombstones(pendingApprovals, pendingInputs, replay)
		if len(result.Diagnostics) > 0 || len(replay.InvalidGateIDs) > 0 {
			fmt.Fprintf(os.Stderr, "analytix runtime: pending gate replay failed closed thread=%s diagnostics=%d invalidGateIds=%d\n", threadID, len(result.Diagnostics), len(replay.InvalidGateIDs))
		}
	}
	pendingTurnKeys := map[string]bool{}
	for _, state := range pendingApprovals {
		if state.Restored {
			pendingTurnKeys[controlapp.TurnKey(state.Record.ThreadID, state.Record.TurnID)] = true
		}
	}
	for _, state := range pendingInputs {
		if state.Restored {
			pendingTurnKeys[controlapp.TurnKey(state.Record.ThreadID, state.Record.TurnID)] = true
		}
	}
	if err := threadapp.AbortStaleRuntimeTurnsAfterRestartV1(
		restorableThreadIDs, pendingTurnKeys, restartGrantOutcomes, h.runtimeRestartTurnReconciliationDependencies(),
	); err != nil {
		return err
	}
	if h.jobs != nil {
		maxTurnSeq = appturn.RestoredRuntimeSequenceV1(maxTurnSeq, nil, h.jobs.AllRecords())
	}
	h.mu.Lock()
	h.turnSeq = maxTurnSeq
	// Legacy usage events are not signed cache-observation authority and must
	// never seed a comparison baseline after restart. A future durable adapter
	// may restore only verified CacheVisibleShapeV1 observations bound to the
	// current continuity digest.
	h.cachePrefixShapes = map[string]appusage.PrefixBaseline{}
	h.mu.Unlock()
	for id, state := range pendingApprovals {
		record := state.Record
		h.gates.RestoreApproval(id, state)
		if state.Restored {
			h.gate.RequestApproval(id, controlapp.GateRecordToolName(record))
		}
	}
	for id, state := range pendingInputs {
		record := state.Record
		h.gates.RestoreUserInput(id, state)
		if state.Restored {
			h.gate.RequestUserInput(id, controlapp.GateRecordPrompt(record))
		}
	}
	return nil
}

func (h *runtimeServerHandler) runtimeRestartGateInventoryDependencies() gatecontinuationapp.RestartInventoryDependencies {
	return gatecontinuationapp.RestartInventoryDependencies{
		Continuations:              h.continuations,
		GetThread:                  h.store.GetThread,
		EnsureGateRequestItemExact: h.store.EnsureGateRequestItemExact,
		RecordRequest:              h.recordPendingGateRequest,
		RecordResolution:           h.recordPendingGateResolution,
		PatchItemStatus: func(thread map[string]any, record controlapp.GateRecord, status string) error {
			return threadapp.PatchRestartedGateItemStatusV1(thread, record, status, h.store.PatchTurnItemStatus)
		},
		LoadEvents: func(threadID string) ([]map[string]any, error) {
			replay, err := h.store.LoadEventsSince(threadID, 0)
			return replay.Events, err
		},
	}
}

func (h *runtimeServerHandler) runtimeRestartGateReconciliationDependencies() threadapp.RestartGateReconciliationDependenciesV1 {
	return threadapp.RestartGateReconciliationDependenciesV1{
		Continuations: h.continuations, ProviderResolver: h.providerConfig,
		AuthorizePending: h.authorizeRuntimePending, PatchTurnItemStatus: h.store.PatchTurnItemStatus,
		RecordResolution: h.recordPendingGateResolution, RecordCancellations: h.recordPendingGateCancellations,
	}
}

func (h *runtimeServerHandler) runtimeRestartTurnReconciliationDependencies() threadapp.RestartTurnReconciliationDependenciesV1 {
	return threadapp.RestartTurnReconciliationDependenciesV1{
		GetThread:       h.store.GetThread,
		OwnsRestartTurn: h.pendingWork.OwnsRestartTurnV1,
		ReconcileTerminal: func(threadID, turnID string, outcomes executiongrantapp.RestartGrantOutcomesV1) error {
			_, err := executiongrantapp.ReconcileOpenTurnGrantsOnRestart(
				h.store, threadID, turnID, time.Now().UTC(), outcomes,
			)
			return err
		},
		AbortActive: h.abortStaleRuntimeTurnAfterRestart,
	}
}

func (h *runtimeServerHandler) abortStaleRuntimeTurnAfterRestart(
	threadID, turnID string,
	restartGrantOutcomes executiongrantapp.RestartGrantOutcomesV1,
) error {
	securityContext, err := appturn.LoadFrozenSecurityContext(h.store, threadID, turnID)
	if err != nil && !errors.Is(err, appturn.ErrFrozenSecurityContextMissing) {
		return err
	}
	if errors.Is(err, appturn.ErrFrozenSecurityContextMissing) {
		caseSensitive, classifyErr := h.threadRequiresCaseFinalGate(threadID)
		if classifyErr != nil {
			return classifyErr
		}
		if caseSensitive {
			return errors.New("case-sensitive restarted turn is missing its frozen security authority")
		}
		return errors.New("general restarted turn is missing its frozen security authority")
	}
	if _, err := executiongrantapp.ReconcileOpenTurnGrantsOnRestart(
		h.store, threadID, turnID, time.Now().UTC(), restartGrantOutcomes,
	); err != nil {
		return err
	}
	if err == nil && apploop.CasePublicationRequired(securityContext) {
		hostContext, cancel := newHostAuthorityContext()
		defer cancel()
		_, err := h.runtimePublicationAuthority().PersistCurrentCaseFixed(hostContext, evidenceapp.PersistCaseBoundaryInput{
			Store: h.store, Context: securityContext, TerminalReason: evidenceapp.TerminalRestart,
			ThreadID: threadID, TurnID: turnID, AcceptedAt: time.Now().UTC(),
		})
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	hostContext, cancel := newHostAuthorityContext()
	defer cancel()
	return h.runtimePublicationAuthority().WithCurrentGeneralFixedTerminal(hostContext, securityContext, func(context.Context) error {
		return appturn.PersistFailure(appturn.PersistFailureInput{
			Store: h.store, SecurityContext: securityContext, TerminalReason: "restart",
			ThreadID: threadID, TurnID: turnID, FinishedAt: now,
			Failure: domainfailure.New("runtime_restarted", nil),
		})
	})
}
