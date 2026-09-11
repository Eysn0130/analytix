package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeToolExecutionObservationRouteReachesPrivateServiceBoundary(t *testing.T) {
	handler := &runtimeServerHandler{
		runtimeToken: "runtime-observation-token",
		sandboxMode:  "danger-full-access",
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/runtime/tool-executions/observe",
		strings.NewReader(`{"threadId":"thread-1","turnId":"turn-1","toolName":"bash","workspace":"/workspace","arguments":{"command":"private-command"}}`),
	)
	request.Header.Set("Authorization", "Bearer runtime-observation-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(recorder.Body.String(), "internal_error") ||
		strings.Contains(recorder.Body.String(), "private-command") {
		t.Fatalf("observation route did not reach its fail-closed service boundary: status=%d headers=%v body=%s",
			recorder.Code, recorder.Header(), recorder.Body.String())
	}
}
