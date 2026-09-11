package httpapi

import (
	"context"
	"net/http"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

type TurnActionControl interface {
	SteerTurn(context.Context, controlapp.SteerTurnRequest) (controlapp.ActionResult, error)
	InterruptTurn(context.Context, controlapp.InterruptTurnRequest) (controlapp.ActionResult, error)
}

type TurnActionHandlers struct {
	Control TurnActionControl
}

func (h TurnActionHandlers) HandleThreadTurnAction(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	parts := strings.SplitN(rest, "/turns/", 2)
	if len(parts) != 2 {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if h.Control == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_control_missing", "message": "runtime control missing"})
		return
	}
	threadID := parts[0]
	actionPath := parts[1]
	switch {
	case strings.HasSuffix(actionPath, "/steer"):
		h.handleSteer(w, r, threadID, strings.TrimSuffix(actionPath, "/steer"))
	case strings.HasSuffix(actionPath, "/interrupt"):
		h.handleInterrupt(w, r, threadID, strings.TrimSuffix(actionPath, "/interrupt"))
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h TurnActionHandlers) handleSteer(w http.ResponseWriter, r *http.Request, threadID string, turnID string) {
	body, err := readTurnActionBody(r)
	if err != nil {
		writeInvalidTurnActionBody(w, "invalid steer turn body")
		return
	}
	request, err := DecodeSteerTurnRequest(threadID, turnID, body)
	if err != nil {
		writeInvalidTurnActionBody(w, "invalid steer turn body")
		return
	}
	result, err := h.Control.SteerTurn(r.Context(), request)
	WriteControlActionResult(w, result, err)
}

func (h TurnActionHandlers) handleInterrupt(w http.ResponseWriter, r *http.Request, threadID string, turnID string) {
	body, err := readTurnActionBody(r)
	if err != nil {
		writeInvalidTurnActionBody(w, "invalid interrupt turn body")
		return
	}
	request, err := DecodeInterruptTurnRequest(threadID, turnID, body)
	if err != nil {
		writeInvalidTurnActionBody(w, "invalid interrupt turn body")
		return
	}
	result, err := h.Control.InterruptTurn(r.Context(), request)
	WriteControlActionResult(w, result, err)
}
