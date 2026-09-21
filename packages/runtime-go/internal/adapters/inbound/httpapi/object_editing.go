package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

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
	if err != nil || jsonstrict.Validate(body, jsonstrict.Options{RequireObject: true, MaxBytes: 10 << 20, MaxDepth: 4, MaxStringBytes: fileport.MaxTextBytes}) != nil {
		writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
		return
	}
	var envelope struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(body, &envelope) != nil || !validObjectEditingRequest(body, envelope.Action) {
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
	case "image-open":
		var request struct {
			Action   string `json:"action"`
			ThreadID string `json:"threadId"`
			Path     string `json:"path"`
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		image, err := h.Service.OpenImage(r.Context(), request.ThreadID, request.Path)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "image": image})
	case "image-annotation-read":
		var request struct {
			Action    string `json:"action"`
			SessionID string `json:"sessionId"`
			ThreadID  string `json:"threadId"`
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.ReadImageAnnotation(r.Context(), request.SessionID, request.ThreadID)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "annotation": value})
	case "image-annotation-write":
		var request struct {
			Action string `json:"action"`
			editingapp.ImageAnnotationWrite
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.WriteImageAnnotation(r.Context(), request.ImageAnnotationWrite)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "annotation": value})
	case "image-scope-capture":
		var request struct {
			Action string `json:"action"`
			editingapp.ImageScopeCapture
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.CaptureImageScope(r.Context(), request.ImageScopeCapture)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": value})
	case "image-scope-read", "image-scope-revoke":
		var request struct {
			Action string `json:"action"`
			editingapp.ImageScopeBinding
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		if envelope.Action == "image-scope-revoke" {
			if err := h.Service.RevokeImageScope(r.Context(), request.ImageScopeBinding); err != nil {
				writeObjectEditingError(w, err, fileport.Receipt{})
				return
			}
			writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": true})
			return
		}
		value, err := h.Service.ReadImageScope(r.Context(), request.ImageScopeBinding)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": value})
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

	case "export-snapshot":
		var request struct {
			Action string `json:"action"`
			editingapp.ExportSnapshotInput
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.ReadExportSnapshot(r.Context(), request.ExportSnapshotInput)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "snapshot": value})
	case "draft-update":
		var request struct {
			Action string `json:"action"`
			editingapp.UpdateDraftInput
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.UpdateDraft(r.Context(), request.UpdateDraftInput)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "draft": value})
	case "draft-read":
		var request struct {
			Action    string `json:"action"`
			SessionID string `json:"sessionId"`
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.ReadDraft(r.Context(), request.SessionID)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "draft": value})
	case "scope-capture":
		var request struct {
			Action string `json:"action"`
			editingapp.CaptureScopeInput
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.CaptureScope(r.Context(), request.CaptureScopeInput)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": value})
	case "scope-read", "scope-revoke":
		var request struct {
			Action string `json:"action"`
			editingapp.ScopeBinding
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		if request.Action == "scope-revoke" {
			if err := h.Service.RevokeScope(r.Context(), request.ScopeBinding); err != nil {
				writeObjectEditingError(w, err, fileport.Receipt{})
				return
			}
			writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": true})
			return
		}
		value, err := h.Service.ReadScope(r.Context(), request.ScopeBinding)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": value})
	case "proposal-create":
		var request struct {
			Action string `json:"action"`
			editingapp.ProposeInput
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		value, err := h.Service.Propose(r.Context(), request.ProposeInput)
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "proposal": value})
	case "proposal-accept", "proposal-reject":
		var request struct {
			Action string `json:"action"`
			editingapp.DecisionInput
		}
		if !decode(&request) {
			writeObjectEditingError(w, fileport.ErrInvalidInput, fileport.Receipt{})
			return
		}
		var value editingapp.Decision
		var err error
		if request.Action == "proposal-accept" {
			value, err = h.Service.Accept(r.Context(), request.DecisionInput)
		} else {
			value, err = h.Service.Reject(r.Context(), request.DecisionInput)
		}
		if err != nil {
			writeObjectEditingError(w, err, fileport.Receipt{})
			return
		}
		writeObjectEditingJSON(w, http.StatusOK, map[string]any{"ok": true, "decision": value})
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
	case errors.Is(err, editingapp.ErrDraftStale):
		code, status = "draft_stale", http.StatusConflict
	case errors.Is(err, editingapp.ErrScope):
		code, status = "scope_invalid", http.StatusConflict
	case errors.Is(err, editingapp.ErrProposal):
		code, status = "proposal_invalid", http.StatusConflict
	case errors.Is(err, editingapp.ErrProjection):
		code, status = "projection_unavailable", http.StatusServiceUnavailable
	case errors.Is(err, editingapp.ErrProtected):
		code, status = "protected_span_invalid", http.StatusBadRequest
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

// Required exact keys are checked before Go's case-insensitive struct decoder.
// jsonstrict already rejects duplicate keys, invalid Unicode and excess depth.
func objectEditingExactFields(raw json.RawMessage, keys string) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, false
	}
	expected := strings.Fields(keys)
	if len(fields) != len(expected) {
		return nil, false
	}
	for _, key := range expected {
		value, ok := fields[key]
		if !ok || key != "region" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, false
		}
	}
	return fields, true
}

var objectEditingKeys = map[string]string{
	"image-open":             "action threadId path",
	"image-annotation-read":  "action sessionId threadId",
	"image-annotation-write": "action sessionId threadId sourceRevision expectedAnnotationRevision region note",
	"image-scope-capture":    "action sessionId threadId sourceRevision annotationRevision",
	"image-scope-read":       "action sessionId threadId scopeId",
	"image-scope-revoke":     "action sessionId threadId scopeId",
	"open":                   "action workspace path",
	"export-snapshot":        "action sessionId objectId threadId baseRevision draftVersion",
	"commit":                 "action sessionId operationId baseRevision content",
	"status":                 "action sessionId operationId", "close": "action sessionId",
	"draft-update": "action sessionId baseRevision expectedVersion content", "draft-read": "action sessionId",
	"scope-capture":   "action sessionId draftVersion threadId purpose range",
	"scope-read":      "action sessionId scopeId threadId purpose draftVersion",
	"scope-revoke":    "action sessionId scopeId threadId purpose draftVersion",
	"proposal-create": "action sessionId scopeId threadId purpose draftVersion operationId parts",
	"proposal-accept": "action sessionId scopeId threadId purpose draftVersion operationId proposalId",
	"proposal-reject": "action sessionId scopeId threadId purpose draftVersion operationId proposalId",
}
var objectEditingToken = regexp.MustCompile(`^[a-f0-9]{48}$`)
var objectEditingHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var objectEditingOperation = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)
var objectEditingThread = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var objectEditingProtected = regexp.MustCompile(`^protected_[a-f0-9]{48}$`)

func validObjectEditingRequest(body []byte, action string) bool {
	keys, ok := objectEditingKeys[action]
	if !ok {
		return false
	}
	fields, ok := objectEditingExactFields(body, keys)
	if !ok {
		return false
	}
	for key, raw := range fields {
		if key == "region" {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			if _, ok := objectEditingExactFields(raw, "x y width height"); !ok {
				return false
			}
			var region fileport.ImageRegion
			if json.Unmarshal(raw, &region) != nil || !fileport.ValidImageAnnotationWrite("", strings.Repeat("0", 64), "", &region) {
				return false
			}
			continue
		}
		if key == "range" {
			if _, ok := objectEditingExactFields(raw, "start end"); !ok {
				return false
			}
			var value editingapp.UTF16Range
			if json.Unmarshal(raw, &value) != nil || value.Start < 0 || value.End < value.Start || value.End > fileport.MaxTextBytes {
				return false
			}
			continue
		}
		if key == "parts" {
			var parts []json.RawMessage
			if json.Unmarshal(raw, &parts) != nil || parts == nil || len(parts) > editingapp.MaxPatchParts {
				return false
			}
			total := 0
			for _, part := range parts {
				var value editingapp.PatchPart
				if json.Unmarshal(part, &value) != nil {
					return false
				}
				switch value.Kind {
				case "literal":
					if _, ok := objectEditingExactFields(part, "kind text"); !ok || value.Text == "" {
						return false
					}
					total += len(value.Text)
				case "protected":
					if _, ok := objectEditingExactFields(part, "kind protectedRef"); !ok || !objectEditingProtected.MatchString(value.ProtectedRef) {
						return false
					}
				default:
					return false
				}
				if total > editingapp.MaxPatchBytes {
					return false
				}
			}
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return false
		}
		switch key {
		case "sessionId", "scopeId", "proposalId", "draftVersion":
			if !objectEditingToken.MatchString(value) {
				return false
			}
		case "expectedVersion":
			if value != "" && !objectEditingToken.MatchString(value) {
				return false
			}
		case "baseRevision", "objectId", "sourceRevision", "annotationRevision":
			if !objectEditingHash.MatchString(value) {
				return false
			}
		case "expectedAnnotationRevision":
			if value != "" && !objectEditingHash.MatchString(value) {
				return false
			}
		case "note":
			if !fileport.ValidAnnotationWrite("", value, strings.Repeat("0", 64)) {
				return false
			}
		case "operationId":
			if !objectEditingOperation.MatchString(value) {
				return false
			}
		case "threadId":
			if !objectEditingThread.MatchString(value) {
				return false
			}
		case "purpose":
			if value != "edit" && value != "discuss" {
				return false
			}
		case "workspace":
			if !filepath.IsAbs(value) || strings.ContainsRune(value, 0) {
				return false
			}
		case "path":
			if value == "" || strings.ContainsRune(value, 0) || action == "image-open" && (len(value) > 4096 || strings.TrimSpace(value) == "") {
				return false
			}
		case "content":
			if len(value) > fileport.MaxTextBytes {
				return false
			}
		}
	}
	return true
}
