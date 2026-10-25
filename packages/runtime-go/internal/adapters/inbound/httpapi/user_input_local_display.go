package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	displayapp "analytix.local/runtime-go/internal/app/userinputdisplay"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

type UserInputLocalDisplayReaderV1 interface {
	Read(context.Context, port.SelectorV1) (displayapp.DisplayV1, error)
}

type UserInputLocalDisplayHandlerV1 struct{ Service UserInputLocalDisplayReaderV1 }

func (h UserInputLocalDisplayHandlerV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, domaincontinuation.MaxUserInputDraftBytesV1+4096))
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: domaincontinuation.MaxUserInputDraftBytesV1 + 4096, MaxDepth: 6, MaxTokens: 200_000, MaxStringBytes: domaincontinuation.MaxUserInputDraftBytesV1}) != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	if raw, command := fields["action"]; command {
		var action string
		if json.Unmarshal(raw, &action) == nil && action == "saveDraft" {
			h.saveDraft(w, r, body, fields)
			return
		}
		if json.Unmarshal(raw, &action) != nil || action != "discardLateReply" || domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: 4096, MaxDepth: 2, MaxTokens: 32, MaxStringBytes: 256}) != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
			return
		}
		h.discardLateReply(w, r, body)
		return
	}
	if domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: 4096, MaxDepth: 2, MaxTokens: 32, MaxStringBytes: 256}) != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	var selector port.SelectorV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&selector) != nil || decoder.Decode(&struct{}{}) != io.EOF || !port.ValidateSelectorV1(selector) {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	if h.Service == nil {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	display, err := h.Service.Read(r.Context(), selector)
	if err != nil || !displayapp.ValidateDisplayV1(display, selector) {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	encoded, err := json.Marshal(display)
	if err != nil || len(encoded) > port.MaxResponseBytesV1 || r.Context().Err() != nil {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func writeUserInputLocalDisplayUnavailableV1(w http.ResponseWriter) {
	WriteJSON(w, http.StatusNotFound, map[string]any{"code": "user_input_local_display_unavailable"})
}

type UserInputLocalDisplayDiscarderV1 interface {
	DiscardLateReply(context.Context, displayapp.DiscardLateReplyRequestV1) (displayapp.DiscardLateReplyResponseV1, error)
}

func (h UserInputLocalDisplayHandlerV1) discardLateReply(w http.ResponseWriter, r *http.Request, body []byte) {
	var request displayapp.DiscardLateReplyRequestV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF || !displayapp.ValidateDiscardLateReplyRequestV1(request) {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	service, ok := h.Service.(UserInputLocalDisplayDiscarderV1)
	if !ok || service == nil {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	response, err := service.DiscardLateReply(r.Context(), request)
	if errors.Is(err, displayapp.ErrDiscardConflictV1) {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "user_input_late_reply_conflict"})
		return
	}
	if err != nil || !displayapp.ValidateDiscardLateReplyResponseV1(response, request) || r.Context().Err() != nil {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	WriteJSON(w, http.StatusOK, response)
}

type UserInputLocalDisplayDraftSaverV1 interface {
	SaveDraft(context.Context, displayapp.SaveDraftRequestV1) (displayapp.SaveDraftResponseV1, error)
}

func (h UserInputLocalDisplayHandlerV1) saveDraft(w http.ResponseWriter, r *http.Request, body []byte, fields map[string]json.RawMessage) {
	var request displayapp.SaveDraftRequestV1
	if len(fields) != 8 {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	for _, key := range []string{"threadId", "turnId", "itemId", "requestId", "action", "revision", "expectedRevision", "draft"} {
		raw, found := fields[key]
		if !found || key != "expectedRevision" && bytes.Equal(raw, []byte("null")) {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
			return
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF || !displayapp.ValidateSaveDraftRequestV1(request) {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	service, ok := h.Service.(UserInputLocalDisplayDraftSaverV1)
	if !ok || service == nil {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	response, err := service.SaveDraft(r.Context(), request)
	switch {
	case errors.Is(err, continuationapp.ErrUserInputDraftConflictV1):
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "user_input_draft_conflict"})
		return
	case errors.Is(err, continuationapp.ErrUserInputDraftCapacityV1):
		WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"code": "user_input_draft_capacity"})
		return
	case errors.Is(err, continuationapp.ErrUserInputDraftInvalidV1):
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "user_input_local_display_invalid"})
		return
	}
	if err != nil || !displayapp.ValidateSaveDraftResponseV1(response, request) || r.Context().Err() != nil {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	body, err = json.Marshal(response)
	if err != nil || len(body) > port.MaxResponseBytesV1 {
		writeUserInputLocalDisplayUnavailableV1(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
