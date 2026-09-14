package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

const ObjectEditingPath = "/v1/local-display/object-editing"

type ObjectEditingHandler struct {
	Service             *editingapp.Service
	UnsupportedPlatform bool
}

func (h ObjectEditingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.UnsupportedPlatform {
		writeObjectEditingJSON(w, http.StatusNotImplemented, map[string]any{"ok": false, "code": "unsupported_platform", "message": "Versioned text persistence is not available on this platform."})
		return
	}
	if h.Service == nil {
		writeObjectEditingError(w, editingapp.ErrUnavailable, fileport.Receipt{})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (10<<20)+1))
	if err != nil || jsonstrict.Validate(body, jsonstrict.Options{RequireObject: true, MaxBytes: 10 << 20, MaxDepth: 4}) != nil {
		writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
		return
	}
	var envelope struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
		return
	}
	decode := func(value any) bool {
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		return d.Decode(value) == nil
	}
	sessionPattern := regexp.MustCompile(`^[a-f0-9]{48}$`)
	operationPattern := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)
	revisionPattern := regexp.MustCompile(`^[a-f0-9]{64}$`)
	switch envelope.Action {
	case "open":
		var request struct {
			Action    string `json:"action"`
			Workspace string `json:"workspace"`
			Path      string `json:"path"`
		}
		if !decode(&request) || !filepath.IsAbs(request.Workspace) || request.Path == "" {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		document, err := h.Service.Open(r.Context(), request.Workspace, request.Path)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "document": document})
	case "commit":
		var request struct {
			Action       string  `json:"action"`
			SessionID    string  `json:"sessionId"`
			OperationID  string  `json:"operationId"`
			BaseRevision string  `json:"baseRevision"`
			Content      *string `json:"content"`
		}
		if !decode(&request) || request.Content == nil || !sessionPattern.MatchString(request.SessionID) || !operationPattern.MatchString(request.OperationID) || !revisionPattern.MatchString(request.BaseRevision) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		receipt, err := h.Service.Commit(r.Context(), request.SessionID, request.OperationID, request.BaseRevision, *request.Content)
		if err != nil {
			writeObjectEditingError(w, err, receipt)
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "receipt": objectEditingReceipt(receipt)})
	case "close":
		var request struct {
			Action    string `json:"action"`
			SessionID string `json:"sessionId"`
		}
		if !decode(&request) || !sessionPattern.MatchString(request.SessionID) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		if err := h.Service.Close(r.Context(), request.SessionID); err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "closed": true})
	case "status":
		var request struct {
			Action      string `json:"action"`
			SessionID   string `json:"sessionId"`
			OperationID string `json:"operationId,omitempty"`
		}
		if !decode(&request) || !sessionPattern.MatchString(request.SessionID) || !operationPattern.MatchString(request.OperationID) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		receipt, err := h.Service.Status(r.Context(), request.SessionID, request.OperationID)
		if err != nil {
			writeObjectEditingError(w, err, receipt)
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "receipt": objectEditingReceipt(receipt)})
	default:
		writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
	}
}

func objectEditingReceipt(r fileport.Receipt) map[string]any {
	return map[string]any{"operationId": r.OperationID, "revision": r.Revision, "status": r.Status, "savedAt": r.SavedAt}
}

func writeObjectEditingError(w http.ResponseWriter, err error, receipt fileport.Receipt) {
	code, status := "unavailable", http.StatusServiceUnavailable
	switch {
	case errors.Is(err, fileport.ErrInvalidInput):
		code, status = "invalid_request", http.StatusBadRequest
	case errors.Is(err, editingapp.ErrSession):
		code, status = "session_invalid", http.StatusConflict
	case errors.Is(err, editingapp.ErrCapacity):
		code, status = "capacity", http.StatusConflict
	case errors.Is(err, fileport.ErrForbidden):
		code, status = "forbidden", http.StatusForbidden
	case errors.Is(err, fileport.ErrNotText):
		code, status = "not_text", http.StatusUnsupportedMediaType
	case errors.Is(err, fileport.ErrTooLarge):
		code, status = "too_large", http.StatusRequestEntityTooLarge
	case errors.Is(err, fileport.ErrConflict):
		code, status = "conflict", http.StatusConflict
	case errors.Is(err, fileport.ErrOperationMismatch):
		code, status = "operation_mismatch", http.StatusConflict
	case errors.Is(err, fileport.ErrOperationNotFound):
		code, status = "operation_not_found", http.StatusNotFound
	case errors.Is(err, fileport.ErrPersistence):
		code, status = "persistence_failure", http.StatusInternalServerError
	}
	response := map[string]any{"ok": false, "code": code, "message": "The object operation could not be completed."}
	if receipt.OperationID != "" {
		response["receipt"] = objectEditingReceipt(receipt)
	}
	writeObjectEditingJSON(w, status, response)
}

// The protected-local contract has a closed host-authored error vocabulary and
// hash/enum-only receipts. The ordinary public-turn projection intentionally
// removes these fields and must not be used for this typed private response.
func writeObjectEditingJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
