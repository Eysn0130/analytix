package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

func TestWriteControlActionResultEncodesValidationErrors(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteControlActionResult(recorder, controlapp.ActionResult{}, controlapp.ErrMissingPrompt)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", recorder.Code)
	}
	body := decodeControlResultBody(t, recorder)
	if body["code"] != "validation_error" || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected validation body: %#v", body)
	}
}

func TestGateHTTPNeverLeaksInternalError(t *testing.T) {
	recorder := httptest.NewRecorder()
	sentinel := "api_key=sk-secret /private/case.csv account=6222021234567890 SELECT * FROM secret MCP_PAYLOAD"
	WriteControlActionResult(recorder, controlapp.ActionResult{}, errors.New(sentinel))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected internal error, got %d", recorder.Code)
	}
	body := decodeControlResultBody(t, recorder)
	if body["code"] != "internal_error" || body["message"] != controlapp.SafeInternalControlMessage {
		t.Fatalf("unexpected internal body: %#v", body)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(sentinel)) {
		t.Fatalf("internal error sentinel leaked through HTTP: %s", recorder.Body.Bytes())
	}

	recorder = httptest.NewRecorder()
	WriteControlActionResult(recorder, controlapp.GateTerminalActionResult(nil, errors.New(sentinel)), nil)
	if recorder.Code != http.StatusInternalServerError || bytes.Contains(recorder.Body.Bytes(), []byte(sentinel)) {
		t.Fatalf("gate terminal ActionResult leaked internal error: status=%d body=%s", recorder.Code, recorder.Body.Bytes())
	}
}

func TestWriteControlActionResultWritesResult(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteControlActionResult(recorder, controlapp.ActionResult{StatusCode: http.StatusAccepted, Body: map[string]any{"turnId": "turn_1"}}, nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted, got %d", recorder.Code)
	}
	body := decodeControlResultBody(t, recorder)
	if body["turnId"] != "turn_1" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestWriteControlActionResultPreservesClosedNewTurnRequiredBoundary(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteControlActionResult(recorder, controlapp.ActionResult{
		StatusCode: http.StatusConflict,
		Body:       controlapp.NewTurnRequiredResponse("thread_1", "turn_1", controlapp.SteerBlockerContextChangingData),
	}, nil)
	body := decodeControlResultBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["code"] != "new_turn_required" ||
		body["blockerCode"] != controlapp.SteerBlockerContextChangingData {
		t.Fatalf("closed new-turn boundary was not preserved: code=%d body=%#v", recorder.Code, body)
	}
	if _, ok := body["threadId"]; ok {
		t.Fatalf("HTTP failure projection reflected a turn identifier: %#v", body)
	}
}

func decodeControlResultBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}
