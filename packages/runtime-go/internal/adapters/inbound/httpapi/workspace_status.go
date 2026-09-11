package httpapi

import (
	"net/http"

	sessionapp "analytix.local/runtime-go/internal/app/session"
)

type WorkspaceStatusHandlers struct {
	Service sessionapp.WorkspaceStatusService
}

func (h WorkspaceStatusHandlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	WriteJSON(w, http.StatusOK, h.Service.Status(r.Context(), r.URL.Query().Get("path")))
}
