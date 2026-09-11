//go:build !analytix_prod

package livelocal

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	contracts "analytix.local/runtime-go/internal/contracts"
	"analytix.local/runtime-go/internal/protocol"
	"analytix.local/runtime-go/internal/server"
)

const DefaultRuntimeToken = httpapi.DefaultRuntimeToken

type G2RouteReplayCase = protocol.G2RouteReplayCase
type G2RouteResponse = protocol.G2RouteResponse
type DurableEventSessionStore = server.DurableEventSessionStore
type DurableJSONLDiagnostic = server.DurableJSONLDiagnostic
type DurableLoadEventsResult = server.DurableLoadEventsResult

var errDurableTurnNotFound = threadapp.ErrTurnNotFound

func NewTempDurableEventSessionStore(root string) (*DurableEventSessionStore, error) {
	return server.NewTempDurableEventSessionStore(root)
}

func ensureFixtureThread(store *DurableEventSessionStore, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if store == nil || threadID == "" {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"threads": []any{map[string]any{
			"id":     threadID,
			"title":  "Conformance fixture",
			"status": "idle",
			"turns":  []any{},
		}},
	})
	if err != nil {
		return err
	}
	return store.SeedFromG2Routes([]G2RouteReplayCase{{
		ID:           "thread-list-default",
		ResponseKind: "json",
		Response:     G2RouteResponse{Status: http.StatusOK, Body: body},
	}})
}

func Authorized(r *http.Request, token string, insecure ...bool) bool {
	return httpapi.Authorized(r, token, insecure...)
}

func MethodNotAllowed(w http.ResponseWriter) {
	httpapi.MethodNotAllowed(w)
}

func RequestBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	return httpapi.RequestBody(w, r)
}

func RouteKey(method string, path string, body json.RawMessage) string {
	return httpapi.RouteKey(method, path, body)
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	httpapi.WriteJSON(w, status, body)
}

func WriteRawJSON(w http.ResponseWriter, status int, body json.RawMessage) {
	httpapi.WriteRawJSON(w, status, body)
}

func WriteSSE(w http.ResponseWriter, status int, frames []string) {
	httpapi.WriteSSE(w, status, frames)
}

func WriteDurableSSE(w http.ResponseWriter, status int, events []map[string]any) {
	httpapi.WriteDurableSSE(w, status, events)
}

func HealthResponse() map[string]any {
	return httpapi.HealthResponse()
}

func RuntimeInfoResponse(startedAt string) any {
	return httpapi.RuntimeInfoResponse(startedAt)
}

func RuntimeToolsResponse() any {
	return httpapi.RuntimeToolsResponse()
}

func IntQuery(values url.Values, key string) int {
	return httpapi.IntQuery(values, key)
}

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}

func cloneMap(value map[string]any) map[string]any {
	return contracts.CloneMap(value)
}

func threadSummary(thread map[string]any) map[string]any {
	return contracts.ThreadSummary(thread)
}

func threadListIncludesSide(include string) bool {
	for _, part := range strings.Split(include, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "side") {
			return true
		}
	}
	return false
}
