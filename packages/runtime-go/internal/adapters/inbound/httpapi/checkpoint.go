package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
)

type CheckpointService interface {
	ThreadWorkspace(threadID string) (string, error)
	RewindPlan(threadID string, checkpointID string, workspace string, scope string) map[string]any
	RewindApply(context.Context, string, string, string, map[string]any, map[string]any) map[string]any
}

type CheckpointHandlers struct {
	Service CheckpointService
}

func (h CheckpointHandlers) HandleThreadCheckpointPath(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "checkpoint_service_missing", "message": "checkpoint service missing"})
		return
	}
	route, ok := parseCheckpointPath(rest)
	if !ok {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	body, ok := RequestMapBody(w, r, "invalid checkpoint body")
	if !ok {
		return
	}
	workspace, err := h.Service.ThreadWorkspace(route.ThreadID)
	if errors.Is(err, os.ErrNotExist) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	if route.Action == "plan" {
		scope := stringField(body, "scope")
		if scope == "" {
			scope = "combined"
		}
		if !checkpointapp.ValidScope(scope) {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_checkpoint_scope", "message": "checkpoint scope must be code, conversation, or combined"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"plan": h.Service.RewindPlan(route.ThreadID, route.CheckpointID, workspace, scope)})
		return
	}
	plan, _ := body["plan"].(map[string]any)
	WriteJSON(w, http.StatusOK, map[string]any{"apply": h.Service.RewindApply(r.Context(), route.ThreadID, route.CheckpointID, workspace, body, plan)})
}

type checkpointRoute struct {
	ThreadID     string
	CheckpointID string
	Action       string
}

func parseCheckpointPath(rest string) (checkpointRoute, bool) {
	parts := strings.SplitN(rest, "/checkpoints/", 2)
	if len(parts) != 2 {
		return checkpointRoute{}, false
	}
	threadID := parts[0]
	checkpointPath := parts[1]
	if strings.HasSuffix(checkpointPath, "/rewind-plan") {
		return checkpointRoute{
			ThreadID:     threadID,
			CheckpointID: strings.TrimSuffix(checkpointPath, "/rewind-plan"),
			Action:       "plan",
		}, true
	}
	if strings.HasSuffix(checkpointPath, "/rewind-apply") {
		return checkpointRoute{
			ThreadID:     threadID,
			CheckpointID: strings.TrimSuffix(checkpointPath, "/rewind-apply"),
			Action:       "apply",
		}, true
	}
	return checkpointRoute{}, false
}
