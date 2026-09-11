package server

import (
	"context"
	"net/http"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	apploop "analytix.local/runtime-go/internal/app/loop"
)

func (h *runtimeServerHandler) handleApprovalPath(w http.ResponseWriter, r *http.Request) {
	httpapi.GateHandlers{Control: h.runtimeControl()}.HandleApproval(w, r)
}

func (h *runtimeServerHandler) approveRuntimeTool(ctx context.Context, request controlapp.ApprovalDecision) (controlapp.ActionResult, error) {
	return gatecontinuationapp.ApproveRegistered(ctx, request, h.runtimeGateContinuationDependencies(), h.runtimeControl()), nil
}

func (h *runtimeServerHandler) handleUserInputPath(w http.ResponseWriter, r *http.Request, prefix string) {
	httpapi.GateHandlers{Control: h.runtimeControl()}.HandleUserInput(w, r, prefix)
}

func (h *runtimeServerHandler) respondRuntimeUserInput(ctx context.Context, request controlapp.UserInputResponse) (controlapp.ActionResult, error) {
	return gatecontinuationapp.RespondUserInputRegistered(ctx, request, h.runtimeGateContinuationDependencies(), h.runtimeControl()), nil
}

func (h *runtimeServerHandler) runtimeGateContinuationDependencies() gatecontinuationapp.Dependencies {
	return gatecontinuationapp.Dependencies{
		Store:                 h.store,
		WorkspaceReader:       filestore.CaseBindingReader{},
		SecurityAuthority:     h.turnSecurity,
		Source:                h.mcp,
		Registry:              h.gates,
		Manager:               h.gate,
		Continuations:         h.continuations,
		AcquireContextEffect:  h.runtimeSubagentState().AcquireContextEffect,
		AcquireOrdinaryEffect: h.runtimeSubagentState().AcquireOrdinaryContextEffect,
		ValidateCatalog:       h.validateRuntimePendingCatalog,
		ExecuteAndSettle: func(ctx context.Context, pending runtimePendingToolCall) (apploop.SettledToolExecution, error) {
			return h.executeAndSettleRuntimeTool(ctx, pending, nil, nil)
		},
		PersistToolResult: func(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) error {
			_, err := h.persistRuntimeToolResultAndMessage(ctx, pending, output, isError)
			return err
		},
		Complete:        h.completeRuntimeContinuation,
		CompleteSettled: h.completeRuntimeContinuationWithSettled,
		Finalize: func(ctx context.Context, pending runtimePendingToolCall, result runtimeAgentLoopResult, reason evidenceapp.TerminalReason) error {
			return h.finalizeRuntimeTurnAfterLoop(ctx, pending, result, reason)
		},
		RecordFailure:           h.recordRuntimeTurnFailureForPending,
		TerminalStatus:          h.runtimeTurnTerminalStatus,
		RecordCancellations:     h.recordPendingGateCancellations,
		RecordRequest:           h.recordPendingGateRequest,
		RecordGrantTransition:   h.recordApprovalGrantTransition,
		EnsureGrantTransition:   h.store.EnsureApprovalTransitionItemExact,
		RecordResolution:        h.recordPendingGateResolution,
		TerminalTakeoverPending: h.runtimeControl().HostTerminalTakeoverReserved,
	}
}

func (h *runtimeServerHandler) runtimeGateEventOutboxDependencies() gatecontinuationapp.EventOutboxDependencies {
	return gatecontinuationapp.EventOutboxDependencies{
		GetThread: h.store.GetThread,
		LoadEvents: func(threadID string) ([]map[string]any, error) {
			replay, err := h.store.LoadEventsSince(threadID, 0)
			return replay.Events, err
		},
		RecordEvent: func(event map[string]any) error {
			_, _, err := h.store.RecordEvent(event)
			return err
		},
	}
}

func (h *runtimeServerHandler) recordApprovalGrantTransition(event map[string]any) error {
	h.gateCancellationMu.Lock()
	defer h.gateCancellationMu.Unlock()
	return gatecontinuationapp.RecordApprovalGrantTransitionV1(event, h.runtimeGateEventOutboxDependencies())
}
