//go:build !analytix_prod

package livelocal

import (
	"net/http"
	"strings"
	"time"
)

type G1ShadowConfig struct {
	RuntimeToken string
	StartedAt    string
}

func NewG1ShadowHandler(config G1ShadowConfig) http.Handler {
	if strings.TrimSpace(config.RuntimeToken) == "" {
		config.RuntimeToken = DefaultRuntimeToken
	}
	if strings.TrimSpace(config.StartedAt) == "" {
		config.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			MethodNotAllowed(w)
			return
		}
		WriteJSON(w, http.StatusOK, HealthResponse())
	})
	mux.HandleFunc("/v1/runtime/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			MethodNotAllowed(w)
			return
		}
		if !Authorized(r, config.RuntimeToken) {
			WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		WriteJSON(w, http.StatusOK, RuntimeInfoResponse(config.StartedAt))
	})
	mux.HandleFunc("/v1/runtime/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			MethodNotAllowed(w)
			return
		}
		if !Authorized(r, config.RuntimeToken) {
			WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		WriteJSON(w, http.StatusOK, RuntimeToolsResponse())
	})
	return mux
}
