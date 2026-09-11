package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	mediaexecutionapp "analytix.local/runtime-go/internal/app/mediaexecution"
)

const (
	MediaExecutionPathV1       = "/v1/runtime/_private/media-execution"
	mediaExecutionMaxBodyBytes = 24 << 20
)

type MediaExecutionService interface {
	Execute(context.Context, mediaexecutionapp.Request) (mediaexecutionapp.Result, error)
}

type MediaExecutionHandlers struct {
	Service MediaExecutionService
}

type mediaExecutionRequestV1 struct {
	SchemaVersion int                                `json:"schemaVersion"`
	Operation     mediaexecutionapp.Operation        `json:"operation"`
	Prompt        string                             `json:"prompt,omitempty"`
	Size          string                             `json:"size,omitempty"`
	AudioBase64   string                             `json:"audioBase64,omitempty"`
	MIMEType      string                             `json:"mimeType,omitempty"`
	Language      string                             `json:"language,omitempty"`
	Images        []mediaexecutionapp.ReferenceImage `json:"images,omitempty"`
	TimeoutMS     int                                `json:"timeoutMs,omitempty"`
}

func (handlers MediaExecutionHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	mediaExecutionNoStore(w)
	if r.URL.Path != MediaExecutionPathV1 || r.URL.RawQuery != "" || r.Method != http.MethodPost {
		writeMediaExecutionFailure(w, http.StatusNotFound, "not_found")
		return
	}
	if handlers.Service == nil {
		writeMediaExecutionFailure(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, mediaExecutionMaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > mediaExecutionMaxBodyBytes {
		clear(raw)
		writeMediaExecutionFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	defer clear(raw)
	var input mediaExecutionRequestV1
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || input.SchemaVersion != 1 {
		writeMediaExecutionFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.Execute(r.Context(), mediaexecutionapp.Request{
		Operation: input.Operation, Prompt: input.Prompt, Size: input.Size,
		AudioBase64: input.AudioBase64, MIMEType: input.MIMEType, Language: input.Language,
		Images: input.Images, TimeoutMS: input.TimeoutMS,
	})
	if err != nil {
		switch {
		case errors.Is(err, mediaexecutionapp.ErrInvalidRequest):
			writeMediaExecutionFailure(w, http.StatusBadRequest, "invalid_request")
		case errors.Is(err, mediaexecutionapp.ErrAuthorityChanged):
			writeMediaExecutionFailure(w, http.StatusConflict, "authority_changed")
		case errors.Is(err, mediaexecutionapp.ErrUnavailable):
			writeMediaExecutionFailure(w, http.StatusServiceUnavailable, "unavailable")
		default:
			writeMediaExecutionFailure(w, http.StatusBadGateway, "provider_failed")
		}
		return
	}
	if (len(result.Image) == 0) == (result.Transcript == "") {
		writeMediaExecutionFailure(w, http.StatusBadGateway, "provider_failed")
		return
	}
	response := map[string]any{"schemaVersion": 1, "status": "ok"}
	if len(result.Image) != 0 {
		response["imageBase64"] = base64.StdEncoding.EncodeToString(result.Image)
		response["mimeType"] = result.MIMEType
	}
	if result.Transcript != "" {
		response["transcript"] = result.Transcript
	}
	WriteJSON(w, http.StatusOK, response)
}

func mediaExecutionNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeMediaExecutionFailure(w http.ResponseWriter, status int, code string) {
	message := "media request failed"
	if code == "invalid_request" {
		message = "media request is invalid"
	} else if code == "unavailable" {
		message = "media provider is unavailable"
	} else if code == "authority_changed" {
		message = "media provider authority changed"
	}
	WriteJSON(w, status, map[string]any{
		"schemaVersion": 1,
		"status":        "error",
		"code":          code,
		"message":       message,
	})
}
