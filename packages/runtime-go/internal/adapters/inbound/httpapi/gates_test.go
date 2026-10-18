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

func TestUserInputSelectorClosedWireAndURLBinding(t *testing.T) {
	id := "input_" + strings.Repeat("a", 64)
	selector := `{"threadId":"source","turnId":"turn","itemId":"item_` + id + `","requestId":"` + id + `"}`
	for _, command := range []string{`"cancelled":true`, `"action":"continue"`, `"answers":[{"id":"` + id + `_1","kind":"skip"}]`} {
		if response, ok := decodeUserInputResponse([]byte(`{"selector":` + selector + `,` + command + `}`)); !ok || response.Selector == nil || response.Selector.ThreadID != "source" {
			t.Fatal("closed selector command rejected")
		}
	}
	for _, invalid := range []string{"null", strings.Replace(selector, "threadId", "ThreadId", 1), strings.Replace(selector, `"source"`, `null`, 1), strings.Replace(selector, `"turnId":"turn",`, "", 1), strings.Replace(selector, `"threadId":"source"`, `"threadId":"source","extra":true`, 1)} {
		if _, ok := decodeUserInputResponse([]byte(`{"selector":` + invalid + `,"cancelled":true}`)); ok {
			t.Fatal("open or incomplete selector accepted")
		}
	}
	stub := &gateControlStub{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/user-inputs/input_"+strings.Repeat("b", 64), strings.NewReader(`{"selector":`+selector+`,"cancelled":true}`))
	GateHandlers{Control: stub}.HandleUserInput(w, r, "/v1/user-inputs/")
	if w.Code != http.StatusBadRequest || stub.input.InputID != "" {
		t.Fatal("foreign route reached controller")
	}
}

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

func TestGateHandlersUserInputAcceptsClosedTypedBatch(t *testing.T) {
	stub := &gateControlStub{inputResult: controlapp.ActionResult{Body: map[string]any{"inputId": "input-1", "status": "submitted"}}}
	handler := GateHandlers{Control: stub}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/user-inputs/input-1", strings.NewReader(`{"answers":[{"id":"input-1_1","kind":"answer","optionIds":["input-1_1_option_1"],"customText":"more"},{"id":"input-1_2","kind":"skip"}]}`))
	handler.HandleUserInput(recorder, request, "/v1/user-inputs/")
	if recorder.Code != http.StatusOK || stub.input.InputID != "input-1" || stub.input.Answers != nil || len(stub.input.StructuredAnswers) != 2 || *stub.input.StructuredAnswers[0].CustomText != "more" || stub.input.StructuredAnswers[1].Kind != "skip" {
		t.Fatalf("typed carrier not preserved: code=%d request=%#v", recorder.Code, stub.input)
	}
}

func TestGateHandlersUserInputRejectsAmbiguousOrExtendedBodies(t *testing.T) {
	for name, body := range map[string]string{
		"case ambiguous envelope": `{"Answers":[],"answers":[]}`,
		"unknown envelope":        `{"answers":[],"authority":"client"}`,
		"unknown typed field":     `{"answers":[{"id":"q1","kind":"skip","value":"secret"}]}`,
		"unknown legacy field":    `{"answers":[{"id":"q1","value":"yes","customText":"secret"}]}`,
		"mixed batch":             `{"answers":[{"id":"q1","kind":"skip"},{"id":"q2","value":"yes"}]}`,
		"cancel with answers":     `{"cancelled":true,"answers":[]}`,
		"null cancelled":          `{"answers":[],"cancelled":null}`,
		"null answers":            `{"answers":null}`,
		"missing answers":         `{}`,
		"null answer":             `{"answers":[null]}`,
		"null custom text":        `{"answers":[{"id":"q1","kind":"answer","optionIds":[],"customText":null}]}`,
		"null option list":        `{"answers":[{"id":"q1","kind":"answer","optionIds":null}]}`,
		"skip with empty options": `{"answers":[{"id":"q1","kind":"skip","optionIds":[]}]}`,
		"answer without options":  `{"answers":[{"id":"q1","kind":"answer"}]}`,
		"duplicate key":           `{"answers":[],"answers":[]}`,
		"trailing document":       `{"answers":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			stub := &gateControlStub{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/user-inputs/input-1", strings.NewReader(body))
			GateHandlers{Control: stub}.HandleUserInput(recorder, request, "/v1/user-inputs/")
			if recorder.Code != http.StatusBadRequest || stub.input.InputID != "" {
				t.Fatalf("invalid body reached control: code=%d request=%#v", recorder.Code, stub.input)
			}
		})
	}
}

func TestTimedQuestionCommandAndLateReplyAreClosedCarriers(t *testing.T) {
	request, ok := decodeUserInputResponse([]byte(`{"action":"continue"}`))
	if !ok || !request.Continue || request.Answers != nil || request.LateReply != nil {
		t.Fatal("continue carrier")
	}
	late := `{"answers":[{"id":"input_1","kind":"skip"}],"lateReply":{"clientUserMessageId":"00000000-0000-4000-8000-000000000001","expectedTurnId":"turn_1"}}`
	request, ok = decodeUserInputResponse([]byte(late))
	if !ok || request.LateReply == nil || request.LateReply.ExpectedTurnID != "turn_1" {
		t.Fatal("late carrier")
	}
	for _, body := range []string{
		`{"action":"continue","answers":[]}`, `{"action":"continue","cancelled":false}`, `{"action":"approve"}`,
		`{"cancelled":true,"lateReply":{"clientUserMessageId":"00000000-0000-4000-8000-000000000001","expectedTurnId":"turn_1"}}`,
		`{"answers":[],"lateReply":null}`,
		`{"answers":[],"lateReply":{"clientUserMessageId":"private body","expectedTurnId":"turn_1"}}`,
		`{"answers":[],"lateReply":{"clientUserMessageId":"00000000-0000-4000-8000-000000000001","expectedTurnId":"turn_1","sourceInputId":"input_2"}}`,
	} {
		if _, ok := decodeUserInputResponse([]byte(body)); ok {
			t.Fatal("open timed carrier accepted", body)
		}
	}
}
