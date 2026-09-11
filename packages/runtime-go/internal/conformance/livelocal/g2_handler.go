//go:build !analytix_prod

package livelocal

import (
	"net/http"
	"strings"
)

type G2ShadowConfig struct {
	RuntimeToken string
	Routes       []G2RouteReplayCase
}

func NewG2ShadowHandler(config G2ShadowConfig) http.Handler {
	if strings.TrimSpace(config.RuntimeToken) == "" {
		config.RuntimeToken = DefaultRuntimeToken
	}
	routes := make(map[string]G2RouteReplayCase, len(config.Routes))
	for _, route := range config.Routes {
		routes[RouteKey(route.Method, route.Path, route.Body)] = route
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Authorized(r, config.RuntimeToken) {
			WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
			return
		}
		body, ok := RequestBody(w, r)
		if !ok {
			return
		}
		route, ok := routes[RouteKey(r.Method, r.URL.RequestURI(), body)]
		if !ok {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
			return
		}
		if route.ResponseKind == "sse" {
			WriteSSE(w, route.Response.Status, route.SSEFrames)
			return
		}
		WriteRawJSON(w, route.Response.Status, route.Response.Body)
	})
}
