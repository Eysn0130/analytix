package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	displayapp "analytix.local/runtime-go/internal/app/userinputdisplay"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

type userInputHTTPFixtureV1 struct {
	display displayapp.DisplayV1
	calls   int
	err     error
}

func (f *userInputHTTPFixtureV1) Read(context.Context, port.SelectorV1) (displayapp.DisplayV1, error) {
	f.calls++
	return f.display, f.err
}

func userInputHTTPDisplayV1() (displayapp.DisplayV1, port.SelectorV1) {
	selector := port.SelectorV1{ThreadID: "thread-a", TurnID: "turn-a", ItemID: "item_input_" + strings.Repeat("a", 64), RequestID: "input_" + strings.Repeat("a", 64)}
	return displayapp.DisplayV1{SchemaVersion: 1, ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, RequestID: selector.RequestID, Status: "pending", AnswerState: "pending",
		Questions: []displayapp.QuestionV1{{ID: selector.RequestID + "_1", Header: "Input", Question: " original question ", Options: []displayapp.OptionV1{}}}}, selector
}

func TestUserInputLocalDisplayStrictSelectorAndFixedErrors(t *testing.T) {
	display, selector := userInputHTTPDisplayV1()
	valid, _ := json.Marshal(selector)
	for _, test := range []struct {
		body   []byte
		method string
		status int
	}{
		{valid, "GET", 405},
		{[]byte(strings.Replace(string(valid), `"threadId":"thread-a"`, `"threadId":"thread-a","threadId":"foreign"`, 1)), "POST", 400},
		{[]byte(strings.Replace(string(valid), `"threadId":`, `"privateBody":"unsafe","threadId":`, 1)), "POST", 400},
		{append(append([]byte{}, valid...), []byte(`{}`)...), "POST", 400},
		{[]byte(strings.Repeat("x", 4097)), "POST", 400},
		{bytes.Replace(valid, []byte("thread-a"), []byte{0xff}, 1), "POST", 400},
		{[]byte(strings.Replace(string(valid), "thread-a", `\ud800`, 1)), "POST", 400},
		{[]byte(strings.Replace(string(valid), selector.RequestID, "q1", 1)), "POST", 400},
	} {
		fixture := &userInputHTTPFixtureV1{display: display}
		w := httptest.NewRecorder()
		UserInputLocalDisplayHandlerV1{Service: fixture}.ServeHTTP(w, httptest.NewRequest(test.method, UserInputLocalDisplayPathV1, bytes.NewReader(test.body)))
		if w.Code != test.status || fixture.calls != 0 || strings.Contains(w.Body.String(), "unsafe") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("invalid wire reached private service or leaked a raw failure")
		}
	}
	fixture := &userInputHTTPFixtureV1{err: errors.New("private unsafe original receipt")}
	w := httptest.NewRecorder()
	UserInputLocalDisplayHandlerV1{Service: fixture}.ServeHTTP(w, httptest.NewRequest("POST", UserInputLocalDisplayPathV1, bytes.NewReader(valid)))
	// WriteJSON keeps failures in the existing fixed, closed public envelope.
	if w.Code != 404 || fixture.calls != 1 || strings.TrimSpace(w.Body.String()) != `{"code":"not_found","message":"The requested resource was not found."}` {
		t.Fatal("unavailable response exposed private error")
	}
}

func TestUserInputLocalDisplayResponseIsClosedAndBounded(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*displayapp.DisplayV1)
		status int
	}{
		{"valid_original", func(*displayapp.DisplayV1) {}, 200},
		{"foreign_item", func(d *displayapp.DisplayV1) { d.ItemID = "item-b" }, 404},
		{"pending_answers", func(d *displayapp.DisplayV1) { d.Submission = &domaincontinuation.UserInputSubmissionV1{} }, 404},
		{"invented_status", func(d *displayapp.DisplayV1) { d.Status = "expired" }, 404},
		{"encoded_overflow", func(d *displayapp.DisplayV1) {
			d.Questions[0].Question = strings.Repeat("x", 4<<20)
			d.Questions[0].Header = strings.Repeat("x", 4<<20)
			detail := strings.Repeat("x", 128<<10)
			d.Questions[0].Detail = &detail
		}, 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			display, selector := userInputHTTPDisplayV1()
			test.change(&display)
			fixture := &userInputHTTPFixtureV1{display: display}
			body, _ := json.Marshal(selector)
			w := httptest.NewRecorder()
			UserInputLocalDisplayHandlerV1{Service: fixture}.ServeHTTP(w, httptest.NewRequest("POST", UserInputLocalDisplayPathV1, bytes.NewReader(body)))
			if w.Code != test.status || len(w.Body.Bytes()) > port.MaxResponseBytesV1 {
				t.Fatal("invalid private response was not fail-closed")
			}
			if test.status == 200 {
				var response map[string]any
				if json.Unmarshal(w.Body.Bytes(), &response) != nil || response["draft"] != nil || response["draftRevision"] != nil || response["arguments"] != nil || response["executionGrant"] != nil || response["submission"] != nil || !strings.Contains(w.Body.String(), " original question ") {
					t.Fatal("response ceased to be the closed original-question DTO")
				}
				expectedKeys := []string{
					"answerState", "callId", "continuedReason", "draft", "draftRevision",
					"itemId", "lateReply", "questions", "requestId", "schemaVersion",
					"status", "threadId", "timedWait", "turnId",
				}
				if len(response) != len(expectedKeys) {
					t.Fatal("original-question DTO has unexpected top-level keys")
				}
				for _, key := range expectedKeys {
					if _, present := response[key]; !present {
						t.Fatalf("original-question DTO is missing key %s", key)
					}
				}
				if reason, present := response["continuedReason"]; !present || reason != nil {
					t.Fatal("legacy private display lost nullable continuedReason")
				}
				for _, key := range []string{"timedWait", "lateReply", "callId"} {
					if value, present := response[key]; !present || value != nil {
						t.Fatal("legacy private display lost actual nullable timed metadata")
					}
				}
			}
		})
	}
}

func TestUserInputLocalDisplayUsesExistingPrivateMuxAuthority(t *testing.T) {
	calls := 0
	leaf := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(200) })
	for _, test := range []struct {
		token, bearer, header string
		insecure              bool
		status                int
	}{
		{"host-secret", "", LocalDisplayHeaderValueV1, false, 401},
		{"host-secret", "Bearer host-secret", "", false, 403},
		{"", "", LocalDisplayHeaderValueV1, true, 401},
		{"host-secret", "Bearer host-secret", LocalDisplayHeaderValueV1, true, 200},
	} {
		r := httptest.NewRequest("POST", UserInputLocalDisplayPathV1, strings.NewReader(`{}`))
		r.Header.Set("Authorization", test.bearer)
		r.Header.Set(LocalDisplayHeaderV1, test.header)
		w := httptest.NewRecorder()
		LocalDisplayMuxV1{RuntimeToken: test.token, Insecure: test.insecure, UserInputs: leaf}.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatal("private user-input route did not retain existing bearer/header authority")
		}
	}
	r := httptest.NewRequest("POST", UserInputLocalDisplayPathV1, strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer host-secret")
	r.Header.Add(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
	r.Header.Add(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	LocalDisplayMuxV1{RuntimeToken: "host-secret", UserInputs: leaf}.ServeHTTP(w, r)
	if w.Code != 403 || calls != 1 {
		t.Fatal("duplicate header or unauthorized request reached the protected leaf")
	}
}

type userInputDiscardHTTPFixtureV1 struct {
	userInputHTTPFixtureV1
	response     displayapp.DiscardLateReplyResponseV1
	discardCalls int
	discardErr   error
}

func (f *userInputDiscardHTTPFixtureV1) DiscardLateReply(context.Context, displayapp.DiscardLateReplyRequestV1) (displayapp.DiscardLateReplyResponseV1, error) {
	f.discardCalls++
	return f.response, f.discardErr
}

func TestUserInputDiscardHTTPUsesClosedExistingPrivateRoute(t *testing.T) {
	_, selector := userInputHTTPDisplayV1()
	request := displayapp.DiscardLateReplyRequestV1{SelectorV1: selector, Action: "discardLateReply", ClientUserMessageID: "00000000-0000-4000-8000-000000000001", ExpectedTurnID: "turn-target"}
	valid, _ := json.Marshal(request)
	for _, body := range [][]byte{
		bytes.Replace(valid, []byte(`"action":"discardLateReply"`), []byte(`"action":"cancel"`), 1),
		bytes.Replace(valid, []byte(`"action":"discardLateReply"`), []byte(`"action":null`), 1),
		bytes.Replace(valid, []byte(`"action":`), []byte(`"receiptId":"unsafe","action":`), 1),
		bytes.Replace(valid, []byte(`"action":`), []byte(`"action":"discardLateReply","action":`), 1),
		bytes.Replace(valid, []byte(request.ClientUserMessageID), []byte("arbitrary-text"), 1),
		bytes.Replace(valid, []byte(request.ExpectedTurnID), []byte("../target"), 1),
	} {
		fixture := &userInputDiscardHTTPFixtureV1{}
		response := httptest.NewRecorder()
		UserInputLocalDisplayHandlerV1{Service: fixture}.ServeHTTP(response, httptest.NewRequest(http.MethodPost, UserInputLocalDisplayPathV1, bytes.NewReader(body)))
		if response.Code != http.StatusBadRequest || fixture.calls != 0 || fixture.discardCalls != 0 || strings.Contains(response.Body.String(), "unsafe") {
			t.Fatal("open or malformed discard command reached the service")
		}
	}
	for _, test := range []struct {
		state  string
		err    error
		status int
	}{
		{"discarded", nil, http.StatusOK}, {"accepted", nil, http.StatusNotFound},
		{"", errors.New("unsafe missing delivery"), http.StatusNotFound},
		{"", displayapp.ErrDiscardConflictV1, http.StatusConflict},
	} {
		fixture := &userInputDiscardHTTPFixtureV1{discardErr: test.err, response: displayapp.DiscardLateReplyResponseV1{
			SelectorV1: selector, SchemaVersion: 1, Status: "continued", LateReply: continuationapp.UserInputLateReplyDeliveryV1{ClientUserMessageID: request.ClientUserMessageID, TurnID: request.ExpectedTurnID, State: test.state},
		}}
		response := httptest.NewRecorder()
		UserInputLocalDisplayHandlerV1{Service: fixture}.ServeHTTP(response, httptest.NewRequest(http.MethodPost, UserInputLocalDisplayPathV1, bytes.NewReader(valid)))
		if response.Code != test.status || fixture.discardCalls != 1 || fixture.calls != 0 || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "unsafe") {
			t.Fatal("private discard result did not preserve its actual disposition")
		}
		if test.status == http.StatusOK {
			var ack displayapp.DiscardLateReplyResponseV1
			if json.Unmarshal(response.Body.Bytes(), &ack) != nil || !displayapp.ValidateDiscardLateReplyResponseV1(ack, request) {
				t.Fatal("discard acknowledgement lost exact Host binding")
			}
		}
	}
}

type userInputHTTPDraftFixtureV1 struct {
	*userInputHTTPFixtureV1
	saves   int
	saveErr error
}

func (f *userInputHTTPDraftFixtureV1) SaveDraft(_ context.Context, request displayapp.SaveDraftRequestV1) (displayapp.SaveDraftResponseV1, error) {
	f.saves++
	return displayapp.SaveDraftResponseV1{SelectorV1: request.SelectorV1, SchemaVersion: 1, Status: "pending", DraftRevision: request.Revision, Draft: request.Draft}, f.saveErr
}
func TestPrivateQuestionDraftHTTPClosedNullableAndPartialCommand(t *testing.T) {
	display, selector := userInputHTTPDisplayV1()
	request := displayapp.SaveDraftRequestV1{SelectorV1: selector, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000001", Draft: domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: selector.RequestID + "_1", OptionIDs: []string{}, CustomText: strings.Repeat("x", 5000)}}}}
	valid, _ := json.Marshal(request)
	for _, raw := range [][]byte{
		bytes.Replace(valid, []byte(`"expectedRevision":null,`), nil, 1),
		bytes.Replace(valid, []byte(`"currentPage":0`), []byte(`"currentPage":0,"currentPage":1`), 1),
		bytes.Replace(valid, []byte(`"skipped":false`), []byte(`"skipped":false,"raw":"unsafe"`), 1),
		bytes.Replace(valid, []byte(`"optionIds":[]`), []byte(`"optionIds":null`), 1),
		bytes.Replace(valid, []byte(`"revision":`), []byte(`"receiptId":"unsafe","revision":`), 1),
	} {
		f := &userInputHTTPDraftFixtureV1{userInputHTTPFixtureV1: &userInputHTTPFixtureV1{display: display}}
		w := httptest.NewRecorder()
		UserInputLocalDisplayHandlerV1{Service: f}.ServeHTTP(w, httptest.NewRequest(http.MethodPost, UserInputLocalDisplayPathV1, bytes.NewReader(raw)))
		if w.Code != http.StatusBadRequest || f.saves != 0 || strings.Contains(w.Body.String(), "unsafe") {
			t.Fatal("open nested draft reached producer or raw error")
		}
	}
	for _, test := range []struct {
		err  error
		want int
	}{{nil, 200}, {continuationapp.ErrUserInputDraftConflictV1, 409}, {continuationapp.ErrUserInputDraftCapacityV1, 413}, {continuationapp.ErrUserInputDraftInvalidV1, 400}, {errors.New("unsafe synthetic body"), 404}} {
		f := &userInputHTTPDraftFixtureV1{userInputHTTPFixtureV1: &userInputHTTPFixtureV1{display: display}, saveErr: test.err}
		w := httptest.NewRecorder()
		UserInputLocalDisplayHandlerV1{Service: f}.ServeHTTP(w, httptest.NewRequest(http.MethodPost, UserInputLocalDisplayPathV1, bytes.NewReader(valid)))
		if w.Code != test.want || f.saves != 1 || w.Header().Get("Cache-Control") != "no-store" || test.want != 200 && strings.Contains(w.Body.String(), "unsafe") {
			t.Fatal("invalid save ACK/failure projection")
		}

		if test.want != http.StatusOK {
			var failure map[string]any
			if json.Unmarshal(w.Body.Bytes(), &failure) != nil || len(failure) != 2 {
				t.Fatal("draft failure body was not closed")
			}
			code, message := "validation_error", "The request did not satisfy the runtime contract."
			if test.want == http.StatusNotFound {
				code, message = "not_found", "The requested resource was not found."
			}
			if test.want == http.StatusConflict {
				code, message = "conflict", "The request conflicts with the current runtime state."
			}
			if failure["code"] != code || failure["message"] != message || strings.Contains(w.Body.String(), request.Draft.Answers[0].CustomText) {
				t.Fatal("draft capacity/unavailability failure returned a variable or raw body")
			}
		}
	}
}
