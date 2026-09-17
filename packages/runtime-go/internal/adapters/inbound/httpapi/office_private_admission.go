package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"regexp"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const OfficePrivateAdmissionPath = "/v1/local-display/office-private-admission"

var officeAdmissionNonce = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[1-8][a-fA-F0-9]{3}-[89abAB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$`)
var officeAdmissionDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type OfficePrivateAssets interface {
	Current(context.Context) bool
	Root() string
	QualificationDigest() string
}

// Protected local display only. Core composition provides the witness from a
// verified executable resource seal; no request can supply a directory or flags.
type OfficePrivateAdmissionHandler struct{ Assets OfficePrivateAssets }

func (h OfficePrivateAdmissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	unavailable := func() {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"ok":false,"code":"unavailable"}`)
	}
	if r.Method != http.MethodPost || r.URL.Path != OfficePrivateAdmissionPath || r.URL.RawQuery != "" || r.URL.Fragment != "" || r.Body == nil {
		unavailable()
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1025))
	if err != nil || jsonstrict.Validate(raw, jsonstrict.Options{RequireObject: true, MaxBytes: 1024, MaxDepth: 2, MaxTokens: 8, MaxStringBytes: 64}) != nil {
		unavailable()
		return
	}
	var request struct {
		RequestID string `json:"requestId"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || !officeAdmissionNonce.MatchString(request.RequestID) || h.Assets == nil || !h.Assets.Current(r.Context()) {
		unavailable()
		return
	}
	root, digest := h.Assets.Root(), h.Assets.QualificationDigest()
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || len(root) > 32768 || !officeAdmissionDigest.MatchString(digest) || !h.Assets.Current(r.Context()) {
		unavailable()
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "requestId": request.RequestID, "root": root, "qualificationDigest": digest})
}
