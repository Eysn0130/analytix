package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

func TestGateHandlersApprovalValidatesBodyAndMethod(t *testing.T) {
	handler := GateHandlers{Control: &gateControlStub{}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/approvals/appr-1", nil)
	handler.HandleApproval(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/approvals/appr-1", strings.NewReader(`{"decision":"maybe"}`))
	handler.HandleApproval(recorder, request)
	body := decodeGateBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected invalid approval response code=%d body=%#v", recorder.Code, body)
	}
}

func TestGateHandlersApprovalCallsControl(t *testing.T) {
	stub := &gateControlStub{approvalResult: controlapp.ActionResult{StatusCode: http.StatusAccepted, Body: map[string]any{"ok": true}}}
	handler := GateHandlers{Control: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/approvals/appr-1", strings.NewReader(`{"decision":"allow","reason":"reviewed"}`))
	handler.HandleApproval(recorder, request)

	body := decodeGateBody(t, recorder)
	if recorder.Code != http.StatusAccepted || body["ok"] != true {
		t.Fatalf("unexpected approval response code=%d body=%#v", recorder.Code, body)
	}
	if stub.approval.ApprovalID != "appr-1" || stub.approval.Decision != "allow" || stub.approval.Reason != "reviewed" {
		t.Fatalf("unexpected approval request: %#v", stub.approval)
	}
}

func TestGateHandlersUserInputValidatesBodyAndCallsControl(t *testing.T) {
	stub := &gateControlStub{inputResult: controlapp.ActionResult{Body: map[string]any{"status": "submitted"}}}
	handler := GateHandlers{Control: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/user-inputs/input-1", strings.NewReader(`{`))
	handler.HandleUserInput(recorder, request, "/v1/user-inputs/")
	body := decodeGateBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected invalid input response code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/user-input/input-1", strings.NewReader(`{"answers":[{"value":"yes"}],"cancelled":false}`))
	handler.HandleUserInput(recorder, request, "/v1/user-input/")
	body = decodeGateBody(t, recorder)
	if recorder.Code != http.StatusOK || body["status"] != "submitted" {
		t.Fatalf("unexpected user input response code=%d body=%#v", recorder.Code, body)
	}
	if stub.input.InputID != "input-1" || len(stub.input.Answers) != 1 || stub.input.Answers[0]["value"] != "yes" {
		t.Fatalf("unexpected user input request: %#v", stub.input)
	}
}

func TestGateHandlersMapControlErrors(t *testing.T) {
	handler := GateHandlers{Control: &gateControlStub{approvalErr: controlapp.ErrMissingApprovalID}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/approvals/", strings.NewReader(`{"decision":"allow"}`))
	handler.HandleApproval(recorder, request)
	body := decodeGateBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected validation error response code=%d body=%#v", recorder.Code, body)
	}

	handler = GateHandlers{Control: &gateControlStub{inputErr: errors.New("driver failed")}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/user-inputs/input-1", strings.NewReader(`{"answers":[]}`))
	handler.HandleUserInput(recorder, request, "/v1/user-inputs/")
	body = decodeGateBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["message"] != controlapp.SafeInternalControlMessage {
		t.Fatalf("unexpected internal error response code=%d body=%#v", recorder.Code, body)
	}
}

func decodeGateBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type gateControlStub struct {
	approval       controlapp.ApprovalDecision
	approvalResult controlapp.ActionResult
	approvalErr    error
	input          controlapp.UserInputResponse
	inputResult    controlapp.ActionResult
	inputErr       error
}

func (s *gateControlStub) ApproveTool(_ context.Context, request controlapp.ApprovalDecision) (controlapp.ActionResult, error) {
	s.approval = request
	return s.approvalResult, s.approvalErr
}

func (s *gateControlStub) RespondUserInput(_ context.Context, request controlapp.UserInputResponse) (controlapp.ActionResult, error) {
	s.input = request
	return s.inputResult, s.inputErr
}
