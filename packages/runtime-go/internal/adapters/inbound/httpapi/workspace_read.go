package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	readapp "analytix.local/runtime-go/internal/app/workspaceread"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const WorkspaceReadPath = "/v1/local-display/workspace-read"

type WorkspaceReadHandler struct{ Service *readapp.Service }

func (h WorkspaceReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4097))
	request, valid := decodeWorkspaceReadRequest(body)
	if err != nil || !valid {
		writeWorkspaceReadJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "code": "invalid_request"})
		return
	}
	snapshot, err := h.Service.Read(r.Context(), request)
	if err != nil {
		writeWorkspaceReadJSON(w, http.StatusForbidden, map[string]any{"ok": false, "code": "unavailable"})
		return
	}
	writeWorkspaceReadJSON(w, http.StatusOK, map[string]any{"ok": true, "snapshot": snapshot})
}

// This local-display endpoint owns a closed response contract, including its
// fixed error codes. Do not project it into the generic runtime error envelope.
func writeWorkspaceReadJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeWorkspaceReadRequest(body []byte) (readapp.Request, bool) {
	object, err := jsonstrict.DecodeObject(body, jsonstrict.Options{MaxBytes: 4096, MaxDepth: 2, MaxStringBytes: 256, MaxTokens: 16})
	if err != nil {
		return readapp.Request{}, false
	}
	action, actionOK := object["action"].(string)
	threadID, threadOK := object["threadId"].(string)
	if !actionOK || !threadOK {
		return readapp.Request{}, false
	}
	request := readapp.Request{Action: action, ThreadID: threadID}
	// Map lookup is case-sensitive, unlike encoding/json's struct matching.
	// Exact key counts reject unknown fields and even zero-valued extras.
	switch action {
	case "authorize":
		if len(object) != 2 {
			return readapp.Request{}, false
		}
	case "validate", "scan":
		var ok bool
		request.Binding, ok = object["binding"].(string)
		if !ok {
			return readapp.Request{}, false
		}
		if action == "validate" {
			if len(object) != 3 {
				return readapp.Request{}, false
			}
		} else {
			request.IncludePDF, ok = object["includePdf"].(bool)
			if !ok || len(object) != 4 {
				return readapp.Request{}, false
			}
		}
	default:
		return readapp.Request{}, false
	}
	return request, request.Valid()
}
