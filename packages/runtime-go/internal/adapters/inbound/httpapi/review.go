package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	threadapp "analytix.local/runtime-go/internal/app/thread"
)

type ReviewService interface {
	StartReview(context.Context, string, threadapp.ReviewRequest) (map[string]any, error)
}

type ReviewHandlers struct {
	Service ReviewService
}

func (h ReviewHandlers) HandleReview(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "review_service_missing", "message": "review service missing"})
		return
	}
	body, ok := RequestMapBody(w, r, "invalid review body")
	if !ok {
		return
	}
	target, _ := body["target"].(map[string]any)
	response, err := h.Service.StartReview(r.Context(), threadID, threadapp.ReviewRequest{
		Target:     target,
		Model:      strings.TrimSpace(stringField(body, "model")),
		ProviderID: strings.TrimSpace(stringField(body, "providerId")),
	})
	if errors.Is(err, threadapp.ErrThreadNotFound) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "review_failed", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusAccepted, response)
}
