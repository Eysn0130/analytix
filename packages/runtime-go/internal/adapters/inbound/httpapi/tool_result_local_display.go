package httpapi

import (
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

type ToolResultLocalDisplayHandlerV1 struct{ Service *snapshotapp.Service }

func (h ToolResultLocalDisplayHandlerV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: 4096, MaxDepth: 2, MaxTokens: 32, MaxStringBytes: 256}) != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "tool_result_local_display_invalid"})
		return
	}
	var selector snapshotport.SelectorV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&selector) != nil || decoder.Decode(&struct{}{}) != io.EOF || !snapshotapp.ValidateSelectorV1(selector) {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "tool_result_local_display_invalid"})
		return
	}
	display, err := h.Service.Read(r.Context(), selector)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "tool_result_local_display_unavailable"})
		return
	}
	WriteJSON(w, http.StatusOK, display)
}
func (h ToolResultLocalDisplayHandlerV1) Shutdown(context.Context) error {
	if h.Service == nil || h.Service.Store == nil {
		return nil
	}
	if owner, ok := h.Service.Store.(interface{ Close() error }); ok {
		return owner.Close()
	}
	return nil
}
