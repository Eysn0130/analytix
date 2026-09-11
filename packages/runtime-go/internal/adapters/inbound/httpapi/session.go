package httpapi

import (
	"errors"
	"net/http"
	"os"
	"strings"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type SessionService interface {
	Resume(sessionID string, request map[string]any) (map[string]any, error)
}

type SessionHandlers struct {
	Service SessionService
}

func (h SessionHandlers) HandlePath(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionResumeID(r.URL.Path)
	if !ok {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if !domainthread.IsCanonicalRecordID(sessionID) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "session not found"})
		return
	}
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "session_service_missing", "message": "session service missing"})
		return
	}
	request, ok := RequestMapBody(w, r, "invalid resume body")
	if !ok {
		return
	}
	response, err := h.Service.Resume(sessionID, request)
	if errors.Is(err, os.ErrNotExist) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "session not found"})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, response)
}

func sessionResumeID(path string) (string, bool) {
	rest := strings.TrimPrefix(path, "/v1/sessions/")
	if strings.HasSuffix(rest, "/resume-thread") {
		return strings.TrimSuffix(rest, "/resume-thread"), true
	}
	if strings.HasSuffix(rest, "/resume") {
		return strings.TrimSuffix(rest, "/resume"), true
	}
	return "", false
}
