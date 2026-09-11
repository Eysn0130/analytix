package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func TestReviewHandlersStartReview(t *testing.T) {
	stub := &reviewServiceStub{response: map[string]any{"turnId": "turn_1", "reviewItemId": "item_review"}}
	handler := ReviewHandlers{Service: stub}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"target":{"path":"README.md"},"model":" model-a ","providerId":" provider-a "}`))

	handler.HandleReview(recorder, request, "thr_1")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusAccepted || body["turnId"] != "turn_1" || body["reviewItemId"] != "item_review" {
		t.Fatalf("review response mismatch: code=%d body=%#v", recorder.Code, body)
	}
	if stub.threadID != "thr_1" || stub.request.Model != "model-a" || stub.request.ProviderID != "provider-a" || stub.request.Target["path"] != "README.md" {
		t.Fatalf("review request mismatch: %#v", stub)
	}
}

func TestReviewHandlersErrorMapping(t *testing.T) {
	handler := ReviewHandlers{Service: &reviewServiceStub{err: threadapp.ErrThreadNotFound}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"target":{}}`))
	handler.HandleReview(recorder, request, "missing")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("not found mapping mismatch: code=%d body=%#v", recorder.Code, body)
	}

	handler = ReviewHandlers{Service: &reviewServiceStub{err: errors.New("provider failed")}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"target":{}}`))
	handler.HandleReview(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["code"] != "internal_error" || body["message"] != testInternalFailureMessage {
		t.Fatalf("internal mapping mismatch: code=%d body=%#v", recorder.Code, body)
	}
}

type reviewServiceStub struct {
	threadID string
	request  threadapp.ReviewRequest
	response map[string]any
	err      error
}

func (s *reviewServiceStub) StartReview(_ context.Context, threadID string, request threadapp.ReviewRequest) (map[string]any, error) {
	s.threadID = threadID
	s.request = request
	return s.response, s.err
}
