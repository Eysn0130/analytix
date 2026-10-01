package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	editing "analytix.local/runtime-go/internal/app/objectediting"
)

// Actual Core identity/thread/privacy/tool + private HTTP seam. Main's document
// proof is supplied synthetically here; Electron is a separate validation seam.
func TestBrowserPrivateTransportAndCoreModelProjection(t *testing.T) {
	f := newImageProductFixture(t)
	h := httpapi.LocalDisplayMuxV1{RuntimeToken: "browser-fixture-token", LocalDisplay: httpapi.LocalDisplayHandlerV1{BrowserSelection: httpapi.BrowserSelectionHandler{Service: f.service}}}
	call := func(value any, mode string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(value)
		r := httptest.NewRequest(http.MethodPost, httpapi.BrowserSelectionPath, bytes.NewReader(raw))
		if mode != "no-bearer" {
			r.Header.Set("Authorization", "Bearer browser-fixture-token")
		}
		if mode != "no-local" {
			r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	in := map[string]any{"action": "capture", "capture": editing.BrowserCapture{ThreadID: f.thread, DocumentID: strings.Repeat("a", 48), SelectionID: strings.Repeat("b", 48), Text: "Allowed selection alice@example.com"}}
	for _, mode := range []string{"no-bearer", "no-local"} {
		if w := call(in, mode); w.Code == 200 {
			t.Fatal("unauthorized capture", mode)
		}
	}
	w := call(in, "")
	var result struct {
		Scope editing.BrowserScope `json:"scope"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Scope.ScopeID == "" {
		t.Fatal("capture failed", w.Code, w.Body.String())
	}
	v := result.Scope
	wrong := v
	wrong.DocumentID = strings.Repeat("f", 48)
	if w = call(map[string]any{"action": "validate", "scope": wrong}, ""); w.Code == 200 {
		t.Fatal("altered document binding validated")
	}
	// An invalid full scope must not consume or revoke the legitimate scope.
	if !f.service.HasBrowserScopes() {
		t.Fatal("foreign validation retired the legitimate scope")
	}
	if raw, failed := f.model("foreign-thread", "native_selection_read", v.ScopeID); !failed || strings.Contains(raw, "Allowed selection") {
		t.Fatal("foreign thread")
	}
	if _, failed := f.model(f.thread, "native_selection_propose", v.ScopeID); !failed {
		t.Fatal("browser edits admitted")
	}
	for i := 0; i < 2; i++ {
		out := make(chan struct {
			raw    string
			failed bool
		}, 1)
		go func() {
			raw, failed := f.model(f.thread, "native_selection_read", v.ScopeID)
			out <- struct {
				raw    string
				failed bool
			}{raw, failed}
		}()
		w = call(map[string]any{"action": "next", "scope": v}, "")
		var challenge struct {
			Nonce string `json:"nonce"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &challenge)
		if w.Code != 200 || challenge.Nonce == "" {
			t.Fatal("challenge missing")
		}
		w = call(map[string]any{"action": "answer", "scope": v, "nonce": challenge.Nonce, "current": i == 0}, "")
		if w.Code != 200 {
			t.Fatal("answer failed")
		}
		got := <-out
		if i == 0 && (got.failed || !strings.Contains(got.raw, "Allowed selection") || strings.Contains(got.raw, "alice@example.com") || strings.Contains(got.raw, f.workspace)) {
			t.Fatal("unsafe/missing model projection", got.raw)
		}
		if i == 1 && (!got.failed || strings.Contains(got.raw, "Allowed selection")) {
			t.Fatal("stale model read accepted")
		}
	}
	// Private Browser proof never inherits diagnostic insecure mode.
	h.Insecure = true
	h.RuntimeToken = ""
	if w = call(in, "no-bearer"); w.Code != http.StatusUnauthorized {
		t.Fatal("insecure mode granted Main authority", w.Code)
	}
	if _, found, err := f.service.ReadBrowserScopeForModel(context.Background(), f.thread, v.ScopeID); found || err != nil {
		t.Fatal("revoked scope retained")
	}
}
