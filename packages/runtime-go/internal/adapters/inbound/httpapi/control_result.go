package httpapi

import (
	"errors"
	"net/http"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

func WriteControlActionResult(w http.ResponseWriter, result controlapp.ActionResult, err error) {
	if err != nil {
		if message, ok := safeControlValidationMessage(err); ok {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": message})
			return
		}
		if errors.Is(err, controlapp.ErrThreadNotFound) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if errors.Is(err, controlapp.ErrAttachmentNotAuthorized) {
			WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": controlapp.ErrAttachmentNotAuthorized.Error()})
			return
		}
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": controlapp.SafeInternalControlMessage})
		return
	}
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusOK
	}
	if result.Body == nil {
		result.Body = map[string]any{"ok": true}
	}
	if result.StatusCode >= http.StatusInternalServerError {
		code := "internal_error"
		if candidate, ok := result.Body["code"].(string); ok && (candidate == "internal_error" || candidate == "turn_failed") {
			code = candidate
		}
		result.Body = map[string]any{"code": code, "message": controlapp.SafeInternalControlMessage}
	}
	WriteJSON(w, result.StatusCode, result.Body)
}

func safeControlValidationMessage(err error) (string, bool) {
	for _, candidate := range []error{
		controlapp.ErrMissingThreadID,
		controlapp.ErrMissingTurnID,
		controlapp.ErrMissingPrompt,
		controlapp.ErrMissingApprovalID,
		controlapp.ErrInvalidDecision,
		controlapp.ErrMissingInputID,
		controlapp.ErrMissingSteerText,
	} {
		if errors.Is(err, candidate) {
			return candidate.Error(), true
		}
	}
	return "", false
}
