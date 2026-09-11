package httpapi

import "net/http"

type RuntimeDispatcher interface {
	RuntimeInfo(http.ResponseWriter, *http.Request)
	RuntimeTools(http.ResponseWriter, *http.Request)
	ToolExecutionObservation(http.ResponseWriter, *http.Request)
	RuntimeTaskJobs(http.ResponseWriter, *http.Request)
	Usage(http.ResponseWriter, *http.Request)
	Skills(http.ResponseWriter, *http.Request)
	Attachments(http.ResponseWriter, *http.Request)
	AttachmentDiagnostics(http.ResponseWriter, *http.Request)
	AttachmentPath(http.ResponseWriter, *http.Request)
	Memory(http.ResponseWriter, *http.Request)
	MemoryDiagnostics(http.ResponseWriter, *http.Request)
	MemoryRecordPath(http.ResponseWriter, *http.Request)
	WorkspaceStatus(http.ResponseWriter, *http.Request)
	ProviderRegistry(http.ResponseWriter, *http.Request)
	MediaExecution(http.ResponseWriter, *http.Request)
	CaseProjects(http.ResponseWriter, *http.Request)
	Threads(http.ResponseWriter, *http.Request)
	ThreadPath(http.ResponseWriter, *http.Request)
	SessionPath(http.ResponseWriter, *http.Request)
	ApprovalPath(http.ResponseWriter, *http.Request)
	UserInputPath(http.ResponseWriter, *http.Request, string)
}

type RuntimeHandler struct {
	RuntimeToken string
	Insecure     bool
	Dispatcher   RuntimeDispatcher
}

func (h RuntimeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	match := MatchRuntimeRoute(r.URL.Path)
	if match.Route == RouteMediaExecution {
		mediaExecutionNoStore(w)
	}
	if match.Route == RouteHealth {
		if r.Method != http.MethodGet {
			MethodNotAllowed(w)
			return
		}
		WriteJSON(w, http.StatusOK, HealthResponse())
		return
	}
	if match.Route == RouteToolExecutionObserve {
		toolExecutionObservationNoStoreV1(w)
		if !toolExecutionObservationAuthorizedV1(r, h.RuntimeToken) {
			WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
			return
		}
		if !toolExecutionObservationCanonicalRequestTargetV1(r) {
			writeToolExecutionObservationInvalidV1(w)
			return
		}
		if h.Dispatcher == nil {
			writeToolExecutionObservationUnavailableV1(w)
			return
		}
		h.Dispatcher.ToolExecutionObservation(w, r)
		return
	}
	if !Authorized(r, h.RuntimeToken, h.Insecure) {
		if match.Route == RouteProviderRegistry {
			writeProviderRegistryFailure(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	if h.Dispatcher == nil {
		if match.Route == RouteProviderRegistry {
			writeProviderRegistryFailure(w, http.StatusServiceUnavailable, "persistence_failure")
			return
		}
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_dispatcher_missing", "message": "runtime dispatcher missing"})
		return
	}
	switch match.Route {
	case RouteRuntimeInfo:
		h.Dispatcher.RuntimeInfo(w, r)
	case RouteRuntimeTools:
		h.Dispatcher.RuntimeTools(w, r)
	case RouteRuntimeTaskJobs:
		h.Dispatcher.RuntimeTaskJobs(w, r)
	case RouteUsage:
		h.Dispatcher.Usage(w, r)
	case RouteSkills:
		h.Dispatcher.Skills(w, r)
	case RouteAttachments:
		h.Dispatcher.Attachments(w, r)
	case RouteAttachmentDiagnostics:
		h.Dispatcher.AttachmentDiagnostics(w, r)
	case RouteAttachmentPath:
		h.Dispatcher.AttachmentPath(w, r)
	case RouteMemory:
		h.Dispatcher.Memory(w, r)
	case RouteMemoryDiagnostics:
		h.Dispatcher.MemoryDiagnostics(w, r)
	case RouteMemoryRecordPath:
		h.Dispatcher.MemoryRecordPath(w, r)
	case RouteWorkspaceStatus:
		h.Dispatcher.WorkspaceStatus(w, r)
	case RouteProviderRegistry:
		h.Dispatcher.ProviderRegistry(w, r)
	case RouteMediaExecution:
		h.Dispatcher.MediaExecution(w, r)
	case RouteCaseProjects:
		h.Dispatcher.CaseProjects(w, r)
	case RouteThreads:
		h.Dispatcher.Threads(w, r)
	case RouteThreadPath:
		h.Dispatcher.ThreadPath(w, r)
	case RouteSessionPath:
		h.Dispatcher.SessionPath(w, r)
	case RouteApprovalPath:
		h.Dispatcher.ApprovalPath(w, r)
	case RouteUserInputsPath, RouteUserInputPath:
		h.Dispatcher.UserInputPath(w, r, match.UserInputPrefix)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}
