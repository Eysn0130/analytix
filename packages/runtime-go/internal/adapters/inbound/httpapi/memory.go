package httpapi

import (
	"errors"
	"net/http"
	"strings"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	domainmemory "analytix.local/runtime-go/internal/domain/memory"
)

type MemoryStore interface {
	List(includeDeleted bool, workspace string) ([]any, error)
	Create(body map[string]any) (map[string]any, error)
	Patch(id string, body map[string]any) (map[string]any, bool, error)
	Delete(id string) (map[string]any, bool, error)
	Diagnostics() map[string]any
}

type MemoryHandlers struct {
	Store MemoryStore
}

func (h MemoryHandlers) HandleCollection(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "memory_store_missing", "message": "memory store missing"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		includeDeleted := r.URL.Query().Get("include_deleted") == "true"
		workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
		memories, err := h.Store.List(includeDeleted, workspace)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"memories": memories})
	case http.MethodPost:
		body, ok := RequestMapBody(w, r, "invalid memory create body")
		if !ok {
			return
		}
		memory, err := h.Store.Create(body)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"memory": memory})
	default:
		MethodNotAllowed(w)
	}
}

func (h MemoryHandlers) HandleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Store == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "memory_store_missing", "message": "memory store missing"})
		return
	}
	writeRuntimeDiagnosticsJSON(w, runtimeinfoapp.ProjectPublicMemory(h.Store.Diagnostics(), true))
}

func (h MemoryHandlers) HandleRecordPath(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "memory_store_missing", "message": "memory store missing"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/memory/")
	if !domainmemory.IsCanonicalRecordID(id) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "memory not found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		body, ok := RequestMapBody(w, r, "invalid memory update body")
		if !ok {
			return
		}
		memory, found, err := h.Store.Patch(id, body)
		if err != nil {
			if errors.Is(err, domainmemory.ErrContentNotAdmissibleV1) || errors.Is(err, domainmemory.ErrMutationNotAdmissibleV1) {
				WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
				return
			}
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		if !found {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "memory not found"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"memory": memory})
	case http.MethodDelete:
		memory, found, err := h.Store.Delete(id)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		if !found {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "memory not found"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"memory": memory})
	default:
		MethodNotAllowed(w)
	}
}
