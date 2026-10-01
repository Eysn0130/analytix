package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	editing "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const BrowserSelectionPath = "/v1/local-display/browser-selection"

type BrowserSelectionHandler struct{ Service *editing.Service }

func (h BrowserSelectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fail := func() {
		WriteJSON(w, http.StatusConflict, map[string]any{"ok": false, "code": "selection_unavailable"})
	}
	if h.Service == nil {
		fail()
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 20001))
	if err != nil || jsonstrict.Validate(body, jsonstrict.Options{RequireObject: true, MaxBytes: 20000, MaxDepth: 3, MaxStringBytes: 16384}) != nil {
		fail()
		return
	}
	var req struct {
		Action  string                  `json:"action"`
		Capture *editing.BrowserCapture `json:"capture,omitempty"`
		Scope   *editing.BrowserScope   `json:"scope,omitempty"`
		Nonce   *string                 `json:"nonce,omitempty"`
		Current *bool                   `json:"current,omitempty"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil {
		fail()
		return
	}
	if req.Action == "capture" {
		if req.Capture == nil || req.Scope != nil || req.Nonce != nil || req.Current != nil {
			fail()
			return
		}
		v, err := h.Service.CaptureBrowser(r.Context(), *req.Capture)
		if err != nil {
			fail()
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": v})
		return
	}
	if req.Scope == nil || req.Capture != nil {
		fail()
		return
	}
	if req.Action != "answer" && (req.Nonce != nil || req.Current != nil) {
		fail()
		return
	}
	switch req.Action {
	case "next":
		nonce, err := h.Service.NextBrowserCheck(r.Context(), *req.Scope)
		if err != nil {
			fail()
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "nonce": nonce})
		return
	case "answer":
		if req.Nonce == nil || req.Current == nil {
			fail()
			return
		}
		err = h.Service.AnswerBrowserCheck(r.Context(), *req.Scope, *req.Nonce, *req.Current)
	case "revoke":
		err = h.Service.RevokeBrowser(r.Context(), *req.Scope)
	case "validate":
		err = h.Service.ValidateBrowserScope(r.Context(), *req.Scope)
	default:
		fail()
		return
	}
	if err != nil {
		fail()
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
