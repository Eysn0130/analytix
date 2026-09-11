package httpapi

import (
	"net/http"
	"strings"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
)

type RuntimeToolsRequest struct {
	Refresh bool
}

type RuntimeDiagnosticsService interface {
	RuntimeInfo() runtimeinfoapp.PublicRuntimeInfoV2
	RuntimeTools(RuntimeToolsRequest) runtimeinfoapp.PublicRuntimeToolsV2
}

type RuntimeDiagnosticsHandlers struct {
	Service RuntimeDiagnosticsService
}

func (h RuntimeDiagnosticsHandlers) HandleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_diagnostics_missing", "message": "runtime diagnostics missing"})
		return
	}
	writeRuntimeDiagnosticsJSON(w, h.Service.RuntimeInfo())
}

func (h RuntimeDiagnosticsHandlers) HandleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_diagnostics_missing", "message": "runtime diagnostics missing"})
		return
	}
	writeRuntimeDiagnosticsJSON(w, h.Service.RuntimeTools(RuntimeToolsRequest{Refresh: boolQuery(r, "refresh")}))
}

func writeRuntimeDiagnosticsJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	WriteJSON(w, http.StatusOK, body)
}

func boolQuery(r *http.Request, key string) bool {
	value := r.URL.Query().Get(key)
	return value == "1" || strings.EqualFold(value, "true")
}
