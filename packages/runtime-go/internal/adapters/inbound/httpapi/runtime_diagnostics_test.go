package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
)

func TestRuntimeDiagnosticsHandlersInfoAndTools(t *testing.T) {
	stub := &runtimeDiagnosticsStub{
		info:  runtimeinfoapp.PublicRuntimeInfoV2{SchemaVersion: 2, Status: "ready"},
		tools: runtimeinfoapp.PublicRuntimeToolsV2{SchemaVersion: 2, Commands: []runtimeinfoapp.PublicCommandDiagnosticV2{}},
	}
	handler := RuntimeDiagnosticsHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil)
	handler.HandleInfo(recorder, request)
	body := decodeRuntimeDiagnosticsBody(t, recorder)
	if recorder.Code != http.StatusOK || body["status"] != "ready" || body["schemaVersion"] != float64(2) {
		t.Fatalf("unexpected info response code=%d body=%#v", recorder.Code, body)
	}
	assertRuntimeDiagnosticHeaders(t, recorder)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/runtime/tools?refresh=true&threadId=foreign-thread", nil)
	handler.HandleTools(recorder, request)
	body = decodeRuntimeDiagnosticsBody(t, recorder)
	if recorder.Code != http.StatusOK || body["commands"] == nil || !stub.toolsRequest.Refresh {
		t.Fatalf("unexpected tools response code=%d body=%#v request=%#v", recorder.Code, body, stub.toolsRequest)
	}
	assertRuntimeDiagnosticHeaders(t, recorder)
}

func TestRuntimeDiagnosticsHandlersValidateMethodAndService(t *testing.T) {
	handler := RuntimeDiagnosticsHandlers{Service: &runtimeDiagnosticsStub{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/info", nil)
	handler.HandleInfo(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	handler = RuntimeDiagnosticsHandlers{}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/runtime/tools", nil)
	handler.HandleTools(recorder, request)
	body := decodeRuntimeDiagnosticsBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["message"] != testInternalFailureMessage {
		t.Fatalf("unexpected missing service response code=%d body=%#v", recorder.Code, body)
	}
}

func assertRuntimeDiagnosticHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("runtime diagnostics missing privacy headers: %#v", recorder.Header())
	}
}

func decodeRuntimeDiagnosticsBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type runtimeDiagnosticsStub struct {
	info         runtimeinfoapp.PublicRuntimeInfoV2
	tools        runtimeinfoapp.PublicRuntimeToolsV2
	toolsRequest RuntimeToolsRequest
}

func (s *runtimeDiagnosticsStub) RuntimeInfo() runtimeinfoapp.PublicRuntimeInfoV2 {
	return s.info
}

func (s *runtimeDiagnosticsStub) RuntimeTools(request RuntimeToolsRequest) runtimeinfoapp.PublicRuntimeToolsV2 {
	s.toolsRequest = request
	return s.tools
}

var _ RuntimeDiagnosticsService = (*runtimeDiagnosticsStub)(nil)
