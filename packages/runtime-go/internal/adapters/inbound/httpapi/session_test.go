package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSessionHandlersValidateRouteAndMethod(t *testing.T) {
	handler := SessionHandlers{Service: &sessionServiceStub{}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1/resume-thread", nil)
	handler.HandlePath(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1/unknown", nil)
	handler.HandlePath(recorder, request)
	body := decodeSessionBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected unknown route response code=%d body=%#v", recorder.Code, body)
	}
}

func TestSessionHandlersDecodeBodyAndCallService(t *testing.T) {
	stub := &sessionServiceStub{response: map[string]any{"threadId": "thread-1"}}
	handler := SessionHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1/resume-thread", strings.NewReader(`{"threadId":"thread-1"}`))
	handler.HandlePath(recorder, request)

	body := decodeSessionBody(t, recorder)
	if recorder.Code != http.StatusCreated || body["threadId"] != "thread-1" {
		t.Fatalf("unexpected success response code=%d body=%#v", recorder.Code, body)
	}
	if stub.sessionID != "session-1" || stub.request["threadId"] != "thread-1" {
		t.Fatalf("service received unexpected request session=%q request=%#v", stub.sessionID, stub.request)
	}
}

func TestSessionHandlersRejectSafeRecordAliasResume(t *testing.T) {
	for _, path := range []string{
		"/v1/sessions/session:1/resume", "/v1/sessions/session%3A1/resume-thread",
	} {
		stub := &sessionServiceStub{response: map[string]any{"threadId": "must-not-exist"}}
		handler := SessionHandlers{Service: stub}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		handler.HandlePath(recorder, request)
		body := decodeSessionBody(t, recorder)
		if recorder.Code != http.StatusNotFound || body["code"] != "not_found" || stub.sessionID != "" || stub.request != nil {
			t.Fatalf("storage alias reached resume service: path=%q code=%d body=%#v session=%q request=%#v", path, recorder.Code, body, stub.sessionID, stub.request)
		}
	}
}

func TestSessionHandlersMapErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "missing session", err: os.ErrNotExist, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "validation", err: errors.New("invalid thread"), wantStatus: http.StatusBadRequest, wantCode: "validation_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := SessionHandlers{Service: &sessionServiceStub{err: tc.err}}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1/resume", strings.NewReader(`{}`))
			handler.HandlePath(recorder, request)
			body := decodeSessionBody(t, recorder)
			if recorder.Code != tc.wantStatus || body["code"] != tc.wantCode {
				t.Fatalf("unexpected error response code=%d body=%#v", recorder.Code, body)
			}
		})
	}
}

func TestSessionHandlersRejectInvalidBody(t *testing.T) {
	handler := SessionHandlers{Service: &sessionServiceStub{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1/resume", strings.NewReader(`[`))
	handler.HandlePath(recorder, request)
	body := decodeSessionBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected invalid body response code=%d body=%#v", recorder.Code, body)
	}
}

func decodeSessionBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type sessionServiceStub struct {
	sessionID string
	request   map[string]any
	response  map[string]any
	err       error
}

func (s *sessionServiceStub) Resume(sessionID string, request map[string]any) (map[string]any, error) {
	s.sessionID = sessionID
	s.request = request
	return s.response, s.err
}
