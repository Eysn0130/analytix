package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

const (
	PluginPackageHostPath     = "/v1/local-display/plugin-package-host"
	maxPluginHostRequestBytes = hostapp.MaxInputBytes + 4096
	// The desktop JSON contract must round-trip revisions without number rounding.
	maxPluginHostJSONRevision uint64 = (1 << 53) - 1
)

var pluginHostOperationPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

type PluginPackageHostService interface {
	List(context.Context) ([]hostapp.PackageView, error)
	SetDesiredState(context.Context, hostapp.SetDesiredStateRequest) (hostapp.PackageView, error)
	Invoke(context.Context, hostapp.InvokeRequest) (hostapp.InvokeResult, error)
}

// PluginPackageHostHandler must be registered behind LocalDisplayMuxV1, which
// owns runtime bearer authorization and the exact typed local-display header.
// This handler does not create a second token or registration authority. It uses
// the protected-local response writer rather than the public-turn projection.
// Route classification and Runtime assembly are owned by the composition layer.
type PluginPackageHostHandler struct{ Service PluginPackageHostService }

type pluginHostListRequest struct {
	Action string `json:"action"`
}
type pluginHostSetRequest struct {
	Action           string                      `json:"action"`
	PackageID        string                      `json:"packageId"`
	GenerationID     string                      `json:"generationId"`
	ExpectedRevision *uint64                     `json:"expectedRevision"`
	DesiredState     domainplugin.DesiredStateV1 `json:"desiredState"`
}
type pluginHostInvokeRequest struct {
	Action           string          `json:"action"`
	PackageID        string          `json:"packageId"`
	GenerationID     string          `json:"generationId"`
	ExpectedRevision *uint64         `json:"expectedRevision"`
	ContributionID   string          `json:"contributionId"`
	Operation        string          `json:"operation"`
	Input            json.RawMessage `json:"input"`
}

type pluginHostListResponse struct {
	OK       bool                  `json:"ok"`
	Packages []hostapp.PackageView `json:"packages"`
}
type pluginHostSetResponse struct {
	OK      bool                `json:"ok"`
	Package hostapp.PackageView `json:"package"`
}
type pluginHostInvokeResponse struct {
	OK     bool            `json:"ok"`
	Output json.RawMessage `json:"output"`
}
type pluginHostErrorResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Relist  bool   `json:"relist"`
}

func (h PluginPackageHostHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writePluginHostJSON(w, http.StatusMethodNotAllowed, pluginHostErrorResponse{Code: "invalid_request", Message: "Use POST for plugin operations."})
		return
	}
	if r.Body == nil || r.URL.RawQuery != "" {
		writePluginHostError(w, hostapp.ErrInvalid, false)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPluginHostRequestBytes+1))
	if err != nil {
		writePluginHostError(w, hostapp.ErrInvalid, false)
		return
	}
	fields, err := jsonstrict.DecodeRawObject(body, jsonstrict.Options{MaxBytes: maxPluginHostRequestBytes, MaxDepth: 18, MaxTokens: 33000, MaxStringBytes: hostapp.MaxInputBytes})
	if err != nil {
		writePluginHostError(w, hostapp.ErrInvalid, false)
		return
	}
	var action string
	if json.Unmarshal(fields["action"], &action) != nil {
		writePluginHostError(w, hostapp.ErrInvalid, false)
		return
	}
	// encoding/json alone accepts case-insensitive names and omitted/null numbers.
	// Check exact per-action keys as well as typed decode before touching Core.
	decode := func(value any, keys ...string) bool {
		if len(fields) != len(keys) {
			return false
		}
		for _, key := range keys {
			if _, ok := fields[key]; !ok {
				return false
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		return decoder.Decode(value) == nil
	}
	switch action {
	case "list":
		var request pluginHostListRequest
		if !decode(&request, "action") {
			writePluginHostError(w, hostapp.ErrInvalid, false)
			return
		}
		if h.Service == nil {
			writePluginHostError(w, hostapp.ErrUnavailable, false)
			return
		}
		packages, err := h.Service.List(r.Context())
		if err != nil {
			writePluginHostError(w, err, false)
			return
		}
		for _, pkg := range packages {
			if pkg.ActivationRevision > maxPluginHostJSONRevision {
				writePluginHostError(w, hostapp.ErrUnavailable, false)
				return
			}
		}
		if packages == nil {
			packages = []hostapp.PackageView{}
		}
		writePluginHostJSON(w, http.StatusOK, pluginHostListResponse{OK: true, Packages: packages})
	case "setDesiredState":
		var request pluginHostSetRequest
		if !decode(&request, "action", "packageId", "generationId", "expectedRevision", "desiredState") ||
			!domainpackage.ValidDevelopmentSourcePackageIDV1(request.PackageID) || !domainplugin.IsCanonicalSHA256V1(request.GenerationID) ||
			request.ExpectedRevision == nil || *request.ExpectedRevision >= maxPluginHostJSONRevision || !domainplugin.ValidDesiredStateV1(request.DesiredState) {
			writePluginHostError(w, hostapp.ErrInvalid, true)
			return
		}
		if h.Service == nil {
			writePluginHostError(w, hostapp.ErrUnavailable, true)
			return
		}
		view, err := h.Service.SetDesiredState(r.Context(), hostapp.SetDesiredStateRequest{PackageID: request.PackageID, GenerationID: request.GenerationID, ExpectedRevision: *request.ExpectedRevision, DesiredState: request.DesiredState})
		if err != nil {
			writePluginHostError(w, err, true)
			return
		}
		if view.ActivationRevision > maxPluginHostJSONRevision {
			writePluginHostError(w, hostapp.ErrPersistence, true)
			return
		}
		writePluginHostJSON(w, http.StatusOK, pluginHostSetResponse{OK: true, Package: view})
	case "invoke":
		var request pluginHostInvokeRequest
		if !decode(&request, "action", "packageId", "generationId", "expectedRevision", "contributionId", "operation", "input") ||
			!domainpackage.ValidDevelopmentSourcePackageIDV1(request.PackageID) || !domainplugin.IsCanonicalSHA256V1(request.GenerationID) ||
			request.ExpectedRevision == nil || *request.ExpectedRevision == 0 || *request.ExpectedRevision > maxPluginHostJSONRevision ||
			request.ContributionID != hostapp.WorkspaceEditorContributionID || !pluginHostOperationPattern.MatchString(request.Operation) ||
			jsonstrict.Validate(request.Input, jsonstrict.Options{RequireObject: true, MaxBytes: hostapp.MaxInputBytes, MaxDepth: 16, MaxTokens: 32768, MaxStringBytes: hostapp.MaxInputBytes}) != nil {
			writePluginHostError(w, hostapp.ErrInvalid, true)
			return
		}
		if h.Service == nil {
			writePluginHostError(w, hostapp.ErrUnavailable, true)
			return
		}
		result, err := h.Service.Invoke(r.Context(), hostapp.InvokeRequest{PackageID: request.PackageID, GenerationID: request.GenerationID, ExpectedRevision: *request.ExpectedRevision, ContributionID: request.ContributionID, Operation: request.Operation, Input: request.Input})
		if err != nil {
			writePluginHostError(w, err, true)
			return
		}
		if jsonstrict.Validate(result.Output, jsonstrict.Options{MaxBytes: hostapp.MaxOutputBytes, MaxDepth: 32, MaxTokens: 131072, MaxStringBytes: hostapp.MaxOutputBytes}) != nil {
			writePluginHostError(w, hostapp.ErrAdapterUnavailable, true)
			return
		}
		writePluginHostJSON(w, http.StatusOK, pluginHostInvokeResponse{OK: true, Output: result.Output})
	default:
		writePluginHostError(w, hostapp.ErrInvalid, false)
	}
}

func writePluginHostError(w http.ResponseWriter, err error, relist bool) {
	code, status := "unavailable", http.StatusServiceUnavailable
	switch {
	case errors.Is(err, hostapp.ErrInvalid):
		code, status = "invalid_request", http.StatusBadRequest
	case errors.Is(err, hostapp.ErrIdentity):
		code, status = "identity_invalid", http.StatusForbidden
	case errors.Is(err, hostapp.ErrNotFound):
		code, status = "package_not_found", http.StatusNotFound
	case errors.Is(err, hostapp.ErrConflict):
		code, status = "conflict", http.StatusConflict
	case errors.Is(err, hostapp.ErrDisabled):
		code, status = "disabled", http.StatusConflict
	case errors.Is(err, hostapp.ErrAdapterUnavailable):
		code, status = "adapter_unavailable", http.StatusServiceUnavailable
	case errors.Is(err, hostapp.ErrPersistence):
		code, status = "persistence_failure", http.StatusInternalServerError
	}
	message := "The plugin operation could not be completed."
	if relist {
		message = "The plugin result could not be confirmed. Refresh the plugin list and check the operation's state before taking further action."
	}
	writePluginHostJSON(w, status, pluginHostErrorResponse{Code: code, Message: message, Relist: relist})
}

func writePluginHostJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
