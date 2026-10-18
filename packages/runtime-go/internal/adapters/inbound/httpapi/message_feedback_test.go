package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type feedbackHTTPStubV1 struct {
	calls   int
	request domainthread.MessageFeedbackPutRequestV1
	failure error
}

func (s *feedbackHTTPStubV1) ListMessageFeedbackV1(context.Context, string) ([]domainthread.MessageFeedbackItemV1, error) {
	s.calls++
	return nil, s.failure
}
func (s *feedbackHTTPStubV1) PutMessageFeedbackV1(_ context.Context, _ string, messageID string, request domainthread.MessageFeedbackPutRequestV1) (domainthread.MessageFeedbackItemV1, error) {
	s.calls++
	s.request = request
	return domainthread.MessageFeedbackItemV1{MessageID: messageID, Rating: request.Rating, Version: "11111111-1111-4111-8111-111111111111", CreatedAt: 1, UpdatedAt: 1}, s.failure
}
func (s *feedbackHTTPStubV1) DeleteMessageFeedbackV1(context.Context, string, string, string) error {
	s.calls++
	return s.failure
}

func TestMessageFeedbackHTTPClosedRequestV1(t *testing.T) {
	for _, tc := range []struct {
		method, rest, body string
		want               int
	}{
		{http.MethodGet, "thread/message-feedback", "", 200},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive","ifVersion":null}`, 200},
		{http.MethodDelete, "thread/message-feedback/message", `{"ifVersion":"11111111-1111-4111-8111-111111111111"}`, 200},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive"}`, 400},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive","ifVersion":null,"note":null}`, 400},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive","ifVersion":null,"category":null}`, 400},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive","ifVersion":null,"replyText":"no"}`, 400},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive","rating":"negative","ifVersion":null}`, 400},
		{http.MethodPut, "thread/message-feedback/message", `{"rating":"positive","ifVersion":null,"note":"\ud800"}`, 400},
		{http.MethodDelete, "thread/message-feedback/message", `{"ifVersion":null}`, 400},
		{http.MethodGet, "thread/message-feedback/message", "", 405},
		{http.MethodPost, "thread/message-feedback/message", `{}`, 405},
		{http.MethodPut, "thread/message-feedback/message/extra", `{}`, 404},
		{http.MethodPut, "../message-feedback/message", `{}`, 404},
	} {
		s := &feedbackHTTPStubV1{}
		response := httptest.NewRecorder()
		MessageFeedbackHandlersV1{Service: s}.Handle(response, httptest.NewRequest(tc.method, "/v1/threads/"+tc.rest, strings.NewReader(tc.body)), tc.rest)
		if response.Code != tc.want {
			t.Fatalf("%s %s got %d want %d", tc.method, tc.rest, response.Code, tc.want)
		}
		if tc.want != 200 && s.calls != 0 {
			t.Fatal("invalid input reached service")
		}
		if tc.method == http.MethodGet && tc.want == 200 && strings.TrimSpace(response.Body.String()) != `{"items":[]}` {
			t.Fatal("empty list not explicit")
		}
	}
}

func TestMessageFeedbackHTTPConflictAndFailureProjectionV1(t *testing.T) {
	item := &domainthread.MessageFeedbackItemV1{MessageID: "message", Rating: "negative", Version: "11111111-1111-4111-8111-111111111111", CreatedAt: 1, UpdatedAt: 1}
	for _, current := range []*domainthread.MessageFeedbackItemV1{nil, item} {
		s := &feedbackHTTPStubV1{failure: &domainthread.MessageFeedbackVersionConflictV1{Current: current}}
		response := httptest.NewRecorder()
		MessageFeedbackHandlersV1{Service: s}.Handle(response, httptest.NewRequest(http.MethodPut, "/v1/threads/thread/message-feedback/message", strings.NewReader(`{"rating":"positive","ifVersion":null}`)), "thread/message-feedback/message")
		var body map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 409 || body["current"] == nil {
			t.Fatal("conflict did not preserve current", err)
		}
		var actual *domainthread.MessageFeedbackItemV1
		err := json.Unmarshal(body["current"], &actual)
		if err != nil || !reflect.DeepEqual(actual, current) || string(body["code"]) != `"message_feedback_version_conflict"` || string(body["message"]) != `"Message feedback changed. Review the current rating and submit again."` {
			t.Fatal("conflict envelope diverged from the closed public contract", err)
		}
	}
	unsafeNote := "<think>PRIVATE_UNSAFE_FEEDBACK_CURRENT</think>"
	for _, current := range []*domainthread.MessageFeedbackItemV1{
		{MessageID: "other", Rating: "negative", Version: item.Version, CreatedAt: 1, UpdatedAt: 1},
		{MessageID: "message", Rating: "negative", Version: "invalid", CreatedAt: 1, UpdatedAt: 1},
		{MessageID: "message", Rating: "negative", Note: &unsafeNote, Version: item.Version, CreatedAt: 1, UpdatedAt: 1},
	} {
		s := &feedbackHTTPStubV1{failure: &domainthread.MessageFeedbackVersionConflictV1{Current: current}}
		response := httptest.NewRecorder()
		MessageFeedbackHandlersV1{Service: s}.Handle(response, httptest.NewRequest(http.MethodPut, "/v1/threads/thread/message-feedback/message", strings.NewReader(`{"rating":"positive","ifVersion":null}`)), "thread/message-feedback/message")
		if response.Code != 503 || strings.Contains(response.Body.String(), `"current"`) || strings.Contains(response.Body.String(), "PRIVATE_") {
			t.Fatal("unsafe or mismatched conflict current escaped its route owner")
		}
	}
	s := &feedbackHTTPStubV1{failure: errors.New("PRIVATE_UNSAFE_DETAIL")}
	response := httptest.NewRecorder()
	MessageFeedbackHandlersV1{Service: s}.Handle(response, httptest.NewRequest(http.MethodGet, "/v1/threads/thread/message-feedback", nil), "thread/message-feedback")
	if response.Code != 503 || strings.Contains(response.Body.String(), "PRIVATE_") {
		t.Fatal("internal failure body escaped")
	}
}
