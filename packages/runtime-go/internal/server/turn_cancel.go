package server

import (
	"context"
	"errors"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
)

func (h *runtimeServerHandler) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	controller := h.runtimeControl()
	controller.BeginShutdown()
	controller.CancelAllRegisteredTurns()
	backgroundWait := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < backgroundWait {
		backgroundWait = time.Until(deadline)
	}
	_, _, backgroundErr := h.runtimeSubagentState().CancelBackgroundJobsAndWait(backgroundWait)
	waitErr := controller.WaitForTurnOperations(ctx)
	var gateErr error
	if waitErr == nil && backgroundErr == nil {
		hostContext, cancel := newHostAuthorityContext()
		drained := h.gates.DrainAll()
		gateErr = gatecontinuationapp.CloseDrainedForShutdown(hostContext, drained, h.runtimeGateContinuationDependencies())
		cancel()
		if len(drained) > 0 {
			if gateErr != nil && !h.gates.RestoreDrained(drained) {
				gateErr = errors.Join(gateErr, errors.New("shutdown gate settlement claim could not be retained"))
			} else if gateErr == nil && !h.gates.CommitDrained(drained) {
				gateErr = errors.New("shutdown gate settlement claim could not be committed")
			}
		}
	}
	var disconnectErr error
	var nativeErr error
	// Native containment is an independent safety obligation. Persistence or
	// gate-drain failures must not suppress the only lifecycle owner cleanup.
	if h.nativeAuthority != nil {
		nativeErr = h.nativeAuthority.Close()
	}
	if h.mcp != nil {
		done := make(chan struct{})
		go func() {
			h.mcp.Disconnect()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			disconnectErr = ctx.Err()
		}
	}
	return errors.Join(backgroundErr, waitErr, gateErr, nativeErr, disconnectErr)
}

func (h *runtimeServerHandler) settlePendingRuntimeGatesForTurn(
	threadID string,
	turnID string,
) ([]controlapp.DrainedGate[runtimePendingToolCall], []controlapp.PendingGateCancellation, error) {
	drained := h.gates.DrainForTurn(threadID, turnID)
	cancellations, err := gatecontinuationapp.SettleDrainedForTerminal(
		drained,
		"turn_interrupted",
		h.runtimeGateContinuationDependencies(),
	)
	return drained, cancellations, err
}

func (h *runtimeServerHandler) recordPendingGateCancellations(cancellations []controlapp.PendingGateCancellation, reason string) error {
	events := controlapp.PendingGateCancellationEvents(cancellations, reason)
	if len(events) == 0 {
		return nil
	}
	for _, event := range events {
		if err := h.recordPendingGateResolution(event); err != nil {
			return err
		}
	}
	return nil
}

func (h *runtimeServerHandler) recordPendingGateRequest(event map[string]any) error {
	h.gateCancellationMu.Lock()
	defer h.gateCancellationMu.Unlock()
	return gatecontinuationapp.RecordPendingGateRequestV1(event, h.runtimeGateEventOutboxDependencies())
}

func (h *runtimeServerHandler) recordPendingGateResolution(event map[string]any) error {
	h.gateCancellationMu.Lock()
	defer h.gateCancellationMu.Unlock()
	return gatecontinuationapp.RecordPendingGateResolutionV1(event, h.runtimeGateEventOutboxDependencies())
}

func (h *runtimeServerHandler) runtimeTurnTerminalStatus(threadID, turnID string) (string, bool, error) {
	return controlapp.TerminalTurnStatus(h.store, threadID, turnID)
}
