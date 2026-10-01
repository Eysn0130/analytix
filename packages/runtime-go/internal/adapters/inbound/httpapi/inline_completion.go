package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	apploop "analytix.local/runtime-go/internal/app/loop"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const InlineCompletionPath = "/v1/local-display/inline-completion"

type InlineCompletionService interface {
	Complete(context.Context, apploop.InlineCompletionRequest) (apploop.InlineCompletionResult, error)
}

type InlineCompletionHandler struct{ Service InlineCompletionService }

func (h InlineCompletionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// JSON escaping can expand each UTF-8 byte to six ASCII bytes. The decoded
	// prompt has its own smaller limit; the transport envelope remains bounded.
	const maxBody = 6*apploop.InlineCompletionMaxPromptBytes + 32*1024
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	request, valid := decodeInlineCompletionRequest(body, maxBody)
	write := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if err != nil || !valid {
		write(http.StatusBadRequest, map[string]any{"ok": false, "code": "invalid_request"})
		return
	}
	if h.Service == nil {
		write(http.StatusServiceUnavailable, map[string]any{"ok": false, "code": "unavailable"})
		return
	}
	result, err := h.Service.Complete(r.Context(), request)
	if err != nil {
		code, status := "unavailable", http.StatusServiceUnavailable
		switch {
		case errors.Is(err, apploop.ErrInlineCompletionInvalid):
			code, status = "invalid_request", http.StatusBadRequest
		case errors.Is(err, apploop.ErrInlineCompletionBusy):
			code, status = "busy", http.StatusConflict
		case errors.Is(err, apploop.ErrInlineCompletionCanceled), r.Context().Err() != nil:
			code, status = "canceled", http.StatusRequestTimeout
		}
		write(status, map[string]any{"ok": false, "code": code})
		return
	}
	write(http.StatusOK, map[string]any{"ok": true, "result": result})
}

func decodeInlineCompletionRequest(body []byte, maxBytes int) (apploop.InlineCompletionRequest, bool) {
	object, err := jsonstrict.DecodeObject(body, jsonstrict.Options{MaxBytes: maxBytes, MaxDepth: 3, MaxStringBytes: apploop.InlineCompletionMaxPromptBytes, MaxTokens: 32})
	var request apploop.InlineCompletionRequest
	if err != nil || len(object) != 5 {
		return request, false
	}
	for key, target := range map[string]*string{"threadId": &request.ThreadID, "requestId": &request.RequestID, "prompt": &request.Prompt, "model": &request.Model} {
		value, ok := object[key].(string)
		if !ok {
			return request, false
		}
		*target = value
	}
	doc, ok := object["document"].(map[string]any)
	if !ok {
		return request, false
	}
	if len(doc) == 1 {
		request.Document.Path, ok = doc["path"].(string)
		if !ok {
			return request, false
		}
	} else if len(doc) == 3 {
		for key, target := range map[string]*string{"sessionId": &request.Document.SessionID, "objectId": &request.Document.ObjectID, "baseRevision": &request.Document.BaseRevision} {
			value, ok := doc[key].(string)
			if !ok {
				return request, false
			}
			*target = value
		}
	} else {
		return request, false
	}
	return request, apploop.ValidateInlineCompletionRequest(request) == nil
}
