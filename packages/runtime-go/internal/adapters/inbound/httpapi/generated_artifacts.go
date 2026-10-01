package httpapi

import (
	"context"
	"net/http"

	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
)

const GeneratedArtifactPath = "/v1/local-display/generated-artifact"

type GeneratedArtifactHandler struct {
	Resolve func(context.Context, string, string) (generationapp.Resolved, error)
}

func (handler GeneratedArtifactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ThreadID   string `json:"threadId"`
		ArtifactID string `json:"artifactId"`
	}
	if decodeLocalDisplayRequestV1(r, &request) != nil {
		writeLocalDisplayInvalidV1(w)
		return
	}
	if handler.Resolve == nil {
		writeLocalDisplayUnavailableV1(w)
		return
	}
	result, err := handler.Resolve(r.Context(), request.ThreadID, request.ArtifactID)
	if err != nil {
		WriteJSON(w, http.StatusConflict, map[string]any{"ok": false, "code": "artifact_unavailable"})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "artifact": result})
}
