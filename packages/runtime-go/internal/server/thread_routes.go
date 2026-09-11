package server

import (
	"context"
	"net/http"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func (h *runtimeServerHandler) handleThreads(w http.ResponseWriter, r *http.Request) {
	httpapi.ThreadHandlers{Service: h.runtimeThreadService()}.HandleThreads(w, r)
}

func (h *runtimeServerHandler) handleThreadPath(w http.ResponseWriter, r *http.Request) {
	httpapi.ThreadPathHandlers{
		Summary:     h.handleThreadSummaryPath,
		Events:      h.handleThreadEvents,
		Fork:        h.handleThreadFork,
		Goal:        h.handleThreadGoal,
		Todos:       h.handleThreadTodos,
		Compact:     h.handleThreadCompact,
		Review:      h.handleThreadReview,
		Checkpoints: h.handleThreadCheckpointPath,
		TurnAction:  h.handleThreadTurnAction,
		Turns:       h.handleThreadTurns,
		Rewind:      h.handleThreadRewind,
		Record:      h.handleThreadRecord,
	}.Handle(w, r)
}

func (h *runtimeServerHandler) handleThreadRewind(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ThreadHandlers{Service: h.runtimeThreadService()}.HandleRewind(w, r, threadID)
}

func (h *runtimeServerHandler) handleThreadRecord(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ThreadHandlers{
		Service:                      h.runtimeThreadService(),
		HydrateAcceptedFinalDelivery: h.hydrateAcceptedFinalDelivery,
	}.HandleRecord(w, r, threadID)
}

func (h *runtimeServerHandler) hydrateAcceptedFinalDelivery(
	ctx context.Context,
	threadID string,
	projectedThread map[string]any,
) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
	dependencies := threadapp.AcceptedFinalHydrationDependenciesV1{}
	if h != nil && h.store != nil {
		dependencies.ReadAuthorityThread, dependencies.Projector = h.store.GetThread, h.publicProjector
		dependencies.ReadDurableEvents = func(ctx context.Context, threadID string) ([]map[string]any, int, error) {
			replay, err := h.store.LoadPublicEventsSinceContext(ctx, threadID, 0)
			return replay.Events, len(replay.Diagnostics), err
		}
	}
	return threadapp.HydrateAcceptedFinalDeliveriesV1(ctx, threadID, projectedThread, dependencies)
}

func (h *runtimeServerHandler) handleThreadFork(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ThreadHandlers{Service: h.runtimeThreadService()}.HandleFork(w, r, threadID)
}

func (h *runtimeServerHandler) runtimeThreadService() *threadapp.Service {
	state := h.runtimeSubagentState()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.threads == nil {
		h.threads = threadapp.NewService(threadapp.Dependencies{
			Repository:               h.store,
			DataDir:                  h.dataDir,
			DefaultApprovalPolicy:    h.approvalPolicy,
			DefaultSandboxMode:       h.sandboxMode,
			UsageSnapshot:            h.threadUsageSnapshot,
			PendingGateSnapshot:      h.pendingGateIDsFromReplay,
			PublicProjector:          h.publicProjector,
			CaseThreads:              h.caseThreads,
			WorkspaceReader:          filestore.CaseBindingReader{},
			WorkspaceSecurity:        h.turnSecurity,
			BeginTransition:          h.beginRuntimeThreadContextTransition,
			BeginScopeTransition:     h.beginRuntimeThreadScopeTransition,
			BeginWorkspaceTransition: h.beginRuntimeWorkspaceScopeTransition,
			BeginMetadataRead:        state.AcquireSecurityScopeRead,
			QuiesceThreadTurns:       h.quiesceRuntimeThreadForMutation,
		})
	}
	return h.threads
}

func (h *runtimeServerHandler) pendingGateIDsFromReplay(threadID string) ([]string, []string, error) {
	result, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		return nil, nil, err
	}
	approvals, inputs := threadapp.PendingGateIDsFromEventsV1(result.Events)
	return approvals, inputs, nil
}

func (h *runtimeServerHandler) handleThreadGoal(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ThreadHandlers{Service: h.runtimeThreadService()}.HandleGoal(w, r, threadID)
}

func (h *runtimeServerHandler) handleThreadTodos(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ThreadHandlers{Service: h.runtimeThreadService()}.HandleTodos(w, r, threadID)
}

func (h *runtimeServerHandler) handleThreadCompact(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ThreadHandlers{Service: h.runtimeThreadService()}.HandleCompact(w, r, threadID)
}

func (h *runtimeServerHandler) handleThreadReview(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.ReviewHandlers{Service: threadapp.NewReviewService(threadapp.ReviewDependencies{
		Turns:      h.runtimeControl(),
		Repository: h.store,
		Now:        time.Now,
	})}.HandleReview(w, r, threadID)
}
