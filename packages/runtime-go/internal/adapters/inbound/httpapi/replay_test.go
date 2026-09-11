package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyCanonicalRouteKeyAndRawJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/example", strings.NewReader(`{"b":2,"a":1}`))
	body, ok := RequestBody(httptest.NewRecorder(), request)
	if !ok {
		t.Fatal("expected request body to decode")
	}
	if got := RouteKey(http.MethodPost, "/v1/example", body); got != `POST /v1/example {"a":1,"b":2}` {
		t.Fatalf("unexpected route key: %q", got)
	}
	recorder := httptest.NewRecorder()
	WriteRawJSON(recorder, http.StatusCreated, json.RawMessage(`{"ok":true}`))
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected raw json response: status=%d headers=%v", recorder.Code, recorder.Header())
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"ok":true}` {
		t.Fatalf("unexpected raw json body: %q", got)
	}
}

func TestRequestMapBodyKeepsValidationErrorContract(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/example", strings.NewReader(`{"name":"alpha"}`))
	body, ok := RequestMapBody(httptest.NewRecorder(), request, "invalid example body")
	if !ok || body["name"] != "alpha" {
		t.Fatalf("unexpected request map: ok=%v body=%#v", ok, body)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/example", strings.NewReader(`{`))
	recorder := httptest.NewRecorder()
	body, ok = RequestMapBody(recorder, request, "invalid example body")
	if ok || body != nil {
		t.Fatalf("invalid JSON should fail: ok=%v body=%#v", ok, body)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	var errorBody map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &errorBody); err != nil {
		t.Fatalf("decode validation response: %v", err)
	}
	if errorBody["code"] != "validation_error" || errorBody["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected validation response: %#v", errorBody)
	}
}

func TestWriteSSEAndDurableSSEEventShape(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteSSE(recorder, http.StatusOK, []string{"event: ready\ndata: {}"})
	if recorder.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatalf("unexpected SSE content type: %q", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), "event: ready\n") {
		t.Fatalf("unexpected SSE body: %q", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	WriteDurableSSE(recorder, http.StatusOK, []map[string]any{{"seq": float64(7), "kind": "heartbeat", "threadId": "t1"}})
	body := recorder.Body.String()
	if !strings.Contains(body, "id: 7\n") || !strings.Contains(body, "event: heartbeat\n") || !strings.Contains(body, `"threadId":"t1"`) {
		t.Fatalf("unexpected durable SSE body: %q", body)
	}
}
