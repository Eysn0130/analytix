package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type memoryStoreStub struct {
	listIncludeDeleted bool
	listWorkspace      string
	record             map[string]any
	found              bool
	err                error
	createErr          error
}

func (s *memoryStoreStub) List(includeDeleted bool, workspace string) ([]any, error) {
	s.listIncludeDeleted = includeDeleted
	s.listWorkspace = workspace
	if s.err != nil {
		return nil, s.err
	}
	return []any{map[string]any{"id": "mem_1"}}, nil
}

func (s *memoryStoreStub) Create(body map[string]any) (map[string]any, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return map[string]any{"id": "mem_created", "content": body["content"]}, nil
}

func (s *memoryStoreStub) Patch(id string, body map[string]any) (map[string]any, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if !s.found {
		return nil, false, nil
	}
	return map[string]any{"id": id, "content": body["content"]}, true, nil
}

func (s *memoryStoreStub) Delete(id string) (map[string]any, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if !s.found {
		return nil, false, nil
	}
	return map[string]any{"id": id, "deletedAt": "2026-07-10T00:00:00Z"}, true, nil
}

func (s *memoryStoreStub) Diagnostics() map[string]any {
	return map[string]any{"enabled": true}
}

func TestMemoryHandlersCollectionDiagnosticsAndRecordContracts(t *testing.T) {
	store := &memoryStoreStub{found: true}
	handlers := MemoryHandlers{Store: store}

	recorder := httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodGet, "/v1/memory?include_deleted=true&workspace=/tmp/work", nil))
	if recorder.Code != http.StatusOK || !store.listIncludeDeleted || store.listWorkspace != "/tmp/work" {
		t.Fatalf("list contract mismatch: status=%d include=%v workspace=%q body=%s", recorder.Code, store.listIncludeDeleted, store.listWorkspace, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/memory", strings.NewReader(`{"content":"remember"}`)))
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), "mem_created") {
		t.Fatalf("create contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleDiagnostics(recorder, httptest.NewRequest(http.MethodGet, "/v1/memory/diagnostics", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":true`) {
		t.Fatalf("diagnostics contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleRecordPath(recorder, httptest.NewRequest(http.MethodPatch, "/v1/memory/mem_go_1", strings.NewReader(`{"content":"updated"}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "updated") {
		t.Fatalf("patch contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleRecordPath(recorder, httptest.NewRequest(http.MethodDelete, "/v1/memory/mem_go_1", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "mem_go_1") ||
		!strings.Contains(recorder.Body.String(), "deletedAt") || strings.Contains(recorder.Body.String(), "content") {
		t.Fatalf("delete contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMemoryHandlersValidationNotFoundAndErrorContracts(t *testing.T) {
	store := &memoryStoreStub{createErr: errors.New("content required")}
	handlers := MemoryHandlers{Store: store}

	recorder := httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/memory", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), testValidationFailureMessage) || strings.Contains(recorder.Body.String(), "content required") {
		t.Fatalf("validation contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleRecordPath(recorder, httptest.NewRequest(http.MethodPatch, "/v1/memory/missing", strings.NewReader(`{"content":"x"}`)))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("not found contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	store.err = errors.New("disk failed")
	recorder = httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodGet, "/v1/memory", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("internal error contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleRecordPath(recorder, httptest.NewRequest(http.MethodGet, "/v1/memory/mem_go_1", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMemoryHandlersRejectFilesystemAliasBeforeStoreMutation(t *testing.T) {
	store := &memoryStoreStub{found: true}
	handlers := MemoryHandlers{Store: store}
	for _, path := range []string{"/v1/memory/mem:go_1", "/v1/memory/mem%3Ago_1", "/v1/memory/mem_go_01"} {
		for _, method := range []string{http.MethodPatch, http.MethodDelete} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, path, strings.NewReader(`{"content":"attacker"}`))
			handlers.HandleRecordPath(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("memory alias accepted: method=%s path=%s code=%d body=%s", method, path, recorder.Code, recorder.Body.String())
			}
		}
	}
}
