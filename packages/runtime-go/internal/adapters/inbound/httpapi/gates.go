package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

type GateControl interface {
	ApproveTool(context.Context, controlapp.ApprovalDecision) (controlapp.ActionResult, error)
	RespondUserInput(context.Context, controlapp.UserInputResponse) (controlapp.ActionResult, error)
}

type GateHandlers struct {
	Control GateControl
}

func (h GateHandlers) HandleApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	var request struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if len(body) == 0 || json.Unmarshal(body, &request) != nil || (request.Decision != "allow" && request.Decision != "deny") {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid approval body"})
		return
	}
	if h.Control == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_control_missing", "message": "runtime control missing"})
		return
	}
	result, err := h.Control.ApproveTool(r.Context(), controlapp.ApprovalDecision{
		ApprovalID: strings.TrimPrefix(r.URL.Path, "/v1/approvals/"),
		Decision:   request.Decision,
		Reason:     request.Reason,
	})
	WriteControlActionResult(w, result, err)
}

func (h GateHandlers) HandleUserInput(w http.ResponseWriter, r *http.Request, prefix string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	var request struct {
		Answers   []map[string]string `json:"answers"`
		Cancelled bool                `json:"cancelled"`
	}
	if len(body) == 0 || json.Unmarshal(body, &request) != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid user input body"})
		return
	}
	if h.Control == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_control_missing", "message": "runtime control missing"})
		return
	}
	result, err := h.Control.RespondUserInput(r.Context(), controlapp.UserInputResponse{
		InputID:   strings.TrimPrefix(r.URL.Path, prefix),
		Answers:   request.Answers,
		Cancelled: request.Cancelled,
	})
	WriteControlActionResult(w, result, err)
}
