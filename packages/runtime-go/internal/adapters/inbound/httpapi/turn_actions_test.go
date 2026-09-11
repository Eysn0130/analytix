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

func TestTurnActionHandlersValidateRouteAndMethod(t *testing.T) {
	handler := TurnActionHandlers{Control: &turnActionControlStub{}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadTurnAction(recorder, request, "thread-1/turns/turn-1/interrupt")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadTurnAction(recorder, request, "thread-1/messages/turn-1")
	body := decodeTurnActionBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected route response code=%d body=%#v", recorder.Code, body)
	}
}

func TestTurnActionHandlersSteerCallsControl(t *testing.T) {
	stub := &turnActionControlStub{steerResult: controlapp.ActionResult{StatusCode: http.StatusAccepted, Body: map[string]any{"steered": true}}}
	handler := TurnActionHandlers{Control: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"text":" continue ","displayText":"Continue","riskIntent":"case","clientUserMessageId":"msg-1","expectedTurnId":"turn-1","attachmentIds":[" att-1 "],"fileReferences":[{"path":"/workspace/a.txt","relativePath":"a.txt","name":"a.txt","kind":"file"}],"delivery":"steer"}`))
	handler.HandleThreadTurnAction(recorder, request, "thread-1/turns/turn-1/steer")

	body := decodeTurnActionBody(t, recorder)
	if recorder.Code != http.StatusAccepted || body["steered"] != true {
		t.Fatalf("unexpected steer response code=%d body=%#v", recorder.Code, body)
	}
	if stub.steer.ThreadID != "thread-1" || stub.steer.TurnID != "turn-1" || stub.steer.Text != "continue" || stub.steer.RiskIntent != "case" {
		t.Fatalf("unexpected steer request: %#v", stub.steer)
	}
	if len(stub.steer.AttachmentIDs) != 1 || stub.steer.AttachmentIDs[0] != "att-1" || len(stub.steer.FileReferences) != 1 {
		t.Fatalf("unexpected steer attachments/files: %#v", stub.steer)
	}
}

func TestStrictSteerRejectsAmbiguousOrOpenPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown", body: `{"text":"continue","publicationPolicy":"general_guidance"}`},
		{name: "host case", body: `{"text":"continue","caseId":"spoofed"}`},
		{name: "duplicate", body: `{"text":"continue","text":"replace"}`},
		{name: "trailing", body: `{"text":"continue"}{"text":"again"}`},
		{name: "null object", body: `null`},
		{name: "null text", body: `{"text":null}`},
		{name: "wrong text type", body: `{"text":7}`},
		{name: "blank text", body: `{"text":"  "}`},
		{name: "wrong attachment type", body: `{"text":"continue","attachmentIds":"att-1"}`},
		{name: "mixed attachment array", body: `{"text":"continue","attachmentIds":["att-1",7]}`},
		{name: "blank attachment", body: `{"text":"continue","attachmentIds":["att-1","  "]}`},
		{name: "open file reference", body: `{"text":"continue","fileReferences":[{"path":"/a","relativePath":"a","name":"a","supportStatus":"verified"}]}`},
		{name: "partial file reference", body: `{"text":"continue","fileReferences":[{"path":"/a"}]}`},
		{name: "invalid risk downgrade", body: `{"text":"continue","riskIntent":"general"}`},
		{name: "invalid delivery", body: `{"text":"continue","delivery":"message"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &turnActionControlStub{}
			handler := TurnActionHandlers{Control: stub}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(test.body))
			handler.HandleThreadTurnAction(recorder, request, "thread-1/turns/turn-1/steer")
			body := decodeTurnActionBody(t, recorder)
			if recorder.Code != http.StatusBadRequest || body["code"] != "validation_error" {
				t.Fatalf("payload passed strict steer contract: code=%d body=%#v", recorder.Code, body)
			}
			if stub.steer.Text != "" {
				t.Fatalf("invalid payload reached control: %#v", stub.steer)
			}
		})
	}
}

func TestStrictSteerEnforcesResourceBounds(t *testing.T) {
	tooDeep := `{"text":"continue","attachmentIds":` + strings.Repeat(`[`, maxStartTurnJSONDepth+1) + `"att"` + strings.Repeat(`]`, maxStartTurnJSONDepth+1) + `}`
	if _, err := DecodeSteerTurnRequest("thread", "turn", []byte(tooDeep)); err == nil {
		t.Fatal("over-depth steer body passed strict decoder")
	}
	tooLongString := `{"text":"` + strings.Repeat("x", maxStartTurnJSONString+1) + `"}`
	if _, err := DecodeSteerTurnRequest("thread", "turn", []byte(tooLongString)); err == nil || !strings.Contains(err.Error(), "string limit") {
		t.Fatalf("overlong steer string was not rejected by string bound: %v", err)
	}
	tokenItems := strings.Repeat(`"att",`, maxStartTurnJSONTokens)
	tooManyTokens := `{"text":"continue","attachmentIds":[` + strings.TrimSuffix(tokenItems, ",") + `]}`
	if _, err := DecodeSteerTurnRequest("thread", "turn", []byte(tooManyTokens)); err == nil || !strings.Contains(err.Error(), "token limit") {
		t.Fatalf("over-token steer body was not rejected by token bound: %v", err)
	}
	tooLarge := `{"text":"` + strings.Repeat("x", maxStartTurnRequestBytes) + `"}`
	if _, err := DecodeSteerTurnRequest("thread", "turn", []byte(tooLarge)); err == nil {
		t.Fatal("oversized steer body passed strict decoder")
	}
	attachments := make([]string, maxStartTurnListItemCount+1)
	for index := range attachments {
		attachments[index] = `"att"`
	}
	tooManyItems := `{"text":"continue","attachmentIds":[` + strings.Join(attachments, ",") + `]}`
	if _, err := DecodeSteerTurnRequest("thread", "turn", []byte(tooManyItems)); err == nil {
		t.Fatal("oversized steer array passed strict decoder")
	}
}

func TestTurnActionHandlersInterruptCallsControl(t *testing.T) {
	stub := &turnActionControlStub{interruptResult: controlapp.ActionResult{Body: map[string]any{"status": "aborted"}}}
	handler := TurnActionHandlers{Control: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"discard":true}`))
	handler.HandleThreadTurnAction(recorder, request, "thread-1/turns/turn-1/interrupt")

	body := decodeTurnActionBody(t, recorder)
	if recorder.Code != http.StatusOK || body["status"] != "aborted" {
		t.Fatalf("unexpected interrupt response code=%d body=%#v", recorder.Code, body)
	}
	if stub.interrupt.ThreadID != "thread-1" || stub.interrupt.TurnID != "turn-1" || !stub.interrupt.Discard {
		t.Fatalf("unexpected interrupt request: %#v", stub.interrupt)
	}
}

func TestStrictInterruptAcceptsOnlyOptionalBooleanDiscard(t *testing.T) {
	valid := []struct {
		body    string
		discard bool
	}{{body: `{}`, discard: false}, {body: `{"discard":true}`, discard: true}}
	for _, test := range valid {
		request, err := DecodeInterruptTurnRequest("thread", "turn", []byte(test.body))
		if err != nil || request.Discard != test.discard {
			t.Fatalf("valid interrupt body rejected: body=%s request=%#v err=%v", test.body, request, err)
		}
	}
	for _, body := range []string{
		`null`, `[]`, `{"discard":null}`, `{"discard":"true"}`,
		`{"discard":true,"discard":false}`, `{"discard":true,"reason":"caller-owned"}`,
		`{"discard":true}{"discard":false}`,
	} {
		if _, err := DecodeInterruptTurnRequest("thread", "turn", []byte(body)); err == nil {
			t.Fatalf("invalid interrupt body passed strict decoder: %s", body)
		}
	}
}

func TestTurnActionHandlersMapControlErrors(t *testing.T) {
	handler := TurnActionHandlers{Control: &turnActionControlStub{steerErr: controlapp.ErrMissingSteerText}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"text":"valid"}`))
	handler.HandleThreadTurnAction(recorder, request, "thread-1/turns/turn-1/steer")
	body := decodeTurnActionBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected validation response code=%d body=%#v", recorder.Code, body)
	}

	handler = TurnActionHandlers{Control: &turnActionControlStub{interruptErr: errors.New("driver failed")}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{}`))
	handler.HandleThreadTurnAction(recorder, request, "thread-1/turns/turn-1/interrupt")
	body = decodeTurnActionBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["message"] != controlapp.SafeInternalControlMessage {
		t.Fatalf("unexpected internal response code=%d body=%#v", recorder.Code, body)
	}
}

func decodeTurnActionBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type turnActionControlStub struct {
	steer           controlapp.SteerTurnRequest
	steerResult     controlapp.ActionResult
	steerErr        error
	interrupt       controlapp.InterruptTurnRequest
	interruptResult controlapp.ActionResult
	interruptErr    error
}

func (s *turnActionControlStub) SteerTurn(_ context.Context, request controlapp.SteerTurnRequest) (controlapp.ActionResult, error) {
	s.steer = request
	return s.steerResult, s.steerErr
}

func (s *turnActionControlStub) InterruptTurn(_ context.Context, request controlapp.InterruptTurnRequest) (controlapp.ActionResult, error) {
	s.interrupt = request
	return s.interruptResult, s.interruptErr
}
