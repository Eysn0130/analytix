package server

import (
	"context"
	"net/http"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	inboundsse "analytix.local/runtime-go/internal/adapters/inbound/sse"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	apploop "analytix.local/runtime-go/internal/app/loop"
	sessionapp "analytix.local/runtime-go/internal/app/session"
	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func (h *runtimeServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	httpapi.RuntimeHandler{
		RuntimeToken: h.runtimeToken,
		Insecure:     h.insecure,
		Dispatcher:   runtimeHTTPDispatcher{handler: h},
	}.ServeHTTP(w, r)
}

type runtimeHTTPDispatcher struct {
	handler *runtimeServerHandler
}

func (d runtimeHTTPDispatcher) RuntimeInfo(w http.ResponseWriter, r *http.Request) {
	d.handler.handleRuntimeInfo(w, r)
}

func (d runtimeHTTPDispatcher) RuntimeTools(w http.ResponseWriter, r *http.Request) {
	d.handler.handleRuntimeTools(w, r)
}

func (d runtimeHTTPDispatcher) ToolExecutionObservation(w http.ResponseWriter, r *http.Request) {
	var service httpapi.SuccessfulToolExecutionObserverV1
	if d.handler != nil && d.handler.pendingWork != nil {
		service = d.handler.pendingWork
	}
	httpapi.ToolExecutionObservationHandlersV1{
		Service: service,
		ResolveMutationPath: func(workspaceRealPath string, requestedPath string) (string, bool) {
			return filestore.ResolveMutationIdentityPath(workspaceRealPath, requestedPath)
		},
		ResolveReadPath: func(workspaceRealPath string, requestedPath string) (string, bool) {
			resolved, ok := filestore.ResolveReadPathInfo(
				workspaceRealPath,
				requestedPath,
				d.handler.sandboxMode,
				d.handler.allowWriteRoots,
			)
			return resolved.Path, ok
		},
	}.Handle(w, r)
}

func (d runtimeHTTPDispatcher) RuntimeTaskJobs(w http.ResponseWriter, r *http.Request) {
	httpapi.TaskJobHandlers{Service: d.handler.runtimeTaskJobHTTPService()}.Handle(w, r)
}

func (d runtimeHTTPDispatcher) Usage(w http.ResponseWriter, r *http.Request) {
	d.handler.handleUsage(w, r)
}

func (d runtimeHTTPDispatcher) Skills(w http.ResponseWriter, r *http.Request) {
	d.handler.handleSkills(w, r)
}

func (d runtimeHTTPDispatcher) Attachments(w http.ResponseWriter, r *http.Request) {
	d.handler.attachmentHandlers().HandleCollection(w, r)
}

func (d runtimeHTTPDispatcher) AttachmentDiagnostics(w http.ResponseWriter, r *http.Request) {
	d.handler.attachmentHandlers().HandleDiagnostics(w, r)
}

func (d runtimeHTTPDispatcher) AttachmentPath(w http.ResponseWriter, r *http.Request) {
	d.handler.attachmentHandlers().HandlePath(w, r)
}

func (d runtimeHTTPDispatcher) Memory(w http.ResponseWriter, r *http.Request) {
	d.handler.memoryHandlers().HandleCollection(w, r)
}

func (d runtimeHTTPDispatcher) MemoryDiagnostics(w http.ResponseWriter, r *http.Request) {
	d.handler.memoryHandlers().HandleDiagnostics(w, r)
}

func (d runtimeHTTPDispatcher) MemoryRecordPath(w http.ResponseWriter, r *http.Request) {
	d.handler.memoryHandlers().HandleRecordPath(w, r)
}

func (d runtimeHTTPDispatcher) WorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	d.handler.workspaceStatusHandlers().HandleStatus(w, r)
}

func (d runtimeHTTPDispatcher) ProviderRegistry(w http.ResponseWriter, r *http.Request) {
	var service httpapi.ProviderRegistryService
	if d.handler != nil {
		service = d.handler.providerRegistry
	}
	httpapi.ProviderRegistryHandlers{Service: service}.Handle(w, r)
}

func (d runtimeHTTPDispatcher) MediaExecution(w http.ResponseWriter, r *http.Request) {
	var service httpapi.MediaExecutionService
	if d.handler != nil {
		service = d.handler.mediaExecution
	}
	httpapi.MediaExecutionHandlers{Service: service}.Handle(w, r)
}

func (d runtimeHTTPDispatcher) CaseProjects(w http.ResponseWriter, r *http.Request) {
	httpapi.CaseProjectHandlers{Service: d.handler.runtimeThreadService()}.Handle(w, r)
}

func (d runtimeHTTPDispatcher) Threads(w http.ResponseWriter, r *http.Request) {
	d.handler.handleThreads(w, r)
}

func (d runtimeHTTPDispatcher) ThreadPath(w http.ResponseWriter, r *http.Request) {
	d.handler.handleThreadPath(w, r)
}

func (d runtimeHTTPDispatcher) SessionPath(w http.ResponseWriter, r *http.Request) {
	httpapi.SessionHandlers{Service: d.handler.runtimeSessionService()}.HandlePath(w, r)
}

func (d runtimeHTTPDispatcher) ApprovalPath(w http.ResponseWriter, r *http.Request) {
	d.handler.handleApprovalPath(w, r)
}

func (d runtimeHTTPDispatcher) UserInputPath(w http.ResponseWriter, r *http.Request, prefix string) {
	d.handler.handleUserInputPath(w, r, prefix)
}

func (h *runtimeServerHandler) attachmentHandlers() httpapi.AttachmentHandlers {
	handlers := httpapi.AttachmentHandlers{Store: h.attachments}
	if h.attachmentAccess != nil && h.attachmentAccess.UploadAvailable() {
		handlers.Upload = func(ctx context.Context, body map[string]any) (map[string]any, error) {
			return h.attachmentAccess.Upload(ctx, body, h.attachments)
		}
		handlers.Authorize = h.attachmentAccess.AuthorizeContent
		handlers.Project = h.attachmentAccess.PublicMetadata
	}
	return handlers
}

func (h *runtimeServerHandler) attachmentDiagnostics() map[string]any {
	return h.attachments.Diagnostics()
}

func (h *runtimeServerHandler) memoryHandlers() httpapi.MemoryHandlers {
	return httpapi.MemoryHandlers{Store: h.memories}
}

func (h *runtimeServerHandler) memoryDiagnostics() map[string]any {
	return h.memories.Diagnostics()
}

func (h *runtimeServerHandler) workspaceStatusHandlers() httpapi.WorkspaceStatusHandlers {
	return httpapi.WorkspaceStatusHandlers{
		Service: sessionapp.WorkspaceStatusService{
			Probe: h.workspaceProbe,
		},
	}
}

func (h *runtimeServerHandler) runtimeSessionService() *sessionapp.Service {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions == nil {
		h.sessions = sessionapp.NewService(sessionapp.Dependencies{Repository: h.store})
	}
	return h.sessions
}

func (h *runtimeServerHandler) handleThreadEvents(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodGet {
		httpapi.MethodNotAllowed(w)
		return
	}
	if safeDurableID(threadID) != threadID {
		httpapi.WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if thread, err := h.store.GetThread(threadID); err != nil || thread == nil || stringField(thread, "id") != threadID {
		httpapi.WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	inboundsse.ThreadEventsHandler{
		Store: inboundsse.ThreadEventStore{
			HighestSeq: h.store.HighestSeq,
			LoadEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				result, err := h.store.LoadEventsSince(threadID, afterSeq)
				if err != nil {
					return nil, err
				}
				return result.Events, nil
			},
			LoadEventsSinceContext: func(ctx context.Context, threadID string, afterSeq int) ([]map[string]any, error) {
				result, err := h.store.LoadEventsSinceContext(ctx, threadID, afterSeq)
				if err != nil {
					return nil, err
				}
				return result.Events, nil
			},
			LoadPublicEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				result, err := h.store.LoadPublicEventsSince(threadID, afterSeq)
				if err != nil {
					return nil, err
				}
				return result.Events, nil
			},
			LoadPublicEventsSinceContext: func(ctx context.Context, threadID string, afterSeq int) ([]map[string]any, error) {
				result, err := h.store.LoadPublicEventsSinceContext(ctx, threadID, afterSeq)
				if err != nil {
					return nil, err
				}
				return result.Events, nil
			},
			SubscribeEvents:               h.store.SubscribeEvents,
			SealAcceptedFinalDelivery:     threadapp.NewAcceptedFinalDeliverySealer(r.Context(), h.publicProjector),
			ValidateAcceptedFinalDelivery: threadapp.NewAcceptedFinalDeliveryValidator(r.Context(), h.publicProjector),
			PreflightPublic:               threadapp.NewPublicProjectionPreflight(h.store, h.publicProjector),
			ProjectPublic:                 threadapp.NewPublicEventProjector(h.store, h.publicProjector),
		},
		HeartbeatInterval: h.heartbeatInterval,
		SanitizeError:     apploop.SanitizeProviderRetryMessage,
	}.ServeHTTP(w, r, threadID)
}
