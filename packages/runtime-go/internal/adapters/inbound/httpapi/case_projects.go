package httpapi

import (
	"errors"
	"net/http"
	"strings"

	threadapp "analytix.local/runtime-go/internal/app/thread"
)

// CaseProjectService is the application boundary required by the case-project
// HTTP adapter. The adapter owns routing and status-code projection only.
type CaseProjectService interface {
	ListCaseProjectSummaries(limit int) ([]map[string]any, string, error)
	ListCaseProjectThreads(caseProjectID string, limit int) ([]map[string]any, error)
	GetCaseProjectDetail(caseProjectID string, limit int) (map[string]any, error)
}

type CaseProjectHandlers struct {
	Service CaseProjectService
}

func (h CaseProjectHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "case_project_service_missing", "message": "case project service missing"})
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/case-projects")
	rest = strings.Trim(rest, "/")
	limit := IntQuery(r.URL.Query(), "limit")
	if rest == "" {
		projects, indexStatus, err := h.Service.ListCaseProjectSummaries(limit)
		if err != nil {
			writeCaseProjectInternalError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"caseProjects": projects, "indexStatus": indexStatus})
		return
	}
	if strings.HasSuffix(rest, "/threads") {
		caseProjectID := strings.Trim(strings.TrimSuffix(rest, "/threads"), "/")
		threads, err := h.Service.ListCaseProjectThreads(caseProjectID, limit)
		if err != nil {
			writeCaseProjectInternalError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"threads": threads})
		return
	}
	if strings.HasSuffix(rest, "/detail") {
		caseProjectID := strings.Trim(strings.TrimSuffix(rest, "/detail"), "/")
		detail, err := h.Service.GetCaseProjectDetail(caseProjectID, limit)
		if errors.Is(err, threadapp.ErrCaseProjectNotFound) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "case project not found"})
			return
		}
		if err != nil {
			writeCaseProjectInternalError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, detail)
		return
	}
	WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
}

func writeCaseProjectInternalError(w http.ResponseWriter, err error) {
	WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
}
