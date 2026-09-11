package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

const DefaultRuntimeToken = "tok-1"

func Authorized(r *http.Request, token string, insecure ...bool) bool {
	if len(insecure) > 0 && insecure[0] {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+token
}

func MethodNotAllowed(w http.ResponseWriter) {
	WriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	if status >= http.StatusBadRequest {
		body = domainfailure.ProjectHTTPFailure(status, body)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func IntQuery(values url.Values, key string) int {
	value, err := strconv.Atoi(values.Get(key))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func HealthResponse() map[string]any {
	return map[string]any{
		"status":  "ok",
		"service": "analytix",
		"mode":    "serve",
	}
}

func RuntimeInfoResponse(startedAt string) runtimeinfoapp.PublicRuntimeInfoV2 {
	return runtimeinfoapp.RuntimeInfoResponse(startedAt)
}

func RuntimeCapabilitiesResponse() map[string]any {
	return runtimeinfoapp.RuntimeCapabilitiesResponse()
}

func RuntimeServerContractCapabilities() map[string]any {
	return runtimeinfoapp.RuntimeServerContractCapabilities()
}

func RuntimeToolsResponse() runtimeinfoapp.PublicRuntimeToolsV2 {
	return runtimeinfoapp.RuntimeToolsResponse()
}

func AvailableState() map[string]any {
	return runtimeinfoapp.AvailableState()
}

func UnavailableState(reason string) map[string]any {
	return runtimeinfoapp.UnavailableState(reason)
}

func DisabledState(reason string) map[string]any {
	return runtimeinfoapp.DisabledState(reason)
}
