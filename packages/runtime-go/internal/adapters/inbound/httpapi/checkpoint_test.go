package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCheckpointHandlersPlanAndApply(t *testing.T) {
	stub := &checkpointServiceStub{workspace: "/workspace"}
	handler := CheckpointHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"scope":"code"}`))
	handler.HandleThreadCheckpointPath(recorder, request, "thr_1/checkpoints/cp_1/rewind-plan")
	body := decodeCheckpointBody(t, recorder)
	if recorder.Code != http.StatusOK || body["plan"].(map[string]any)["checkpointId"] != "cp_1" {
		t.Fatalf("unexpected plan response code=%d body=%#v", recorder.Code, body)
	}
	if stub.threadID != "thr_1" || stub.planScope != "code" || stub.planWorkspace != "/workspace" {
		t.Fatalf("unexpected plan service inputs: %#v", stub)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"plan":{"planId":"p1"},"confirmation":{"approved":true}}`))
	handler.HandleThreadCheckpointPath(recorder, request, "thr_1/checkpoints/cp_1/rewind-apply")
	body = decodeCheckpointBody(t, recorder)
	if recorder.Code != http.StatusOK || body["apply"].(map[string]any)["checkpointId"] != "cp_1" {
		t.Fatalf("unexpected apply response code=%d body=%#v", recorder.Code, body)
	}
	if stub.applyPlan["planId"] != "p1" || stub.applyBody["confirmation"] == nil {
		t.Fatalf("unexpected apply service inputs: body=%#v plan=%#v", stub.applyBody, stub.applyPlan)
	}
}

func TestCheckpointHandlersRejectUnknownScopeBeforeService(t *testing.T) {
	stub := &checkpointServiceStub{workspace: "/workspace"}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"scope":"files"}`))
	CheckpointHandlers{Service: stub}.HandleThreadCheckpointPath(recorder, request, "thr_1/checkpoints/cp_1/rewind-plan")
	body := decodeCheckpointBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["code"] != "invalid_checkpoint_scope" || stub.planScope != "" {
		t.Fatalf("unknown scope was not rejected before planning: code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}
}

func TestCheckpointHandlersErrors(t *testing.T) {
	handler := CheckpointHandlers{Service: &checkpointServiceStub{}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadCheckpointPath(recorder, request, "thr_1/checkpoints/cp_1/rewind-plan")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadCheckpointPath(recorder, request, "thr_1/checkpoints/cp_1")
	body := decodeCheckpointBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected route response code=%d body=%#v", recorder.Code, body)
	}

	handler = CheckpointHandlers{Service: &checkpointServiceStub{err: os.ErrNotExist}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{}`))
	handler.HandleThreadCheckpointPath(recorder, request, "missing/checkpoints/cp_1/rewind-plan")
	body = decodeCheckpointBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected not found response code=%d body=%#v", recorder.Code, body)
	}

	handler = CheckpointHandlers{Service: &checkpointServiceStub{err: errors.New("store failed")}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{}`))
	handler.HandleThreadCheckpointPath(recorder, request, "thr_1/checkpoints/cp_1/rewind-plan")
	body = decodeCheckpointBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["message"] != testInternalFailureMessage {
		t.Fatalf("unexpected internal response code=%d body=%#v", recorder.Code, body)
	}
}

func decodeCheckpointBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type checkpointServiceStub struct {
	threadID      string
	workspace     string
	err           error
	planScope     string
	planWorkspace string
	applyBody     map[string]any
	applyPlan     map[string]any
}

func (s *checkpointServiceStub) ThreadWorkspace(threadID string) (string, error) {
	s.threadID = threadID
	if s.workspace == "" {
		s.workspace = "/workspace"
	}
	return s.workspace, s.err
}

func (s *checkpointServiceStub) RewindPlan(threadID string, checkpointID string, workspace string, scope string) map[string]any {
	s.threadID = threadID
	s.planScope = scope
	s.planWorkspace = workspace
	return map[string]any{"threadId": threadID, "checkpointId": checkpointID, "scope": scope}
}

func (s *checkpointServiceStub) RewindApply(_ context.Context, threadID string, checkpointID string, workspace string, body map[string]any, plan map[string]any) map[string]any {
	s.threadID = threadID
	s.applyBody = body
	s.applyPlan = plan
	return map[string]any{"threadId": threadID, "checkpointId": checkpointID, "workspace": workspace}
}

var _ CheckpointService = (*checkpointServiceStub)(nil)
