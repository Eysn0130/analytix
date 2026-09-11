//go:build !analytix_prod

package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

const LiveLocalG3ProviderPrefix = "/v1/conformance/g3/provider"
const defaultRuntimeToken = "tok-1"

type LiveLocalG3ReplayStore interface {
	NoteG3UsageReplay()
	NoteG3ShapeReplay()
	NoteG3StreamReplay()
	NoteG3CacheReplay()
}

type LiveLocalG3ProviderHandler struct {
	runtimeToken string
	contract     G3ProviderConformanceContract
	store        LiveLocalG3ReplayStore
	enabled      bool
}

type liveLocalG3ProviderRequest struct {
	CaseID string `json:"caseId"`
}

func NewLiveLocalG3ProviderHandler(runtimeToken string, contract G3ProviderConformanceContract, store LiveLocalG3ReplayStore) *LiveLocalG3ProviderHandler {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = defaultRuntimeToken
	}
	return &LiveLocalG3ProviderHandler{
		runtimeToken: runtimeToken,
		contract:     contract,
		store:        store,
		enabled:      contract.ID != "",
	}
}

func (h *LiveLocalG3ProviderHandler) Handles(path string) bool {
	return h.enabled && strings.HasPrefix(path, LiveLocalG3ProviderPrefix)
}

func (h *LiveLocalG3ProviderHandler) Enabled() bool {
	return h != nil && h.enabled
}

func (h *LiveLocalG3ProviderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, h.runtimeToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}

	switch r.URL.Path {
	case LiveLocalG3ProviderPrefix + "/boundary":
		h.handleBoundary(w, r)
	case LiveLocalG3ProviderPrefix + "/usage":
		h.handleUsageReplay(w, r)
	case LiveLocalG3ProviderPrefix + "/request-shape":
		h.handleRequestShapeReplay(w, r)
	case LiveLocalG3ProviderPrefix + "/stream":
		h.handleStreamingReplay(w, r)
	case LiveLocalG3ProviderPrefix + "/cache-accounting":
		h.handleCacheAccountingReplay(w, r)
	case LiveLocalG3ProviderPrefix + "/cache-drift":
		h.handleCacheDriftReplay(w, r)
	case LiveLocalG3ProviderPrefix + "/cache-diagnostics":
		h.handleCacheDiagnosticsReplay(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h *LiveLocalG3ProviderHandler) handleBoundary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":             true,
		"externalNetworkUsed":     false,
		"providerCredentialsUsed": false,
		"apiKeyRead":              false,
		"liveProviderCall":        false,
		"productBoundary":         contracts.LiveLocalSidecarProductBoundary(),
	})
}

func (h *LiveLocalG3ProviderHandler) handleUsageReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, ok := decodeG3ProviderRequest(w, r)
	if !ok {
		return
	}
	item, found := h.usageCaseByID(request.CaseID)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "provider usage case not found"})
		return
	}
	parsed := ParsedProviderUsageFromRawPayload(item.EndpointFormat, item.BaseURL, item.ResponseBody)
	h.store.NoteG3UsageReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":          true,
		"externalNetworkUsed":  false,
		"apiKeyRead":           false,
		"caseId":               item.ID,
		"endpointFormat":       item.EndpointFormat,
		"baseUrl":              item.BaseURL,
		"model":                item.Model,
		"parsedUsage":          parsed,
		"expectedUsage":        item.ExpectedUsage,
		"matchesExpectedUsage": UsageSummaryMatchesExpected(parsed, item.ExpectedUsage),
	})
}

func (h *LiveLocalG3ProviderHandler) handleRequestShapeReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	request, ok := decodeG3ProviderRequest(w, r)
	if !ok {
		return
	}
	item, found := h.requestShapeCaseByID(request.CaseID)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "provider request-shape case not found"})
		return
	}
	derivedURL := DerivedG3ProviderRequestURL(item)
	requiredHeaders := DerivedG3ProviderRequiredHeaders(item)
	forbiddenHeaders := DerivedG3ProviderForbiddenHeaders(item)
	requiredBodyFields := DerivedG3ProviderRequiredBodyFields(item)
	forbiddenBodyFields := DerivedG3ProviderForbiddenBodyFields(item)
	toolShape := DerivedG3ProviderToolShape(item)
	h.store.NoteG3ShapeReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                true,
		"externalNetworkUsed":        false,
		"apiKeyRead":                 false,
		"caseId":                     item.ID,
		"endpointFormat":             item.EndpointFormat,
		"baseUrl":                    item.BaseURL,
		"model":                      item.Model,
		"derivedUrl":                 derivedURL,
		"expectedUrl":                item.ExpectedURL,
		"urlMatches":                 derivedURL == item.ExpectedURL,
		"requiredHeaders":            requiredHeaders,
		"forbiddenHeaders":           forbiddenHeaders,
		"headerShapeMatches":         sameStringSlice(requiredHeaders, item.RequiredHeaders) && sameStringSlice(forbiddenHeaders, item.ForbiddenHeaders),
		"requiredBodyFields":         requiredBodyFields,
		"forbiddenBodyFields":        forbiddenBodyFields,
		"bodyShapeMatches":           sameStringSlice(requiredBodyFields, item.RequiredBodyFields) && sameStringSlice(forbiddenBodyFields, item.ForbiddenBodyFields),
		"derivedToolShape":           toolShape,
		"expectedToolShape":          item.ExpectedToolShape,
		"toolShapeMatches":           toolShape == item.ExpectedToolShape,
		"customFullEndpointExactUrl": item.EndpointFormat == "custom_endpoint" && derivedURL == item.BaseURL && item.ExpectedURL == item.BaseURL,
	})
}

func (h *LiveLocalG3ProviderHandler) handleStreamingReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if threadID := r.URL.Query().Get("thread_id"); threadID != "" && threadID != h.contract.Streaming.ThreadID {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "provider stream thread not found"})
		return
	}
	if sinceSeq := r.URL.Query().Get("since_seq"); sinceSeq != "" && sinceSeq != intString(h.contract.Streaming.SinceSeq) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "since_seq does not match fixture"})
		return
	}
	h.store.NoteG3StreamReplay()
	writeSSE(w, http.StatusOK, h.contract.Streaming.SSEFrames)
}

func (h *LiveLocalG3ProviderHandler) handleCacheAccountingReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG3CacheReplay()
	writeJSON(w, http.StatusOK, BuildG3ProviderCacheAccounting(h.contract.ProviderUsageMatrix))
}

func (h *LiveLocalG3ProviderHandler) handleCacheDriftReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG3CacheReplay()
	writeJSON(w, http.StatusOK, BuildProviderDriftAttribution(h.contract.CacheDriftAttribution))
}

func (h *LiveLocalG3ProviderHandler) handleCacheDiagnosticsReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	diagnostics := h.contract.CacheDiagnostics
	h.store.NoteG3CacheReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                        true,
		"prefixHash":                         diagnostics.PrefixHash,
		"systemHash":                         diagnostics.SystemHash,
		"prefixItemsHash":                    diagnostics.PrefixItemsHash,
		"toolsHash":                          diagnostics.ToolsHash,
		"toolSchemaTokens":                   diagnostics.ToolSchemaTokens,
		"provider":                           diagnostics.Provider,
		"providerId":                         diagnostics.ProviderID,
		"endpointFormat":                     diagnostics.EndpointFormat,
		"model":                              diagnostics.Model,
		"sanitizedRequestUrl":                diagnostics.SanitizedRequestURL,
		"dynamicStateInStablePrefix":         false,
		"diagnosticsLeakForbiddenSubstring":  false,
		"forbiddenDiagnosticsSubstringCount": len(diagnostics.ForbiddenDiagnosticsSubstrings),
		"externalNetworkUsed":                false,
		"providerCredentialsUsed":            false,
		"apiKeyRead":                         false,
	})
}

func decodeG3ProviderRequest(w http.ResponseWriter, r *http.Request) (liveLocalG3ProviderRequest, bool) {
	body, ok := requestBody(w, r)
	if !ok {
		return liveLocalG3ProviderRequest{}, false
	}
	var request liveLocalG3ProviderRequest
	if err := json.Unmarshal(body, &request); err != nil || strings.TrimSpace(request.CaseID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "caseId is required"})
		return liveLocalG3ProviderRequest{}, false
	}
	return request, true
}

func (h *LiveLocalG3ProviderHandler) usageCaseByID(id string) (G3ProviderUsageCaseSummary, bool) {
	for _, item := range h.contract.ProviderUsageMatrix {
		if item.ID == id {
			return item, true
		}
	}
	return G3ProviderUsageCaseSummary{}, false
}

func (h *LiveLocalG3ProviderHandler) requestShapeCaseByID(id string) (G3ProviderRequestShapeCase, bool) {
	for _, item := range h.contract.RequestShapeMatrix {
		if item.ID == id {
			return item, true
		}
	}
	return G3ProviderRequestShapeCase{}, false
}

func intString(value int) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func authorized(r *http.Request, token string) bool {
	return r.Header.Get("Authorization") == "Bearer "+token
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requestBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	if r.Body == nil {
		return nil, true
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid request body"})
		return nil, false
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, true
	}
	return json.RawMessage(data), true
}

func writeSSE(w http.ResponseWriter, status int, frames []string) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(status)
	for _, frame := range frames {
		_, _ = w.Write([]byte(frame))
		_, _ = w.Write([]byte("\n\n"))
	}
}
