package server

import (
	"context"
	"errors"
	"net/http"
	"os"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
)

func (h *runtimeServerHandler) handleThreadCheckpointPath(w http.ResponseWriter, r *http.Request, rest string) {
	httpapi.CheckpointHandlers{Service: runtimeCheckpointHTTPService{handler: h}}.HandleThreadCheckpointPath(w, r, rest)
}

type runtimeCheckpointHTTPService struct {
	handler *runtimeServerHandler
}

func (s runtimeCheckpointHTTPService) ThreadWorkspace(threadID string) (string, error) {
	if s.handler == nil || s.handler.store == nil {
		return "", os.ErrNotExist
	}
	thread, err := s.handler.store.GetThread(threadID)
	if errors.Is(err, os.ErrNotExist) || thread == nil {
		return "", os.ErrNotExist
	}
	if err != nil {
		return "", err
	}
	return stringField(thread, "workspace"), nil
}

func (s runtimeCheckpointHTTPService) RewindPlan(threadID string, checkpointID string, workspace string, scope string) map[string]any {
	return s.handler.checkpointPlan(threadID, checkpointID, workspace, scope)
}

func (s runtimeCheckpointHTTPService) RewindApply(ctx context.Context, threadID string, checkpointID string, workspace string, body map[string]any, plan map[string]any) map[string]any {
	return s.handler.checkpointApply(ctx, threadID, checkpointID, workspace, body, plan)
}
