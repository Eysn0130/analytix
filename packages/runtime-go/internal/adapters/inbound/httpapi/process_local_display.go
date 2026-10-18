package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	processapp "analytix.local/runtime-go/internal/app/processdisplay"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	security "analytix.local/runtime-go/internal/domain/security"
)

const ProcessLocalDisplayReadPathV1 = "/v1/local-display/process/read"
const ProcessLocalDisplayTargetPathV1 = "/v1/local-display/process/target"

type ProcessLocalDisplayHandlerV1 struct{ Service *processapp.ServiceV1 }
type processDisplayRequestV1 struct {
	SchemaVersion    int      `json:"schemaVersion"`
	Action           string   `json:"action"`
	ViewerToken      string   `json:"viewerToken"`
	DocumentNonce    string   `json:"documentNonce,omitempty"`
	ThreadID         string   `json:"threadId,omitempty"`
	TurnID           string   `json:"turnId,omitempty"`
	AfterRevision    string   `json:"afterRevision,omitempty"`
	WaitMs           int      `json:"waitMs,omitempty"`
	ExpandedBlockIDs []string `json:"expandedBlockIds,omitempty"`
	BlockID          string   `json:"blockId,omitempty"`
	CaptureDigest    string   `json:"captureDigest,omitempty"`
	InputDigest      string   `json:"inputDigest,omitempty"`
	Role             string   `json:"role,omitempty"`
}

func (h ProcessLocalDisplayHandlerV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLocalDisplayRequestBytesV1))
	invalid := func() { WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "process_local_display_invalid"}) }
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: maxLocalDisplayRequestBytesV1, MaxDepth: 3, MaxTokens: 512, MaxStringBytes: 256}) != nil {
		invalid()
		return
	}
	var req processDisplayRequestV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || decoder.Decode(&struct{}{}) != io.EOF || req.SchemaVersion != 2 {
		invalid()
		return
	}
	var properties map[string]json.RawMessage
	if json.Unmarshal(body, &properties) != nil {
		invalid()
		return
	}
	// JSON null is not an optional scalar/array value in this closed wire.
	// encoding/json otherwise silently converts it (and null []string items)
	// to zero values, so validate the original property tokens as well.
	for _, value := range properties {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			invalid()
			return
		}
	}
	if value, present := properties["expandedBlockIds"]; present {
		var ids []json.RawMessage
		if json.Unmarshal(value, &ids) != nil {
			invalid()
			return
		}
		for _, id := range ids {
			if bytes.Equal(bytes.TrimSpace(id), []byte("null")) {
				invalid()
				return
			}
		}
	}
	allowed := map[string]bool{"schemaVersion": true, "action": true, "viewerToken": true}
	response := processapp.ResponseV1{Status: "unavailable"}
	switch {
	case r.URL.Path == ProcessLocalDisplayReadPathV1 && req.Action == "authorize":
		for _, key := range []string{"documentNonce", "threadId", "turnId"} {
			allowed[key] = true
		}
		if req.DocumentNonce == "" || req.ThreadID == "" || req.TurnID == "" {
			invalid()
			return
		}
	case r.URL.Path == ProcessLocalDisplayReadPathV1 && req.Action == "read":
		for _, key := range []string{"afterRevision", "waitMs", "expandedBlockIds"} {
			allowed[key] = true
		}
		if req.WaitMs < 0 || req.WaitMs > 25000 || len(req.ExpandedBlockIDs) > 128 {
			invalid()
			return
		}
	case r.URL.Path == ProcessLocalDisplayReadPathV1 && req.Action == "revoke":
	case r.URL.Path == ProcessLocalDisplayTargetPathV1 && req.Action == "open_capture":
		allowed["blockId"] = true
		allowed["captureDigest"] = true
		if req.BlockID == "" || !security.IsSHA256Hex(req.CaptureDigest) {
			invalid()
			return
		}
	case r.URL.Path == ProcessLocalDisplayTargetPathV1 && req.Action == "open_web_link":
		allowed["blockId"] = true
		allowed["inputDigest"] = true
		allowed["role"] = true
		if req.BlockID == "" || !security.IsSHA256Hex(req.InputDigest) || req.Role != "request" {
			invalid()
			return
		}
	default:
		invalid()
		return
	}
	if req.ViewerToken == "" {
		invalid()
		return
	}
	for key := range properties {
		if !allowed[key] {
			invalid()
			return
		}
	}
	var wire any = response
	if h.Service != nil {
		switch req.Action {
		case "authorize":
			response = h.Service.AuthorizeV1(r.Context(), req.ViewerToken, req.DocumentNonce, req.ThreadID, req.TurnID)
		case "read":
			response = h.Service.ReadV1(r.Context(), req.ViewerToken, req.AfterRevision, time.Duration(req.WaitMs)*time.Millisecond, req.ExpandedBlockIDs)
		case "revoke":
			response = h.Service.RevokeV1(req.ViewerToken)
		case "open_capture":
			response = h.Service.OpenCaptureV1(r.Context(), req.ViewerToken, req.BlockID, req.CaptureDigest)
		case "open_web_link":
			wire = h.Service.OpenWebLinkV1(r.Context(), req.ViewerToken, req.BlockID, req.InputDigest, req.Role)
		}
	}
	if req.Action != "open_web_link" {
		wire = response
	}
	encoded, err := json.Marshal(wire)
	if err != nil || len(encoded) > 256<<10 {
		WriteJSON(w, http.StatusOK, processapp.ResponseV1{Status: "unavailable"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}
func (h ProcessLocalDisplayHandlerV1) Shutdown(context.Context) error {
	if h.Service != nil {
		return h.Service.Close()
	}
	return nil
}
